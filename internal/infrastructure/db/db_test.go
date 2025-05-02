package db_test

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"testing"

	"github.com/ArkLabsHQ/fulmine/internal/core/domain"
	"github.com/ArkLabsHQ/fulmine/internal/core/ports"
	"github.com/ArkLabsHQ/fulmine/internal/infrastructure/db"
	"github.com/ArkLabsHQ/fulmine/pkg/vhtlc"
	"github.com/ark-network/ark/common"
	"github.com/decred/dcrd/dcrec/secp256k1/v4"
	"github.com/stretchr/testify/require"
)

var (
	ctx = context.Background()

	testSettings = domain.Settings{
		ApiRoot:     "apiroot",
		ServerUrl:   "serverurl",
		Currency:    "cur",
		EventServer: "eventserver",
		FullNode:    "fullnode",
		LnUrl:       "lndconnect",
		Unit:        "unit",
	}

	testRolloverTarget = domain.VtxoRolloverTarget{
		Address:            "test_address",
		TaprootTree:        []string{"tapscript1", "tapscript2"},
		DestinationAddress: "destination_address",
	}
	secondRolloverTarget = domain.VtxoRolloverTarget{
		Address:            "second_address",
		TaprootTree:        []string{"other_tapscript1", "other_tapscript2"},
		DestinationAddress: "other_destination_address",
	}

	testVHTLC = func() vhtlc.Opts {
		randBytes := make([]byte, 20)
		_, _ = rand.Read(randBytes)

		serverKey, _ := secp256k1.GeneratePrivateKey()
		senderKey, _ := secp256k1.GeneratePrivateKey()
		receiverKey, _ := secp256k1.GeneratePrivateKey()

		return vhtlc.Opts{
			PreimageHash:   randBytes,
			Sender:         senderKey.PubKey(),
			Receiver:       receiverKey.PubKey(),
			Server:         serverKey.PubKey(),
			RefundLocktime: common.AbsoluteLocktime(100 * 600),
			UnilateralClaimDelay: common.RelativeLocktime{
				Type:  common.LocktimeTypeBlock,
				Value: 300,
			},
			UnilateralRefundDelay: common.RelativeLocktime{
				Type:  common.LocktimeTypeBlock,
				Value: 400,
			},
			UnilateralRefundWithoutReceiverDelay: common.RelativeLocktime{
				Type:  common.LocktimeTypeBlock,
				Value: 500,
			},
		}
	}()
)

func TestRepoManager(t *testing.T) {
	dbDir := t.TempDir()
	tests := []struct {
		name   string
		config db.ServiceConfig
	}{
		{
			name: "badger",
			config: db.ServiceConfig{
				DbType:   "badger",
				DbConfig: []any{"", nil},
			},
		},
		{
			name: "sqlite",
			config: db.ServiceConfig{
				DbType:   "sqlite",
				DbConfig: []any{dbDir},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, err := db.NewService(tt.config)
			require.NoError(t, err)
			defer svc.Close()

			testSettingsRepository(t, svc)
			testVHTLCRepository(t, svc)
			testVtxoRolloverRepository(t, svc)
		})
	}
}

func testSettingsRepository(t *testing.T, svc ports.RepoManager) {
	t.Run("settings repository", func(t *testing.T) {
		testAddSettings(t, svc.Settings())
		testUpdateSettings(t, svc.Settings())
		testCleanSettings(t, svc.Settings())
	})
}

func testVHTLCRepository(t *testing.T, svc ports.RepoManager) {
	t.Run("vHTLC repository", func(t *testing.T) {
		testAddVHTLC(t, svc.VHTLC())
		testGetAllVHTLC(t, svc.VHTLC())
		testDeleteVHTLC(t, svc.VHTLC())
	})
}

func testVtxoRolloverRepository(t *testing.T, svc ports.RepoManager) {
	t.Run("vtxo rollover repository", func(t *testing.T) {
		testAddVtxoRolloverTarget(t, svc.VtxoRollover())
		testGetAllVtxoRolloverTargets(t, svc.VtxoRollover())
		testDeleteVtxoRolloverTarget(t, svc.VtxoRollover())
	})
}

func testAddSettings(t *testing.T, repo domain.SettingsRepository) {
	t.Run("add settings", func(t *testing.T) {
		settings, err := repo.GetSettings(ctx)
		require.Error(t, err)
		require.Nil(t, settings)

		err = repo.AddSettings(ctx, testSettings)
		require.NoError(t, err)

		settings, err = repo.GetSettings(ctx)
		require.NoError(t, err)
		require.Equal(t, testSettings, *settings)

		err = repo.AddSettings(ctx, testSettings)
		require.Error(t, err)

		err = repo.CleanSettings(ctx)
		require.NoError(t, err)
	})
}

func testUpdateSettings(t *testing.T, repo domain.SettingsRepository) {
	t.Run("update settings", func(t *testing.T) {
		newSettings := domain.Settings{
			ApiRoot: "updated apiroot",
		}

		err := repo.UpdateSettings(ctx, newSettings)
		require.Error(t, err)

		err = repo.AddSettings(ctx, testSettings)
		require.NoError(t, err)

		expectedSettings := testSettings
		expectedSettings.ApiRoot = newSettings.ApiRoot

		err = repo.UpdateSettings(ctx, newSettings)
		require.NoError(t, err)

		settings, err := repo.GetSettings(ctx)
		require.NoError(t, err)
		require.NotNil(t, settings)
		require.Equal(t, expectedSettings, *settings)

		newSettings = domain.Settings{
			ServerUrl: "updated serverurl",
			Currency:  "updated cur",
		}
		expectedSettings.ServerUrl = newSettings.ServerUrl
		expectedSettings.Currency = newSettings.Currency

		err = repo.UpdateSettings(ctx, newSettings)
		require.NoError(t, err)
		require.NotNil(t, settings)

		settings, err = repo.GetSettings(ctx)
		require.NoError(t, err)
		require.NotNil(t, settings)
		require.Equal(t, expectedSettings, *settings)
		require.Equal(t, expectedSettings, *settings)
	})
}

