package db_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"testing"

	"github.com/ArkLabsHQ/ark-node/internal/core/domain"
	badgerdb "github.com/ArkLabsHQ/ark-node/internal/infrastructure/db/badger"
	"github.com/ArkLabsHQ/ark-node/pkg/vhtlc"
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

	testSettings = domain.Settings{
		ApiRoot:     "apiroot",
		ServerUrl:   "serverurl",
		Currency:    "cur",
		EventServer: "eventserver",
		FullNode:    "fullnode",
		LnUrl:       "lndconnect",
		Unit:        "unit",
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
