package handlers

import (
	"context"
	"errors"
	"testing"
	"time"

	clientTypes "github.com/arkade-os/arkd/pkg/client-lib/types"
	"github.com/stretchr/testify/require"
)

// mirrors the server rejection seen when the selected input was already spent by
// a previously-submitted (but unfinalized) ark tx.
var errAlreadySpent = errors.New(
	"rpc error: code = InvalidArgument desc = VTXO_ALREADY_SPENT (6): " +
		"1d99140b685d1c1d8cadfdf4f7e3a660ed1c72bd24df565df5d7fab7b0eddf6a:0 already spent",
)

func TestSendOffChainWithRecovery(t *testing.T) {
	receivers := []clientTypes.Receiver{{To: "ark1test", Amount: 1000}}

	t.Run("finalizes pending txs on already-spent then retries to success", func(t *testing.T) {
		var sendCalls, finalizeCalls int
		send := func(context.Context, []clientTypes.Receiver) (string, error) {
			sendCalls++
			// The stranded input is only cleared once the pending tx is
			// finalized and the local db reconciles; model that by succeeding
			// only after a finalize has happened.
			if finalizeCalls == 0 {
				return "", errAlreadySpent
			}
			return "goodtxid", nil
		}
		finalize := func(context.Context, *time.Time) ([]string, error) {
			finalizeCalls++
			return []string{"finalizedtxid"}, nil
		}

		txid, err := sendOffChainWithRecovery(context.Background(), receivers, send, finalize, 3, 0)
		require.NoError(t, err)
		require.Equal(t, "goodtxid", txid)
		require.Equal(t, 1, finalizeCalls, "should finalize stranded pending txs to recover")
		require.Equal(t, 2, sendCalls, "should retry the send after finalizing")
	})

	t.Run("returns unrelated errors immediately without finalizing", func(t *testing.T) {
		var finalizeCalls int
		send := func(context.Context, []clientTypes.Receiver) (string, error) {
			return "", errors.New("insufficient funds")
		}
		finalize := func(context.Context, *time.Time) ([]string, error) {
			finalizeCalls++
			return nil, nil
		}

		_, err := sendOffChainWithRecovery(context.Background(), receivers, send, finalize, 3, 0)
		require.ErrorContains(t, err, "insufficient funds")
		require.Equal(t, 0, finalizeCalls, "must not finalize for errors unrelated to spent vtxos")
	})

	t.Run("gives up after max attempts but still finalizes to self-heal", func(t *testing.T) {
		var sendCalls, finalizeCalls int
		send := func(context.Context, []clientTypes.Receiver) (string, error) {
			sendCalls++
			return "", errAlreadySpent
		}
		finalize := func(context.Context, *time.Time) ([]string, error) {
			finalizeCalls++
			return nil, nil
		}

		_, err := sendOffChainWithRecovery(context.Background(), receivers, send, finalize, 3, 0)
		require.ErrorContains(t, err, "VTXO_ALREADY_SPENT")
		require.Equal(t, 3, sendCalls, "should exhaust all attempts")
		require.Equal(t, 3, finalizeCalls, "each already-spent failure should trigger a finalize attempt")
	})

	t.Run("first attempt succeeds without finalizing", func(t *testing.T) {
		var finalizeCalls int
		send := func(context.Context, []clientTypes.Receiver) (string, error) {
			return "immediate", nil
		}
		finalize := func(context.Context, *time.Time) ([]string, error) {
			finalizeCalls++
			return nil, nil
		}

		txid, err := sendOffChainWithRecovery(context.Background(), receivers, send, finalize, 3, 0)
		require.NoError(t, err)
		require.Equal(t, "immediate", txid)
		require.Equal(t, 0, finalizeCalls)
	})

	t.Run("aborts the retry wait when the context is cancelled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		send := func(context.Context, []clientTypes.Receiver) (string, error) {
			return "", errAlreadySpent
		}
		finalize := func(context.Context, *time.Time) ([]string, error) {
			cancel() // simulate the caller giving up during recovery
			return nil, nil
		}

		_, err := sendOffChainWithRecovery(ctx, receivers, send, finalize, 3, time.Hour)
		require.ErrorIs(t, err, context.Canceled)
	})
}
