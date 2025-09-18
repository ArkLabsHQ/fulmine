package macaroon_test

import (
	"context"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	mk "github.com/ArkLabsHQ/fulmine/pkg/macaroon"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/metadata"
	"gopkg.in/macaroon-bakery.v2/bakery"
	"gopkg.in/macaroon.v2"
)

const (
	password        = "password"
	macFileName     = "admin.macaroon"
	macaroonsFolder = "macaroons"
	whitelistedEP   = "/health.Service/Check"
	protectedEP     = "/fulmine.Service/GetInfo"
)

func TestService(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	tmp := t.TempDir()

	datadir := filepath.Join(tmp, macaroonsFolder)

	macFiles := map[string][]bakery.Op{
		macFileName: {
			{Entity: "admin", Action: "access"},
		},
	}
	whitelisted := map[string][]bakery.Op{
		whitelistedEP: {},
	}
	allMethods := map[string][]bakery.Op{
		protectedEP:   {{Entity: "admin", Action: "access"}},
		whitelistedEP: {},
	}

	svc, err := mk.NewService(tmp, macaroonsFolder, macFiles, whitelisted, allMethods)
	require.NoError(t, err)
	require.NotNil(t, svc)

	t.Run("setup", func(t *testing.T) {
		err := svc.Unlock(ctx, password)
		require.NoError(t, err)

		err = svc.Generate(ctx)
		require.NoError(t, err)

		wantMacPath := filepath.Join(datadir, macFileName)
		_, err = os.Stat(wantMacPath)
		require.NoError(t, err)

		macaroonBytes, err := os.ReadFile(wantMacPath)
		require.NoError(t, err)
		require.NotEmpty(t, macaroonBytes)

		var macaroon macaroon.Macaroon
		err = macaroon.UnmarshalBinary(macaroonBytes)
		require.NoError(t, err)

		macaroonHex := hex.EncodeToString(macaroonBytes)
		md := metadata.Pairs("macaroon", macaroonHex)
		ctx = metadata.NewIncomingContext(ctx, md)
	})

	t.Run("auth", func(t *testing.T) {
		err = svc.Auth(context.TODO(), whitelistedEP)
		require.NoError(t, err)

		err := svc.Auth(ctx, whitelistedEP)
		require.NoError(t, err)

		err = svc.Auth(ctx, "/unknown.Service/Foo")
		require.Error(t, err)

		err = svc.Auth(ctx, protectedEP)
		require.NoError(t, err)

		err = svc.Auth(context.TODO(), protectedEP)
		require.Error(t, err)
	})

	t.Run("reset", func(t *testing.T) {
		err := svc.Reset(ctx)
		require.NoError(t, err)

		fi, err := os.Stat(datadir)
		require.NoError(t, err)
		require.NotNil(t, fi)
		require.True(t, fi.IsDir())

		err = svc.Auth(ctx, whitelistedEP)
		require.NoError(t, err)

		err = svc.Auth(ctx, protectedEP)
		require.Error(t, err)
	})
}
