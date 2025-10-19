package esplora

import (
	"context"
	"strings"
	"testing"
	"time"
)

// These tests require network access and are skipped in CI environments
// Run with: go test -v ./internal/infrastructure/esplora/... -tags=integration

func TestElectrumClient_GetBlockchainHeight(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping network test in short mode")
	}

	// Test with blockstream.info mainnet electrum server
	client := NewElectrumClient("blockstream.info:700", 10*time.Second)
	defer client.Close()

	ctx := context.Background()
	height, err := client.GetBlockchainHeight(ctx)
	if err != nil {
		t.Logf("GetBlockchainHeight failed (expected in CI): %v", err)
		t.Skip("Network test skipped due to connectivity issues")
		return
	}

	// Sanity check: mainnet should have a significant height
	if height < 800000 {
		t.Errorf("Height %d seems too low for mainnet", height)
	}

	t.Logf("Current blockchain height: %d", height)
}

func TestService_GetBlockHeight_Electrum(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping network test in short mode")
	}

	// Test with Electrum server
	svc := NewService("", "blockstream.info:700")
	
	ctx := context.Background()
	height, err := svc.GetBlockHeight(ctx)
	if err != nil {
		t.Logf("GetBlockchainHeight failed (expected in CI): %v", err)
		t.Skip("Network test skipped due to connectivity issues")
		return
	}

	// Sanity check: mainnet should have a significant height
	if height < 800000 {
		t.Errorf("Height %d seems too low for mainnet", height)
	}

	t.Logf("Current blockchain height via service: %d", height)
}

func TestService_GetBlockHeight_HTTP(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping network test in short mode")
	}

	// Test with HTTP Esplora API (for comparison)
	svc := NewService("https://blockstream.info/api", "")
	
	ctx := context.Background()
	height, err := svc.GetBlockHeight(ctx)
	if err != nil {
		t.Logf("GetBlockHeight failed (expected in CI): %v", err)
		t.Skip("Network test skipped due to connectivity issues")
		return
	}

	// Sanity check: mainnet should have a significant height
	if height < 800000 {
		t.Errorf("Height %d seems too low for mainnet", height)
	}

	t.Logf("Current blockchain height via HTTP: %d", height)
}

func TestService_PrioritizesElectrum(t *testing.T) {
	tests := []struct {
		name           string
		esploraURL     string
		electrumURL    string
		expectedType   string
		supportsSubcr  bool
	}{
		{
			name:           "Electrum URL provided - uses Electrum",
			esploraURL:     "https://mempool.space/api",
			electrumURL:    "blockstream.info:700",
			expectedType:   "*esplora.electrumService",
			supportsSubcr:  true,
		},
		{
			name:           "Only Esplora URL provided - uses HTTP",
			esploraURL:     "https://blockstream.info/api",
			electrumURL:    "",
			expectedType:   "*esplora.httpService",
			supportsSubcr:  false,
		},
		{
			name:           "Both provided - prioritizes Electrum",
			esploraURL:     "https://mempool.space/api",
			electrumURL:    "blockstream.info:700",
			expectedType:   "*esplora.electrumService",
			supportsSubcr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := NewService(tt.esploraURL, tt.electrumURL)
			
			// Verify the service was created
			if svc == nil {
				t.Fatal("Service should not be nil")
			}

			// Test that subscription returns appropriate result
			ctx := context.Background()
			_, err := svc.SubscribeScriptHash(ctx, "test")
			
			if tt.supportsSubcr {
				// Electrum service - expect network error or connection error, not "not supported"
				if err != nil && strings.Contains(err.Error(), "not supported") {
					t.Errorf("Electrum service should support subscriptions, got: %v", err)
				}
			} else {
				// HTTP service - expect "not supported" error
				if err == nil || !strings.Contains(err.Error(), "not supported") {
					t.Errorf("HTTP service should not support subscriptions, got: %v", err)
				}
			}
		})
	}
}

func TestElectrumClient_TLSDetection(t *testing.T) {
	tests := []struct {
		name    string
		address string
		useTLS  bool
	}{
		{
			name:    "Port 700 should use TLS",
			address: "blockstream.info:700",
			useTLS:  true,
		},
		{
			name:    "Port 50002 should use TLS",
			address: "server.example.com:50002",
			useTLS:  true,
		},
		{
			name:    "Port 50001 should not use TLS",
			address: "server.example.com:50001",
			useTLS:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := NewElectrumClient(tt.address, 10*time.Second)
			if client.useTLS != tt.useTLS {
				t.Errorf("Expected useTLS=%v, got %v", tt.useTLS, client.useTLS)
			}
		})
	}
}
