// Command seed-singlekey creates a fully initialized single-key ("legacy",
// pre-HD) fulmine datadir so the e2e suite can exercise the legacy identity
// code paths.
//
// This is test-only tooling. Fulmine itself has no way to create a single-key
// wallet — Setup (internal/core/application/service.go:249) requires a valid
// BIP39 mnemonic and always builds an HD wallet — and nothing in cmd/ or
// internal/core imports this package.
//
// It MUST run inside the arkade-regtest network: Init persists the ark server
// URL into the config store, and fulmine-in-container resolves "arkd", which
// the host cannot.
package main

import (
	"context"
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"strings"

	singlekeyidentity "github.com/arkade-os/arkd/pkg/client-lib/identity/singlekey"
	singlekeyfilestore "github.com/arkade-os/arkd/pkg/client-lib/identity/singlekey/store/file"
	arksdk "github.com/arkade-os/go-sdk"
)

func main() {
	datadir := flag.String("datadir", "", "datadir to seed; must match FULMINE_DATADIR")
	serverURL := flag.String("server-url", "", "ark server URL, as reachable from the fulmine container")
	explorerURL := flag.String("explorer-url", "", "esplora URL, as reachable from the fulmine container")
	password := flag.String("password", "", "wallet password")
	key := flag.String("key", "", "hex-encoded private key; empty generates a random one")
	flag.Parse()

	pubkey, err := run(*datadir, *serverURL, *explorerURL, *password, *key)
	if err != nil {
		fmt.Fprintf(os.Stderr, "seed-singlekey: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(pubkey)
}

// run seeds datadir and returns the wallet's compressed pubkey in hex.
func run(datadir, serverURL, explorerURL, password, seed string) (string, error) {
	if datadir == "" || serverURL == "" || explorerURL == "" || password == "" {
		return "", fmt.Errorf(
			"-datadir, -server-url, -explorer-url and -password are all required",
		)
	}

	store, err := singlekeyfilestore.NewStore(datadir)
	if err != nil {
		return "", fmt.Errorf("failed to open identity store: %w", err)
	}

	identitySvc, err := singlekeyidentity.NewIdentity(store)
	if err != nil {
		return "", fmt.Errorf("failed to build single-key identity: %w", err)
	}

	// Check if the datadir is already fully seeded by attempting to load the wallet.
	// LoadWallet will fail with "not initialized" if the datadir has only state.json
	// but no config store (partial seed). We use this same check that fulmine uses
	// at boot to distinguish fully-seeded (don't re-init) from partial/empty
	// (heal by re-running Init).
	_, err = arksdk.LoadWallet(datadir, arksdk.WithIdentity(identitySvc))
	if err == nil {
		// Fully seeded: return the pubkey without calling Init.
		data, err := store.Get()
		if err != nil {
			return "", fmt.Errorf("failed to read identity store: %w", err)
		}
		if data == nil {
			return "", fmt.Errorf("identity store empty despite successful LoadWallet")
		}
		return hex.EncodeToString(data.PubKey.SerializeCompressed()), nil
	}

	// Check if this is a partial seed (state.json present but config store missing).
	// Match what fulmine does at service.go:1249: use substring check.
	if !strings.Contains(err.Error(), "not initialized") {
		return "", fmt.Errorf("failed to load wallet: %w", err)
	}

	// Either the datadir is empty or partially seeded. Complete the seed with Init.
	wallet, err := arksdk.NewWallet(datadir, arksdk.WithIdentity(identitySvc))
	if err != nil {
		return "", fmt.Errorf("failed to create wallet: %w", err)
	}

	// Init writes BOTH artifacts the legacy boot path needs: it calls
	// identity.Create (encrypts the key, writes state.json) and then persists the
	// SDK config store. With only one of the two, LoadWallet reports "not
	// initialized" and fulmine silently falls back to a fresh HD wallet.
	if err := wallet.Init(
		context.Background(), serverURL, seed, password,
		arksdk.WithExplorerURL(explorerURL),
	); err != nil {
		return "", fmt.Errorf("failed to init wallet: %w", err)
	}

	data, err := store.Get()
	if err != nil {
		return "", fmt.Errorf("failed to re-read identity store: %w", err)
	}
	if data == nil {
		return "", fmt.Errorf("identity store still empty after init")
	}
	return hex.EncodeToString(data.PubKey.SerializeCompressed()), nil
}
