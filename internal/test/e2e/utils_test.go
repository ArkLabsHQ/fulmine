package e2e_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

var httpClient = &http.Client{}

func faucetOffchain(address string, amount string) error {
	cmd := exec.Command("docker", "exec", "-t", "arkd", "ark", "send", "--to", address, "--amount", amount, "--password", "secret")
	_, err := cmd.Output()
	if err != nil {
		return err
	}
	time.Sleep(time.Second)
	return nil
}

func faucet(address string, amount string) (string, error) {
	cmd := exec.Command("nigiri", "faucet", address, amount)
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	txid := strings.TrimPrefix(string(output), "txId: ")
	time.Sleep(6 * time.Second)
	return strings.TrimSpace(txid), nil
}

type onboardResponse struct {
	Address string `json:"address"`
}

type balanceResponse struct {
	Amount string `json:"amount"`
}

func getBalance() (uint64, error) {
	resp, err := httpClient.Get("http://localhost:7001/api/v1/balance")
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	var balanceResp balanceResponse
	if err := json.NewDecoder(resp.Body).Decode(&balanceResp); err != nil {
		return 0, err
	}
	amount, err := strconv.ParseUint(balanceResp.Amount, 10, 64)
	if err != nil {
		return 0, err
	}
	return amount, nil
}

func getOnboardAddress(amount uint64) (string, error) {
	payload := map[string]uint64{
		"amount": amount,
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	resp, err := httpClient.Post("http://localhost:7001/api/v1/onboard", "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var onboardResp onboardResponse
	if err := json.NewDecoder(resp.Body).Decode(&onboardResp); err != nil {
		return "", err
	}
	return onboardResp.Address, nil
}

type SettleResponse struct {
	Txid string `json:"txid"`
}

func settle() (string, error) {
	resp, err := httpClient.Get("http://localhost:7001/api/v1/settle")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var settleResp SettleResponse
	if err := json.NewDecoder(resp.Body).Decode(&settleResp); err != nil {
		return "", err
	}
	time.Sleep(time.Second)
	return settleResp.Txid, nil
}

func sendOffChain(address string, amount uint64) (string, error) {
	payload := map[string]interface{}{
		"address": address,
		"amount":  amount,
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	resp, err := httpClient.Post("http://localhost:7001/api/v1/send/offchain", "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var sendResp struct {
		Txid string `json:"txid"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&sendResp); err != nil {
		return "", err
	}
	time.Sleep(time.Second)
	return sendResp.Txid, nil
}

func sendOnChain(address string, amount uint64) (string, error) {
	payload := map[string]interface{}{
		"address": address,
		"amount":  amount,
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	resp, err := httpClient.Post("http://localhost:7001/api/v1/send/onchain", "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var sendResp struct {
		Txid string `json:"txid"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&sendResp); err != nil {
		return "", err
	}
	time.Sleep(time.Second)
	return sendResp.Txid, nil
}

type transactionInfo struct {
	RoundTxid    string `json:"roundTxid"`
	RedeemTxid   string `json:"redeemTxid"`
	BoardingTxid string `json:"boardingTxid"`
	Type         string `json:"type"`
	Amount       string `json:"amount"`
	Timestamp    int64  `json:"timestamp"`
	Settled      bool   `json:"settled"`
}

type transactionHistoryResponse struct {
	Transactions []transactionInfo `json:"transactions"`
}

func getTransactionHistory() ([]transactionInfo, error) {
	resp, err := httpClient.Get("http://localhost:7001/api/v1/transactions")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var historyResp transactionHistoryResponse
	if err := json.NewDecoder(resp.Body).Decode(&historyResp); err != nil {
		return nil, err
	}

	return historyResp.Transactions, nil
}

type transactionType int

const (
	boarding transactionType = iota
	redeem
	round
)

func findInHistory(txid string, history []transactionInfo, txType transactionType) (transactionInfo, error) {
	for _, tx := range history {
		switch txType {
		case boarding:
			if tx.BoardingTxid == txid {
				return tx, nil
			}
		case redeem:
			if tx.RedeemTxid == txid {
				return tx, nil
			}
		case round:
			if tx.RoundTxid == txid {
				return tx, nil
			}
		}
	}
	return transactionInfo{}, fmt.Errorf("transaction not found %s", txid)
}

type createVHTLCResponse struct {
	Address                              string `json:"address"`
	ClaimPubkey                          string `json:"claimPubkey"`
	RefundPubkey                         string `json:"refundPubkey"`
	ServerPubkey                         string `json:"serverPubkey"`
	RefundLocktime                       string `json:"refundLocktime"`
	UnilateralClaimDelay                 string `json:"unilateralClaimDelay"`
	UnilateralRefundDelay                string `json:"unilateralRefundDelay"`
	UnilateralRefundWithoutReceiverDelay string `json:"unilateralRefundWithoutReceiverDelay"`
}

func createVHTLC(preimageHash, receiverPubkey string) (*createVHTLCResponse, error) {
	payload := map[string]interface{}{
		"preimage_hash":   preimageHash,
		"receiver_pubkey": receiverPubkey,
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	resp, err := httpClient.Post("http://localhost:7001/api/v1/vhtlc", "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to create VHTLC: %s, body: %s", resp.Status, string(body))
	}

	var vhtlcResp createVHTLCResponse
	if err := json.NewDecoder(resp.Body).Decode(&vhtlcResp); err != nil {
		return nil, err
	}

	return &vhtlcResp, nil
}

type ClaimVHTLCResponse struct {
	RedeemTxid string `json:"redeemTxid"`
}

func claimVHTLC(preimage string) (string, error) {
	payload := map[string]string{
		"preimage": preimage,
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	resp, err := httpClient.Post("http://localhost:7001/api/v1/vhtlc/claim", "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		payload, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("failed to claim VHTLC: %d %s", resp.StatusCode, string(payload))
	}

	var claimResp ClaimVHTLCResponse
	if err := json.NewDecoder(resp.Body).Decode(&claimResp); err != nil {
		return "", err
	}
	time.Sleep(time.Second)
	return claimResp.RedeemTxid, nil
}

type Vtxo struct {
	PreimageHash string `json:"preimageHash"`
	Address      string `json:"address"`
	Amount       string `json:"amount"`
}

type ListVHTLCResponse struct {
	Vhtlcs []Vtxo `json:"vhtlcs"`
}

func listVHTLC(preimageHashFilter string) ([]Vtxo, error) {
	url := "http://localhost:7001/api/v1/vhtlc"
	if preimageHashFilter != "" {
		url += "?preimage_hash_filter=" + preimageHashFilter
	}
	resp, err := httpClient.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var listResp ListVHTLCResponse
	if err := json.NewDecoder(resp.Body).Decode(&listResp); err != nil {
		return nil, err
	}
	return listResp.Vhtlcs, nil
}
