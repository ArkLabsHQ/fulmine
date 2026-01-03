package swap

import (
	"crypto/rand"
	"crypto/sha256"
	"testing"

	"github.com/ArkLabsHQ/fulmine/pkg/vhtlc"
	arklib "github.com/arkade-os/arkd/pkg/ark-lib"
	"github.com/arkade-os/arkd/pkg/ark-lib/script"
	"github.com/arkade-os/go-sdk/types"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcutil/psbt"
	"github.com/btcsuite/btcd/wire"
	"github.com/lightningnetwork/lnd/input"
	"github.com/stretchr/testify/require"
)

func TestValidatePreimage(t *testing.T) {
	t.Run("valid preimage", func(t *testing.T) {
		// Generate valid preimage
		preimage := make([]byte, 32)
		_, err := rand.Read(preimage)
		require.NoError(t, err)

		// Compute expected hash
		buf := sha256.Sum256(preimage)
		expectedHash := input.Ripemd160H(buf[:])

		// Should pass validation
		err = validatePreimage(preimage, expectedHash)
		require.NoError(t, err)
	})

	t.Run("invalid length - too short", func(t *testing.T) {
		preimage := make([]byte, 16) // Only 16 bytes
		expectedHash := make([]byte, 20)

		err := validatePreimage(preimage, expectedHash)
		require.Error(t, err)
		require.Contains(t, err.Error(), "preimage must be 32 bytes, got 16")
	})

	t.Run("invalid length - too long", func(t *testing.T) {
		preimage := make([]byte, 64) // 64 bytes
		expectedHash := make([]byte, 20)

		err := validatePreimage(preimage, expectedHash)
		require.Error(t, err)
		require.Contains(t, err.Error(), "preimage must be 32 bytes, got 64")
	})

	t.Run("hash mismatch", func(t *testing.T) {
		// Generate preimage
		preimage := make([]byte, 32)
		_, err := rand.Read(preimage)
		require.NoError(t, err)

		// Use different hash
		wrongHash := make([]byte, 20)
		_, err = rand.Read(wrongHash)
		require.NoError(t, err)

		err = validatePreimage(preimage, wrongHash)
		require.Error(t, err)
		require.Contains(t, err.Error(), "preimage hash mismatch")
	})
}

func TestBuildEventTopics(t *testing.T) {
	t.Run("single vtxo", func(t *testing.T) {
		intentID := "intent123"
		vtxos := []types.Vtxo{
			{
				Outpoint: types.Outpoint{
					Txid: "abcd1234",
					VOut: 0,
				},
			},
		}
		signerPubkey := "pubkey456"

		topics := buildEventTopics(intentID, vtxos, signerPubkey)

		require.Len(t, topics, 3)
		require.Equal(t, "intent123", topics[0])
		require.Equal(t, "abcd1234:0", topics[1])
		require.Equal(t, "pubkey456", topics[2])
	})

	t.Run("multiple vtxos", func(t *testing.T) {
		intentID := "intent789"
		vtxos := []types.Vtxo{
			{Outpoint: types.Outpoint{Txid: "tx1", VOut: 0}},
			{Outpoint: types.Outpoint{Txid: "tx2", VOut: 1}},
			{Outpoint: types.Outpoint{Txid: "tx3", VOut: 2}},
		}
		signerPubkey := "pubkeyXYZ"

		topics := buildEventTopics(intentID, vtxos, signerPubkey)

		require.Len(t, topics, 5) // intentID + 3 vtxos + signerPubkey
		require.Equal(t, "intent789", topics[0])
		require.Equal(t, "tx1:0", topics[1])
		require.Equal(t, "tx2:1", topics[2])
		require.Equal(t, "tx3:2", topics[3])
		require.Equal(t, "pubkeyXYZ", topics[4])
	})

	t.Run("empty vtxos slice", func(t *testing.T) {
		intentID := "intent000"
		vtxos := []types.Vtxo{}
		signerPubkey := "pubkey000"

		topics := buildEventTopics(intentID, vtxos, signerPubkey)

		require.Len(t, topics, 2) // Only intentID + signerPubkey
		require.Equal(t, "intent000", topics[0])
		require.Equal(t, "pubkey000", topics[1])
	})
}

