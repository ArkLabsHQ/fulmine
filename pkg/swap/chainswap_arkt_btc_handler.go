package swap

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"

	"github.com/ArkLabsHQ/fulmine/pkg/boltz"
	"github.com/arkade-os/go-sdk/types"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/btcsuite/btcd/btcec/v2/schnorr/musig2"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	log "github.com/sirupsen/logrus"
)

type arkToBtcHandler struct {
	swapHandler           *SwapHandler
	chainSwapState        ChainSwapState
	network               *chaincfg.Params
	btcClaimPrivKey       *btcec.PrivateKey
	preimage              []byte
	btcDestinationAddress string
	swapResp              *boltz.CreateChainSwapResponse
	boltzClaimPubKey      *btcec.PublicKey
	swapTree              boltz.SwapTree
	userLockupTxid        string
}

func NewArkToBtcHandler(
	swapHandler *SwapHandler,
	state ChainSwapState,
	network *chaincfg.Params,
	btcClaimPrivKey *btcec.PrivateKey,
	preimage []byte,
	btcDestinationAddress string,
	swapResp *boltz.CreateChainSwapResponse,
	boltzClaimPubKey *btcec.PublicKey,
	swapTree boltz.SwapTree,
) ChainSwapEventHandler {
	return &arkToBtcHandler{
		swapHandler:           swapHandler,
		chainSwapState:        state,
		network:               network,
		btcClaimPrivKey:       btcClaimPrivKey,
		preimage:              preimage,
		btcDestinationAddress: btcDestinationAddress,
		swapResp:              swapResp,
		boltzClaimPubKey:      boltzClaimPubKey,
		swapTree:              swapTree,
		userLockupTxid:        "",
	}
}

func (h *arkToBtcHandler) HandleSwapCreated(ctx context.Context, update boltz.SwapUpdate) error {
	return h.handleArkToBtcSwapCreated(ctx, update)
}

func (h *arkToBtcHandler) HandleLockupFailed(ctx context.Context, update boltz.SwapUpdate) error {
	//TODO fetch and approve new quote
	log.Infof("Lockup failed for swap %s", h.chainSwapState.SwapID)
	return nil
}

func (h *arkToBtcHandler) HandleUserLockedMempool(ctx context.Context, update boltz.SwapUpdate) error {
	log.Infof("User lockup transaction for swap %s detected in mempool", h.chainSwapState.SwapID)
	return nil
}

func (h *arkToBtcHandler) HandleUserLocked(ctx context.Context, update boltz.SwapUpdate) error {
	return h.handleArkToBtcUserLocked(ctx, update)
}

func (h *arkToBtcHandler) HandleServerLockedMempool(ctx context.Context, update boltz.SwapUpdate) error {
	return h.handleArkToBtcServerLocked(ctx, update)
}

func (h *arkToBtcHandler) HandleServerLocked(ctx context.Context, update boltz.SwapUpdate) error {
	// we claim BTC only when it is confirmed
	return h.handleArkToBtcServerLocked(ctx, update)
}

func (h *arkToBtcHandler) HandleSwapExpired(ctx context.Context, update boltz.SwapUpdate) error {
	return h.handleArkToBtcFailure(ctx, update, "swap expired")
}

func (h *arkToBtcHandler) HandleTransactionFailed(ctx context.Context, update boltz.SwapUpdate) error {
	return h.handleArkToBtcFailure(ctx, update, "transaction failed")
}

func (h *arkToBtcHandler) GetState() ChainSwapState {
	return h.chainSwapState
}

func (h *arkToBtcHandler) handleArkToBtcSwapCreated(
	ctx context.Context,
	_ boltz.SwapUpdate,
) error {
	log.Infof("Swap %s created on Boltz", h.chainSwapState.SwapID)

	receivers := []types.Receiver{
		{
			To:     h.swapResp.LockupDetails.LockupAddress,
			Amount: h.chainSwapState.Swap.Amount,
		},
	}

	txId, err := h.swapHandler.arkClient.SendOffChain(ctx, receivers)
	if err != nil {
		return fmt.Errorf("failed to fund VHTLC: %w", err)
	}

	h.userLockupTxid = txId
	log.Infof("Funded ARK VHTLC with txid: %s", h.userLockupTxid)

	return nil
}

