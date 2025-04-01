package vhtlc

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"testing"
	"time"

	"github.com/ark-network/ark/common"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	"github.com/decred/dcrd/dcrec/secp256k1/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Helper function to generate a random private key
func generatePrivateKey(t *testing.T) *secp256k1.PrivateKey {
	t.Helper()
	privKey, err := btcec.NewPrivateKey()
	require.NoError(t, err)
	return secp256k1.PrivKeyFromBytes(privKey.Serialize())
}

// Helper function to generate a random preimage
func generatePreimage(t *testing.T) []byte {
	t.Helper()
	preimage := make([]byte, 32)
	_, err := rand.Read(preimage)
	require.NoError(t, err)
	return preimage
}

// Helper function to calculate hash160 of a preimage
func calculatePreimageHash(preimage []byte) []byte {
	sha := sha256.Sum256(preimage)
	return sha[:20] // Take first 20 bytes for hash160
}

func TestVHTLCCreateClaimRefund(t *testing.T) {
	// Generate keys for the test
	senderPrivKey := generatePrivateKey(t)
	receiverPrivKey := generatePrivateKey(t)
	serverPrivKey := generatePrivateKey(t)

	senderPubKey := senderPrivKey.PubKey()
	receiverPubKey := receiverPrivKey.PubKey()
	serverPubKey := serverPrivKey.PubKey()

	// Generate a random preimage and calculate its hash
	preimage := generatePreimage(t)
	preimageHash := calculatePreimageHash(preimage)

	// Create VHTLC options
	opts := Opts{
		Sender:                               senderPubKey,
		Receiver:                             receiverPubKey,
		Server:                               serverPubKey,
		PreimageHash:                         preimageHash,
		RefundLocktime:                       common.AbsoluteLocktime(time.Now().Add(24 * time.Hour).Unix()),
		UnilateralClaimDelay:                 common.RelativeLocktime{Type: common.LocktimeTypeBlock, Value: 144}, // ~1 day in blocks
		UnilateralRefundDelay:                common.RelativeLocktime{Type: common.LocktimeTypeBlock, Value: 72},  // ~12 hours in blocks
		UnilateralRefundWithoutReceiverDelay: common.RelativeLocktime{Type: common.LocktimeTypeBlock, Value: 288}, // ~2 days in blocks
	}

	// Test 1: Create VHTLC
	t.Run("Create", func(t *testing.T) {
		vtxoScript, err := NewVHTLCScript(opts)
		require.NoError(t, err)
		require.NotNil(t, vtxoScript)

		// Verify all closures are properly set
		assert.NotNil(t, vtxoScript.ClaimClosure)
		assert.NotNil(t, vtxoScript.RefundClosure)
		assert.NotNil(t, vtxoScript.RefundWithoutReceiverClosure)
		assert.NotNil(t, vtxoScript.UnilateralClaimClosure)
		assert.NotNil(t, vtxoScript.UnilateralRefundClosure)
		assert.NotNil(t, vtxoScript.UnilateralRefundWithoutReceiverClosure)

		// Test GetRevealedTapscripts
		scripts := vtxoScript.GetRevealedTapscripts()
		assert.NotEmpty(t, scripts)
		assert.GreaterOrEqual(t, len(scripts), 6) // Should have at least 6 scripts
	})

	// Create a test transaction and VHTLC for claim/refund tests
	vtxoScript, err := NewVHTLCScript(opts)
	require.NoError(t, err)

	// Create a taproot output
	taprootKey, tapTree, err := vtxoScript.TapTree()
	require.NoError(t, err)

	// Create a funding transaction
	fundingTx := wire.NewMsgTx(2)

	// Add a dummy input
	dummyHash := chainhash.Hash{}
	fundingTx.AddTxIn(wire.NewTxIn(wire.NewOutPoint(&dummyHash, 0), nil, nil))

	// Create taproot script
	taprootScript, err := txscript.PayToTaprootScript(taprootKey)
	require.NoError(t, err)

	// Add the output
	fundingTx.AddTxOut(wire.NewTxOut(100000, taprootScript))

	// Test 2: Claim VHTLC
	t.Run("Claim", func(t *testing.T) {
		// Get the claim script
		claimScript, err := vtxoScript.ClaimClosure.Script()
		require.NoError(t, err)

		// Get the merkle proof for the claim script
		claimLeafHash := txscript.NewBaseTapLeaf(claimScript).TapHash()
		claimProof, err := tapTree.GetTaprootMerkleProof(claimLeafHash)
		require.NoError(t, err)

		// Create a spending transaction
		spendingTx := wire.NewMsgTx(2)

		// Add input from the funding transaction
		txHash := fundingTx.TxHash()
		spendingTx.AddTxIn(wire.NewTxIn(
			wire.NewOutPoint(&txHash, 0),
			nil, nil,
		))

		// Add a destination output
		receiverTaprootScript, err := txscript.PayToTaprootScript(receiverPubKey)
		require.NoError(t, err)
		spendingTx.AddTxOut(wire.NewTxOut(99000, receiverTaprootScript)) // With fee

		// Build a claim transaction using bitcointree
		_, err = txscript.ParseControlBlock(claimProof.ControlBlock)
		require.NoError(t, err)

		// Verify the claim script requires the preimage
		assert.Contains(t, hex.EncodeToString(claimScript), hex.EncodeToString(preimageHash))
	})

	// Test 3: Refund VHTLC
	t.Run("Refund", func(t *testing.T) {
		// Get the refund script
		refundScript, err := vtxoScript.RefundClosure.Script()
		require.NoError(t, err)

		// Get the merkle proof for the refund script
		refundLeafHash := txscript.NewBaseTapLeaf(refundScript).TapHash()
		refundProof, err := tapTree.GetTaprootMerkleProof(refundLeafHash)
		require.NoError(t, err)

		// Create a spending transaction
		spendingTx := wire.NewMsgTx(2)

		// Add input from the funding transaction
		txHash := fundingTx.TxHash()
		spendingTx.AddTxIn(wire.NewTxIn(
			wire.NewOutPoint(&txHash, 0),
			nil, nil,
		))

		// Add a destination output
		senderTaprootScript, err := txscript.PayToTaprootScript(senderPubKey)
		require.NoError(t, err)
		spendingTx.AddTxOut(wire.NewTxOut(99000, senderTaprootScript)) // With fee

		// Build a refund transaction using bitcointree
		_, err = txscript.ParseControlBlock(refundProof.ControlBlock)
		require.NoError(t, err)
	})
}
