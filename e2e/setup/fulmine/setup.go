package fulmine

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/sirupsen/logrus"
)

const (
	baseURL = "http://localhost:7001/api/v1"

	defaultPassword   = "secret"
	waitAttempts      = 30
	waitDelay         = 2 * time.Second
	fundingThreshold  = uint64(150_000)
	fundingAmountSats = uint64(100_000)
	fundingAmountBtc  = "0.001"
	fundingRounds     = 3
)

var logger = logrus.StandardLogger()

var httpClient = &http.Client{
	Timeout: 15 * time.Second,
}

type walletStatus struct {
	Initialized bool `json:"initialized"`
	Unlocked    bool `json:"unlocked"`
	Synced      bool `json:"synced"`
}

type TestFulmine struct {
	baseURL string
}

func NewTestFulmine(baseUrl string) *TestFulmine {
	return &TestFulmine{
		baseURL: baseUrl,
	}

}

// EnsureReady initialises and unlocks Fulmine if it has not been configured yet.
func (f *TestFulmine) EnsureReady(ctx context.Context) error {
	initialized, unlocked, synced, err := f.checkWalletStatus(ctx)
	if err != nil {
		return fmt.Errorf("check wallet status: %w", err)
	}
	if initialized && unlocked && synced {
		if err := f.ensureFunding(ctx); err != nil {
			return fmt.Errorf("fund wallet: %w", err)
		}
		logger.Info("Fulmine wallet already initialised")
		return nil
	}

	if !initialized {
		if err := f.createWallet(ctx, defaultPassword); err != nil {
			return fmt.Errorf("create wallet: %w", err)
		}
		logger.Info("initialising Fulmine wallet for e2e tests")
	}

	if !unlocked {
		if err := f.unlockWallet(ctx, defaultPassword); err != nil {
			return fmt.Errorf("unlock wallet: %w", err)
		}
		logger.Info("unlocking Fulmine wallet for e2e tests")
	}

	if err := f.waitForWalletReady(ctx, waitAttempts, waitDelay); err != nil {
		return fmt.Errorf("wallet not ready: %w", err)
	}

	if err := f.ensureFunding(ctx); err != nil {
		return fmt.Errorf("fund wallet: %w", err)
	}

	if err := f.logServerInfo(ctx); err != nil {
		return fmt.Errorf("fetch server info: %w", err)
	}

	logger.Info("Fulmine wallet ready")
	return nil
}

func (f *TestFulmine) checkWalletStatus(ctx context.Context) (bool, bool, bool, error) {
	statusEndpoint := f.baseURL + "/wallet/status"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, statusEndpoint, nil)
	if err != nil {
		return false, false, false, err
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return false, false, false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return false, false, false, fmt.Errorf("wallet status check failed: %s %s", resp.Status, strings.TrimSpace(string(body)))
	}

	var status walletStatus
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		return false, false, false, err
	}
	return status.Initialized, status.Unlocked, status.Synced, nil
}

func (f *TestFulmine) waitForWalletReady(ctx context.Context, maxRetries int, retryDelay time.Duration) error {
	for attempt := 1; attempt <= maxRetries; attempt++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		initialized, unlocked, synced, err := f.checkWalletStatus(ctx)

		if err != nil {
			logger.WithError(err).Warn("error checking wallet status, will retry")
			time.Sleep(retryDelay)
			continue
		}

		if initialized && unlocked && synced {
			return nil
		}
		logger.WithFields(logrus.Fields{
			"attempt": attempt,
			"max":     maxRetries,
		}).Info("waiting for Fulmine wallet to be ready")
		time.Sleep(retryDelay)
	}
	return fmt.Errorf("wallet failed to be ready after %d attempts", maxRetries)
}

func (f *TestFulmine) createWallet(ctx context.Context, password string) error {
	prvkey, err := btcec.NewPrivateKey()
	if err != nil {
		return err
	}
	payload := map[string]string{
		"private_key": hex.EncodeToString(prvkey.Serialize()),
		"password":    password,
		"server_url":  "http://arkd:7070",
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	createWalletEndpoint := f.baseURL + "/wallet/create"

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, createWalletEndpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to create wallet: %s", bytes.TrimSpace(respBody))
	}

	return nil
}

func (f *TestFulmine) unlockWallet(ctx context.Context, password string) error {
	payload := map[string]string{
		"password": password,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	unlockWalletEndpoint := f.baseURL + "/wallet/unlock"

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, unlockWalletEndpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to unlock wallet: %s", bytes.TrimSpace(respBody))
	}
	return nil
}

func (f *TestFulmine) logServerInfo(ctx context.Context) error {
	infoEndpoint := f.baseURL + "/info"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, infoEndpoint, nil)
	if err != nil {
		return err
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("fetch server info failed: %s %s", resp.Status, strings.TrimSpace(string(body)))
	}

	var serverInfo struct {
		Pubkey string `json:"pubkey"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&serverInfo); err != nil {
		return err
	}

	logger.WithField("pubkey", serverInfo.Pubkey).Info("Fulmine server info")
	return nil
}

func (f *TestFulmine) ensureFunding(ctx context.Context) error {
	balance, err := f.getBalance(ctx)
	if err != nil {
		return err
	}
	if balance >= fundingThreshold {
		return nil
	}

	for i := 0; i < fundingRounds; i++ {
		address, err := f.getOnboardAddress(ctx, fundingAmountSats)
		if err != nil {
			return err
		}
		if err := nigiriFaucet(ctx, address, fundingAmountBtc); err != nil {
			return err
		}
	}

	if err := nigiriGenerate(ctx, 1); err != nil {
		return err
	}

	time.Sleep(11 * time.Second)

	if err := f.settle(ctx); err != nil {
		return err
	}

	return nil
}

func (f *TestFulmine) getBalance(ctx context.Context) (uint64, error) {
	endpoint := f.baseURL + "/balance"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return 0, err
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return 0, fmt.Errorf("get balance failed: %s %s", resp.Status, strings.TrimSpace(string(body)))
	}

	var body struct {
		Amount string `json:"amount"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return 0, err
	}
	value, err := strconv.ParseUint(body.Amount, 10, 64)
	if err != nil {
		return 0, err
	}
	return value, nil
}

func (f *TestFulmine) getOnboardAddress(ctx context.Context, amount uint64) (string, error) {
	payload := map[string]uint64{"amount": amount}
	buf, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	endpoint := f.baseURL + "/onboard"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(buf))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("onboard failed: %s %s", resp.Status, strings.TrimSpace(string(body)))
	}

	var body struct {
		Address string `json:"address"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return "", err
	}
	return body.Address, nil
}

func (f *TestFulmine) settle(ctx context.Context) error {
	endpoint := f.baseURL + "/settle"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("settle failed: %s %s", resp.Status, strings.TrimSpace(string(body)))
	}
	return nil
}

func nigiriFaucet(ctx context.Context, address, amount string) error {
	if address == "" {
		return fmt.Errorf("empty faucet address")
	}
	_, err := runNigiri(ctx, "faucet", address, amount)
	return err
}

func nigiriGenerate(ctx context.Context, blocks int) error {
	if blocks <= 0 {
		return nil
	}
	_, err := runNigiri(ctx, "rpc", "--generate", strconv.Itoa(blocks))
	return err
}

func runNigiri(ctx context.Context, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "nigiri", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("nigiri %s: %w (output: %s)", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return out, nil
}