func (h *arkToBtcHandler) handleArkToBtcUserLocked(
	_ context.Context,
	update boltz.SwapUpdate,
) error {
	log.Infof("User lockup transaction for swap %s detected", h.chainSwapState.SwapID)

	h.chainSwapState.Swap.UserLock(update.Transaction.Id)

	return nil
}

func (h *arkToBtcHandler) handleArkToBtcServerLocked(
	ctx context.Context,
	update boltz.SwapUpdate,
) error {
	status := boltz.ParseEvent(update.Status)

	if status == boltz.TransactionServerMempoool {
		log.Infof("Boltz server lockup for swap %s detected in mempool", h.chainSwapState.SwapID)
		h.chainSwapState.Swap.ServerLock(update.Transaction.Id)

		return nil
	}

	log.Infof("Boltz locked BTC for swap %s (confirmed), proceeding with claim", h.chainSwapState.SwapID)

	serverLockupTxID := update.Transaction.Id
	serverLockupTxHex := update.Transaction.Hex

	h.chainSwapState.Swap.ServerLock(serverLockupTxID)

	claimTxid, err := h.claimBtcLockup(
		ctx,
		h.chainSwapState.SwapID,
		h.preimage,
		h.btcClaimPrivKey,
		h.btcDestinationAddress,
		h.network,
		h.swapTree,
		h.boltzClaimPubKey,
		serverLockupTxHex,
	)
	if err != nil {
		h.chainSwapState.Swap.Fail(fmt.Sprintf("claim failed: %v", err))

		return fmt.Errorf("failed to claim BTC lockup: %w", err)
	}

	h.chainSwapState.Swap.Claim(claimTxid)
	log.Infof("Successfully claimed BTC in transaction: %s", claimTxid)

	return nil
}

func (h *arkToBtcHandler) handleArkToBtcFailure(
	ctx context.Context,
	_ boltz.SwapUpdate,
	reason string,
) error {
	if reason == getQuote {
		log.Warnf("User lockup failed for swap %s (amount mismatch), fetching quote", h.chainSwapState.SwapID)

		quote, err := h.swapHandler.boltzSvc.GetChainSwapQuote(h.chainSwapState.SwapID)
		if err != nil {
			h.chainSwapState.Swap.UserLockedFailed(fmt.Sprintf("lockup failed, quote error: %v", err))
			return fmt.Errorf("failed to get quote: %w", err)
		}

		log.Infof("Quote for swap %s: amount=%d, onchainAmount=%d",
			h.chainSwapState.SwapID, quote.Amount, quote.OnchainAmount)

		if err := h.swapHandler.boltzSvc.AcceptChainSwapQuote(h.chainSwapState.SwapID, *quote); err != nil {
			h.chainSwapState.Swap.UserLockedFailed(fmt.Sprintf("quote acceptance failed: %v", err))
			return fmt.Errorf("failed to accept quote: %w", err)
		}

		log.Infof("Quote accepted for swap %s, waiting for Boltz to send VTXOs", h.chainSwapState.SwapID)
		return nil
	}
	log.Warnf("Swap %s %s, attempting refund", h.chainSwapState.SwapID, reason)

	refundTxid, err := h.swapHandler.RefundArkToBTCSwap(
		ctx, h.chainSwapState.SwapID, h.chainSwapState.Swap.VhtlcOpts, h.chainSwapState.UnilateralRefundCallback,
	)
	if err != nil {
		return fmt.Errorf("refund failed: %w", err)
	}

	log.Infof("Refund successful for swap %s: %s", h.chainSwapState.SwapID, refundTxid)
	h.chainSwapState.Swap.Refund(refundTxid)

	return nil
}