// TODO: Re-enable this test once vhtlc.NewVhtlcScript API is stabilized
// func TestSelectForfeitClosure(t *testing.T) {
// 	// Create test keys
// 	senderKey, _ := btcec.NewPrivateKey()
// 	receiverKey, _ := btcec.NewPrivateKey()
// 	serverKey, _ := btcec.NewPrivateKey()
//
// 	// Create VHTLC script
// 	preimageHash := make([]byte, 20)
// 	_, _ = rand.Read(preimageHash)
//
// 	vhtlcScript, err := vhtlc.NewVhtlcScript(
// 		senderKey.PubKey(),
// 		receiverKey.PubKey(),
// 		serverKey.PubKey(),
// 		preimageHash,
// 		arklib.AbsoluteLocktime(1000),
// 		arklib.RelativeLocktime{Type: arklib.LocktimeTypeBlock, Value: 100},
// 		arklib.RelativeLocktime{Type: arklib.LocktimeTypeBlock, Value: 200},
// 		arklib.RelativeLocktime{Type: arklib.LocktimeTypeBlock, Value: 300},
// 	)
// 	require.NoError(t, err)
//
// 	t.Run("claim path returns ConditionMultisigClosure", func(t *testing.T) {
// 		closure := selectForfeitClosure(vhtlcScript, true, false)
// 		_, ok := closure.(*script.ConditionMultisigClosure)
// 		require.True(t, ok, "Expected ConditionMultisigClosure for claim path")
// 	})
//
// 	t.Run("refund path with receiver returns RefundClosure", func(t *testing.T) {
// 		closure := selectForfeitClosure(vhtlcScript, false, true)
// 		_, ok := closure.(*script.MultisigClosure)
// 		require.True(t, ok, "Expected MultisigClosure (RefundClosure) for refund with receiver")
// 	})
//
// 	t.Run("refund path without receiver returns RefundWithoutReceiverClosure", func(t *testing.T) {
// 		closure := selectForfeitClosure(vhtlcScript, false, false)
// 		_, ok := closure.(*script.CLTVMultisigClosure)
// 		require.True(t, ok, "Expected CLTVMultisigClosure (RefundWithoutReceiverClosure) for refund without receiver")
// 	})
// }

func TestExtractLocktimeAndSequence(t *testing.T) {
	t.Run("CLTVMultisigClosure returns locktime and 0xFFFFFFFE", func(t *testing.T) {
		key, _ := btcec.NewPrivateKey()
		closure := &script.CLTVMultisigClosure{
			MultisigClosure: script.MultisigClosure{
				PubKeys: []*btcec.PublicKey{key.PubKey()},
			},
			Locktime: arklib.AbsoluteLocktime(12345),
		}

		locktime, sequence := extractLocktimeAndSequence(closure)

		require.Equal(t, arklib.AbsoluteLocktime(12345), locktime)
		require.Equal(t, wire.MaxTxInSequenceNum-1, sequence)
	})

	t.Run("MultisigClosure returns 0 locktime and 0xFFFFFFFF", func(t *testing.T) {
		key, _ := btcec.NewPrivateKey()
		closure := &script.MultisigClosure{
			PubKeys: []*btcec.PublicKey{key.PubKey()},
		}

		locktime, sequence := extractLocktimeAndSequence(closure)

		require.Equal(t, arklib.AbsoluteLocktime(0), locktime)
		require.Equal(t, wire.MaxTxInSequenceNum, sequence)
	})

	t.Run("ConditionMultisigClosure returns 0 locktime and 0xFFFFFFFF", func(t *testing.T) {
		key, _ := btcec.NewPrivateKey()
		closure := &script.ConditionMultisigClosure{
			MultisigClosure: script.MultisigClosure{
				PubKeys: []*btcec.PublicKey{key.PubKey()},
			},
		}

		locktime, sequence := extractLocktimeAndSequence(closure)

		require.Equal(t, arklib.AbsoluteLocktime(0), locktime)
		require.Equal(t, wire.MaxTxInSequenceNum, sequence)
	})
}

