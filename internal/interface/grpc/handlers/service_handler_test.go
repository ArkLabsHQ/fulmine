package handlers

import (
	"context"
	"testing"

	pb "github.com/ArkLabsHQ/fulmine/api-spec/protobuf/gen/go/fulmine/v1"
	"github.com/stretchr/testify/require"
)

// TestGetVirtualTxs tests the input handling logic
func TestGetVirtualTxs(t *testing.T) {
	// TODO (@aruokhai): add invalid cases once we added mocked app svc
	t.Run("valid", func(t *testing.T) {
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
				// TODO (@aruokhai): use mocked app svc
				handler := &serviceHandler{svc: nil}

				resp, err := handler.GetVirtualTxs(context.Background(), tt.request)
				require.NoError(t, err)
				require.NotNil(t, resp)
				require.Equal(t, tt.expectedTxs, resp.Txs)
			})
		}
	})
}