// claimBtcLockup performs a BTC lockup claim
// First tries cooperative MuSig2 claim (key path), then falls back to script-path if needed
func (h *arkToBtcHandler) claimBtcLockup(
	ctx context.Context,
	swapId string,
	preimage []byte,
	claimKey *btcec.PrivateKey,
	btcAddress string,
	network *chaincfg.Params,
	swapTree boltz.SwapTree,
	serverPubKey *btcec.PublicKey,
	serverLockupHex string,
) (string, error) {
	log.Infof("Claiming BTC lockup for swap %s", swapId)
	log.Infof("Attempting cooperative MuSig2 claim for swap %s", swapId)
	txid, err := h.claimBtcLockupCooperative(
		ctx,
		swapId,
		preimage,
		claimKey,
		btcAddress,
		network,
		swapTree,
		serverPubKey,
		serverLockupHex,
	)
	if err != nil {
		log.WithError(err).Warnf("Cooperative claim failed for swap %s, falling back to script-path claim", swapId)

		return h.claimBtcLockupScriptPath(
			ctx,
			swapId,
			preimage,
			claimKey,
			btcAddress,
			network,
			swapTree,
			serverPubKey,
			serverLockupHex,
		)
	}

	log.Infof("Successfully claimed BTC via cooperative path: %s", txid)
	return txid, nil
}

func (h *arkToBtcHandler) claimBtcLockupCooperative(
	_ context.Context,
	swapId string,
	preimage []byte,
	claimKey *btcec.PrivateKey,
	btcAddress string,
	network *chaincfg.Params,
	swapTree boltz.SwapTree,
	serverPubKey *btcec.PublicKey,
	serverLockupHex string,
) (string, error) {
	log.Infof("Performing cooperative MuSig2 claim for swap %s", swapId)

	setup, err := h.prepareClaimTransaction(
		serverPubKey,
		claimKey.PubKey(),
		swapTree,
		serverLockupHex,
		btcAddress,
		network,
	)
	if err != nil {
		return "", err
	}

	musigCtx, err := NewMuSigContext(claimKey, serverPubKey)
	if err != nil {
		return "", fmt.Errorf("musig context: %w", err)
	}

	ourNonce, err := musigCtx.GenerateNonce()
	if err != nil {
		return "", fmt.Errorf("generate nonce: %w", err)
	}

	claimTxHex, err := serializeTransaction(setup.claimTx)
	if err != nil {
		return "", fmt.Errorf("serialize claim tx: %w", err)
	}

	boltzSigResp, err := h.swapHandler.boltzSvc.SubmitChainSwapClaim(swapId, boltz.ChainSwapClaimRequest{
		Preimage: hex.EncodeToString(preimage),
		ToSign: boltz.ToSign{
			Nonce:   SerializePubNonce(ourNonce),
			ClaimTx: claimTxHex,
			Index:   0,
		},
	})
	if err != nil {
		return "", fmt.Errorf("submit claim to boltz: %w", err)
	}

	boltzNonce, err := ParsePubNonce(boltzSigResp.PubNonce)
	if err != nil {
		return "", fmt.Errorf("parse boltz nonce: %w", err)
	}

	prevOutFetcher := txscript.NewMultiPrevOutFetcher(map[wire.OutPoint]*wire.TxOut{
		setup.prevOutPoint: setup.prevOut,
	})

	msg, err := TaprootMessage(setup.claimTx, 0, prevOutFetcher)
	if err != nil {
		return "", fmt.Errorf("taproot message: %w", err)
	}

	combinedNonce, err := musigCtx.AggregateNonces(boltzNonce)
	if err != nil {
		return "", fmt.Errorf("aggregate nonces: %w", err)
	}

	keys := musigCtx.Keys()
	ourPartial, err := musigCtx.OurPartialSign(combinedNonce, keys, msg, setup.swapInfo.merkleRoot)
	if err != nil {
		return "", fmt.Errorf("our partial sig: %w", err)
	}

	boltzPartial, err := ParsePartialSignatureScalar32(boltzSigResp.PartialSignature)
	if err != nil {
		return "", fmt.Errorf("parse boltz partial sig: %w", err)
	}

	if ourPartial.R == nil {
		return "", fmt.Errorf("missing nonce point (ourPartial.R is nil)")
	}

	allPartials := []*musig2.PartialSignature{ourPartial, boltzPartial}
	finalSig, err := CombineFinalSig(ourPartial.R, allPartials, keys, msg, setup.swapInfo.merkleRoot)
	if err != nil {
		return "", fmt.Errorf("combine sigs: %w", err)
	}

	tweakedKey, err := ComputeTweakedOutputKey(keys, setup.swapInfo.merkleRoot)
	if err != nil {
		return "", fmt.Errorf("compute tweaked key: %w", err)
	}
	if err := VerifyFinalSig(msg, finalSig, tweakedKey); err != nil {
		return "", fmt.Errorf("final sig verify failed: %w", err)
	}

	setup.claimTx.TxIn[0].Witness = [][]byte{finalSig.Serialize()}

	txid, err := h.swapHandler.explorerClient.BroadcastTransaction(setup.claimTx)
	if err != nil {
		return "", fmt.Errorf("broadcast: %w", err)
	}

	return txid, nil
}

