package swap

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/ArkLabsHQ/fulmine/pkg/boltz"
	"github.com/btcsuite/btcd/btcec/v2"
	log "github.com/sirupsen/logrus"
)

const (
	getQuote = "get_quote"
)

type btcToArkHandler struct {
	swapHandler    *SwapHandler
	chainSwapState ChainSwapState
	preimage       []byte
	refundKey      *btcec.PrivateKey
	swapResp       *boltz.CreateChainSwapResponse
}

func NewBtcToArkHandler(
	swapHandler *SwapHandler,
	chainSwapState ChainSwapState,
	preimage []byte,
	refundKey *btcec.PrivateKey,
	swapResp *boltz.CreateChainSwapResponse,
) ChainSwapEventHandler {
	return &btcToArkHandler{
		swapHandler:    swapHandler,
		chainSwapState: chainSwapState,
		preimage:       preimage,
		refundKey:      refundKey,
		swapResp:       swapResp,
	}
}

func (b *btcToArkHandler) HandleSwapCreated(ctx context.Context, update boltz.SwapUpdate) error {
	return b.handleBtcToArkSwapCreated(ctx, update)
}

func (b *btcToArkHandler) HandleLockupFailed(ctx context.Context, update boltz.SwapUpdate) error {
	return b.handleBtcToArkFailure(ctx, update, getQuote)
}

func (b *btcToArkHandler) HandleUserLockedMempool(ctx context.Context, update boltz.SwapUpdate) error {
	return b.handleBtcToArkUserLocked(ctx, update)
}

func (b *btcToArkHandler) HandleUserLocked(ctx context.Context, update boltz.SwapUpdate) error {
	return b.handleBtcToArkUserLocked(ctx, update)
}

func (b *btcToArkHandler) HandleServerLockedMempool(ctx context.Context, update boltz.SwapUpdate) error {
	//Boltz trusts out BTC lockup that is now in mempool and we claim VTXO immediately
	return b.handleBtcToArkServerLocked(ctx, update)
}

func (b *btcToArkHandler) HandleServerLocked(ctx context.Context, update boltz.SwapUpdate) error {
	return nil
}

func (b *btcToArkHandler) HandleSwapExpired(ctx context.Context, update boltz.SwapUpdate) error {
	return b.handleBtcToArkFailure(ctx, update, "swap expired")
}

func (b *btcToArkHandler) HandleTransactionFailed(ctx context.Context, update boltz.SwapUpdate) error {
	return b.handleBtcToArkFailure(ctx, update, "transaction expired")
}

func (b *btcToArkHandler) GetState() ChainSwapState {
	return b.chainSwapState
}

func (b *btcToArkHandler) handleBtcToArkUserLocked(
	_ context.Context,
	update boltz.SwapUpdate,
) error {
	status := boltz.ParseEvent(update.Status)

	if status == boltz.TransactionMempool {
		log.Infof("User BTC lockup for swap %s detected in mempool", b.chainSwapState.SwapID)
	} else {
		log.Infof("User BTC lockup for swap %s confirmed", b.chainSwapState.SwapID)
	}

	b.chainSwapState.Swap.UserLock(update.Transaction.Id)

	return nil
}

func (b *btcToArkHandler) handleBtcToArkSwapCreated(
	_ context.Context,
	_ boltz.SwapUpdate,
) error {
	log.Infof("Swap %s created, waiting for user to lock BTC", b.chainSwapState.SwapID)

	return nil
}

func (b *btcToArkHandler) handleBtcToArkServerLocked(
	ctx context.Context,
	update boltz.SwapUpdate,
) error {
	log.Infof("Boltz sent Ark VTXOs for swap %s (mempool), claiming now", b.chainSwapState.SwapID)

	serverLockupTxID := update.Transaction.Id
	b.chainSwapState.Swap.ServerLock(serverLockupTxID)

	// Claim Ark VTXOs locup
	claimTxid, err := b.swapHandler.ClaimVHTLC(ctx, b.preimage, b.chainSwapState.Swap.VhtlcOpts)
	if err != nil {
		// ChainSwap.Fail() emits FailEvent automatically
		b.chainSwapState.Swap.Fail(fmt.Sprintf("claim failed: %v", err))
		return fmt.Errorf("failed to claim Ark VTXOs: %w", err)
	}

	b.chainSwapState.Swap.Claim(claimTxid)
	log.Infof("Claimed Ark VTXOs in transaction: %s", claimTxid)

	time.Sleep(5 * time.Second)

	// cooperatively sign for Boltz to claim our BTC lockup so that Boltz doesnt need to claim with preimage
	// which is more expensive since keypath(cooperative) witness is smaller than script-path(preimage)
	if err := b.signBoltzBtcClaim(ctx, b.chainSwapState.SwapID, b.refundKey, b.swapResp); err != nil {
		log.WithError(err).Warnf("Failed to provide cooperative signature for Boltz BTC claim (non-critical)")
		// Non-critical: Boltz can claim via script-path after timeout
	} else {
		log.Infof("Successfully provided cooperative signature for Boltz BTC claim")
	}

	return nil
}

