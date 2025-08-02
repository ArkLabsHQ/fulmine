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
	onboardAddress, err := getOnboardAddress(1000)
	require.NoError(t, err)

	require.False(t, utils.IsBip21(onboardAddress))
	txid, err := faucet(onboardAddress, "0.00001")
	require.NoError(t, err)
	require.NotEmpty(t, txid)

	time.Sleep(11 * time.Second) // onchain polling interval is 10 seconds

	history, err := getTransactionHistory()
	require.NoError(t, err)
	require.NotEmpty(t, history)

	tx, err := findInHistory(txid, history, boarding)
	require.NoError(t, err)
	require.Equal(t, tx.Amount, "1000")
	require.False(t, tx.Settled)

	settleTxid, err := settle()
	require.NoError(t, err)
	require.NotEmpty(t, settleTxid)

	history, err = getTransactionHistory()
	require.NoError(t, err)
	require.NotEmpty(t, history)

	tx, err = findInHistory(txid, history, boarding)
	require.NoError(t, err)
	require.True(t, tx.Settled, txid)
}

func TestSendOffChain(t *testing.T) {
	const receivingAddr = "tark1qz9fhwclk24f9w240hgt8x597vwjqn6ckswx96s3944dzj9f3qfg2dk2u4fadt0jj54kf8s3y42gr4fzl4f8xc5hfgl5kazuvk5cwsj5zg4aet"

	onboardAddress, err := getOnboardAddress(1000)
	require.NoError(t, err)

	require.False(t, utils.IsBip21(onboardAddress))
	txid, err := faucet(onboardAddress, "0.00001")
	require.NoError(t, err)
	require.NotEmpty(t, txid)

	txid, err = settle()
	require.NoError(t, err)
	require.NotEmpty(t, txid)

	initialBalance, err := getBalance()
	require.NoError(t, err)

	txid, err = sendOffChain(receivingAddr, 1000)
	require.NoError(t, err)
	require.NotEmpty(t, txid)

	balance, err := getBalance()
	require.NoError(t, err)
	require.Equal(t, int(initialBalance-1000), int(balance))
}

func TestSendOnChain(t *testing.T) {
	const receivingAddr = "bcrt1qqn8ttrwd8r3zee2e7fsdf6ylk23jphrpszu3tx"

	onboardAddress, err := getOnboardAddress(1000)
	require.NoError(t, err)

	require.False(t, utils.IsBip21(onboardAddress))
	txid, err := faucet(onboardAddress, "0.00001")
	require.NoError(t, err)
	require.NotEmpty(t, txid)

	txid, err = settle()
	require.NoError(t, err)
	require.NotEmpty(t, txid)

	initialBalance, err := getBalance()
	require.NoError(t, err)

	txid, err = sendOnChain(receivingAddr, 1000)
	require.NoError(t, err)
	require.NotEmpty(t, txid)

	balance, err := getBalance()
	require.NoError(t, err)
	require.Equal(t, int(initialBalance-1000), int(balance))
}

func TestVHTLC(t *testing.T) {
	// Create a VHTLC
	preimage := make([]byte, 32) // 32 bytes is a common size for preimages
	_, err := rand.Read(preimage)
	require.NoError(t, err)
	sha256Hash := sha256.Sum256(preimage)
	preimageHash := hex.EncodeToString(input.Ripemd160H(sha256Hash[:]))
	// hardcoded wallet's pubkey, here sender = receiver in order to test the claim RPC
	receiverPubkey := "02cdd6cf3ae57f1bafef11048c3bc1164e106cfd4b0d538bfb2d936866a2f19202"

	vhtlc, err := createVHTLC(
		preimageHash,
		receiverPubkey,
	)
	require.NoError(t, err)
	require.NotEmpty(t, vhtlc.Address)
	require.NotEmpty(t, vhtlc.ClaimPubkey)
	require.NotEmpty(t, vhtlc.RefundPubkey)
	require.NotEmpty(t, vhtlc.ServerPubkey)

	// fund the vhtlc
	err = faucetOffchain(vhtlc.Address, "1000")
	require.NoError(t, err)

	// list VHTLCs and verify our new one is there
	vhtlcs, err := listVHTLC(preimageHash)
	require.NoError(t, err)
	require.Len(t, vhtlcs, 1)

	// Claim the VHTLC
	redeemTxid, err := claimVHTLC(hex.EncodeToString(preimage))
	require.NoError(t, err)
	require.NotEmpty(t, redeemTxid)
}