func testCleanSettings(t *testing.T, repo domain.SettingsRepository) {
	t.Run("clean settings", func(t *testing.T) {
		settings, err := repo.GetSettings(ctx)
		require.NoError(t, err)
		require.NotNil(t, settings)

		err = repo.CleanSettings(ctx)
		require.NoError(t, err)

		settings, err = repo.GetSettings(ctx)
		require.Error(t, err)
		require.Nil(t, settings)

		err = repo.CleanSettings(ctx)
		require.Error(t, err)
	})
}

func testAddVHTLC(t *testing.T, repo domain.VHTLCRepository) {
	t.Run("add vHTLC", func(t *testing.T) {
		opt, err := repo.Get(ctx, hex.EncodeToString(testVHTLC.PreimageHash))
		require.Error(t, err)
		require.Nil(t, opt)

		err = repo.Add(ctx, testVHTLC)
		require.NoError(t, err)

		opt, err = repo.Get(ctx, hex.EncodeToString(testVHTLC.PreimageHash))
		require.NoError(t, err)
		require.NotNil(t, opt)
		require.Equal(t, testVHTLC, *opt)

		err = repo.Add(ctx, testVHTLC)
		require.Error(t, err)
	})
}

func testGetAllVHTLC(t *testing.T, repo domain.VHTLCRepository) {
	t.Run("get all vHTLCs", func(t *testing.T) {
		opts, err := repo.GetAll(ctx)
		require.NoError(t, err)
		require.Len(t, opts, 1)

		// Add another vHTLC
		secondVHTLC := testVHTLC
		secondVHTLC.PreimageHash = []byte("second_preimage_hash")
		err = repo.Add(ctx, secondVHTLC)
		require.NoError(t, err)

		// Get all vHTLCs
		opts, err = repo.GetAll(ctx)
		require.NoError(t, err)
		require.Len(t, opts, 2)
		require.Subset(t, []vhtlc.Opts{testVHTLC, secondVHTLC}, opts)
	})
}

func testDeleteVHTLC(t *testing.T, repo domain.VHTLCRepository) {
	t.Run("delete vHTLC", func(t *testing.T) {
		err := repo.Delete(ctx, "non_existent_hash")
		require.Error(t, err)

		// Delete existing vHTLCs
		err = repo.Delete(ctx, hex.EncodeToString(testVHTLC.PreimageHash))
		require.NoError(t, err)
		secondVHTLC := testVHTLC
		secondVHTLC.PreimageHash = []byte("second_preimage_hash")
		err = repo.Delete(ctx, hex.EncodeToString(secondVHTLC.PreimageHash))
		require.NoError(t, err)

		// Verify it was deleted
		opt, err := repo.Get(ctx, hex.EncodeToString(testVHTLC.PreimageHash))
		require.Error(t, err)
		require.Nil(t, opt)

		opts, err := repo.GetAll(ctx)
		require.NoError(t, err)
		require.Empty(t, opts)
	})
}

func testAddVtxoRolloverTarget(t *testing.T, repo domain.VtxoRolloverRepository) {
	t.Run("add rollover target", func(t *testing.T) {
		target, err := repo.GetTarget(ctx, testRolloverTarget.Address)
		require.Error(t, err)
		require.Nil(t, target)

		// Add new target
		err = repo.AddTarget(ctx, testRolloverTarget)
		require.NoError(t, err)

		// Verify the target was added correctly
		target, err = repo.GetTarget(ctx, testRolloverTarget.Address)
		require.NoError(t, err)
		require.NotNil(t, target)
		require.Equal(t, testRolloverTarget, *target)

		// Try to add the same target again, should not error
		err = repo.AddTarget(ctx, testRolloverTarget)
		require.NoError(t, err)
	})
}

func testGetAllVtxoRolloverTargets(t *testing.T, repo domain.VtxoRolloverRepository) {
	t.Run("get all rollover targets", func(t *testing.T) {
		targets, err := repo.GetAllTargets(ctx)
		require.NoError(t, err)
		require.Len(t, targets, 1)

		// Add a second target
		err = repo.AddTarget(ctx, secondRolloverTarget)
		require.NoError(t, err)

		// Get all targets
		targets, err = repo.GetAllTargets(ctx)
		require.NoError(t, err)
		require.Len(t, targets, 2)
		require.Subset(t, []domain.VtxoRolloverTarget{testRolloverTarget, secondRolloverTarget}, targets)
	})
}

func testDeleteVtxoRolloverTarget(t *testing.T, repo domain.VtxoRolloverRepository) {
	t.Run("delete rollover target", func(t *testing.T) {
		// Delete existing targets
		err := repo.DeleteTarget(ctx, testRolloverTarget.Address)
		require.NoError(t, err)
		err = repo.DeleteTarget(ctx, secondRolloverTarget.Address)
		require.NoError(t, err)

		// Verify it was removed
		target, err := repo.GetTarget(ctx, testRolloverTarget.Address)
		require.Error(t, err)
		require.Nil(t, target)

		targets, err := repo.GetAllTargets(ctx)
		require.NoError(t, err)
		require.Empty(t, targets)

		// Try to remove it again, should not error
		err = repo.DeleteTarget(ctx, testRolloverTarget.Address)
		require.NoError(t, err)
	})
}