func (b *btcToArkHandler) handleBtcToArkFailure(
	ctx context.Context,
	update boltz.SwapUpdate,
	reason string,
) error {
	if reason == getQuote {
		log.Warnf("User lockup failed for swap %s (amount mismatch), fetching quote", b.chainSwapState.SwapID)

		quote, err := b.swapHandler.boltzSvc.GetChainSwapQuote(b.chainSwapState.SwapID)
		if err != nil {
			b.chainSwapState.Swap.UserLockedFailed(fmt.Sprintf("lockup failed, quote error: %v", err))
			return fmt.Errorf("failed to get quote: %w", err)
		}

		log.Infof("Quote for swap %s: amount=%d, onchainAmount=%d",
			b.chainSwapState.SwapID, quote.Amount, quote.OnchainAmount)

		if err := b.swapHandler.boltzSvc.AcceptChainSwapQuote(b.chainSwapState.SwapID, *quote); err != nil {
			b.chainSwapState.Swap.UserLockedFailed(fmt.Sprintf("quote acceptance failed: %v", err))
			return fmt.Errorf("failed to accept quote: %w", err)
		}

		log.Infof("Quote accepted for swap %s, waiting for Boltz to send VTXOs", b.chainSwapState.SwapID)
		return nil
	}

	// Since fulmine is not a BTC wallet, we claim BTC to a boarding address and then settle to convert to VTXO
	log.Warnf("Swap %s failed: %s, attempting BTC refund via boarding address", b.chainSwapState.SwapID, reason)

	refundTxid, err := b.refundBtcToArkSwap(ctx)
	if err != nil {
		log.WithError(err).Errorf("BTC refund failed for swap %s", b.chainSwapState.SwapID)
		b.chainSwapState.Swap.RefundFailed(fmt.Sprintf("refund failed: %v", err))
		return fmt.Errorf("failed to refund BTC: %w", err)
	}

	log.Infof("BTC refund successful for swap %s: txid=%s", b.chainSwapState.SwapID, refundTxid)
	b.chainSwapState.Swap.RefundUnilaterally(refundTxid)

	return nil
}

func (b *btcToArkHandler) refundBtcToArkSwap(ctx context.Context) (string, error) {
	refundTxid, err := b.swapHandler.RefundBtcToArkSwap(
		ctx, b.chainSwapState.SwapID,
		b.chainSwapState.Swap.Amount,
		b.chainSwapState.Swap.UserLockTxid,
		b.chainSwapState.Swap.SwapRespJson,
	)
	if err != nil {
		return "", fmt.Errorf("failed to refund BTC VTXOs: %w", err)
	}

	return refundTxid, nil
}

// signBoltzBtcClaim provides a cooperative signature for Boltz to claim the user's BTC lockup
func (b *btcToArkHandler) signBoltzBtcClaim(
	_ context.Context,
	swapId string,
	refundKey *btcec.PrivateKey,
	swapResp *boltz.CreateChainSwapResponse,
) error {
	log.Infof("Providing cooperative signature for Boltz to claim BTC lockup for swap %s", swapId)

	claimDetails, err := b.swapHandler.boltzSvc.GetChainSwapClaimDetails(swapId)
	if err != nil {
		return fmt.Errorf("failed to get claim details: %w", err)
	}

	boltzPubKeyBytes, err := hex.DecodeString(claimDetails.PublicKey)
	if err != nil {
		return fmt.Errorf("failed to decode Boltz public key: %w", err)
	}
	boltzPubKey, err := btcec.ParsePubKey(boltzPubKeyBytes)
	if err != nil {
		return fmt.Errorf("failed to parse Boltz public key: %w", err)
	}

	musigCtx, err := NewMuSigContext(refundKey, boltzPubKey)
	if err != nil {
		return fmt.Errorf("musig context: %w", err)
	}

	ourNonce, err := musigCtx.GenerateNonce()
	if err != nil {
		return fmt.Errorf("generate nonce: %w", err)
	}

	boltzNonce, err := ParsePubNonce(claimDetails.PubNonce)
	if err != nil {
		return fmt.Errorf("parse boltz nonce: %w", err)
	}

	txHashBytes, err := hex.DecodeString(claimDetails.TransactionHash)
	if err != nil {
		return fmt.Errorf("decode transaction hash: %w", err)
	}
	var msg [32]byte
	copy(msg[:], txHashBytes)

	combinedNonce, err := musigCtx.AggregateNonces(boltzNonce)
	if err != nil {
		return fmt.Errorf("aggregate nonces: %w", err)
	}

	merkleRoot, err := computeSwapTreeMerkleRoot(swapResp.GetSwapTree(false))
	if err != nil {
		return fmt.Errorf("compute merkle root: %w", err)
	}

	keys := musigCtx.Keys()
	partialSig, err := musigCtx.OurPartialSign(combinedNonce, keys, msg, merkleRoot)
	if err != nil {
		return fmt.Errorf("our partial sig: %w", err)
	}

	var buf bytes.Buffer
	if err := partialSig.Encode(&buf); err != nil {
		return fmt.Errorf("encode partial sig: %w", err)
	}

	if _, err = b.swapHandler.boltzSvc.SubmitChainSwapClaim(swapId, boltz.ChainSwapClaimRequest{
		Signature: boltz.CrossSignSignature{
			PubNonce:         SerializePubNonce(ourNonce),
			PartialSignature: hex.EncodeToString(buf.Bytes()),
		},
	}); err != nil {
		return fmt.Errorf("submit claim to boltz: %w", err)
	}

	log.Infof("Successfully provided cooperative signature for Boltz to claim BTC")
	return nil
}
