package domain_test

import (
	"encoding/json"
	"testing"

	"github.com/ArkLabsHQ/fulmine/internal/core/domain"
	"github.com/ArkLabsHQ/fulmine/pkg/boltz"
	"github.com/stretchr/testify/require"
)

func TestChainSwapTimeoutBlockHeight(t *testing.T) {
	// A BTC->ARK creation response carries the BTC lockup's CLTV timeout under
	// lockupDetails.timeoutBlockHeight; the accessor must surface it (this is the
	// value a caller needs to know when a unilateral refund becomes spendable).
	resp := boltz.CreateChainSwapResponse{
		LockupDetails: boltz.SwapLeg{TimeoutBlockHeight: 362},
	}
	raw, err := json.Marshal(resp)
	require.NoError(t, err)

	cs := domain.ChainSwap{BoltzCreateResponseJSON: string(raw)}
	require.Equal(t, uint32(362), cs.TimeoutBlockHeight())

	// Degrade to 0 (never panic) when the height can't be determined.
	require.Equal(t, uint32(0), domain.ChainSwap{}.TimeoutBlockHeight())
	require.Equal(t, uint32(0), domain.ChainSwap{BoltzCreateResponseJSON: "{not json"}.TimeoutBlockHeight())
}
