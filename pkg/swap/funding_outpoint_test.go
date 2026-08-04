package swap

import (
	"fmt"
	"testing"

	clientTypes "github.com/arkade-os/arkd/pkg/client-lib/types"
	"github.com/stretchr/testify/require"
)

func vtxoAt(txid string, vout uint32) clientTypes.Vtxo {
	return clientTypes.Vtxo{
		Outpoint: clientTypes.Outpoint{Txid: txid, VOut: vout},
	}
}

func TestSelectOutpointByFundingTxid(t *testing.T) {
	const funding = "aa11"
	const other = "bb22"

	t.Run("no funding txid leaves selection unchanged", func(t *testing.T) {
		outpoint, err := selectOutpointByFundingTxid(
			[]clientTypes.Vtxo{vtxoAt(other, 0)}, nil, "",
		)
		require.NoError(t, err)
		require.Nil(t, outpoint)
	})

	// Reporting ErrorNoVtxosFound rather than a nil outpoint is what keeps the
	// reverse swap waiting for the funding vtxo instead of falling back to
	// whatever else is already sitting at the address.
	t.Run("unmatched funding txid reports no vtxos found", func(t *testing.T) {
		outpoint, err := selectOutpointByFundingTxid(
			[]clientTypes.Vtxo{vtxoAt(other, 0), vtxoAt("cc33", 1)}, nil, funding,
		)
		require.ErrorIs(t, err, ErrorNoVtxosFound)
		require.Nil(t, outpoint)
	})

	t.Run("no vtxos at all reports no vtxos found", func(t *testing.T) {
		outpoint, err := selectOutpointByFundingTxid(nil, nil, funding)
		require.ErrorIs(t, err, ErrorNoVtxosFound)
		require.Nil(t, outpoint)

		// The empty address is the ordinary "still waiting" case and must stay
		// the bare sentinel: it is produced on every retry tick.
		require.Equal(t, ErrorNoVtxosFound.Error(), err.Error())
	})

	// Vtxos present but none from the funding tx is what a txid-semantics
	// mismatch looks like, so the error names what was there. It still has to
	// satisfy errors.Is, because the retry paths key off the sentinel.
	t.Run("unmatched funding txid names the vtxos that were present", func(t *testing.T) {
		_, err := selectOutpointByFundingTxid(
			[]clientTypes.Vtxo{vtxoAt(other, 0)}, []clientTypes.Vtxo{vtxoAt("cc33", 1)}, funding,
		)
		require.ErrorIs(t, err, ErrorNoVtxosFound)
		require.ErrorContains(t, err, funding)
		require.ErrorContains(t, err, other)
		require.ErrorContains(t, err, "cc33")
	})

	t.Run("reported vtxo list is capped", func(t *testing.T) {
		many := make([]clientTypes.Vtxo, 0, 12)
		for i := range 12 {
			many = append(many, vtxoAt(fmt.Sprintf("tx%02d", i), 0))
		}

		_, err := selectOutpointByFundingTxid(many, nil, funding)
		require.ErrorIs(t, err, ErrorNoVtxosFound)
		require.ErrorContains(t, err, "...")
		require.NotContains(t, err.Error(), "tx11")
	})

	t.Run("picks the funding vtxo over an older decoy", func(t *testing.T) {
		outpoint, err := selectOutpointByFundingTxid(
			[]clientTypes.Vtxo{vtxoAt(other, 0), vtxoAt(funding, 3)}, nil, funding,
		)
		require.NoError(t, err)
		require.NotNil(t, outpoint)
		require.Equal(t, clientTypes.Outpoint{Txid: funding, VOut: 3}, *outpoint)
	})

	t.Run("finds the funding vtxo among pending", func(t *testing.T) {
		outpoint, err := selectOutpointByFundingTxid(
			[]clientTypes.Vtxo{vtxoAt(other, 0)},
			[]clientTypes.Vtxo{vtxoAt(funding, 1)},
			funding,
		)
		require.NoError(t, err)
		require.NotNil(t, outpoint)
		require.Equal(t, clientTypes.Outpoint{Txid: funding, VOut: 1}, *outpoint)
	})

	// A vtxo may be reported as both spendable and pending. That is the same
	// candidate twice, not two competing outputs.
	t.Run("same outpoint in both lists is one candidate", func(t *testing.T) {
		same := vtxoAt(funding, 2)
		outpoint, err := selectOutpointByFundingTxid(
			[]clientTypes.Vtxo{same}, []clientTypes.Vtxo{same}, funding,
		)
		require.NoError(t, err)
		require.NotNil(t, outpoint)
		require.Equal(t, clientTypes.Outpoint{Txid: funding, VOut: 2}, *outpoint)
	})

	// One transaction paying the address twice gives no basis to choose, so it
	// is reported rather than resolved.
	t.Run("two outputs of the funding tx are ambiguous", func(t *testing.T) {
		outpoint, err := selectOutpointByFundingTxid(
			[]clientTypes.Vtxo{vtxoAt(funding, 0), vtxoAt(funding, 1)}, nil, funding,
		)
		require.ErrorContains(t, err, "several outputs")
		require.Nil(t, outpoint)
	})

	t.Run("ambiguity is detected across the two lists", func(t *testing.T) {
		outpoint, err := selectOutpointByFundingTxid(
			[]clientTypes.Vtxo{vtxoAt(funding, 0)},
			[]clientTypes.Vtxo{vtxoAt(funding, 1)},
			funding,
		)
		require.ErrorContains(t, err, "several outputs")
		require.Nil(t, outpoint)
	})
}
