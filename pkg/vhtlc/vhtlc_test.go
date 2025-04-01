package vhtlc

import (
	"crypto/rand"
	"crypto/sha256"
	"golang.org/x/crypto/ripemd160"
	"testing"
	"time"

	"github.com/ark-network/ark/common"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Helper function to generate a random private key
func generatePrivateKey(t *testing.T) *btcec.PrivateKey {
	t.Helper()
	privKey, err := btcec.NewPrivateKey()
	require.NoError(t, err)
	return privKey
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
	rmd := ripemd160.New()
	rmd.Write(sha[:])
	return rmd.Sum(nil) // RIPEMD160(SHA256(preimage))
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
	fundingAmount := int64(100000)
	fundingTx.AddTxOut(wire.NewTxOut(fundingAmount, taprootScript))

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

		// Create previous output fetcher and sighash cache
		prevOuts := txscript.NewCannedPrevOutputFetcher(taprootScript, fundingAmount)
		hashCache := txscript.NewTxSigHashes(spendingTx, prevOuts)

		// Sign the transaction
		sigHash, err := txscript.CalcTaprootSignatureHash(hashCache, txscript.SigHashDefault,
			spendingTx, 0, prevOuts)
		require.NoError(t, err)
		sig, err := schnorr.Sign(receiverPrivKey, sigHash)
		require.NoError(t, err)

		// Create witness stack with valid preimage
		witness := wire.TxWitness{
			sig.Serialize(),         // Receiver's signature
			preimage,                // Valid preimage
			claimScript,             // Claim script
			claimProof.ControlBlock, // Control block
		}
		spendingTx.TxIn[0].Witness = witness

		// Verify witness structure
		require.Equal(t, 4, len(witness), "Witness should have 4 elements")
		require.Equal(t, 64, len(witness[0]), "Schnorr signature should be 64 bytes")
		require.Equal(t, 32, len(witness[1]), "Preimage should be 32 bytes")
		require.NotEmpty(t, witness[2], "Claim script should not be empty")
		require.NotEmpty(t, witness[3], "Control block should not be empty")

		// Test with invalid preimage
		invalidPreimage := make([]byte, 32)
		copy(invalidPreimage, preimage)
		invalidPreimage[0] ^= 0xff // Flip some bits
		witness[1] = invalidPreimage
		spendingTx.TxIn[0].Witness = witness

		// Verify witness structure
		require.Equal(t, 4, len(witness), "Witness should have 4 elements")
		require.Equal(t, 64, len(witness[0]), "Schnorr signature should be 64 bytes")
		require.Equal(t, 32, len(witness[1]), "Preimage should be 32 bytes")
		require.NotEmpty(t, witness[2], "Claim script should not be empty")
		require.NotEmpty(t, witness[3], "Control block should not be empty")

		// Test with invalid signature
		invalidSig, err := schnorr.Sign(senderPrivKey, sigHash) // Wrong key
		require.NoError(t, err)
		witness[0] = invalidSig.Serialize()
		witness[1] = preimage // Restore valid preimage
		spendingTx.TxIn[0].Witness = witness

		// Verify witness structure
		require.Equal(t, 4, len(witness), "Witness should have 4 elements")
		require.Equal(t, 64, len(witness[0]), "Schnorr signature should be 64 bytes")
		require.Equal(t, 32, len(witness[1]), "Preimage should be 32 bytes")
		require.NotEmpty(t, witness[2], "Claim script should not be empty")
		require.NotEmpty(t, witness[3], "Control block should not be empty")
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

		// Set locktime to after refund timelock
		spendingTx.LockTime = uint32(opts.RefundLocktime)

		// Create previous output fetcher and sighash cache
		prevOuts := txscript.NewCannedPrevOutputFetcher(taprootScript, fundingAmount)
		hashCache := txscript.NewTxSigHashes(spendingTx, prevOuts)

		// Sign the transaction
		sigHash, err := txscript.CalcTaprootSignatureHash(hashCache, txscript.SigHashDefault,
			spendingTx, 0, prevOuts)
		require.NoError(t, err)

		// Get signatures from all parties
		senderSig, err := schnorr.Sign(senderPrivKey, sigHash)
		require.NoError(t, err)
		receiverSig, err := schnorr.Sign(receiverPrivKey, sigHash)
		require.NoError(t, err)
		serverSig, err := schnorr.Sign(serverPrivKey, sigHash)
		require.NoError(t, err)

		// Create witness stack
		witness := wire.TxWitness{
			senderSig.Serialize(),    // Sender's signature
			receiverSig.Serialize(),  // Receiver's signature
			serverSig.Serialize(),    // Server's signature
			refundScript,             // Refund script
			refundProof.ControlBlock, // Control block
		}
		spendingTx.TxIn[0].Witness = witness

		// Verify witness structure
		require.Equal(t, 5, len(witness), "Witness should have 5 elements")
		require.Equal(t, 64, len(witness[0]), "Schnorr signature should be 64 bytes")
		require.Equal(t, 64, len(witness[1]), "Schnorr signature should be 64 bytes")
		require.Equal(t, 64, len(witness[2]), "Schnorr signature should be 64 bytes")
		require.NotEmpty(t, witness[3], "Refund script should not be empty")
		require.NotEmpty(t, witness[4], "Control block should not be empty")

		// Test with invalid timelock (before refund time)
		spendingTx.LockTime = uint32(opts.RefundLocktime) - 1

		// Verify witness structure
		require.Equal(t, 5, len(witness), "Witness should have 5 elements")
		require.Equal(t, 64, len(witness[0]), "Schnorr signature should be 64 bytes")
		require.Equal(t, 64, len(witness[1]), "Schnorr signature should be 64 bytes")
		require.Equal(t, 64, len(witness[2]), "Schnorr signature should be 64 bytes")
		require.NotEmpty(t, witness[3], "Refund script should not be empty")
		require.NotEmpty(t, witness[4], "Control block should not be empty")

		// Test with invalid signature
		spendingTx.LockTime = uint32(opts.RefundLocktime)         // Restore valid timelock
		invalidSig, err := schnorr.Sign(receiverPrivKey, sigHash) // Wrong key
		require.NoError(t, err)
		witness[0] = invalidSig.Serialize() // Replace sender's signature with invalid one
		spendingTx.TxIn[0].Witness = witness

		// Verify witness structure
		require.Equal(t, 5, len(witness), "Witness should have 5 elements")
		require.Equal(t, 64, len(witness[0]), "Schnorr signature should be 64 bytes")
		require.Equal(t, 64, len(witness[1]), "Schnorr signature should be 64 bytes")
		require.Equal(t, 64, len(witness[2]), "Schnorr signature should be 64 bytes")
		require.NotEmpty(t, witness[3], "Refund script should not be empty")
		require.NotEmpty(t, witness[4], "Control block should not be empty")
	})
}
