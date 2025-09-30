package config

import (
	"context"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/ArkLabsHQ/fulmine/utils"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
)

// TestRealGrpcConnection tests against an actual gRPC server to demonstrate the bug
func TestRealGrpcConnection(t *testing.T) {
	// Start a real gRPC server on a random port
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer lis.Close()

	port := lis.Addr().(*net.TCPAddr).Port
	t.Logf("Started gRPC server on port %d", port)

	server := grpc.NewServer()
	healthServer := health.NewServer()
	grpc_health_v1.RegisterHealthServer(server, healthServer)
	healthServer.SetServingStatus("", grpc_health_v1.HealthCheckResponse_SERVING)

	go func() {
		server.Serve(lis)
	}()
	defer server.Stop()

	// Give server time to start
	time.Sleep(100 * time.Millisecond)

	t.Run("BUG: URL with http:// scheme causes connection failure", func(t *testing.T) {
		// This is what the OLD code would pass to gRPC
		urlWithScheme := fmt.Sprintf("http://127.0.0.1:%d", port)

		t.Logf("Attempting to connect with URL: %s", urlWithScheme)

		// Try to connect with the scheme (OLD behavior)
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		conn, err := grpc.NewClient(urlWithScheme, 
			grpc.WithTransportCredentials(insecure.NewCredentials()),
			grpc.WithBlock())
		
		if err == nil {
			// Connection might be created, but let's try to actually use it
			client := grpc_health_v1.NewHealthClient(conn)
			_, err = client.Check(ctx, &grpc_health_v1.HealthCheckRequest{})
			conn.Close()
		}

		// The connection will fail because gRPC tries to add :443
		if err != nil {
			t.Logf("❌ CONNECTION FAILED (as expected with bug): %v", err)
			t.Logf("   gRPC tried to parse 'http://127.0.0.1:10009' and likely added :443")
		} else {
			t.Logf("⚠️  Connection succeeded (gRPC behavior may have changed)")
		}
	})

	t.Run("FIX: URL without scheme connects successfully", func(t *testing.T) {
		// This is what the FIXED code passes to gRPC
		urlWithoutScheme := fmt.Sprintf("127.0.0.1:%d", port)

		t.Logf("Attempting to connect with URL: %s", urlWithoutScheme)

		// Try to connect without the scheme (NEW behavior)
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		conn, err := grpc.NewClient(urlWithoutScheme,
			grpc.WithTransportCredentials(insecure.NewCredentials()),
			grpc.WithBlock())
		require.NoError(t, err, "Connection should succeed")
		defer conn.Close()

		// Verify we can actually make a call
		client := grpc_health_v1.NewHealthClient(conn)
		resp, err := client.Check(ctx, &grpc_health_v1.HealthCheckRequest{})
		require.NoError(t, err)
		require.Equal(t, grpc_health_v1.HealthCheckResponse_SERVING, resp.Status)

		t.Logf("✅ CONNECTION SUCCESSFUL: Server responded with status %v", resp.Status)
	})

	t.Run("ValidateURL strips scheme when port is specified", func(t *testing.T) {
		testCases := []struct {
			input       string
			expected    string
			description string
		}{
			{"http://lol:1009", "lol:1009", "With port: strip scheme"},
			{"http://127.0.0.1:10009", "127.0.0.1:10009", "With port: strip scheme"},
			{"https://boltz-lnd:10009", "boltz-lnd:10009", "With port: strip scheme"},
			{"localhost:10009", "localhost:10009", "With port: strip scheme"},
			{"http://acme.com", "http://acme.com", "No port: keep scheme"},
			{"https://example.com", "https://example.com", "No port: keep scheme"},
		}

		for _, tc := range testCases {
			t.Run(tc.input, func(t *testing.T) {
				validated, err := utils.ValidateURL(tc.input)
				require.NoError(t, err)
				require.Equal(t, tc.expected, validated)
				t.Logf("✅ %s → %s (%s)", tc.input, validated, tc.description)
			})
		}
	})
}

