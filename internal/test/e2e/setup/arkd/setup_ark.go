package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

var arkdExec = "docker exec -t arkd"
var log = logrus.New()

func execCommand(command string) (string, error) {
	cmd := exec.Command("sh", "-c", command)
	output, err := cmd.CombinedOutput()
	if err != nil {
		if strings.Contains(string(output), "wallet already initialized") {
			log.Info("Wallet already initialized, continuing...")
			return "", nil
		}
		log.Errorf("Error executing command: %v", err)
		log.Info("command: ", command)
		return "", err
	}
	return string(output), nil
}

func checkWalletStatus() (bool, bool, bool, error) {
	output, err := execCommand(fmt.Sprintf("%s arkd wallet status", arkdExec))
	if err != nil {
		return false, false, false, err
	}
	initialized := strings.Contains(output, "initialized: true")
	unlocked := strings.Contains(output, "unlocked: true")
	synced := strings.Contains(output, "synced: true")
	return initialized, unlocked, synced, nil
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

func setupArkServer() error {
	log.Info("Setting up Ark server...")

	// Create and unlock arkd wallet with deterministic mnemonic
	if _, err := execCommand(fmt.Sprintf(
		"%s arkd wallet create --password secret", arkdExec,
	)); err != nil {
		return err
	}
	log.Info("Wallet created successfully")

	if _, err := execCommand(fmt.Sprintf(
		"%s arkd wallet unlock --password secret", arkdExec,
	)); err != nil {
		return err
	}
	log.Info("Wallet unlocked successfully")

	// Wait for wallet to be ready and synced
	if err := waitForWalletReady(30, 2*time.Second); err != nil {
		return err
	}

	// Get and log the server info
	resp, err := http.Get("http://localhost:7070/v1/info")
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	var serverInfo struct {
		SignerPubkey string `json:"signerPubkey"`
	}
	if err := json.Unmarshal(body, &serverInfo); err != nil {
		return err
	}
	log.Info("Ark signer pubkey: ", serverInfo.SignerPubkey)

	// Get arkd address and fund it with nigiri faucet
	arkdAddress, err := execCommand(fmt.Sprintf("%s arkd wallet address", arkdExec))
	if err != nil {
		return err
	}
	arkdAddress = strings.TrimSpace(arkdAddress)
	log.Infof("Ark server funding address: %s", arkdAddress)
	for i := 0; i < 4; i++ {
		if _, err := execCommand(fmt.Sprintf("nigiri faucet %s", arkdAddress)); err != nil {
			return err
		}
	}
	log.Info("Arkd funded with 4 BTC")

	// Wait for transaction to be confirmed
	time.Sleep(5 * time.Second)

	note, err := execCommand(fmt.Sprintf("%s arkd note --amount 21000000", arkdExec))
	if err != nil {
		return err
	}
	note = strings.TrimSpace(note)

	log.Info("Setting up Ark client...")

	// Initialize ark client
	if _, err = execCommand(fmt.Sprintf(
		"%s ark init --server-url http://localhost:7070 "+
			"--explorer http://chopsticks:3000 --password secret", arkdExec,
	)); err != nil {
		return err
	}

	// Get ark boarding address and fund it
	if _, err := execCommand(fmt.Sprintf(
		"%s ark redeem-notes -n %s --password secret", arkdExec, note,
	)); err != nil {
		return err
	}
	log.Info("Ark client funded with 0.21 BTC")

	log.Info("Ark server and client setup completed successfully")
	return nil
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "nigiri" {
		arkdExec = "nigiri"
	}

	initialized, unlocked, synced, err := checkWalletStatus()
	if err != nil {
		log.WithError(err).Fatal("Error checking wallet status")
	}
	if initialized && unlocked && synced {
		log.Info("Wallet already initialized, skipping setup")
		return
	}

	if err := setupArkServer(); err != nil {
		log.WithError(err).Fatal("Setup failed")
	}
}
