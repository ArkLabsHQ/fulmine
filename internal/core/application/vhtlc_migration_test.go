package application

import (
	"context"
	"encoding/hex"
	"fmt"
	"testing"

	"github.com/ArkLabsHQ/fulmine/internal/core/domain"
	"github.com/arkade-os/arkd/pkg/client-lib/identity"
	"github.com/arkade-os/go-sdk/contract"
	vhtlchandler "github.com/arkade-os/go-sdk/contract/handlers/vhtlc"
	"github.com/arkade-os/go-sdk/types"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/stretchr/testify/require"
)

func TestMigrateVhtlcs(t *testing.T) {
	t.Run("full migratation drops legacy vhtlcs", func(t *testing.T) {
		ctx := t.Context()
		preimage := hex.EncodeToString(make([]byte, 20))
		senderHex, _ := mustPubkeyHex(t)
		receiverHex, receiverPub := mustPubkeyHex(t)
		serverHex, _ := mustPubkeyHex(t)

		ourKeyRef := &identity.KeyRef{Id: "key-0", PubKey: receiverPub}
		store := &fakeStore{
			legacy:  []domain.LegacyVhtlc{legacyRow(preimage, senderHex, receiverHex, serverHex)},
			present: map[string]string{},
		}
		importer := &fakeImporter{tracked: map[string]bool{}}
		build := func(_ context.Context, _ vhtlchandler.ContractArgs) (*types.Contract, error) {
			return &types.Contract{Script: "deadbeef"}, nil
		}

		migrated, skipped, err := migrateVhtlcs(ctx, store, importer, ourKeyRef, build)
		require.NoError(t, err)
		require.Equal(t, 1, migrated)
		require.Equal(t, 0, skipped)
		require.Equal(t, []string{"deadbeef"}, importer.imported)
		require.Equal(t, "deadbeef", store.present[firstKey(store.present)])
		require.True(t, store.dropped)

		// Second run is a clean no-op: legacy dropped => nothing to do.
		migrated, skipped, err = migrateVhtlcs(ctx, store, importer, ourKeyRef, build)
		require.NoError(t, err)
		require.Equal(t, 0, migrated)
		require.Equal(t, 0, skipped)
	})

	t.Run("partial migration keeps legacy vhtlcs", func(t *testing.T) {
		ctx := context.Background()
		preimage := hex.EncodeToString(make([]byte, 20))
		senderHex, _ := mustPubkeyHex(t)
		receiverHex, receiverPub := mustPubkeyHex(t)
		serverHex, _ := mustPubkeyHex(t)
		strangerHex, _ := mustPubkeyHex(t)

		ourKeyRef := &identity.KeyRef{Id: "key-0", PubKey: receiverPub}
		good := legacyRow(preimage, senderHex, receiverHex, serverHex)
		bad := legacyRow(preimage, senderHex, strangerHex, serverHex) // we own neither side
		store := &fakeStore{legacy: []domain.LegacyVhtlc{good, bad}, present: map[string]string{}}
		importer := &fakeImporter{tracked: map[string]bool{}}
		build := func(_ context.Context, _ vhtlchandler.ContractArgs) (*types.Contract, error) {
			return &types.Contract{Script: "deadbeef"}, nil
		}

		migrated, skipped, err := migrateVhtlcs(ctx, store, importer, ourKeyRef, build)
		require.NoError(t, err)
		require.Equal(t, 1, migrated)
		require.Equal(t, 1, skipped)
		require.False(t, store.dropped) // legacy retained because a row was skipped
	})
}

