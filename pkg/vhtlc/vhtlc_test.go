package vhtlc_test

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/ArkLabsHQ/fulmine/pkg/vhtlc"
	arklib "github.com/arkade-os/arkd/pkg/ark-lib"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/lightningnetwork/lnd/input"
	"github.com/stretchr/testify/require"
)

func TestVHTLCAddress(t *testing.T) {
	// Read the fixture file
	fixtureData, err := os.ReadFile("vhtlc_fixtures.json")
	require.NoError(t, err)

	var fixtures struct {
		Valid   []map[string]interface{} `json:"valid"`
		Invalid []map[string]interface{} `json:"invalid"`
	}
	err = json.Unmarshal(fixtureData, &fixtures)
	require.NoError(t, err)

	// Test valid cases
	t.Run("Valid", func(t *testing.T) {
		for i, testCase := range fixtures.Valid {
			t.Run(fmt.Sprintf("Case_%d_%s", i, testCase["description"]), func(t *testing.T) {
				// Parse test case
				preimageHashHex := testCase["preimageHash"].(string)
				receiverHex := testCase["receiver"].(string)
				senderHex := testCase["sender"].(string)
				serverHex := testCase["server"].(string)
				refundLocktime := int64(testCase["refundLocktime"].(float64))

				unilateralClaimDelay := testCase["unilateralClaimDelay"].(map[string]interface{})
				unilateralRefundDelay := testCase["unilateralRefundDelay"].(map[string]interface{})
				unilateralRefundWithoutReceiverDelay := testCase["unilateralRefundWithoutReceiverDelay"].(map[string]interface{})

				expectedAddress := testCase["expected"].(string)

				// Decode hex values
				preimageHashBytes, err := hex.DecodeString(preimageHashHex)
				require.NoError(t, err)

				// Decode compressed public keys and convert to 32-byte format
				receiverBytes, err := hex.DecodeString(receiverHex)
				require.NoError(t, err)
				receiverPubKey, err := btcec.ParsePubKey(receiverBytes)
				require.NoError(t, err)

				senderBytes, err := hex.DecodeString(senderHex)
				require.NoError(t, err)
				senderPubKey, err := btcec.ParsePubKey(senderBytes)
				require.NoError(t, err)

				serverBytes, err := hex.DecodeString(serverHex)
				require.NoError(t, err)
				serverPubKey, err := btcec.ParsePubKey(serverBytes)
				require.NoError(t, err)

				// Parse timelock types and values
				claimDelayType := parseTimelockType(unilateralClaimDelay["type"].(string))
				refundDelayType := parseTimelockType(unilateralRefundDelay["type"].(string))
				refundWithoutReceiverDelayType := parseTimelockType(unilateralRefundWithoutReceiverDelay["type"].(string))

				// Create VHTLC script
				script, err := vhtlc.NewVHTLCScript(vhtlc.Opts{
					Sender:                               senderPubKey,
					Receiver:                             receiverPubKey,
					Server:                               serverPubKey,
					PreimageHash:                         preimageHashBytes,
					RefundLocktime:                       arklib.AbsoluteLocktime(refundLocktime),
					UnilateralClaimDelay:                 arklib.RelativeLocktime{Type: claimDelayType, Value: uint32(unilateralClaimDelay["value"].(float64))},
					UnilateralRefundDelay:                arklib.RelativeLocktime{Type: refundDelayType, Value: uint32(unilateralRefundDelay["value"].(float64))},
					UnilateralRefundWithoutReceiverDelay: arklib.RelativeLocktime{Type: refundWithoutReceiverDelayType, Value: uint32(unilateralRefundWithoutReceiverDelay["value"].(float64))},
				})
				require.NoError(t, err)

				// Generate address
				address, err := script.Address("tark", serverPubKey)
				require.NoError(t, err)
				require.Equal(t, expectedAddress, address)
			})
		}
	})

	// Test invalid cases
	t.Run("Invalid", func(t *testing.T) {
		for i, testCase := range fixtures.Invalid {
			t.Run(fmt.Sprintf("Case_%d_%s", i, testCase["description"]), func(t *testing.T) {
				// Parse test case
				preimageHashHex := testCase["preimageHash"].(string)
				receiverHex := testCase["receiver"].(string)
				senderHex := testCase["sender"].(string)
				serverHex := testCase["server"].(string)
				refundLocktime := int64(testCase["refundLocktime"].(float64))

				unilateralClaimDelay := testCase["unilateralClaimDelay"].(map[string]interface{})
				unilateralRefundDelay := testCase["unilateralRefundDelay"].(map[string]interface{})
				unilateralRefundWithoutReceiverDelay := testCase["unilateralRefundWithoutReceiverDelay"].(map[string]interface{})

				expectedError := testCase["error"].(string)

				// Decode hex values
				preimageHashBytes, err := hex.DecodeString(preimageHashHex)
				require.NoError(t, err)

				// Decode compressed public keys and convert to 32-byte format
				receiverBytes, err := hex.DecodeString(receiverHex)
				require.NoError(t, err)
				receiverPubKey, err := btcec.ParsePubKey(receiverBytes)
				require.NoError(t, err)
				receiverKey, err := schnorr.ParsePubKey(receiverPubKey.SerializeCompressed()[1:]) // Remove prefix byte
				require.NoError(t, err)

				senderBytes, err := hex.DecodeString(senderHex)
				require.NoError(t, err)
				senderPubKey, err := btcec.ParsePubKey(senderBytes)
				require.NoError(t, err)
				senderKey, err := schnorr.ParsePubKey(senderPubKey.SerializeCompressed()[1:]) // Remove prefix byte
				require.NoError(t, err)

				serverBytes, err := hex.DecodeString(serverHex)
				require.NoError(t, err)
				serverPubKey, err := btcec.ParsePubKey(serverBytes)
				require.NoError(t, err)
				serverKey, err := schnorr.ParsePubKey(serverPubKey.SerializeCompressed()[1:]) // Remove prefix byte
				require.NoError(t, err)

				// Parse timelock types and values
				claimDelayType := parseTimelockType(unilateralClaimDelay["type"].(string))
				refundDelayType := parseTimelockType(unilateralRefundDelay["type"].(string))
				refundWithoutReceiverDelayType := parseTimelockType(unilateralRefundWithoutReceiverDelay["type"].(string))

				// Create VHTLC script - this should fail
				_, err = vhtlc.NewVHTLCScript(vhtlc.Opts{
					Sender:                               senderKey,
					Receiver:                             receiverKey,
					Server:                               serverKey,
					PreimageHash:                         preimageHashBytes,
					RefundLocktime:                       arklib.AbsoluteLocktime(refundLocktime),
					UnilateralClaimDelay:                 arklib.RelativeLocktime{Type: claimDelayType, Value: uint32(unilateralClaimDelay["value"].(float64))},
					UnilateralRefundDelay:                arklib.RelativeLocktime{Type: refundDelayType, Value: uint32(unilateralRefundDelay["value"].(float64))},
					UnilateralRefundWithoutReceiverDelay: arklib.RelativeLocktime{Type: refundWithoutReceiverDelayType, Value: uint32(unilateralRefundWithoutReceiverDelay["value"].(float64))},
				})
				require.Error(t, err, arklib.AbsoluteLocktime(refundLocktime))
				require.Contains(t, err.Error(), expectedError)
			})
		}
	})
}

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

// parseTimelockType converts string timelock type to arklib type
func parseTimelockType(typeStr string) arklib.RelativeLocktimeType {
	switch typeStr {
	case "seconds":
		return arklib.LocktimeTypeSecond
	case "blocks":
		return arklib.LocktimeTypeBlock
	default:
		return arklib.LocktimeTypeBlock
	}
}
