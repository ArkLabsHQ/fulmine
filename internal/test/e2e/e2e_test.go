package e2e_test

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"github.com/ArkLabsHQ/fulmine/utils"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ripemd160"
)

func TestOnboard(t *testing.T) {
	onboardAddress, err := getOnboardAddress(1000)
	require.NoError(t, err)

	require.False(t, utils.IsBip21(onboardAddress))
	txid, err := faucet(onboardAddress, "0.00001")
	require.NoError(t, err)
	require.NotEmpty(t, txid)

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
	const receivingAddr = "tark132dmk8aj42ftj4ta6zee4p0nr5sy7k95r33w5yfddtg532vgz2z5kyd6crud92v5nsmt2qkaartxt33292kngqds2up2wzpw9ugfvvcr9s64v"

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
	ripemd160Hasher := ripemd160.New()
	ripemd160Hasher.Write(sha256Hash[:])
	hash160hash := ripemd160Hasher.Sum(nil)
	preimageHash := hex.EncodeToString(hash160hash)
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