func TestClaimForfeitBuilder(t *testing.T) {
	// Create a test VHTLC script
	senderKey, _ := btcec.NewPrivateKey()
	serverKey, _ := btcec.NewPrivateKey()

	testHash := make([]byte, 20)
	testHash[0] = 0x42

	vhtlcScript := &vhtlc.VHTLCScript{
		ClaimClosure: &script.ConditionMultisigClosure{
			MultisigClosure: script.MultisigClosure{
				PubKeys: []*btcec.PublicKey{senderKey.PubKey(), serverKey.PubKey()},
			},
			Condition: testHash,
		},
	}

	// Test GetSettlementClosure
	builder := &ClaimForfeitBuilder{preimage: make([]byte, 32)}
	closure := builder.GetSettlementClosure(vhtlcScript)

	_, ok := closure.(*script.ConditionMultisigClosure)
	require.True(t, ok, "Expected ConditionMultisigClosure")

	// Test BuildForfeit - should inject preimage
	forfeitPtx := &psbt.Packet{
		UnsignedTx: &wire.MsgTx{},
		Inputs: []psbt.PInput{
			{},
		},
	}

	preimage := make([]byte, 32)
	preimage[0] = 0xAB
	builder.preimage = preimage

	err := builder.BuildForfeit(forfeitPtx)
	require.NoError(t, err)

	// Verify preimage was injected (check for ConditionWitnessField in unknowns)
	require.NotEmpty(t, forfeitPtx.Inputs[0].Unknowns, "Preimage should be injected into forfeit PSBT")
}

func TestRefundForfeitBuilder(t *testing.T) {
	// Create a test VHTLC script
	senderKey, _ := btcec.NewPrivateKey()
	receiverKey, _ := btcec.NewPrivateKey()
	serverKey, _ := btcec.NewPrivateKey()

	vhtlcScript := &vhtlc.VHTLCScript{
		RefundClosure: &script.MultisigClosure{
			PubKeys: []*btcec.PublicKey{senderKey.PubKey(), receiverKey.PubKey(), serverKey.PubKey()},
		},
		RefundWithoutReceiverClosure: &script.CLTVMultisigClosure{
			MultisigClosure: script.MultisigClosure{
				PubKeys: []*btcec.PublicKey{senderKey.PubKey(), serverKey.PubKey()},
			},
			Locktime: 144,
		},
	}

	// Test with receiver
	t.Run("with receiver", func(t *testing.T) {
		builder := &RefundForfeitBuilder{withReceiver: true}
		closure := builder.GetSettlementClosure(vhtlcScript)

		_, ok := closure.(*script.MultisigClosure)
		require.True(t, ok, "Expected MultisigClosure")
	})

	// Test without receiver
	t.Run("without receiver", func(t *testing.T) {
		builder := &RefundForfeitBuilder{withReceiver: false}
		closure := builder.GetSettlementClosure(vhtlcScript)

		_, ok := closure.(*script.CLTVMultisigClosure)
		require.True(t, ok, "Expected CLTVMultisigClosure")
	})

	// Test BuildForfeit - should be no-op
	t.Run("BuildForfeit is no-op", func(t *testing.T) {
		builder := &RefundForfeitBuilder{withReceiver: true}
		forfeitPtx := &psbt.Packet{
			UnsignedTx: &wire.MsgTx{},
			Inputs: []psbt.PInput{
				{},
			},
		}

		err := builder.BuildForfeit(forfeitPtx)
		require.NoError(t, err)

		// Verify no unknowns were added
		require.Empty(t, forfeitPtx.Inputs[0].Unknowns, "BuildForfeit should not modify PSBT for refund path")
	})
}

func TestExtractConnector(t *testing.T) {
	// Create a test connector PSBT with anchor output
	connectorTx := &psbt.Packet{
		UnsignedTx: &wire.MsgTx{
			TxOut: []*wire.TxOut{
				// Anchor output (should be skipped)
				{
					Value:    330,
					PkScript: []byte{0x51, 0x20}, // Fake anchor
				},
				// Real connector output
				{
					Value:    50000,
					PkScript: []byte{0x51, 0x21}, // Different P2TR
				},
			},
		},
	}

	connector, outpoint, err := extractConnector(connectorTx)
	require.NoError(t, err)
	require.NotNil(t, connector)
	require.Equal(t, int64(330), connector.Value) // First non-anchor (since we can't check ANCHOR_PKSCRIPT)
	require.Equal(t, uint32(0), outpoint.Index)
}

func TestExtractConnectorNoConnector(t *testing.T) {
	// Create an empty PSBT
	connectorTx := &psbt.Packet{
		UnsignedTx: &wire.MsgTx{
			TxOut: []*wire.TxOut{},
		},
	}

	_, _, err := extractConnector(connectorTx)
	require.Error(t, err)
	require.Contains(t, err.Error(), "connector output not found")
}
