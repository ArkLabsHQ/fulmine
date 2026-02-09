package e2e_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	pb "github.com/ArkLabsHQ/fulmine/api-spec/protobuf/gen/go/fulmine/v1"
	"github.com/creack/pty"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

const (
	lnd = "docker exec lnd lncli --network=regtest"
	cln = "docker exec cln lightning-cli --network=regtest"
)

func newFulmineClient(url string) (pb.ServiceClient, error) {
	opts := grpc.WithTransportCredentials(insecure.NewCredentials())
	conn, err := grpc.NewClient(url, opts)
	if err != nil {
		return nil, err
	}
	return pb.NewServiceClient(conn), nil
}

func newFulmineWalletClient(url string) (pb.WalletServiceClient, error) {
	opts := grpc.WithTransportCredentials(insecure.NewCredentials())
	conn, err := grpc.NewClient(url, opts)
	if err != nil {
		return nil, err
	}
	return pb.NewWalletServiceClient(conn), nil
}

func lndAddInvoice(ctx context.Context, sats int) (string, string, error) {
	command := fmt.Sprintf("%s addinvoice --amt %d", lnd, sats)
	out, err := runCommand(ctx, command)
	if err != nil {
		return "", "", err
	}

	var resp struct {
		PaymentRequest string `json:"payment_request"`
		RHash          string `json:"r_hash"`
	}
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		return "", "", err
	}
	return resp.PaymentRequest, resp.RHash, nil
}

func lndPayInvoice(ctx context.Context, invoice string) error {
	command := fmt.Sprintf("%s payinvoice --force %s", lnd, invoice)
	_, err := runCommand(ctx, command)
	return err
}

func lndCancelInvoice(ctx context.Context, rHash string) error {
	command := fmt.Sprintf("%s cancelinvoice %s", lnd, rHash)
	_, err := runCommand(ctx, command)
	return err
}

func clnAddOffer(ctx context.Context, sats int) (string, string, error) {
	label := fmt.Sprintf("funding-%s", time.Now().Format(time.RFC3339))
	command := fmt.Sprintf(`%s offer %d "%s"`, cln, sats, label)
	out, err := runCommand(ctx, command)
	if err != nil {
		return "", "", err
	}

	var resp struct {
		PaymentHash string `json:"offer_id"`
		Bolt11      string `json:"bolt12"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &resp); err != nil {
		return "", "", err
	}
	return resp.Bolt11, resp.PaymentHash, nil
}

func faucet(ctx context.Context, address string, amount float64) error {
	command := fmt.Sprintf("nigiri faucet %s %.8f", address, amount)
	_, err := runCommand(ctx, command)
	return err
}

func runCommand(ctx context.Context, command string) (string, error) {
	cmd := exec.CommandContext(ctx, "sh", "-c", command)

	ptmx, err := pty.Start(cmd)
	if err != nil {
		return "", err
	}
	defer func() { _ = ptmx.Close() }()

	var out bytes.Buffer
	done := make(chan error, 1)
	go func() {
		_, err := io.Copy(&out, ptmx)
		done <- err
	}()

	select {
	case <-ctx.Done():
		_ = cmd.Process.Kill()
		return "", ctx.Err()
	case copyErr := <-done:
		if copyErr != nil && !errors.Is(copyErr, syscall.EIO) {
			return "", fmt.Errorf("read command output: %w", copyErr)
		}
		if err := cmd.Wait(); err != nil {
			return "", fmt.Errorf("%s", strings.TrimSpace(out.String()))
		}
		return out.String(), nil
	}
}

func restartDockerComposeServices(t *testing.T, ctx context.Context, services ...string) {
	t.Helper()
	composePath := findComposeFile(t)
	requireServices := strings.Join(services, " ")
	command := fmt.Sprintf("docker compose -f %s restart %s", composePath, requireServices)
	_, err := runCommand(ctx, command)
	if err != nil {
		t.Fatalf("restart docker services (%s): %v", requireServices, err)
	}
}

func findComposeFile(t *testing.T) string {
	t.Helper()
	path, err := findComposeFilePath()
	if err != nil {
		t.Fatalf("%v", err)
	}
	return path
}

func findComposeFilePath() (string, error) {
	if env := os.Getenv("FULMINE_COMPOSE_FILE"); env != "" {
		return env, nil
	}

	wd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("getwd failed: %w", err)
	}

	dir := wd
	for {
		candidate := filepath.Join(dir, "test.docker-compose.yml")
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	return "", fmt.Errorf("test.docker-compose.yml not found from %s; set FULMINE_COMPOSE_FILE", wd)
}

func unlockAndSettle(addr string, pass string) error {
	var walletClient pb.WalletServiceClient
	var serviceClient pb.ServiceClient

	// Retry loop: after a docker restart the gRPC server may need a few
	// seconds before it starts accepting connections.
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		wc, err := newFulmineWalletClient(addr)
		if err != nil {
			time.Sleep(500 * time.Millisecond)
			continue
		}
		_, err = wc.Unlock(context.Background(), &pb.UnlockRequest{Password: pass})
		if err != nil {
			// "connection reset" / "connection refused" means the server
			// isn't ready yet – keep retrying.
			errMsg := err.Error()
			if strings.Contains(errMsg, "connection reset") ||
				strings.Contains(errMsg, "connection refused") ||
				strings.Contains(errMsg, "unavailable") {
				time.Sleep(500 * time.Millisecond)
				continue
			}
			return fmt.Errorf("unlock %s: %w", addr, err)
		}
		walletClient = wc
		break
	}
	if walletClient == nil {
		return fmt.Errorf("wallet client %s not ready within 30s", addr)
	}

	sc, err := newFulmineClient(addr)
	if err != nil {
		return err
	}
	serviceClient = sc

	if _, err = serviceClient.Settle(context.Background(), &pb.SettleRequest{}); err != nil {
		return err
	}

	return nil
}
