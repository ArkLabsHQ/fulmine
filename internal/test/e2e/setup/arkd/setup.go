package arkd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

const (
	defaultArkdExec    = "docker exec arkd"
	defaultPassword    = "secret"
	waitAttempts       = 30
	waitDelay          = 10 * time.Second
	serverInfoURL      = "http://localhost:7070/v1/info"
	setupFundingRounds = 4
	redeemNoteAmount   = "21000000"
)

var logger = logrus.StandardLogger()
var httpClient = &http.Client{
	Timeout: 15 * time.Second,
}

// EnsureReady initialises arkd and the embedded client the first time the E2E
// suite runs. Subsequent runs are cheap no-ops.
func EnsureReady(ctx context.Context) error {
	return ensureReadyWithCmd(ctx, defaultArkdExec)
}

func ensureReadyWithCmd(ctx context.Context, arkdExec string) error {
	initialized, unlocked, synced, err := checkWalletStatus(ctx, arkdExec)
	if err != nil {
		return fmt.Errorf("check wallet status: %w", err)
	}
	if initialized && unlocked && synced {
		logger.Info("arkd wallet already initialised")
		return nil
	}

	logger.Info("initialising arkd for e2e tests")

	if err := createWallet(ctx, arkdExec, defaultPassword); err != nil {
		return fmt.Errorf("create arkd wallet: %w", err)
	}

	if err := unlockWallet(ctx, arkdExec, defaultPassword); err != nil {
		return fmt.Errorf("unlock arkd wallet: %w", err)
	}

	if err := waitForWalletReady(ctx, arkdExec, waitAttempts, waitDelay); err != nil {
		return fmt.Errorf("arkd wallet not ready: %w", err)
	}

	if err := logServerInfo(ctx); err != nil {
		return fmt.Errorf("fetch arkd info: %w", err)
	}

	if err := fundArk(ctx, arkdExec); err != nil {
		return fmt.Errorf("fund arkd: %w", err)
	}

	if err := initializeClient(ctx, arkdExec); err != nil {
		return fmt.Errorf("initialise ark client: %w", err)
	}

	logger.Info("arkd ready")
	return nil
}

func checkWalletStatus(ctx context.Context, arkdExec string) (bool, bool, bool, error) {
	output, err := execCommand(ctx, fmt.Sprintf("%s arkd wallet status", arkdExec))
	if err != nil {
		return false, false, false, err
	}
	return strings.Contains(output, "initialized: true"),
		strings.Contains(output, "unlocked: true"),
		strings.Contains(output, "synced: true"), nil
}

func createWallet(ctx context.Context, arkdExec, password string) error {
	_, err := execCommand(ctx, fmt.Sprintf("%s arkd wallet create --password %s", arkdExec, password))
	return err
}

func unlockWallet(ctx context.Context, arkdExec, password string) error {
	_, err := execCommand(ctx, fmt.Sprintf("%s arkd wallet unlock --password %s", arkdExec, password))
	return err
}

func waitForWalletReady(ctx context.Context, arkdExec string, maxRetries int, retryDelay time.Duration) error {
	for attempt := 1; attempt <= maxRetries; attempt++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		initialized, unlocked, synced, _ := checkWalletStatus(ctx, arkdExec)

		if initialized && unlocked && synced {
			return nil
		}
		logger.WithFields(logrus.Fields{
			"attempt": attempt,
			"max":     maxRetries,
		}).Info("waiting for arkd wallet")
		time.Sleep(retryDelay)
	}
	return errors.New("arkd wallet failed to become ready in time")
}

func fundArk(ctx context.Context, arkdExec string) error {
	address, err := execCommand(ctx, fmt.Sprintf("%s arkd wallet address", arkdExec))
	if err != nil {
		return err
	}
	address = strings.TrimSpace(address)
	logger.WithField("address", address).Info("funding arkd address")

	for i := 0; i < setupFundingRounds; i++ {
		if _, err := execCommand(ctx, fmt.Sprintf("nigiri faucet %s", address)); err != nil {
			return err
		}
	}

	time.Sleep(5 * time.Second)
	return nil
}

func initializeClient(ctx context.Context, arkdExec string) error {
	note, err := execCommand(ctx, fmt.Sprintf("%s arkd note --amount %s", arkdExec, redeemNoteAmount))
	if err != nil {
		return err
	}
	note = strings.TrimSpace(note)

	if _, err := execCommand(ctx, fmt.Sprintf(
		"%s ark init --server-url http://localhost:7070 --explorer http://chopsticks:3000 --password %s",
		arkdExec, defaultPassword,
	)); err != nil {
		return err
	}

	if _, err := execCommand(ctx, fmt.Sprintf(
		"%s ark redeem-notes -n %s --password %s", arkdExec, note, defaultPassword,
	)); err != nil {
		return err
	}
	return nil
}

func logServerInfo(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, serverInfoURL, nil)
	if err != nil {
		return err
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	var info struct {
		SignerPubkey string `json:"signerPubkey"`
	}
	if err := json.Unmarshal(body, &info); err != nil {
		return err
	}
	logger.WithField("signerPubkey", info.SignerPubkey).Info("arkd server info")
	return nil
}

func execCommand(ctx context.Context, command string) (string, error) {
	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	output, err := cmd.CombinedOutput()
	if err != nil {
		logged := string(output)
		if strings.Contains(logged, "wallet already initialized") {
			logger.Info("wallet already initialised, continuing")
			return "", nil
		}
		logger.WithError(err).WithField("command", command).Errorf("command failed: %s", logged)
		return "", err
	}
	return string(output), nil
}
