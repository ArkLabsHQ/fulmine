package application

import (
	"testing"
	"time"

	clientTypes "github.com/arkade-os/arkd/pkg/client-lib/types"
	"github.com/stretchr/testify/require"
)

func TestEarliestInputExpiry(t *testing.T) {
	t1 := time.Unix(2000, 0)
	t2 := time.Unix(1000, 0) // earliest
	t3 := time.Unix(3000, 0)
	got, err := earliestInputExpiry([]clientTypes.Vtxo{
		{ExpiresAt: t1}, {ExpiresAt: t2}, {ExpiresAt: t3},
	})
	require.NoError(t, err)
	require.Equal(t, t2, got)
}

func TestEarliestInputExpiryEmpty(t *testing.T) {
	_, err := earliestInputExpiry(nil)
	require.Error(t, err)
}
