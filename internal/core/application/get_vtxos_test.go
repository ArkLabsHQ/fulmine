package application

import (
	"context"
	"testing"
	"time"

	clientTypes "github.com/arkade-os/arkd/pkg/client-lib/types"
	arksdk "github.com/arkade-os/go-sdk"
	"github.com/arkade-os/go-sdk/types"
	"github.com/stretchr/testify/require"
)

// TestGetVtxos verifies Service.GetVtxos filters:
//
//	spendable:   !Spent && !IsRecoverable() && !Unrolled
//	spent:       Spent || Swept || Unrolled
//	recoverable: IsRecoverable() && !Unrolled
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
		Wallet:        fake,
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

// fakeArkClient is a stand-in for the wallet embedded in Service.
//
// arksdk.Wallet is a wide interface and the tests here exercise a narrow slice
// of it, so the interface is embedded rather than implemented: only the methods
// a test actually needs are defined below. Anything else nil-panics when called,
// which is the behaviour we want — a test that starts depending on a new wallet
// method should fail loudly rather than silently receive a zero value.
type fakeArkClient struct {
	arksdk.Wallet

	vtxos      []clientTypes.Vtxo
	spentVtxos []clientTypes.Vtxo
}

func newFakeArkClient() *fakeArkClient {
	return &fakeArkClient{}
}

// setVtxos sets the unspent bucket, mirroring the sql store's GetAllVtxos split.
func (f *fakeArkClient) setVtxos(vtxos ...clientTypes.Vtxo) {
	f.vtxos = vtxos
}

// setSpentVtxos sets the spent bucket (Spent || Unrolled server-side).
func (f *fakeArkClient) setSpentVtxos(vtxos ...clientTypes.Vtxo) {
	f.spentVtxos = vtxos
}

// IsLocked reports the wallet as unlocked so isInitializedAndUnlocked passes;
// the other conditions it checks are plain Service fields the tests set directly.
func (f *fakeArkClient) IsLocked(context.Context) bool {
	return false
}

// ListVtxos returns both buckets as a single page.
//
// The real implementation pages and returns an opaque cursor; returning an empty
// cursor here means GetVtxos's pagination loop runs zero times, so these tests
// cover its filtering rather than its paging.
func (f *fakeArkClient) ListVtxos(
	context.Context, ...arksdk.ListVtxosOption,
) ([]clientTypes.Vtxo, string, error) {
	all := make([]clientTypes.Vtxo, 0, len(f.vtxos)+len(f.spentVtxos))
	all = append(all, f.vtxos...)
	all = append(all, f.spentVtxos...)
	return all, "", nil
}
