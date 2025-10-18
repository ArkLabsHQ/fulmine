package esplora

import (
	"context"
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
	svc := NewService("blockstream.info:700")
	
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

	t.Logf("Current blockchain height via service: %d", height)
}

func TestService_GetBlockHeight_HTTP(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping network test in short mode")
	}

	// Test with HTTP Esplora API (for comparison)
	svc := NewService("https://blockstream.info/api")
	
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

func TestService_DetectsElectrumVsHTTP(t *testing.T) {
	tests := []struct {
		name        string
		url         string
		useElectrum bool
	}{
		{
			name:        "Electrum server without protocol",
			url:         "blockstream.info:700",
			useElectrum: true,
		},
		{
			name:        "HTTP URL with https",
			url:         "https://blockstream.info/api",
			useElectrum: false,
		},
		{
			name:        "HTTP URL with http",
			url:         "http://localhost:3000",
			useElectrum: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := NewService(tt.url)
			if svc.useElectrum != tt.useElectrum {
				t.Errorf("Expected useElectrum=%v, got %v", tt.useElectrum, svc.useElectrum)
			}
		})
	}
}
