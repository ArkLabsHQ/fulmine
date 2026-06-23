package swap

import (
	"crypto/sha256"
	"testing"

	"github.com/ArkLabsHQ/fulmine/pkg/vhtlc"
	arklib "github.com/arkade-os/arkd/pkg/ark-lib"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/wire"
	"github.com/lightningnetwork/lnd/input"
	"github.com/stretchr/testify/require"
)

// newTestVHTLCScript builds a VHTLC script with throwaway test keys, enough to
// exercise the refund-path selection and locktime/sequence logic without any
// live arkd/Boltz state. The specific keys don't matter for these assertions —
// only the closure shapes do.
func newTestVHTLCScript(t *testing.T, refundLocktime arklib.AbsoluteLocktime) *vhtlc.VHTLCScript {
	t.Helper()

	pubKey := func() *btcec.PublicKey {
		k, err := btcec.NewPrivateKey()
		require.NoError(t, err)
		return k.PubKey()
	}

	// VHTLC preimage hashes are RIPEMD160(SHA256(preimage)); a fixed preimage
	// keeps the fixture deterministic.
	sha := sha256.Sum256([]byte("fulmine-unilateral-refund-test"))
	preimageHash := input.Ripemd160H(sha[:])

	vhtlcScript, err := vhtlc.NewVHTLCScriptFromOpts(vhtlc.Opts{
		Sender:                               pubKey(),
		Receiver:                             pubKey(),
		Server:                               pubKey(),
		PreimageHash:                         preimageHash,
		RefundLocktime:                       refundLocktime,
		UnilateralClaimDelay:                 arklib.RelativeLocktime{Type: arklib.LocktimeTypeBlock, Value: 144},
		UnilateralRefundDelay:                arklib.RelativeLocktime{Type: arklib.LocktimeTypeBlock, Value: 72},
		UnilateralRefundWithoutReceiverDelay: arklib.RelativeLocktime{Type: arklib.LocktimeTypeBlock, Value: 224},
	})
	require.NoError(t, err)
	require.NotNil(t, vhtlcScript.RefundClosure)
	require.NotNil(t, vhtlcScript.RefundWithoutReceiverClosure)
	return vhtlcScript
}

// TestRefundForfeitTxBuilderSigningClosure pins the fund-safety-critical refund
// path selection for ARK→BTC chain swaps.
//
// When Boltz (the receiver) refuses to cooperate, the refund must spend the
// without-receiver closure (Sender+Server, CLTV-gated). Spending the
// cooperative RefundClosure (which also needs the receiver's signature) would
// be impossible without Boltz and the user's funds would be stuck — so a
// swapped condition here is a fund-loss bug, not a cosmetic one.
func TestRefundForfeitTxBuilderSigningClosure(t *testing.T) {
	vhtlcScript := newTestVHTLCScript(t, arklib.AbsoluteLocktime(2_000_000_000))

	t.Run("without receiver selects the without-receiver closure", func(t *testing.T) {
		builder := &refundForfeitTxBuilder{withReceiver: false}

		got := builder.getSigningClosure(vhtlcScript)

		require.Same(t, vhtlcScript.RefundWithoutReceiverClosure, got)
		require.NotSame(t, vhtlcScript.RefundClosure, got)
	})

	t.Run("with receiver selects the cooperative closure", func(t *testing.T) {
		builder := &refundForfeitTxBuilder{withReceiver: true}

		got := builder.getSigningClosure(vhtlcScript)

		require.Same(t, vhtlcScript.RefundClosure, got)
		require.NotSame(t, vhtlcScript.RefundWithoutReceiverClosure, got)
	})
}

// TestExtractLocktimeAndSequenceRefundPaths pins the consensus-level gating of
// the refund transaction's VTXO input.
//
// The without-receiver refund is gated by an absolute CLTV locktime, so its
// input sequence must be < 0xFFFFFFFF (here MaxTxInSequenceNum-1) for the
// timelock to be enforced by consensus. Returning MaxTxInSequenceNum would
// silently disable the timelock. The cooperative refund has no timelock, so it
// uses the final sequence and a zero locktime.
func TestExtractLocktimeAndSequenceRefundPaths(t *testing.T) {
	vhtlcScript := newTestVHTLCScript(t, arklib.AbsoluteLocktime(2_000_000_000))

	t.Run("without-receiver (CLTV) closure enables the timelock", func(t *testing.T) {
		locktime, sequence := extractLocktimeAndSequence(vhtlcScript.RefundWithoutReceiverClosure)

		// extract the closure's own absolute locktime...
		require.Equal(t, vhtlcScript.RefundWithoutReceiverClosure.Locktime, locktime)
		// ...and it must be a real, non-zero timelock...
		require.NotZero(t, locktime)
		// ...with a sequence that actually enables nLockTime enforcement.
		require.Equal(t, uint32(wire.MaxTxInSequenceNum-1), sequence)
	})

	t.Run("cooperative (non-CLTV) closure has no timelock", func(t *testing.T) {
		locktime, sequence := extractLocktimeAndSequence(vhtlcScript.RefundClosure)

		require.Equal(t, arklib.AbsoluteLocktime(0), locktime)
		require.Equal(t, uint32(wire.MaxTxInSequenceNum), sequence)
	})
}
