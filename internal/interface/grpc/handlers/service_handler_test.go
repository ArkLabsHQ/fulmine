package handlers

import (
	"context"
	"testing"

	pb "github.com/ArkLabsHQ/fulmine/api-spec/protobuf/gen/go/fulmine/v1"
	"github.com/stretchr/testify/require"
)

// TestGetVirtualTxs_ValidationTests tests the input handling logic
func TestGetVirtualTxs_ValidationTests(t *testing.T) {
	tests := []struct {
		name        string
		request     *pb.GetVirtualTxsRequest
		expectedTxs []string
	}{
		{
			name: "empty txids list returns empty response",
			request: &pb.GetVirtualTxsRequest{
				Txids: []string{},
			},
			expectedTxs: []string{},
		},
		{
			name:        "nil txids returns empty response",
			request:     &pb.GetVirtualTxsRequest{},
			expectedTxs: []string{},
		},
		{
			name: "all empty strings returns empty response",
			request: &pb.GetVirtualTxsRequest{
				Txids: []string{"", "", ""},
			},
			expectedTxs: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a handler with nil service (we're only testing filtering logic)
			handler := &serviceHandler{svc: nil}

			// Call GetVirtualTxs
			resp, err := handler.GetVirtualTxs(context.Background(), tt.request)

			// If expectedTxs is set, we expect empty response without calling service
			if tt.expectedTxs != nil {
				require.NoError(t, err)
				require.NotNil(t, resp)
				require.Equal(t, tt.expectedTxs, resp.Txs)
			}
			// Otherwise, it would call the service (which is nil, so would panic)
			// but we're not testing that path here
		})
	}
}

// TestGetVirtualTxsRequest_MessageStructure tests the protobuf message structure
func TestGetVirtualTxsRequest_MessageStructure(t *testing.T) {
	t.Run("request can be created with txids", func(t *testing.T) {
		req := &pb.GetVirtualTxsRequest{
			Txids: []string{"txid1", "txid2"},
		}
		require.NotNil(t, req)
		require.Len(t, req.Txids, 2)
		require.Equal(t, "txid1", req.Txids[0])
		require.Equal(t, "txid2", req.Txids[1])
	})

	t.Run("response structure", func(t *testing.T) {
		// Verify the response type exists and has the expected structure
		var resp pb.GetVirtualTxsResponse
		resp.Txs = []string{"hex_tx_1", "hex_tx_2"}
		require.Len(t, resp.Txs, 2)
		require.Equal(t, "hex_tx_1", resp.Txs[0])
		require.Equal(t, "hex_tx_2", resp.Txs[1])
	})
}