// claimBtcLockupScriptPath performs a non-cooperative script-path claim
// This method signs with only the claim key and includes the preimage in the witness
func (h *arkToBtcHandler) claimBtcLockupScriptPath(
	_ context.Context,
	swapId string,
	preimage []byte,
	claimKey *btcec.PrivateKey,
	btcAddress string,
	network *chaincfg.Params,
	swapTree boltz.SwapTree,
	serverPubKey *btcec.PublicKey,
	serverLockupHex string,
) (string, error) {
	log.Infof("Performing script-path claim for swap %s (non-cooperative)", swapId)

	setup, err := h.prepareClaimTransaction(
		serverPubKey,
		claimKey.PubKey(),
		swapTree,
		serverLockupHex,
		btcAddress,
		network,
	)
	if err != nil {
		return "", err
	}

	claimScript, err := hex.DecodeString(swapTree.ClaimLeaf.Output)
	if err != nil {
		return "", fmt.Errorf("failed to decode claim script: %w", err)
	}

	claimLeaf := txscript.NewBaseTapLeaf(claimScript)

	// Compute tweaked output key (internal key + merkle root tweak)
	// IMPORTANT: Server pubkey MUST be first for Boltz compatibility
	allPubKeys := []*btcec.PublicKey{serverPubKey, claimKey.PubKey()}
	aggregateKey, _, _, err := musig2.AggregateKeys(
		allPubKeys,
		false, // sort keys
	)
	if err != nil {
		return "", fmt.Errorf("failed to aggregate keys: %w", err)
	}

	internalKey := aggregateKey.FinalKey

	controlBlock, err := createControlBlockFromSwapTree(internalKey, swapTree, true /* isClaimPath */)
	if err != nil {
		return "", fmt.Errorf("failed to create control block: %w", err)
	}

	prevOutFetcher := txscript.NewMultiPrevOutFetcher(map[wire.OutPoint]*wire.TxOut{
		setup.prevOutPoint: setup.prevOut,
	})

	sigHashes := txscript.NewTxSigHashes(setup.claimTx, prevOutFetcher)
	sigHash, err := txscript.CalcTapscriptSignaturehash(
		sigHashes,
		txscript.SigHashDefault,
		setup.claimTx,
		0,
		prevOutFetcher,
		claimLeaf,
	)
	if err != nil {
		return "", fmt.Errorf("failed to calculate tapscript sighash: %w", err)
	}

	var msgHash [32]byte
	copy(msgHash[:], sigHash)
	signature, err := schnorr.Sign(claimKey, msgHash[:])
	if err != nil {
		return "", fmt.Errorf("failed to create Schnorr signature: %w", err)
	}

	// Build witness for script-path spend
	// Witness format: [signature, preimage, claimScript, controlBlock]
	witness := wire.TxWitness{
		signature.Serialize(),
		preimage,
		claimScript,
		controlBlock,
	}

	setup.claimTx.TxIn[0].Witness = witness

	log.Debugf("Script-path claim witness: sig=%d bytes, preimage=%d bytes, script=%d bytes, control=%d bytes",
		len(witness[0]), len(witness[1]), len(witness[2]), len(witness[3]))

	log.Infof("Broadcasting script-path claim transaction for swap %s...", swapId)

	txid, err := h.swapHandler.explorerClient.BroadcastTransaction(setup.claimTx)
	if err != nil {
		return "", fmt.Errorf("failed to broadcast script-path claim transaction: %w", err)
	}

	log.Infof("Successfully broadcast script-path claim transaction: %s", txid)

	return txid, nil
}

