package db_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"testing"

	"github.com/ArkLabsHQ/fulmine/internal/core/domain"
	badgerdb "github.com/ArkLabsHQ/fulmine/internal/infrastructure/db/badger"
	"github.com/ArkLabsHQ/fulmine/pkg/vhtlc"
	"github.com/ark-network/ark/common"
	"github.com/decred/dcrd/dcrec/secp256k1/v4"
	"github.com/stretchr/testify/require"
)

var (
	dbs = map[string]func() (domain.SettingsRepository, error){
		"badger": func() (domain.SettingsRepository, error) {
			return badgerdb.NewSettingsRepo("", nil)
		},
	}

	vhtlcDbs = map[string]func() (domain.VHTLCRepository, error){
		"badger": func() (domain.VHTLCRepository, error) {
			return badgerdb.NewVHTLCRepo("", nil)
		},
	}

	vtxoRolloverDbs = map[string]func() (domain.VtxoRolloverRepository, error){
		"badger": func() (domain.VtxoRolloverRepository, error) {
			return badgerdb.NewVtxoRolloverRepo("", nil)
		},
	}

	testSettings = domain.Settings{
		ApiRoot:     "apiroot",
		ServerUrl:   "serverurl",
		Currency:    "cur",
		EventServer: "eventserver",
		FullNode:    "fullnode",
		LnUrl:       "lndconnect",
		Unit:        "unit",
	}

	testVtxoRolloverTarget = domain.VtxoRolloverTarget{
		Address:            "test_address",
		TaprootTree:        []string{"tapscript1", "tapscript2"},
		DestinationAddress: "destination_address",
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

func TestSettingsRepo(t *testing.T) {
	repos, err := getSettingsRepos()
	require.NoError(t, err)

	for _, v := range repos {
		t.Parallel()

		t.Run(v.name, func(t *testing.T) {
			testAddSettings(t, v.repo)

			testUpdateSettings(t, v.repo)

			testCleanSettings(t, v.repo)
		})
	}
}

func testAddSettings(t *testing.T, repo domain.SettingsRepository) {
	t.Run("add settings", func(t *testing.T) {
		ctx := context.Background()
		settings, err := repo.GetSettings(ctx)
		require.Error(t, err)
		require.Nil(t, settings)

		err = repo.CleanSettings(ctx)
		require.Error(t, err)

		err = repo.AddSettings(ctx, testSettings)
		require.NoError(t, err)

		err = repo.AddSettings(ctx, testSettings)
		require.Error(t, err)

		settings, err = repo.GetSettings(ctx)
		require.NoError(t, err)
		require.Equal(t, testSettings, *settings)
	})
}

func testUpdateSettings(t *testing.T, repo domain.SettingsRepository) {
	t.Run("update settings", func(t *testing.T) {
		ctx := context.Background()
		newSettings := domain.Settings{
			ApiRoot: "updated apiroot",
		}
		expectedSettings := testSettings
		expectedSettings.ApiRoot = newSettings.ApiRoot

		err := repo.UpdateSettings(ctx, newSettings)
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
		ctx := context.Background()

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

type settingsDb struct {
	name string
	repo domain.SettingsRepository
}

func getSettingsRepos() ([]settingsDb, error) {
	var repos []settingsDb
	for dbName, factory := range dbs {
		repo, err := factory()
		if err != nil {
			return nil, err
		}
		repos = append(repos, settingsDb{dbName, repo})
	}
	return repos, nil
}

func TestVHTLCRepo(t *testing.T) {
	repos, err := getVHTLCRepos()
	require.NoError(t, err)

	for _, v := range repos {
		t.Parallel()

		t.Run(v.name, func(t *testing.T) {
			testAddVHTLC(t, v.repo)
			testGetVHTLC(t, v.repo)
			testGetAllVHTLC(t, v.repo)
			testDeleteVHTLC(t, v.repo)
		})
	}
}

func testAddVHTLC(t *testing.T, repo domain.VHTLCRepository) {
	t.Run("add vHTLC", func(t *testing.T) {
		ctx := context.Background()

		// Add new vHTLC
		err := repo.Add(ctx, testVHTLC)
		require.NoError(t, err)

		// Verify Get returns the added vHTLC
		opt, err := repo.Get(ctx, hex.EncodeToString(testVHTLC.PreimageHash))
		require.NoError(t, err)
		require.NotNil(t, opt)
		require.Equal(t, testVHTLC, *opt)
	})
}

func testGetVHTLC(t *testing.T, repo domain.VHTLCRepository) {
	t.Run("get vHTLC", func(t *testing.T) {
		ctx := context.Background()

		// Get non-existent vHTLC
		opt, err := repo.Get(ctx, "non_existent_hash")
		require.Error(t, err)
		require.Nil(t, opt)
	})
}

func testGetAllVHTLC(t *testing.T, repo domain.VHTLCRepository) {
	t.Run("get all vHTLC", func(t *testing.T) {
		ctx := context.Background()

		// Add another vHTLC
		secondVHTLC := testVHTLC
		secondVHTLC.PreimageHash = []byte("second_preimage_hash")
		err := repo.Add(ctx, secondVHTLC)
		require.NoError(t, err)

		// Get all vHTLCs
		opts, err := repo.GetAll(ctx)
		require.NoError(t, err)
		require.Len(t, opts, 2)

		// Verify both vHTLCs are present
		found := 0
		for _, opt := range opts {
			if bytes.Equal(opt.PreimageHash, testVHTLC.PreimageHash) || bytes.Equal(opt.PreimageHash, secondVHTLC.PreimageHash) {
				found++
			}
		}
		require.Equal(t, 2, found)
	})
}

func testDeleteVHTLC(t *testing.T, repo domain.VHTLCRepository) {
	t.Run("delete vHTLC", func(t *testing.T) {
		ctx := context.Background()

		// Delete existing vHTLC
		err := repo.Delete(ctx, hex.EncodeToString(testVHTLC.PreimageHash))
		require.NoError(t, err)

		// Verify it was deleted
		opt, err := repo.Get(ctx, hex.EncodeToString(testVHTLC.PreimageHash))
		require.Error(t, err)
		require.Nil(t, opt)

		err = repo.Delete(ctx, "non_existent_hash")
		require.Error(t, err)
	})
}

type vhtlcDb struct {
	name string
	repo domain.VHTLCRepository
}

func getVHTLCRepos() ([]vhtlcDb, error) {
	var repos []vhtlcDb
	for dbName, factory := range vhtlcDbs {
		repo, err := factory()
		if err != nil {
			return nil, err
		}
		repos = append(repos, vhtlcDb{dbName, repo})
	}
	return repos, nil
}

func TestVtxoRolloverRepo(t *testing.T) {
	repos, err := getVtxoRolloverRepos()
	require.NoError(t, err)

	for _, v := range repos {
		t.Parallel()

		t.Run(v.name, func(t *testing.T) {
			testAddVtxoRolloverTarget(t, v.repo)
			testGetVtxoRolloverTarget(t, v.repo)
			testGetAllVtxoRolloverTargets(t, v.repo)
			testRemoveVtxoRolloverTarget(t, v.repo)
		})
	}
}

func testAddVtxoRolloverTarget(t *testing.T, repo domain.VtxoRolloverRepository) {
	t.Run("add vtxo rollover target", func(t *testing.T) {
		ctx := context.Background()

		// Add new target
		err := repo.AddTarget(ctx, testVtxoRolloverTarget)
		require.NoError(t, err)

		// Try to add the same target again, should not error
		err = repo.AddTarget(ctx, testVtxoRolloverTarget)
		require.NoError(t, err)

		// Verify the target was added correctly
		target, err := repo.GetTarget(ctx, testVtxoRolloverTarget.Address)
		require.NoError(t, err)
		require.NotNil(t, target)
		require.Equal(t, testVtxoRolloverTarget, *target)
	})
}

func testGetVtxoRolloverTarget(t *testing.T, repo domain.VtxoRolloverRepository) {
	t.Run("get vtxo rollover target", func(t *testing.T) {
		ctx := context.Background()

		// Try to get a non-existent target
		target, err := repo.GetTarget(ctx, "non_existent_address")
		require.Error(t, err)
		require.Nil(t, target)

		// Get an existing target
		target, err = repo.GetTarget(ctx, testVtxoRolloverTarget.Address)
		require.NoError(t, err)
		require.NotNil(t, target)
		require.Equal(t, testVtxoRolloverTarget, *target)
	})
}

func testGetAllVtxoRolloverTargets(t *testing.T, repo domain.VtxoRolloverRepository) {
	t.Run("get all vtxo rollover targets", func(t *testing.T) {
		ctx := context.Background()

		// Add a second target
		secondTarget := domain.VtxoRolloverTarget{
			Address:            "second_address",
			TaprootTree:        []string{"other_tapscript1", "other_tapscript2"},
			DestinationAddress: "other_destination_address",
		}
		err := repo.AddTarget(ctx, secondTarget)
		require.NoError(t, err)

		// Get all targets
		targets, err := repo.GetAllTargets(ctx)
		require.NoError(t, err)
		require.GreaterOrEqual(t, len(targets), 2)

		// Verify both targets are present
		found := 0
		for _, t := range targets {
			if t.Address == testVtxoRolloverTarget.Address || t.Address == secondTarget.Address {
				found++
			}
		}
		require.GreaterOrEqual(t, found, 2)
	})
}

func testRemoveVtxoRolloverTarget(t *testing.T, repo domain.VtxoRolloverRepository) {
	t.Run("remove vtxo rollover target", func(t *testing.T) {
		ctx := context.Background()

		// Remove an existing target
		err := repo.RemoveTarget(ctx, testVtxoRolloverTarget.Address)
		require.NoError(t, err)

		// Verify it was removed
		target, err := repo.GetTarget(ctx, testVtxoRolloverTarget.Address)
		require.Error(t, err)
		require.Nil(t, target)

		// Try to remove it again, should not error
		err = repo.RemoveTarget(ctx, testVtxoRolloverTarget.Address)
		require.NoError(t, err)

		// Try to remove a non-existent target, should not error
		err = repo.RemoveTarget(ctx, "non_existent_address")
		require.NoError(t, err)
	})
}

type vtxoRolloverDb struct {
	name string
	repo domain.VtxoRolloverRepository
}

func getVtxoRolloverRepos() ([]vtxoRolloverDb, error) {
	var repos []vtxoRolloverDb
	for dbName, factory := range vtxoRolloverDbs {
		repo, err := factory()
		if err != nil {
			return nil, err
		}
		repos = append(repos, vtxoRolloverDb{dbName, repo})
	}
	return repos, nil
}
