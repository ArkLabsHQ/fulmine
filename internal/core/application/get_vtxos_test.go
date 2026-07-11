package application

import (
	"testing"
	"time"

	clientTypes "github.com/arkade-os/arkd/pkg/client-lib/types"
	"github.com/arkade-os/go-sdk/types"
	"github.com/stretchr/testify/require"
)

// TestGetVtxos verifies Service.GetVtxos filters:
//
//	spendable:   !Spent && !Swept && !Unrolled
//	spent:       Spent || Swept || Unrolled
//	recoverable: (Swept || expired) && !Spent && !Unrolled
func TestGetVtxos(t *testing.T) {
	spendable := storedVtxo("spendable", false, false, false, time.Hour)
	spentV := storedVtxo("spent", true, false, false, time.Hour)
	sweptUnspent := storedVtxo("swept", false, true, false, time.Hour)
	unrolled := storedVtxo("unrolled", false, false, true, time.Hour)
	expiredUnspent := storedVtxo("expired", false, false, false, -time.Hour)

	fake := newFakeArkClient()
	// Buckets mirror the sql store's GetAllVtxos split (spent = Spent||Unrolled),
	// so swept-but-unspent and expired vtxos live in the "spendable" bucket.
	fake.setVtxos(spendable, sweptUnspent, expiredUnspent)
	fake.setSpentVtxos(spentV, unrolled)

	svc := &Service{
		ArkClient:     fake,
		isInitialized: true,
		syncEvent:     &types.SyncEvent{},
	}
	svc.walletReady.Store(true)
	ctx := t.Context()

	testCases := []struct {
		filter string
		want   []string
	}{
		{"all", []string{"spendable", "swept", "expired", "spent", "unrolled"}},
		{"spendable", []string{"spendable"}},
		{"spent", []string{"spent", "swept", "unrolled"}},
		{"recoverable", []string{"swept", "expired"}},
	}
	for _, tc := range testCases {
		t.Run(tc.filter, func(t *testing.T) {
			got, err := svc.GetVtxos(ctx, tc.filter)
			require.NoError(t, err)
			require.ElementsMatch(t, tc.want, vtxoTxids(got))
		})
	}

	t.Run("invalid filter", func(t *testing.T) {
		_, err := svc.GetVtxos(ctx, "wrong")
		require.Error(t, err)
	})
}

func storedVtxo(txid string, spent, swept, unrolled bool, expiresIn time.Duration) clientTypes.Vtxo {
	return clientTypes.Vtxo{
		Outpoint:  clientTypes.Outpoint{Txid: txid, VOut: 0},
		Amount:    1000,
		ExpiresAt: time.Now().Add(expiresIn),
		Spent:     spent,
		Swept:     swept,
		Unrolled:  unrolled,
	}
}

func vtxoTxids(vtxos []clientTypes.Vtxo) []string {
	txids := make([]string, 0, len(vtxos))
	for _, v := range vtxos {
		txids = append(txids, v.Txid)
	}
	return txids
}
