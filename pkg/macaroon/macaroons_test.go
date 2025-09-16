package macaroon_test

import (
	"context"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	mk "github.com/ArkLabsHQ/fulmine/pkg/macaroon"
	"google.golang.org/grpc/metadata"
	"gopkg.in/macaroon-bakery.v2/bakery"
	"gopkg.in/macaroon.v2"
)

func TestService_TempDir_Generate_Auth_Reset(t *testing.T) {
	t.Parallel()

	ctxAuth := context.Background()
	tmp := t.TempDir()

	const (
		macFileName     = "admin.macaroon"
		macaroonsFolder = "macaroons"
		whitelistedEP   = "/health.Service/Check"
		protectedEP     = "/fulmine.Service/GetInfo"
	)
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
	if err != nil {
		t.Fatalf("NewService error: %v", err)
	}

	t.Run("setup: unlock, generate, load macaroon, attach metadata", func(t *testing.T) {
		if err := svc.Unlock(ctxAuth, "passw0rd"); err != nil {
			t.Fatalf("Unlock error: %v", err)
		}
		if err := svc.Generate(ctxAuth); err != nil {
			t.Fatalf("Generate error: %v", err)
		}

		wantMacPath := filepath.Join(datadir, macFileName)
		if _, err := os.Stat(wantMacPath); err != nil {
			t.Fatalf("expected macaroon file not found: %s: %v", wantMacPath, err)
		}

		macaroonBytes, err := os.ReadFile(wantMacPath)
		if err != nil {
			t.Fatalf("ReadFile error: %v", err)
		}
		var macaroon macaroon.Macaroon
		if err := macaroon.UnmarshalBinary(macaroonBytes); err != nil {
			t.Fatalf("UnmarshalBinary error: %v", err)
		}

		macaroonHex := hex.EncodeToString(macaroonBytes)

		md := metadata.Pairs("macaroon", macaroonHex)

		ctxAuth = metadata.NewIncomingContext(ctxAuth, md)
	})

	t.Run("auth respects whitelist, protected path and unknown method", func(t *testing.T) {
		if err := svc.Auth(ctxAuth, whitelistedEP); err != nil {
			t.Fatalf("whitelisted Auth failed: %v", err)
		}
		if err := svc.Auth(ctxAuth, "/unknown.Service/Foo"); err == nil {
			t.Fatalf("expected Auth to fail for unknown method")
		}

		if err := svc.Auth(ctxAuth, protectedEP); err != nil {
			t.Fatalf("protected Auth failed: %v", err)
		}

		if err := svc.Auth(context.TODO(), protectedEP); err == nil {
			t.Fatalf("expected Auth to fail for missing macaroon")
		}
	})

	t.Run("reset recreates datadir and preserves whitelist", func(t *testing.T) {
		if err := svc.Reset(ctxAuth); err != nil {
			t.Fatalf("Reset error: %v", err)
		}
		if fi, err := os.Stat(datadir); err != nil || !fi.IsDir() {
			t.Fatalf("expected datadir to exist after Reset: %s: %v", datadir, err)
		}
		if err := svc.Auth(ctxAuth, whitelistedEP); err != nil {
			t.Fatalf("whitelisted Auth failed after Reset: %v", err)
		}
		if err := svc.Auth(ctxAuth, protectedEP); err == nil {
			t.Fatalf("expected Auth to fail for protected method after Reset")
		}
	})
}
