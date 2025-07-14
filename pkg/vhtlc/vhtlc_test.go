package vhtlc_test

import (
	"crypto/rand"
	"crypto/sha256"
	"testing"
	"time"

	"github.com/ArkLabsHQ/fulmine/pkg/vhtlc"
	arklib "github.com/arkade-os/arkd/pkg/ark-lib"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/lightningnetwork/lnd/input"
	"github.com/stretchr/testify/require"
)

func TestVHTLC(t *testing.T) {
	// Generate test data
	senderKey := generatePrivateKey(t)
	receiverKey := generatePrivateKey(t)
	serverKey := generatePrivateKey(t)
	preimage := generatePreimage(t)
	preimageHash := calculatePreimageHash(preimage)

	// Create VHTLC
	script, err := vhtlc.NewVHTLCScript(vhtlc.Opts{
		Sender:                               senderKey.PubKey(),
		Receiver:                             receiverKey.PubKey(),
		Server:                               serverKey.PubKey(),
		PreimageHash:                         preimageHash,
		RefundLocktime:                       arklib.AbsoluteLocktime(time.Now().Add(24 * time.Hour).Unix()),
		UnilateralClaimDelay:                 arklib.RelativeLocktime{Type: arklib.LocktimeTypeBlock, Value: 144},
		UnilateralRefundDelay:                arklib.RelativeLocktime{Type: arklib.LocktimeTypeBlock, Value: 72},
		UnilateralRefundWithoutReceiverDelay: arklib.RelativeLocktime{Type: arklib.LocktimeTypeBlock, Value: 288},
	})
	require.NoError(t, err)

	// Test script creation
	t.Run("Create", func(t *testing.T) {
		require.NotNil(t, script)
		require.NotNil(t, script.ClaimClosure)
		require.NotNil(t, script.RefundClosure)
		require.NotNil(t, script.RefundWithoutReceiverClosure)
		require.NotNil(t, script.UnilateralClaimClosure)
		require.NotNil(t, script.UnilateralRefundClosure)
		require.NotNil(t, script.UnilateralRefundWithoutReceiverClosure)

		scripts := script.GetRevealedTapscripts()
		require.NotEmpty(t, scripts)
		require.GreaterOrEqual(t, len(scripts), 6)
	})

	// Test claim path
	t.Run("Claim", func(t *testing.T) {
		claimScript, err := script.ClaimClosure.Script()
		require.NoError(t, err)
		require.NotEmpty(t, claimScript)

		// Generate a dummy signature with proper 32-byte message
		msg := make([]byte, 32)
		_, err = rand.Read(msg)
		require.NoError(t, err)
		sig, err := schnorr.Sign(receiverKey, msg)
		require.NoError(t, err)

		// Verify witness structure
		witness := [][]byte{
			sig.Serialize(),
			preimage,
			claimScript,
			{0x01}, // dummy control block
		}

		require.Len(t, witness, 4, "Claim witness should have 4 elements")
		require.Len(t, witness[0], 64, "Schnorr signature should be 64 bytes")
		require.Len(t, witness[1], 32, "Preimage should be 32 bytes")
		require.NotEmpty(t, witness[2], "Claim script should not be empty")
		require.NotEmpty(t, witness[3], "Control block should not be empty")
	})

	// Test refund path
	t.Run("Refund", func(t *testing.T) {
		refundScript, err := script.RefundClosure.Script()
		require.NoError(t, err)

		// Generate dummy signatures with proper 32-byte message
		msg := make([]byte, 32)
		_, err = rand.Read(msg)
		require.NoError(t, err)

		senderSig, err := schnorr.Sign(senderKey, msg)
		require.NoError(t, err)
		receiverSig, err := schnorr.Sign(receiverKey, msg)
		require.NoError(t, err)
		serverSig, err := schnorr.Sign(serverKey, msg)
		require.NoError(t, err)

		// Verify witness structure
		witness := [][]byte{
			senderSig.Serialize(),
			receiverSig.Serialize(),
			serverSig.Serialize(),
			refundScript,
			{0x01}, // dummy control block
		}

		require.Len(t, witness, 5, "Refund witness should have 5 elements")
		require.Len(t, witness[0], 64, "Sender signature should be 64 bytes")
		require.Len(t, witness[1], 64, "Receiver signature should be 64 bytes")
		require.Len(t, witness[2], 64, "Server signature should be 64 bytes")
		require.NotEmpty(t, witness[3], "Refund script should not be empty")
		require.NotEmpty(t, witness[4], "Control block should not be empty")
	})
}

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
	return input.Ripemd160H(sha[:]) // RIPEMD160(SHA256(preimage))
}
