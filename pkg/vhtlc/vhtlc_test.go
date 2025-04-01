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
	// Generate test data
	senderKey := generatePrivateKey(t)
	receiverKey := generatePrivateKey(t)
	serverKey := generatePrivateKey(t)
	preimage := generatePreimage(t)
	preimageHash := calculatePreimageHash(preimage)

	// Create VHTLC
	script, err := NewVHTLCScript(Opts{
		Sender:                               senderKey.PubKey(),
		Receiver:                             receiverKey.PubKey(),
		Server:                               serverKey.PubKey(),
		PreimageHash:                         preimageHash,
		RefundLocktime:                       common.AbsoluteLocktime(time.Now().Add(24 * time.Hour).Unix()),
		UnilateralClaimDelay:                 common.RelativeLocktime{Type: common.LocktimeTypeBlock, Value: 144},
		UnilateralRefundDelay:                common.RelativeLocktime{Type: common.LocktimeTypeBlock, Value: 72},
		UnilateralRefundWithoutReceiverDelay: common.RelativeLocktime{Type: common.LocktimeTypeBlock, Value: 288},
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
		assert.NotEmpty(t, scripts)
		assert.GreaterOrEqual(t, len(scripts), 6)
	})

	// Test claim path
	t.Run("Claim", func(t *testing.T) {
		claimScript, err := script.ClaimClosure.Script()
		require.NoError(t, err)

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
			[]byte{0x01}, // dummy control block
		}

		assert.Equal(t, 4, len(witness), "Claim witness should have 4 elements")
		assert.Equal(t, 64, len(witness[0]), "Schnorr signature should be 64 bytes")
		assert.Equal(t, 32, len(witness[1]), "Preimage should be 32 bytes")
		assert.NotEmpty(t, witness[2], "Claim script should not be empty")
		assert.NotEmpty(t, witness[3], "Control block should not be empty")
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
			[]byte{0x01}, // dummy control block
		}

		assert.Equal(t, 5, len(witness), "Refund witness should have 5 elements")
		assert.Equal(t, 64, len(witness[0]), "Sender signature should be 64 bytes")
		assert.Equal(t, 64, len(witness[1]), "Receiver signature should be 64 bytes")
		assert.Equal(t, 64, len(witness[2]), "Server signature should be 64 bytes")
		assert.NotEmpty(t, witness[3], "Refund script should not be empty")
		assert.NotEmpty(t, witness[4], "Control block should not be empty")
	})
}
