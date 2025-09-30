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

const baseUrl = "http://localhost:7001/api/v1"

var httpClient = &http.Client{}

func faucetOffchain(address string, amount string) error {
	cmd := exec.Command(
		"docker", "exec", "-t", "arkd",
		"ark", "send", "--to", address, "--amount", amount, "--password", "secret",
	)
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
	return strings.TrimSpace(txid), nil
}

type balanceResponse struct {
	Amount string `json:"amount"`
}

func getBalance() (uint64, error) {
	url := fmt.Sprintf("%s/balance", baseUrl)
	resp, err := httpClient.Get(url)
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
	payload := map[string]uint64{"amount": amount}
	jsonData, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	url := fmt.Sprintf("%s/onboard", baseUrl)
	resp, err := httpClient.Post(url, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var onboardResp struct {
		Address string `json:"address"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&onboardResp); err != nil {
		return "", err
	}
	return onboardResp.Address, nil
}

func getPubkey() (string, error) {
	url := fmt.Sprintf("%s/address", baseUrl)
	resp, err := httpClient.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var addrResp struct {
		Pubkey string `json:"pubkey"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&addrResp); err != nil {
		return "", err
	}
	return addrResp.Pubkey, nil
}

func settle() (string, error) {
	url := fmt.Sprintf("%s/settle", baseUrl)
	resp, err := httpClient.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var settleResp struct {
		Txid string `json:"txid"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&settleResp); err != nil {
		return "", err
	}
	time.Sleep(time.Second)
	return settleResp.Txid, nil
}

func sendOffChain(address string, amount uint64) (string, error) {
	payload := map[string]any{
		"address": address,
		"amount":  amount,
	}
	jsonData, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	url := fmt.Sprintf("%s/send/offchain", baseUrl)
	resp, err := httpClient.Post(url, "application/json", bytes.NewBuffer(jsonData))
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
	payload := map[string]any{
		"address": address,
		"amount":  amount,
	}
	jsonData, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	url := fmt.Sprintf("%s/send/onchain", baseUrl)
	resp, err := httpClient.Post(url, "application/json", bytes.NewBuffer(jsonData))
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

	return sendResp.Txid, nil
}

func getReceiverOffchainAddress() (string, error) {
	cmd := exec.Command("docker", "exec", "-t", "arkd", "ark", "receive")
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}

	var out struct {
		Address string `json:"offchain_address"`
	}
	if err := json.Unmarshal(output, &out); err != nil {
		return "", err
	}
	return out.Address, nil
}

func getReceiverOnchainAddress() (string, error) {
	cmd := exec.Command("nigiri", "rpc", "getnewaddress")
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
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

func getTransactionHistory() ([]transactionInfo, error) {
	url := fmt.Sprintf("%s/transactions", baseUrl)
	resp, err := httpClient.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var historyResp struct {
		Transactions []transactionInfo `json:"transactions"`
	}
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
	payload := map[string]any{
		"preimage_hash":   preimageHash,
		"receiver_pubkey": receiverPubkey,
	}
	jsonData, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s/vhtlc", baseUrl)
	resp, err := httpClient.Post(url, "application/json", bytes.NewBuffer(jsonData))
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
	payload := map[string]string{"preimage": preimage}
	jsonData, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	url := fmt.Sprintf("%s/vhtlc/claim", baseUrl)
	resp, err := httpClient.Post(url, "application/json", bytes.NewBuffer(jsonData))
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
	url := fmt.Sprintf("%s/vhtlc", baseUrl)
	if preimageHashFilter != "" {
		url = fmt.Sprintf("%s?preimage_hash_filter=%s", url, preimageHashFilter)
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

type GetVirtualTxsResponse struct {
	Txs []string `json:"txs"`
}

func getVirtualTxs(txids []string) ([]string, error) {
	// Join txids with commas for the URL path parameter
	txidsParam := strings.Join(txids, ",")
	url := fmt.Sprintf("%s/virtualTx/%s", baseUrl, txidsParam)
	
	resp, err := httpClient.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to get virtual txs: %s, body: %s", resp.Status, string(body))
	}

	var virtualTxsResp GetVirtualTxsResponse
	if err := json.NewDecoder(resp.Body).Decode(&virtualTxsResp); err != nil {
		return nil, err
	}
	return virtualTxsResp.Txs, nil
}
