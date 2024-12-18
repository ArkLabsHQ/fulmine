package cln

import (
	"context"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestServiceGetInfo(t *testing.T) {
	s := service{}

	gotVersion, gotPubkey, err := s.GetInfo(context.Background())
	require.NoError(t, err)
	require.NotEmpty(t, gotVersion)
	require.NotEmpty(t, gotPubkey)

	inv, hash, err := s.GetInvoice(context.Background(), 1, "test")
	require.NoError(t, err)
	require.NotEmpty(t, inv)
	require.NotEmpty(t, hash)

	preimage, err := s.PayInvoice(context.Background(), inv)
	require.NoError(t, err)
	require.NotEmpty(t, preimage)
}