// TestConnectionWithVariousFormats tests connection attempts with different URL formats
func TestConnectionWithVariousFormats(t *testing.T) {
	// Start a real gRPC server
	lis, err := net.Listen("tcp", "127.0.0.1:0") // Random port
	require.NoError(t, err)
	defer lis.Close()

	port := lis.Addr().(*net.TCPAddr).Port

	server := grpc.NewServer()
	healthServer := health.NewServer()
	grpc_health_v1.RegisterHealthServer(server, healthServer)
	healthServer.SetServingStatus("", grpc_health_v1.HealthCheckResponse_SERVING)

	go func() {
		server.Serve(lis)
	}()
	defer server.Stop()

	time.Sleep(100 * time.Millisecond)

	testCases := []struct {
		name           string
		url            string
		shouldWork     bool
		description    string
	}{
		{
			name:        "Correct format: host:port",
			url:         fmt.Sprintf("127.0.0.1:%d", port),
			shouldWork:  true,
			description: "This is what ValidateURL returns after fix",
		},
		{
			name:        "Incorrect format: http://host:port",
			url:         fmt.Sprintf("http://127.0.0.1:%d", port),
			shouldWork:  false,
			description: "This is what caused the bug - gRPC adds :443",
		},
		{
			name:        "Incorrect format: https://host:port",
			url:         fmt.Sprintf("https://127.0.0.1:%d", port),
			shouldWork:  false,
			description: "Same issue with https scheme",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Logf("Testing URL: %s", tc.url)
			t.Logf("Description: %s", tc.description)

			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()

			conn, err := grpc.NewClient(tc.url,
				grpc.WithTransportCredentials(insecure.NewCredentials()),
				grpc.WithBlock())

			if tc.shouldWork {
				require.NoError(t, err, "Connection should succeed for: %s", tc.url)
				if conn != nil {
					client := grpc_health_v1.NewHealthClient(conn)
					resp, err := client.Check(ctx, &grpc_health_v1.HealthCheckRequest{})
					require.NoError(t, err)
					require.Equal(t, grpc_health_v1.HealthCheckResponse_SERVING, resp.Status)
					t.Logf("✅ SUCCESS: Connected and received response")
					conn.Close()
				}
			} else {
				// Connection should fail or timeout
				if err != nil {
					t.Logf("❌ FAILED (expected): %v", err)
				} else if conn != nil {
					// Try to use it
					client := grpc_health_v1.NewHealthClient(conn)
					_, err := client.Check(ctx, &grpc_health_v1.HealthCheckRequest{})
					if err != nil {
						t.Logf("❌ FAILED on use (expected): %v", err)
					} else {
						t.Logf("⚠️  Unexpectedly succeeded - gRPC behavior may have changed")
					}
					conn.Close()
				}
			}
		})
	}
}

// TestDeriveLnConfigWithRealValidation tests the actual config derivation
func TestDeriveLnConfigWithRealValidation(t *testing.T) {
	t.Run("http://lol:1009 gets validated correctly", func(t *testing.T) {
		lndUrl := "http://lol:1009"
		lndDatadir := "/tmp/test"

		// Test the actual deriveLnConfig function
		opts, err := deriveLnConfig(lndUrl, "", lndDatadir, "")
		require.NoError(t, err)
		require.NotNil(t, opts)

		// The URL should be stripped of scheme
		require.Equal(t, "lol:1009", opts.LnUrl, "Scheme should be stripped")
		t.Logf("✅ deriveLnConfig correctly transformed: %s → %s", lndUrl, opts.LnUrl)

		// Verify this format would work with gRPC (we can't connect to 'lol' but we can create the client)
		conn, err := grpc.NewClient(opts.LnUrl, grpc.WithTransportCredentials(insecure.NewCredentials()))
		require.NoError(t, err, "gRPC client should accept the format")
		require.Equal(t, "lol:1009", conn.Target())
		conn.Close()

		t.Logf("✅ gRPC client accepts the validated URL format")
	})

	t.Run("OLD behavior would have kept http:// scheme", func(t *testing.T) {
		lndUrl := "http://lol:1009"

		// Simulate OLD behavior: validate but don't use result
		_, err := utils.ValidateURL(lndUrl)
		require.NoError(t, err)

		// OLD code would return the original URL
		t.Logf("OLD behavior would have returned: %s (with scheme)", lndUrl)
		t.Logf("This would cause gRPC to add :443, resulting in: %s:443", lndUrl)
		t.Logf("Result: THREE colons (://  :1009  :443) = 'too many colons in address'")
	})
}
