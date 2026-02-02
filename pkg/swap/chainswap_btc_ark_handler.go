package swap

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/ArkLabsHQ/fulmine/pkg/boltz"
	arklib "github.com/arkade-os/arkd/pkg/ark-lib"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
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
	// Handle quote flow for amount mismatch
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

	// Implement BTC→ARK refund flow
	// Since fulmine is not a BTC wallet, we claim BTC to a boarding address and then settle to convert to VTXO
	log.Warnf("Swap %s failed: %s, attempting BTC refund via boarding address", b.chainSwapState.SwapID, reason)

	refundTxid, err := b.refundBtcToArkSwap(ctx)
	if err != nil {
		log.WithError(err).Errorf("BTC refund failed for swap %s", b.chainSwapState.SwapID)
		b.chainSwapState.Swap.RefundFailed(fmt.Sprintf("refund failed: %v", err))
		return fmt.Errorf("failed to refund BTC: %w", err)
	}

	log.Infof("BTC refund successful for swap %s: txid=%s", b.chainSwapState.SwapID, refundTxid)
	b.chainSwapState.Swap.Refund(refundTxid)

	return nil
}

// refundBtcToArkSwap implements the BTC→ARK refund flow:
// 1. Fetch swap info from Boltz API
// 2. Parse refund script from swap tree (CLTV timeout-based)
// 3. Wait for CLTV timeout to pass
// 4. Create claim transaction spending BTC lockup via refund script path
// 5. Get fulmine boarding address
// 6. Sign and broadcast claim transaction to boarding address
// 7. Wait for confirmation
// 8. Call Settle() to board the BTC as VTXO
func (b *btcToArkHandler) refundBtcToArkSwap(ctx context.Context) (string, error) {
	log.Infof("Starting BTC→ARK refund for swap %s", b.chainSwapState.SwapID)

	// Step 1: Fetch swap transactions from Boltz API
	swapTxs, err := b.swapHandler.boltzSvc.GetChainSwapTransactions(b.chainSwapState.SwapID)
	if err != nil {
		return "", fmt.Errorf("failed to get swap transactions: %w", err)
	}

	if swapTxs.UserLock == nil || swapTxs.UserLock.Id == "" {
		return "", fmt.Errorf("no user lockup transaction found for swap %s", b.chainSwapState.SwapID)
	}

	userLockupTxid := swapTxs.UserLock.Id
	userLockupTxHex := swapTxs.UserLock.Hex
	log.Infof("User lockup txid: %s", userLockupTxid)

	// Step 2: Parse refund script from swap tree
	swapTree := b.swapResp.GetSwapTree(false) // false for BTC→ARK
	refundComponents, err := ValidateRefundLeafScript(swapTree.RefundLeaf.Output)
	if err != nil {
		return "", fmt.Errorf("failed to parse refund script: %w", err)
	}

	log.Infof("Refund script parsed - timeout: %d blocks, refund pubkey: %x",
		refundComponents.Timeout, refundComponents.RefundPubKey)

	// Step 3: Wait for CLTV timeout to pass
	currentHeight, err := b.swapHandler.explorerClient.GetCurrentBlockHeight()
	if err != nil {
		return "", fmt.Errorf("failed to get current block height: %w", err)
	}

	lockupHeight := b.swapResp.LockupDetails.TimeoutBlockHeight
	requiredHeight := lockupHeight + int(refundComponents.Timeout)

	if currentHeight < uint32(requiredHeight) {
		blocksRemaining := requiredHeight - int(currentHeight)
		log.Infof("Waiting for CLTV timeout: current=%d, required=%d, blocks remaining=%d",
			currentHeight, requiredHeight, blocksRemaining)

		// Wait for blocks (approximately 10 minutes per block)
		waitDuration := time.Duration(blocksRemaining*10) * time.Minute
		log.Infof("Estimated wait time: %v", waitDuration)

		// For now, we return an error and expect the user to retry later
		// TODO: Implement a scheduler to automatically retry after timeout
		return "", fmt.Errorf("CLTV timeout not yet reached: current block %d, required %d (wait %d more blocks)",
			currentHeight, requiredHeight, blocksRemaining)
	}

	log.Infof("CLTV timeout reached at block %d (required %d)", currentHeight, requiredHeight)

	// Step 4: Get fulmine boarding address
	_, _, boardingAddr, err := b.swapHandler.arkClient.Receive(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to get boarding address: %w", err)
	}

	log.Infof("Boarding address: %s", boardingAddr)

	// Step 5: Construct refund transaction to boarding address
	lockupAmount := uint64(b.swapResp.LockupDetails.Amount)

	claimTx, err := ConstructClaimTransaction(
		b.swapHandler.explorerClient,
		b.swapHandler.config.Dust,
		ClaimTransactionParams{
			LockupTxid:      userLockupTxid,
			LockupVout:      0, // Assume vout 0 (standard for chain swaps)
			LockupAmount:    lockupAmount,
			DestinationAddr: boardingAddr,
			Network:         networkNameToParams(b.swapHandler.config.Network.Name),
		},
	)
	if err != nil {
		return "", fmt.Errorf("failed to construct claim transaction: %w", err)
	}

	// Step 6: Sign transaction using refund script path
	// Parse the refund script to construct the witness
	refundScript, err := hex.DecodeString(swapTree.RefundLeaf.Output)
	if err != nil {
		return "", fmt.Errorf("failed to decode refund script: %w", err)
	}

	// Compute merkle root for tapscript
	merkleRoot, err := computeSwapTreeMerkleRoot(swapTree)
	if err != nil {
		return "", fmt.Errorf("failed to compute merkle root: %w", err)
	}

	// Create tapscript leaf
	tapLeaf := txscript.NewBaseTapLeaf(refundScript)

	// Compute control block
	internalKey, err := schnorr.ParsePubKey(b.refundKey.PubKey().SerializeCompressed()[1:])
	if err != nil {
		return "", fmt.Errorf("failed to parse internal key: %w", err)
	}

	proof := txscript.TapscriptProof{
		TapLeaf:  tapLeaf,
		RootNode: txscript.NewTapBranch(tapLeaf, txscript.NewBaseTapLeaf(merkleRoot)),
	}

	controlBlock := proof.ToControlBlock(internalKey)
	controlBlockBytes, err := controlBlock.ToBytes()
	if err != nil {
		return "", fmt.Errorf("failed to serialize control block: %w", err)
	}

	// Set transaction locktime to satisfy CLTV
	claimTx.LockTime = refundComponents.Timeout

	// Sign the transaction
	prevOutFetcher, err := parsePrevoutFetcher(userLockupTxHex, claimTx, 0)
	if err != nil {
		return "", fmt.Errorf("failed to parse prevout fetcher: %w", err)
	}

	sigHash, err := txscript.CalcTapscriptSignaturehash(
		txscript.NewTxSigHashes(claimTx, prevOutFetcher),
		txscript.SigHashDefault,
		claimTx,
		0,
		prevOutFetcher,
		tapLeaf,
	)
	if err != nil {
		return "", fmt.Errorf("failed to calculate sighash: %w", err)
	}

	var sigHashBytes [32]byte
	copy(sigHashBytes[:], sigHash)

	signature, err := schnorr.Sign(b.refundKey, sigHashBytes[:])
	if err != nil {
		return "", fmt.Errorf("failed to sign refund transaction: %w", err)
	}

	// Construct witness stack: [signature] [refund_script] [control_block]
	claimTx.TxIn[0].Witness = [][]byte{
		signature.Serialize(),
		refundScript,
		controlBlockBytes,
	}

	// Step 7: Broadcast transaction
	claimTxid, err := b.swapHandler.explorerClient.BroadcastTransaction(claimTx)
	if err != nil {
		return "", fmt.Errorf("failed to broadcast refund transaction: %w", err)
	}

	log.Infof("Refund transaction broadcast: %s", claimTxid)
	log.Infof("BTC sent to boarding address %s, waiting for confirmation...", boardingAddr)

	// Step 8: Wait for confirmation (at least 1 confirmation)
	confirmed := false
	maxWaitTime := 2 * time.Hour
	startTime := time.Now()
	pollInterval := 30 * time.Second

	for time.Since(startTime) < maxWaitTime {
		txStatus, err := b.swapHandler.explorerClient.GetTransactionStatus(claimTxid)
		if err != nil {
			log.WithError(err).Warnf("Failed to get transaction status, will retry")
			time.Sleep(pollInterval)
			continue
		}

		if txStatus.Confirmed {
			log.Infof("Refund transaction %s confirmed at block %d", claimTxid, txStatus.BlockHeight)
			confirmed = true
			break
		}

		log.Debugf("Waiting for refund transaction confirmation... (elapsed: %v)", time.Since(startTime))
		time.Sleep(pollInterval)
	}

	if !confirmed {
		return "", fmt.Errorf("refund transaction %s not confirmed within %v", claimTxid, maxWaitTime)
	}

	// Step 9: Call Settle() to board the BTC as VTXO
	log.Infof("Calling Settle() to board BTC as VTXO...")
	settleTxid, err := b.swapHandler.arkClient.Settle(ctx)
	if err != nil {
		// Log but don't fail - the BTC is now in the boarding address
		// and can be settled in the next round
		log.WithError(err).Warnf("Settle() failed, but BTC is safely in boarding address %s", boardingAddr)
		log.Infof("You can manually settle later to complete the boarding process")
		return claimTxid, nil
	}

	log.Infof("Settle transaction: %s", settleTxid)
	log.Infof("BTC successfully refunded and boarded as VTXO!")

	return claimTxid, nil
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

// networkNameToParams converts arklib network name to chaincfg.Params
func networkNameToParams(networkName string) *chaincfg.Params {
	switch networkName {
	case arklib.Bitcoin.Name:
		return &chaincfg.MainNetParams
	case arklib.BitcoinTestNet.Name:
		return &chaincfg.TestNet3Params
	case arklib.BitcoinRegTest.Name:
		return &chaincfg.RegressionNetParams
	case arklib.BitcoinSigNet.Name, arklib.BitcoinMutinyNet.Name:
		return &chaincfg.SigNetParams
	default:
		return &chaincfg.RegressionNetParams
	}
}

// parsePrevoutFetcher creates a prevout fetcher for transaction signing
func parsePrevoutFetcher(lockupTxHex string, claimTx *wire.MsgTx, inputIndex int) (txscript.PrevOutputFetcher, error) {
	lockupTxBytes, err := hex.DecodeString(lockupTxHex)
	if err != nil {
		return nil, fmt.Errorf("failed to decode lockup tx hex: %w", err)
	}

	var lockupTx wire.MsgTx
	if err := lockupTx.Deserialize(bytes.NewReader(lockupTxBytes)); err != nil {
		return nil, fmt.Errorf("failed to deserialize lockup tx: %w", err)
	}

	prevOut := claimTx.TxIn[inputIndex].PreviousOutPoint
	if int(prevOut.Index) >= len(lockupTx.TxOut) {
		return nil, fmt.Errorf("invalid prevout index %d (lockup tx has %d outputs)", prevOut.Index, len(lockupTx.TxOut))
	}

	prevOutputFetcher := txscript.NewCannedPrevOutputFetcher(
		lockupTx.TxOut[prevOut.Index].PkScript,
		lockupTx.TxOut[prevOut.Index].Value,
	)

	return prevOutputFetcher, nil
}
