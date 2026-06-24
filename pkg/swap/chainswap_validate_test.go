package swap

import (
	"encoding/hex"
	"testing"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/btcsuite/btcd/txscript"
	"github.com/stretchr/testify/require"
)

// buildRefundLeafScript synthesizes a Boltz-format refund-leaf taproot script in
// exactly the layout parseRefundHTLCScriptManually expects:
//
//	<32-byte x-only refund pubkey> OP_CHECKSIGVERIFY <timeout> OP_CHECKLOCKTIMEVERIFY
//
// It mirrors what Boltz emits in the chain-swap tree's RefundLeaf, so the parser
// can be pinned without any live Boltz or regtest state.
func buildRefundLeafScript(t *testing.T, refundPubKey [32]byte, timeout int64) string {
	t.Helper()
	script, err := txscript.NewScriptBuilder().
		AddData(refundPubKey[:]).
		AddOp(txscript.OP_CHECKSIGVERIFY).
		AddInt64(timeout).
		AddOp(txscript.OP_CHECKLOCKTIMEVERIFY).
		Script()
	require.NoError(t, err)
	return hex.EncodeToString(script)
}

func testRefundXOnlyPubKey(t *testing.T) [32]byte {
	t.Helper()
	k, err := btcec.NewPrivateKey()
	require.NoError(t, err)
	var xonly [32]byte
	copy(xonly[:], schnorr.SerializePubKey(k.PubKey()))
	return xonly
}

// TestValidateRefundLeafScript pins the parse of the Boltz refund leaf, the
// fund-safety-critical step that feeds the BTC→ARK unilateral refund's CLTV gate
// (SwapHandler.RefundBtcToArkSwap). A misparsed timeout would let the refund
// broadcast before its timelock (consensus-rejected) or block one already
// reached; a misparsed pubkey yields an unspendable refund. Both strand user
// funds, so this is a fund-loss guard, not a cosmetic check.
//
// This covers the parse in isolation; the surrounding
// fetch→gate→build→sign→broadcast flow still needs a mocked
// ArkClient/ExplorerClient harness (tracked in #425).
func TestValidateRefundLeafScript(t *testing.T) {
	t.Run("valid script round-trips timeout and pubkey", func(t *testing.T) {
		// Boltz refund timeouts are absolute block heights, encoded as a
		// little-endian data push. These values exercise the parser's decode
		// across 2- and 3-byte pushes (the sub-128 / 1-byte case is covered
		// separately below).
		for _, timeout := range []int64{128, 144, 500, 70_000, 1_000_000} {
			pubKey := testRefundXOnlyPubKey(t)

			components, err := ValidateRefundLeafScript(buildRefundLeafScript(t, pubKey, timeout))

			require.NoError(t, err, "timeout %d", timeout)
			require.NotNil(t, components)
			require.Equal(t, uint32(timeout), components.Timeout, "timeout %d", timeout)
			require.Equal(t, pubKey, components.RefundPubKey, "timeout %d", timeout)
		}
	})

	t.Run("a sub-128 timeout falls below the 38-byte floor and is rejected", func(t *testing.T) {
		// A real parser edge surfaced while writing this test: a timeout < 128
		// encodes to a single byte, making the leaf 37 bytes, which trips
		// parseRefundHTLCScriptManually's hard-coded 38-byte minimum. Boltz CLTV
		// timeouts are absolute block heights (current height + timeout blocks)
		// and clear 128 in practice, so this isn't reachable today — but if it
		// ever were, the refund leaf wouldn't parse and the unilateral refund
		// would fail. Pinned so a parser change (e.g. a real minimal-length
		// check) surfaces the behavior shift. See #425.
		pubKey := testRefundXOnlyPubKey(t)
		_, err := ValidateRefundLeafScript(buildRefundLeafScript(t, pubKey, 100))
		require.ErrorContains(t, err, "too short")
	})

	t.Run("rejects non-hex input", func(t *testing.T) {
		_, err := ValidateRefundLeafScript("nothex!!")
		require.Error(t, err)
	})

	t.Run("rejects a script below the 38-byte minimum", func(t *testing.T) {
		_, err := ValidateRefundLeafScript(hex.EncodeToString([]byte{0x20, 0x01, 0x02}))
		require.Error(t, err)
	})

	t.Run("rejects a wrong signature opcode", func(t *testing.T) {
		// well-formed length, but OP_NOP where OP_CHECKSIGVERIFY must be.
		pubKey := testRefundXOnlyPubKey(t)
		var b []byte
		b = append(b, 0x20)
		b = append(b, pubKey[:]...)
		b = append(b, txscript.OP_NOP)
		b = append(b, 0x02, 0x90, 0x00) // 2-byte timeout push (144)
		b = append(b, txscript.OP_CHECKLOCKTIMEVERIFY)
		_, err := ValidateRefundLeafScript(hex.EncodeToString(b))
		require.Error(t, err)
	})

	t.Run("rejects a missing OP_CHECKLOCKTIMEVERIFY", func(t *testing.T) {
		pubKey := testRefundXOnlyPubKey(t)
		var b []byte
		b = append(b, 0x20)
		b = append(b, pubKey[:]...)
		b = append(b, txscript.OP_CHECKSIGVERIFY)
		b = append(b, 0x02, 0x90, 0x00)
		b = append(b, txscript.OP_NOP) // should be OP_CHECKLOCKTIMEVERIFY
		_, err := ValidateRefundLeafScript(hex.EncodeToString(b))
		require.Error(t, err)
	})

	t.Run("rejects trailing bytes after a well-formed script", func(t *testing.T) {
		pubKey := testRefundXOnlyPubKey(t)
		valid, err := hex.DecodeString(buildRefundLeafScript(t, pubKey, 144))
		require.NoError(t, err)
		_, err = ValidateRefundLeafScript(hex.EncodeToString(append(valid, 0xde, 0xad)))
		require.Error(t, err)
	})
}