// prepareClaimTransaction extracts common claim transaction setup logic
// Returns a claimSetup struct with all necessary components for signing
func (h *arkToBtcHandler) prepareClaimTransaction(
	serverPubKey *btcec.PublicKey,
	claimPubKey *btcec.PublicKey,
	swapTree boltz.SwapTree,
	serverLockupHex string,
	btcAddress string,
	network *chaincfg.Params,
) (*claimSetup, error) {
	swapInfo, err := buildSwapRecipe(serverPubKey, claimPubKey, swapTree)
	if err != nil {
		return nil, fmt.Errorf("build btc swap recipe: %w", err)
	}

	lockupTx, err := deserializeTransaction(serverLockupHex)
	if err != nil {
		return nil, fmt.Errorf("deserialize lockup tx: %w", err)
	}

	lockupOutput, err := findLockupOutput(lockupTx, swapInfo.lockupScript)
	if err != nil {
		return nil, fmt.Errorf("find lockup output: %w", err)
	}

	claimTxParams := ClaimTransactionParams{
		LockupTxid:      lockupOutput.txid,
		LockupVout:      lockupOutput.vout,
		LockupAmount:    lockupOutput.amount,
		DestinationAddr: btcAddress,
		Network:         network,
	}

	claimTx, err := constructClaimTransaction(h.swapHandler.explorerClient, h.swapHandler.config.Dust, claimTxParams)
	if err != nil {
		return nil, fmt.Errorf("construct claim tx: %w", err)
	}

	prevOut := &wire.TxOut{
		Value:    int64(lockupOutput.amount),
		PkScript: lockupOutput.pkScript,
	}
	prevOutPoint := wire.OutPoint{
		Hash:  claimTx.TxIn[0].PreviousOutPoint.Hash,
		Index: claimTx.TxIn[0].PreviousOutPoint.Index,
	}

	return &claimSetup{
		swapInfo:     swapInfo,
		lockupTx:     lockupTx,
		lockupOutput: lockupOutput,
		claimTx:      claimTx,
		prevOut:      prevOut,
		prevOutPoint: prevOutPoint,
	}, nil
}

type swapInfo struct {
	serverPubKey *btcec.PublicKey
	claimPubKey  *btcec.PublicKey
	merkleRoot   []byte
	lockupScript []byte
	swapTree     boltz.SwapTree
}

func findLockupOutput(lockupTx *wire.MsgTx, expectedPkScript []byte) (*lockupTxOutput, error) {
	for vout, out := range lockupTx.TxOut {
		if bytes.Equal(out.PkScript, expectedPkScript) {
			return &lockupTxOutput{
				txid:     lockupTx.TxHash().String(),
				vout:     uint32(vout),
				amount:   uint64(out.Value),
				pkScript: out.PkScript,
			}, nil
		}
	}
	return nil, fmt.Errorf("lockup output not found for pkScript=%x", expectedPkScript)
}

func buildSwapRecipe(serverPub, claimPub *btcec.PublicKey, tree boltz.SwapTree) (*swapInfo, error) {
	if err := validateSwapTree(tree); err != nil {
		return nil, err
	}
	mr, err := computeSwapTreeMerkleRoot(tree)
	if err != nil {
		return nil, err
	}
	script, err := computeExpectedLockupScript(serverPub, claimPub, mr)
	if err != nil {
		return nil, err
	}
	return &swapInfo{
		serverPubKey: serverPub,
		claimPubKey:  claimPub,
		merkleRoot:   mr,
		lockupScript: script,
		swapTree:     tree,
	}, nil
}

func computeExpectedLockupScript(
	serverPubKey *btcec.PublicKey,
	claimPubKey *btcec.PublicKey,
	merkleRoot []byte,
) ([]byte, error) {
	allPubKeys := []*btcec.PublicKey{serverPubKey, claimPubKey}
	aggregateKey, _, _, err := musig2.AggregateKeys(
		allPubKeys,
		false,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to aggregate keys: %w", err)
	}

	aggregatedKey := aggregateKey.FinalKey
	tweakedKey := txscript.ComputeTaprootOutputKey(aggregatedKey, merkleRoot)

	script, err := txscript.PayToTaprootScript(tweakedKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create P2TR script: %w", err)
	}

	return script, nil
}

type claimSetup struct {
	swapInfo     *swapInfo
	lockupTx     *wire.MsgTx
	lockupOutput *lockupTxOutput
	claimTx      *wire.MsgTx
	prevOut      *wire.TxOut
	prevOutPoint wire.OutPoint
}

type lockupTxOutput struct {
	txid     string
	vout     uint32
	amount   uint64
	pkScript []byte
}
