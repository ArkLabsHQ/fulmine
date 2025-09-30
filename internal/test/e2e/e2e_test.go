package e2e_test

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"testing"
	"time"

	"github.com/ArkLabsHQ/fulmine/utils"
	"github.com/lightningnetwork/lnd/input"
	"github.com/stretchr/testify/require"
)

func TestOnboard(t *testing.T) {
	onboardAddress, err := getOnboardAddress(100000)
	require.NoError(t, err)
	require.False(t, utils.IsBip21(onboardAddress))

	txid, err := faucet(onboardAddress, "0.001")
	require.NoError(t, err)
	require.NotEmpty(t, txid)

	time.Sleep(11 * time.Second) // onchain polling interval is 10 seconds

	history, err := getTransactionHistory()
	require.NoError(t, err)
	require.NotEmpty(t, history)

	tx, err := findInHistory(txid, history, boarding)
	require.NoError(t, err)
	require.Equal(t, "100000", tx.Amount)
	require.False(t, tx.Settled)

	settleTxid, err := settle()
	require.NoError(t, err)
	require.NotEmpty(t, settleTxid)

	newHistory, err := getTransactionHistory()
	require.NoError(t, err)
	require.Len(t, newHistory, len(history))

	tx, err = findInHistory(txid, newHistory, boarding)
	require.NoError(t, err)
	require.True(t, tx.Settled)
}

func TestSendOffChain(t *testing.T) {
	initialBalance, err := getBalance()
	require.NoError(t, err)
	require.Greater(t, int64(initialBalance), int64(0))

	receiverAddr, err := getReceiverOffchainAddress()
	require.NoError(t, err)
	require.NotEmpty(t, receiverAddr)

	txid, err := sendOffChain(receiverAddr, 1000)
	require.NoError(t, err)
	require.NotEmpty(t, txid)

	time.Sleep(time.Second)

	balance, err := getBalance()
	require.NoError(t, err)
	require.Equal(t, int(initialBalance-1000), int(balance))

	// Test GetVirtualTxs RPC with the txid from the send operation
	virtualTxs, err := getVirtualTxs([]string{txid})
	require.NoError(t, err)
	require.Len(t, virtualTxs, 1, "should return one virtual transaction")
	require.NotEmpty(t, virtualTxs[0], "virtual transaction hex should not be empty")
}

func TestSendOnChain(t *testing.T) {
	initialBalance, err := getBalance()
	require.NoError(t, err)
	require.Greater(t, int64(initialBalance), int64(0))

	receiverAddr, err := getReceiverOnchainAddress()
	require.NoError(t, err)
	require.NotEmpty(t, receiverAddr)

	txid, err := sendOnChain(receiverAddr, 1000)
	require.NoError(t, err)
	require.NotEmpty(t, txid)

	time.Sleep(time.Second)

	balance, err := getBalance()
	require.NoError(t, err)
	require.Equal(t, int(initialBalance-1000), int(balance))
}

func TestGetVirtualTxs(t *testing.T) {
	// Get transaction history to find some txids
	history, err := getTransactionHistory()
	require.NoError(t, err)
	require.NotEmpty(t, history, "need at least one transaction in history")

	// Collect some txids from history
	var txids []string
	for _, tx := range history {
		if tx.RoundTxid != "" {
			txids = append(txids, tx.RoundTxid)
		}
		if len(txids) >= 2 {
			break
		}
	}
	require.NotEmpty(t, txids, "need at least one txid to test")

	// Test with single txid
	virtualTxs, err := getVirtualTxs([]string{txids[0]})
	require.NoError(t, err)
	require.Len(t, virtualTxs, 1, "should return one virtual transaction")
	require.NotEmpty(t, virtualTxs[0], "virtual transaction hex should not be empty")

	// Test with multiple txids if available
	if len(txids) > 1 {
		virtualTxs, err = getVirtualTxs(txids)
		require.NoError(t, err)
		require.Len(t, virtualTxs, len(txids), "should return all requested virtual transactions")
		for i, vtx := range virtualTxs {
			require.NotEmpty(t, vtx, "virtual transaction %d hex should not be empty", i)
		}
	}

	// Test with empty txids list
	virtualTxs, err = getVirtualTxs([]string{})
	require.NoError(t, err)
	require.Empty(t, virtualTxs, "empty txids should return empty response")
}

func TestVHTLC(t *testing.T) {
	// For sake of simplicity, in this test sender = receiver to test both
	// funding and claiming the VHTLC via API
	receiverPubkey, err := getPubkey()
	require.NoError(t, err)
	require.NotEmpty(t, receiverPubkey)

	// Create the VHTLC
	preimage := make([]byte, 32)
	_, err = rand.Read(preimage)
	require.NoError(t, err)
	sha256Hash := sha256.Sum256(preimage)
	preimageHash := hex.EncodeToString(input.Ripemd160H(sha256Hash[:]))

	vhtlc, err := createVHTLC(preimageHash, receiverPubkey)
	require.NoError(t, err)
	require.NotEmpty(t, vhtlc.Address)
	require.NotEmpty(t, vhtlc.ClaimPubkey)
	require.NotEmpty(t, vhtlc.RefundPubkey)
	require.NotEmpty(t, vhtlc.ServerPubkey)

	// Fund the VHTLC
	err = faucetOffchain(vhtlc.Address, "1000")
	require.NoError(t, err)

	// Get the VHTLC
	vhtlcs, err := listVHTLC(preimageHash)
	require.NoError(t, err)
	require.Len(t, vhtlcs, 1)

	// Claim the VHTLC
	redeemTxid, err := claimVHTLC(hex.EncodeToString(preimage))
	require.NoError(t, err)
	require.NotEmpty(t, redeemTxid)
}
