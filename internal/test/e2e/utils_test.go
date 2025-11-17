package e2e_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	pb "github.com/ArkLabsHQ/fulmine/api-spec/protobuf/gen/go/fulmine/v1"
	"github.com/creack/pty"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func newFulmineClient(url string) (pb.ServiceClient, error) {
	opts := grpc.WithTransportCredentials(insecure.NewCredentials())
	conn, err := grpc.NewClient(url, opts)
	if err != nil {
		return nil, err
	}
	return pb.NewServiceClient(conn), nil
}

func lndAddInvoice(ctx context.Context, sats int) (string, string, error) {
	command := fmt.Sprintf("nigiri lnd addinvoice --amt %d", sats)
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
	command := fmt.Sprintf("nigiri lnd payinvoice --force %s", invoice)
	_, err := runCommand(ctx, command)
	return err
}

func lndCancelInvoice(ctx context.Context, rHash string) error {
	command := fmt.Sprintf("nigiri lnd cancelinvoice %s", rHash)
	_, err := runCommand(ctx, command)
	return err
}

func clnAddOffer(ctx context.Context, sats int) (string, string, error) {
	label := fmt.Sprintf("funding-%s", time.Now().Format(time.RFC3339))
	command := fmt.Sprintf(`nigiri cln offer %d "%s"`, sats, label)
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

func clnPayInvoice(ctx context.Context, invoice string) error {
	command := fmt.Sprintf("nigiri cln pay %s", invoice)
	_, err := runCommand(ctx, command)
	return err
}

func faucet(ctx context.Context, address string, amount float64) error {
	command := fmt.Sprintf("nigiri faucet %s %.8f", address, amount)
	_, err := runCommand(ctx, command)
	return err
}

func runCommand(ctx context.Context, command string) (string, error) {
	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	cmd.Dir = projectRoot()

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

func projectRoot() string {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		return "."
	}
	return filepath.Clean(filepath.Join(filepath.Dir(filename), ".."))
}

func doHttpRequest(ctx context.Context, url, method string, body io.Reader) ([]byte, error) {
	httpClient := &http.Client{
		Timeout: 15 * time.Second,
	}
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("resp status %s - body %s", resp.Status, string(respBody))
	}
	return respBody, nil
}
