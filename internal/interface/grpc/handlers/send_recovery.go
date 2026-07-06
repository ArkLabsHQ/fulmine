package handlers

import (
	"context"
	"fmt"
	"strings"
	"time"

	clientTypes "github.com/arkade-os/arkd/pkg/client-lib/types"
	log "github.com/sirupsen/logrus"
)

const (
	// maxSendOffChainAttempts bounds how many times an offchain send is retried
	// when the server rejects the selected input as already spent.
	maxSendOffChainAttempts = 3
	// sendRecoveryRetryDelay gives the SDK's transaction-stream listener a moment
	// to reconcile the local vtxo db after a stranded pending tx is finalized,
	// before coins are re-selected and the send retried.
	sendRecoveryRetryDelay = 1 * time.Second
)

type sendOffChainFunc func(context.Context, []clientTypes.Receiver) (string, error)

type finalizePendingTxsFunc func(context.Context, *time.Time) ([]string, error)

// sendOffChainWithRecovery performs an offchain send and, when the server
// rejects the selected input with VTXO_ALREADY_SPENT, attempts to self-heal
// instead of blindly retrying the same doomed coin selection.
//
// An offchain send whose finalization is interrupted after the server has
// already registered the spend (e.g. a 502 between SubmitTx and FinalizeTx)
// leaves the input spent server-side yet still spendable in the local db. Coin
// selection then keeps re-picking that stranded input and every send fails with
// VTXO_ALREADY_SPENT. Finalizing the pending tx resolves the strand server-side;
// the SDK's tx-stream listener (and, as a backstop, the periodic db refresh)
// then drops the input locally so the retry selects valid coins.
func sendOffChainWithRecovery(
	ctx context.Context,
	receivers []clientTypes.Receiver,
	send sendOffChainFunc,
	finalizePending finalizePendingTxsFunc,
	attempts int,
	retryDelay time.Duration,
) (string, error) {
	if attempts <= 0 {
		return "", fmt.Errorf("attempts must be positive, got %d", attempts)
	}
	var txid string
	var err error
	for attempt := 0; attempt < attempts; attempt++ {
		txid, err = send(ctx, receivers)
		if err == nil {
			return txid, nil
		}
		if !isVtxoAlreadySpent(err) {
			return "", err
		}

		// Best-effort: finalize any stranded pending txs so the local db can
		// reconcile the already-spent input. This is what actually breaks the
		// loop; the original send error is kept if recovery is not possible.
		if _, ferr := finalizePending(ctx, nil); ferr != nil {
			log.WithError(ferr).
				Warn("failed to finalize pending txs while recovering from already-spent vtxo")
		}

		if attempt < attempts-1 {
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(retryDelay):
			}
		}
	}
	return "", err
}

func isVtxoAlreadySpent(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "vtxo_already_spent")
}
