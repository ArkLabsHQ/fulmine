package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/sirupsen/logrus"
)

const privKey = "7dc828f12ef62b9200632f6503ece76f8ef7718e1f36f62d692bffa9e3a3ed7c"

var log = logrus.New()

// Create a custom HTTP client that allows HTTP/0.9 responses
var httpClient = &http.Client{
	Transport: &http.Transport{},
}

type ServerInfo struct {
	Pubkey string `json:"pubkey"`
}

type WalletStatus struct {
	Initialized bool `json:"initialized"`
	Unlocked    bool `json:"unlocked"`
	Synced      bool `json:"synced"`
}

type GenSeedResponse struct {
	Hex  string `json:"hex"`
	Nsec string `json:"nsec"`
}

type AddressResponse struct {
	Address string `json:"address"`
	Pubkey  string `json:"pubkey"`
}

type OnboardResponse struct {
	Address string `json:"address"`
}

func checkWalletStatus() (bool, bool, bool, error) {
	resp, err := httpClient.Get("http://localhost:7001/api/v1/wallet/status")
	if err != nil {
		return false, false, false, err
	}
	defer resp.Body.Close()

	var status WalletStatus
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
	payload := map[string]string{
		"private_key": privKey,
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

func setupFulmineServer() error {
	log.Info("Starting Fulmine server setup process...")

	log.Info("Creating new wallet...")

	password := "secret"
	if err := createWallet(password); err != nil {
		log.WithError(err).Error("Failed to create wallet")
		return err
	}
	log.Info("Wallet created successfully")

	log.Info("Attempting to unlock wallet...")
	if err := unlockWallet(password); err != nil {
		log.WithError(err).Error("Failed to unlock wallet")
		return err
	}
	log.Info("Wallet unlocked successfully")

	// Wait for wallet to be ready and synced
	log.Info("Waiting for wallet to be ready and synced...")
	if err := waitForWalletReady(30, 2*time.Second); err != nil {
		log.WithError(err).Error("Wallet failed to be ready after maximum retries")
		return err
	}
	log.Info("Wallet is ready and synced")

	// Get and log the server info
	log.Info("Fetching server information...")
	resp, err := httpClient.Get("http://localhost:7001/api/v1/info")
	if err != nil {
		log.WithError(err).Error("Failed to fetch server info")
		return err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.WithError(err).Error("Failed to read server info response")
		return err
	}

	var serverInfo ServerInfo
	if err := json.Unmarshal(body, &serverInfo); err != nil {
		log.WithError(err).Error("Failed to parse server info")
		return err
	}
	log.Info("Fulmine Server Public Key: ", serverInfo.Pubkey)

	log.Info("Fulmine server setup completed successfully")
	return nil
}

func main() {
	initialized, unlocked, synced, err := checkWalletStatus()
	if err != nil {
		log.WithError(err).Fatal("Failed to check wallet status")
		return
	}
	if initialized && unlocked && synced {
		log.Info("Fulmine server already initialized, skipping setup")
		return
	}
	if err := setupFulmineServer(); err != nil {
		log.WithError(err).Fatal("Setup failed")
	}
}
