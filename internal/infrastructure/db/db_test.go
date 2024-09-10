package db_test

import (
	"context"
	"testing"

	"github.com/ArkLabsHQ/ark-node/internal/core/domain"
	badgerdb "github.com/ArkLabsHQ/ark-node/internal/infrastructure/db/badger"
	"github.com/stretchr/testify/require"
)

var (
	dbs = map[string]func() (domain.SettingsRepository, error){
		"badger": func() (domain.SettingsRepository, error) {
			return badgerdb.NewSettingsRepo("", nil)
		},
	}
	testSettings = domain.Settings{
		ApiRoot:     "apiroot",
		AspUrl:      "aspurl",
		Currency:    "cur",
		EventServer: "eventserver",
		FullNode:    "fullnode",
		LnConnect:   false,
		LnUrl:       "lndconnect",
		Unit:        "unit",
	}
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
			AspUrl:   "updated aspurl",
			Currency: "updated cur",
		}
		expectedSettings.AspUrl = newSettings.AspUrl
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