func TestBuildVhtlcContractArgs(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		t.Run("we are receiver", func(t *testing.T) {
			preimage := hex.EncodeToString(make([]byte, 20)) // 20-byte hash160
			senderHex, _ := mustPubkeyHex(t)
			receiverHex, receiverPub := mustPubkeyHex(t)
			serverHex, _ := mustPubkeyHex(t)

			ourKeyRef := &identity.KeyRef{Id: "key-0", PubKey: receiverPub}
			row := legacyRow(preimage, senderHex, receiverHex, serverHex)

			args, id, err := buildVhtlcContractArgs(row, ourKeyRef)
			require.NoError(t, err)
			require.Equal(t, "key-0", args.ReceiverKeyId)
			require.Empty(t, args.SenderKeyId)
			require.NotNil(t, args.Sender)
			require.NotNil(t, args.Receiver)
			require.NotNil(t, args.Signer)
			require.Equal(t, serverHex, hex.EncodeToString(args.Signer.SerializeCompressed()))
			require.Len(t, args.PreimageHash, 20)
			require.Equal(t, uint32(224), args.UnilateralRefundWithoutReceiverDelay.Value)
			require.NotEmpty(t, id)
		})

		t.Run("we are sender", func(t *testing.T) {
			preimage := hex.EncodeToString(make([]byte, 20))
			senderHex, senderPub := mustPubkeyHex(t)
			receiverHex, _ := mustPubkeyHex(t)
			serverHex, _ := mustPubkeyHex(t)

			ourKeyRef := &identity.KeyRef{Id: "key-0", PubKey: senderPub}
			row := legacyRow(preimage, senderHex, receiverHex, serverHex)

			args, _, err := buildVhtlcContractArgs(row, ourKeyRef)
			require.NoError(t, err)
			require.Equal(t, "key-0", args.SenderKeyId)
			require.Empty(t, args.ReceiverKeyId)
			require.NotNil(t, args.Signer)
			require.Equal(t, serverHex, hex.EncodeToString(args.Signer.SerializeCompressed()))
		})
	})

	t.Run("invalid", func(t *testing.T) {
		t.Run("not owned", func(t *testing.T) {
			preimage := hex.EncodeToString(make([]byte, 20))
			senderHex, _ := mustPubkeyHex(t)
			receiverHex, _ := mustPubkeyHex(t)
			serverHex, _ := mustPubkeyHex(t)
			_, strangerPub := mustPubkeyHex(t)

			ourKeyRef := &identity.KeyRef{Id: "key-0", PubKey: strangerPub}
			row := legacyRow(preimage, senderHex, receiverHex, serverHex)

			_, _, err := buildVhtlcContractArgs(row, ourKeyRef)
			require.Error(t, err)
		})

		t.Run("bad legacy vhtlc", func(t *testing.T) {
			_, ourPub := mustPubkeyHex(t)
			ourKeyRef := &identity.KeyRef{Id: "key-0", PubKey: ourPub}
			row := legacyRow("zz", "nothex", "nothex", "nothex")

			_, _, err := buildVhtlcContractArgs(row, ourKeyRef)
			require.Error(t, err)
		})
	})
}

func mustPubkeyHex(t *testing.T) (string, *btcec.PublicKey) {
	t.Helper()
	priv, err := btcec.NewPrivateKey()
	require.NoError(t, err)
	pub := priv.PubKey()
	return hex.EncodeToString(pub.SerializeCompressed()), pub
}

func legacyRow(preimage, sender, receiver, server string) domain.LegacyVhtlc {
	return domain.LegacyVhtlc{
		PreimageHash:                             preimage,
		Sender:                                   sender,
		Receiver:                                 receiver,
		Server:                                   server,
		RefundLocktime:                           1000,
		UnilateralClaimDelayType:                 0,
		UnilateralClaimDelayValue:                512,
		UnilateralRefundDelayType:                0,
		UnilateralRefundDelayValue:               1024,
		UnilateralRefundWithoutReceiverDelayType: 1,
		UnilateralRefundWithoutReceiverDelayValue: 224,
	}
}

type fakeStore struct {
	legacy  []domain.LegacyVhtlc
	present map[string]string // id -> script (new vhtlc table)
	dropped bool
}

func (f *fakeStore) HasLegacy(context.Context) (bool, error) {
	return !f.dropped && len(f.legacy) > 0, nil
}
func (f *fakeStore) GetLegacy(context.Context) ([]domain.LegacyVhtlc, error) { return f.legacy, nil }
func (f *fakeStore) DropLegacy(context.Context) error                        { f.dropped = true; return nil }
func (f *fakeStore) Get(_ context.Context, id string) (*domain.Vhtlc, error) {
	if s, ok := f.present[id]; ok {
		return &domain.Vhtlc{Id: id, Script: s}, nil
	}
	return nil, fmt.Errorf("not found")
}
func (f *fakeStore) Add(_ context.Context, v domain.Vhtlc) error {
	f.present[v.Id] = v.Script
	return nil
}

type fakeImporter struct {
	tracked  map[string]bool // script -> tracked
	imported []string
}

func (f *fakeImporter) GetContracts(_ context.Context, opts ...contract.FilterOption) ([]types.Contract, error) {
	// The reconstruction only ever filters by a single script; nothing is
	// pre-tracked in these tests, so ImportContract is always exercised.
	_ = opts
	return []types.Contract{}, nil
}
func (f *fakeImporter) ImportContract(_ context.Context, c types.Contract) error {
	f.imported = append(f.imported, c.Script)
	return nil
}

func firstKey(m map[string]string) string {
	for k := range m {
		return k
	}
	return ""
}
