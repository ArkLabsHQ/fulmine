package main

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/sirupsen/logrus"
)

var log = logrus.New()

// Create a custom HTTP client that allows HTTP/0.9 responses
var httpClient = &http.Client{
	Transport: &http.Transport{},
}

func checkWalletStatus() (bool, bool, bool, error) {
	resp, err := httpClient.Get("http://localhost:7001/api/v1/wallet/status")
	if err != nil {
		return false, false, false, err
	}
	defer resp.Body.Close()

	var status struct {
		Initialized bool `json:"initialized"`
		Unlocked    bool `json:"unlocked"`
		Synced      bool `json:"synced"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		return false, false, false, err
	}
	return status.Initialized, status.Unlocked, status.Synced, nil
}

func waitForWalletReady(maxRetries int, retryDelay time.Duration) error {
	for i := 0; i < maxRetries; i++ {
		initialized, unlocked, synced, err := checkWalletStatus()
		if err != nil {
			return err
		}
		if initialized && unlocked && synced {
			log.Info("Wallet is ready")
			return nil
		}
		log.Infof("Waiting for wallet to be ready (%d/%d)...", i+1, maxRetries)
		time.Sleep(retryDelay)
	}
	return fmt.Errorf("wallet failed to be ready after maximum retries")
}

func createWallet(password string) error {
	prvkey, _ := btcec.NewPrivateKey()
	payload := map[string]string{
		"private_key": hex.EncodeToString(prvkey.Serialize()),
		"password":    password,
		"server_url":  "http://arkd:7070",
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	resp, err := httpClient.Post("http://localhost:7001/api/v1/wallet/create", "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("failed to read response error body: %w", err)
		}
		return fmt.Errorf("failed to create wallet: %d, %s", resp.StatusCode, string(body))
	}
	return nil
}

func unlockWallet(password string) error {
	payload := map[string]string{
		"password": password,
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	resp, err := httpClient.Post("http://localhost:7001/api/v1/wallet/unlock", "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to unlock wallet: %d", resp.StatusCode)
	}
	return nil
}

func setupFulmine() error {
	log.Info("Setting up Fulmine...")

	password := "secret"
	if err := createWallet(password); err != nil {
		log.WithError(err).Error("Failed to create wallet")
		return err
	}
	log.Info("Wallet created successfully")

	if err := unlockWallet(password); err != nil {
		log.WithError(err).Error("Failed to unlock wallet")
		return err
	}
	log.Info("Wallet unlocked successfully")

	// Wait for wallet to be ready and synced
	if err := waitForWalletReady(30, 2*time.Second); err != nil {
		log.WithError(err).Error("Wallet failed to be ready after maximum retries")
		return err
	}

	// Get fulmine pubkey
	resp, err := httpClient.Get("http://localhost:7001/api/v1/info")
	if err != nil {
		log.WithError(err).Error("Failed to fetch Fulmine info")
		return err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.WithError(err).Error("Failed to read Fulmine info response")
		return err
	}

	var serverInfo struct {
		Pubkey string `json:"pubkey"`
	}
	if err := json.Unmarshal(body, &serverInfo); err != nil {
		log.WithError(err).Error("Failed to parse Fulmine info")
		return err
	}
	log.Info("Fulmine pubkey: ", serverInfo.Pubkey)

	log.Info("Fulmine setup completed successfully")
	return nil
}

func main() {
	initialized, unlocked, synced, err := checkWalletStatus()
	if err != nil {
		log.WithError(err).Fatal("Failed to check wallet status")
		return
	}
	if initialized && unlocked && synced {
		log.Info("Fulmine already initialized, skipping setup")
		return
	}
	if err := setupFulmine(); err != nil {
		log.WithError(err).Fatal("Setup failed")
	}
}
