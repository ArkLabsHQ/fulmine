package swap

import (
	"context"
	"fmt"
	"time"

	"github.com/ArkLabsHQ/fulmine/pkg/boltz"
	"github.com/ArkLabsHQ/fulmine/pkg/vhtlc"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/chaincfg"
	log "github.com/sirupsen/logrus"
)

func (h *SwapHandler) monitorAndClaimArkToBtcSwap(
	ctx context.Context,
	network *chaincfg.Params,
	eventCallback ChainSwapEventCallback,
	unilateralRefundCallback func(swapId string, opts vhtlc.Opts) error,
	btcClaimPrivKey *btcec.PrivateKey,
	preimage []byte,
	btcDestinationAddress string,
	swapResp *boltz.CreateChainSwapResponse,
	swap *ChainSwap,
) {
	var (
		arkToBtc = true
		swapTree = swapResp.GetSwapTree(arkToBtc)
		swapId   = swap.Id
	)

	pk, err := parsePubkey(swapResp.ClaimDetails.ServerPublicKey)
	if err != nil {
		log.WithError(err).Errorf("Failed to parse server public key for swap %s", swapId)
		swap.Fail(fmt.Sprintf("parse server public key: %v", err))
		return
	}

	handler := NewArkToBtcHandler(
		h,
		ChainSwapState{
			SwapID:                   swapId,
			Timeout:                  time.Duration(h.timeout) * time.Second,
			EventCallback:            eventCallback,
			UnilateralRefundCallback: unilateralRefundCallback,
			Swap:                     swap,
		},
		network,
		btcClaimPrivKey,
		preimage,
		btcDestinationAddress,
		swapResp,
		pk,
		swapTree,
	)

	h.monitorChainSwap(ctx, handler.GetState(), handler)
}

func (h *SwapHandler) monitorBtcToArkChainSwap(
	ctx context.Context,
	eventCallback ChainSwapEventCallback,
	preimage []byte,
	refundKey *btcec.PrivateKey,
	swapResp *boltz.CreateChainSwapResponse,
	swap *ChainSwap,
) {
	handler := NewBtcToArkHandler(
		h,
		ChainSwapState{
			SwapID:        swap.Id,
			Timeout:       time.Duration(h.timeout) * time.Second,
			EventCallback: eventCallback,
			Swap:          swap,
		},
		preimage,
		refundKey,
		swapResp,
	)

	h.monitorChainSwap(ctx, handler.GetState(), handler)
}

// monitorChainSwap is a generic WebSocket monitoring loop that delegates
// swap-specific event handling to the provided ChainSwapEventHandler.
// This extracts the common monitoring pattern used by both Ark→BTC and BTC→Ark swaps.
func (h *SwapHandler) monitorChainSwap(
	ctx context.Context,
	chainSwapState ChainSwapState,
	handler ChainSwapEventHandler,
) {
	swapId := handler.GetState().SwapID
	log.Infof("Starting WebSocket monitoring for chain swap %s", swapId)

	ws := h.boltzSvc.NewWebsocket()
	if err := ws.ConnectAndSubscribe(ctx, []string{swapId}, 5*time.Second); err != nil {
		log.WithError(err).Errorf("Failed to connect to WebSocket for swap %s", swapId)
		chainSwapState.Swap.Fail(fmt.Sprintf("websocket subscribe failed: %v", err))
		return
	}
	defer ws.Close()

	timeout := time.After(chainSwapState.Timeout)

	for {
		select {
		case update, ok := <-ws.Updates:
			if !ok {
				chainSwapState.Swap.Fail("websocket closed")
				return
			}

			status := boltz.ParseEvent(update.Status)
			log.Infof("Chain swap %s status update: %d (raw: %s)", swapId, status, update.Status)

			var err error
			switch status {
			case boltz.SwapCreated:
				// boltz accepted swap request we created
				err = handler.HandleSwapCreated(ctx, update)

			case boltz.TransactionLockupFailed:
				// user lockup tx failed, we need to fetch approve a new quote
				err = handler.HandleLockupFailed(ctx, update)

			case boltz.TransactionMempool:
				// user lockup tx is in mempool
				err = handler.HandleUserLockedMempool(ctx, update)

			case boltz.TransactionConfirmed:
				// user lockup tx confirmed
				err = handler.HandleUserLocked(ctx, update)

			case boltz.TransactionServerMempoool:
				// boltz lockup tx in mempool
				err = handler.HandleServerLockedMempool(ctx, update)

			case boltz.TransactionServerConfirmed:
				// boltz lockup tx confirmed
				err = handler.HandleServerLocked(ctx, update)

			case boltz.SwapExpired:
				// swap expired
				err = handler.HandleSwapExpired(ctx, update)

			case boltz.TransactionFailed:
				//?
				err = handler.HandleTransactionFailed(ctx, update)

			default:
				log.Warnf("Unknown status %s (%d) for swap %s", update.Status, status, swapId)
				continue
			}

			// Handle errors from event handlers
			if err != nil {
				log.WithError(err).Errorf("Event handler error for swap %s", swapId)
				chainSwapState.Swap.Fail(err.Error())
				return
			}

			// Check if swap completed (success or failure)
			if chainSwapState.Swap.Status == ChainSwapClaimed || chainSwapState.Swap.Status == ChainSwapFailed {
				return
			}

		case <-timeout:
			log.Warnf("Swap %s monitoring timed out after %v", swapId, chainSwapState.Timeout)
			chainSwapState.Swap.Fail(fmt.Sprintf("monitoring timed out after %v", chainSwapState.Timeout))
			return

		case <-ctx.Done():
			log.Infof("Context cancelled for swap %s monitoring", swapId)
			chainSwapState.Swap.Fail("context cancelled")
			return
		}
	}
}

// ChainSwapEventHandler defines swap-specific behavior for different swap directions.
// This interface uses the strategy pattern to extract common WebSocket monitoring logic.
type ChainSwapEventHandler interface {
	// HandleSwapCreated handles initial swap creation
	HandleSwapCreated(ctx context.Context, update boltz.SwapUpdate) error

	// HandleLockupFailed handles various failure scenarios
	HandleLockupFailed(ctx context.Context, update boltz.SwapUpdate) error

	HandleUserLockedMempool(ctx context.Context, update boltz.SwapUpdate) error

	// HandleUserLocked handles user lockup confirmation
	HandleUserLocked(ctx context.Context, update boltz.SwapUpdate) error

	// HandleServerLockedMempool handles server lockup (ready to claim)
	HandleServerLockedMempool(ctx context.Context, update boltz.SwapUpdate) error

	// HandleServerLocked handles server lockup (ready to claim)
	HandleServerLocked(ctx context.Context, update boltz.SwapUpdate) error

	HandleSwapExpired(ctx context.Context, update boltz.SwapUpdate) error

	HandleTransactionFailed(ctx context.Context, update boltz.SwapUpdate) error

	GetState() ChainSwapState
}

type ChainSwapState struct {
	SwapID                   string
	Timeout                  time.Duration
	EventCallback            ChainSwapEventCallback
	UnilateralRefundCallback func(swapId string, opts vhtlc.Opts) error
	Swap                     *ChainSwap
}
