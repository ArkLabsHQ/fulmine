package banco

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"strings"

	log "github.com/sirupsen/logrus"

	"github.com/ArkLabsHQ/introspector/pkg/arkade"
	introclient "github.com/ArkLabsHQ/introspector/pkg/client"
	arklib "github.com/arkade-os/arkd/pkg/ark-lib"
	"github.com/arkade-os/arkd/pkg/ark-lib/asset"
	"github.com/arkade-os/arkd/pkg/ark-lib/extension"
	"github.com/arkade-os/arkd/pkg/ark-lib/offchain"
	"github.com/arkade-os/arkd/pkg/ark-lib/script"
	"github.com/arkade-os/arkd/pkg/client-lib/client"
	"github.com/arkade-os/arkd/pkg/client-lib/indexer"
	clientTypes "github.com/arkade-os/arkd/pkg/client-lib/types"
	arksdk "github.com/arkade-os/go-sdk"
	"github.com/btcsuite/btcd/btcutil/psbt"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	"github.com/btcsuite/btcwallet/waddrmgr"
)

// FulfillResult contains the result of a successful fulfillment.
type FulfillResult struct {
	ArkTxid string
}

// FulfillOffer constructs and submits the fulfillment transaction for a banco offer.
// Matches ts-sdk/src/banco/taker.ts Taker.fulfillOffer().
func FulfillOffer(
	ctx context.Context,
	offer *BancoOffer,
	transportClient client.TransportClient,
	indexerClient indexer.Indexer,
	arkClient arksdk.ArkClient,
	introClient introclient.TransportClient,
) (*FulfillResult, error) {
	cfg, err := arkClient.GetConfigData(ctx)
	if err != nil {
		return nil, err
	}
	checkpointTapscriptBytes, err := hex.DecodeString(cfg.CheckpointTapscript)
	if err != nil {
		return nil, fmt.Errorf("failed to decode checkpoint tapscript: %w", err)
	}

	swapVtxoScript, err := offer.VtxoScript(cfg.SignerPubKey)
	if err != nil {
		return nil, fmt.Errorf("failed to build swap vtxo script: %w", err)
	}

	swapTapKey, swapTapTree, err := swapVtxoScript.TapTree()
	if err != nil {
		return nil, fmt.Errorf("failed to build swap taptree: %w", err)
	}

	swapPkScript, err := script.P2TRScript(swapTapKey)
	if err != nil {
		return nil, fmt.Errorf("failed to build swap pkscript: %w", err)
	}

	if !bytes.Equal(swapPkScript, offer.SwapPkScript) {
		return nil, fmt.Errorf("offer inconsistency: swapAddress does not match reconstructed contract")
	}

	swapPkScriptHex := hex.EncodeToString(swapPkScript)
	vtxosResp, err := indexerClient.GetVtxos(ctx,
		indexer.WithScripts([]string{swapPkScriptHex}),
		indexer.WithSpendableOnly(),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get swap vtxos: %w", err)
	}
	if len(vtxosResp.Vtxos) == 0 {
		return nil, fmt.Errorf("no spendable VTXO found at swap address")
	}
	swapVtxo := vtxosResp.Vtxos[0]

	wp, ok := arkClient.(walletProvider)
	if !ok {
		return nil, fmt.Errorf("arkClient does not provide wallet access")
	}

	spendableVtxos, err := arkClient.ListSpendableVtxos(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list taker vtxos: %w", err)
	}
	if len(spendableVtxos) == 0 {
		return nil, fmt.Errorf("taker wallet has no VTXOs")
	}

	_, offchainAddrs, _, _, err := wp.Wallet().GetAddresses(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get offchain addresses: %w", err)
	}

	takerVtxos := make([]clientTypes.VtxoWithTapTree, 0, len(spendableVtxos))
	for _, addr := range offchainAddrs {
		for _, v := range spendableVtxos {
			vtxoAddr, err := v.Address(cfg.SignerPubKey, cfg.Network)
			if err != nil {
				continue
			}
			if vtxoAddr == addr.Address {
				takerVtxos = append(takerVtxos, clientTypes.VtxoWithTapTree{
					Vtxo:       v,
					Tapscripts: addr.Tapscripts,
				})
			}
		}
	}
	if len(takerVtxos) == 0 {
		return nil, fmt.Errorf("no taker VTXOs matched to offchain addresses")
	}

	takerAddr, err := arkClient.NewOffchainAddress(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get taker address: %w", err)
	}
	takerDecodedAddr, err := arklib.DecodeAddressV0(takerAddr)
	if err != nil {
		return nil, fmt.Errorf("failed to decode taker address: %w", err)
	}
	takerPkScript, err := script.P2TRScript(takerDecodedAddr.VtxoTapKey)
	if err != nil {
		return nil, fmt.Errorf("failed to build taker pkscript: %w", err)
	}

	var totalTakerBtc uint64
	for _, v := range takerVtxos {
		totalTakerBtc += v.Amount
	}

	// When the maker wants an asset (not BTC), the output carries the asset
	// on a dust BTC carrier. The asset amount is routed via the asset packet.
	var makerBtcAmount int64
	if offer.WantAsset != nil {
		makerBtcAmount = 330 // dust limit for asset-carrying outputs
	} else {
		makerBtcAmount = int64(offer.WantAmount)
	}
	btcChange := int64(totalTakerBtc) - makerBtcAmount
	if btcChange < 0 {
		return nil, fmt.Errorf("insufficient BTC: have %d, need %d", totalTakerBtc, makerBtcAmount)
	}

	// Merge BTC change into the taker output to avoid creating sub-dust outputs.
	outputs := []*wire.TxOut{
		{Value: makerBtcAmount, PkScript: offer.MakerPkScript},
		{Value: int64(swapVtxo.Amount) + btcChange, PkScript: takerPkScript},
	}

	fulfillScriptBytes, err := offer.FulfillScript()
	if err != nil {
		return nil, fmt.Errorf("failed to build fulfill script: %w", err)
	}

	introspectorPacket, err := arkade.NewPacket(arkade.IntrospectorEntry{
		Vin:    0,
		Script: fulfillScriptBytes,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to build introspector packet: %w", err)
	}
	ext := extension.Extension{introspectorPacket}

	// Build asset packet tracking asset flows across inputs/outputs.
	// Input 0 = swap VTXO, inputs 1+ = taker VTXOs.
	// Output 0 = maker, output 1 = taker (receives swap assets + BTC change).
	assetPacket, err := buildFulfillAssetPacket(swapVtxo, takerVtxos, offer)
	if err != nil {
		return nil, fmt.Errorf("failed to build asset packet: %w", err)
	}
	if assetPacket != nil {
		ext = append(ext, assetPacket)
	}

	extTxOut, err := ext.TxOut()
	if err != nil {
		return nil, fmt.Errorf("failed to build extension output: %w", err)
	}
	outputs = append(outputs, extTxOut)

	fulfillClosure := swapVtxoScript.Closures[0] // first closure is the fulfill leaf
	fulfillScript, err := fulfillClosure.Script()
	if err != nil {
		return nil, fmt.Errorf("failed to build fulfill closure script: %w", err)
	}
	fulfillLeafHash := txscript.NewBaseTapLeaf(fulfillScript).TapHash()
	fulfillMerkleProof, err := swapTapTree.GetTaprootMerkleProof(fulfillLeafHash)
	if err != nil {
		return nil, fmt.Errorf("failed to get fulfill leaf merkle proof: %w", err)
	}
	fulfillControlBlock, err := txscript.ParseControlBlock(fulfillMerkleProof.ControlBlock)
	if err != nil {
		return nil, fmt.Errorf("failed to parse control block: %w", err)
	}

	swapRevealedTapscripts, err := swapVtxoScript.Encode()
	if err != nil {
		return nil, fmt.Errorf("failed to encode swap tapscripts: %w", err)
	}

	swapVtxoHash, err := chainhash.NewHashFromStr(swapVtxo.Txid)
	if err != nil {
		return nil, fmt.Errorf("failed to parse swap vtxo txid: %w", err)
	}

	vtxoInputs := make([]offchain.VtxoInput, 0, 1+len(takerVtxos))

	vtxoInputs = append(vtxoInputs, offchain.VtxoInput{
		Outpoint: &wire.OutPoint{Hash: *swapVtxoHash, Index: swapVtxo.VOut},
		Amount:   int64(swapVtxo.Amount),
		Tapscript: &waddrmgr.Tapscript{
			ControlBlock:   fulfillControlBlock,
			RevealedScript: fulfillMerkleProof.Script,
		},
		RevealedTapscripts: swapRevealedTapscripts,
	})

	for _, tv := range takerVtxos {
		tvHash, err := chainhash.NewHashFromStr(tv.Txid)
		if err != nil {
			return nil, fmt.Errorf("failed to parse taker vtxo txid: %w", err)
		}

		takerVtxoScripts, err := script.ParseVtxoScript(tv.Tapscripts)
		if err != nil {
			return nil, fmt.Errorf("failed to parse taker vtxo script: %w", err)
		}

		tvTapscriptScript, ok := takerVtxoScripts.(*script.TapscriptsVtxoScript)
		if !ok {
			return nil, fmt.Errorf("unexpected taker vtxo script type")
		}

		forfeitClosures := tvTapscriptScript.ForfeitClosures()
		if len(forfeitClosures) == 0 {
			return nil, fmt.Errorf("taker vtxo has no forfeit closures")
		}

		forfeitClosure := forfeitClosures[0]
		forfeitClosureScript, err := forfeitClosure.Script()
		if err != nil {
			return nil, fmt.Errorf("failed to build forfeit closure script: %w", err)
		}

		_, tvTapTree, err := tvTapscriptScript.TapTree()
		if err != nil {
			return nil, fmt.Errorf("failed to build taker vtxo taptree: %w", err)
		}

		forfeitLeafHash := txscript.NewBaseTapLeaf(forfeitClosureScript).TapHash()
		forfeitMerkleProof, err := tvTapTree.GetTaprootMerkleProof(forfeitLeafHash)
		if err != nil {
			return nil, fmt.Errorf("failed to get forfeit merkle proof: %w", err)
		}
		forfeitControlBlock, err := txscript.ParseControlBlock(forfeitMerkleProof.ControlBlock)
		if err != nil {
			return nil, fmt.Errorf("failed to parse forfeit control block: %w", err)
		}

		revealedScripts, err := tvTapscriptScript.Encode()
		if err != nil {
			return nil, fmt.Errorf("failed to encode taker vtxo tapscripts: %w", err)
		}

		vtxoInputs = append(vtxoInputs, offchain.VtxoInput{
			Outpoint:           &wire.OutPoint{Hash: *tvHash, Index: tv.VOut},
			Amount:             int64(tv.Amount),
			Tapscript:          &waddrmgr.Tapscript{ControlBlock: forfeitControlBlock, RevealedScript: forfeitMerkleProof.Script},
			RevealedTapscripts: revealedScripts,
		})
	}

	arkTx, checkpoints, err := offchain.BuildTxs(vtxoInputs, outputs, checkpointTapscriptBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to build offchain txs: %w", err)
	}

	signedArkTxB64, err := arkTx.B64Encode()
	if err != nil {
		return nil, fmt.Errorf("failed to encode ark tx: %w", err)
	}
	signedArkTxB64, err = arkClient.SignTransaction(ctx, signedArkTxB64)
	if err != nil {
		return nil, fmt.Errorf("failed to sign ark tx: %w", err)
	}

	checkpointB64s := make([]string, 0, len(checkpoints))
	for _, cp := range checkpoints {
		cpB64, err := cp.B64Encode()
		if err != nil {
			return nil, fmt.Errorf("failed to encode checkpoint: %w", err)
		}
		checkpointB64s = append(checkpointB64s, cpB64)
	}

	// Step 1: Introspector signs ark tx + checkpoints (single call).
	introSignedTx, introSignedCheckpoints, err := introClient.SubmitTx(ctx, signedArkTxB64, checkpointB64s)
	if err != nil {
		return nil, fmt.Errorf("introspector submission failed: %w", err)
	}

	// Step 2: Send intro-signed ark tx + checkpoints to arkd.
	arkTxid, _, serverSignedCheckpoints, err := transportClient.SubmitTx(
		ctx, introSignedTx, introSignedCheckpoints,
	)
	if err != nil {
		return nil, fmt.Errorf("ark server submission failed: %w", err)
	}

	// Debug: compare local vs server checkpoint txids to check mismatch
	for i := range serverSignedCheckpoints {
		localCp, _ := psbt.NewFromRawBytes(strings.NewReader(introSignedCheckpoints[i]), true)
		serverCp, _ := psbt.NewFromRawBytes(strings.NewReader(serverSignedCheckpoints[i]), true)
		if localCp != nil && serverCp != nil {
			log.WithFields(log.Fields{
				"index":      i,
				"localTxid":  localCp.UnsignedTx.TxHash().String(),
				"serverTxid": serverCp.UnsignedTx.TxHash().String(),
				"match":      localCp.UnsignedTx.TxHash() == serverCp.UnsignedTx.TxHash(),
			}).Debug("taker: checkpoint comparison")
		}
	}

	// Index introspector-signed checkpoints by txid for matching.
	// Only include checkpoints where the introspector actually added a signature
	// (arkd may return checkpoints in a different order than fulmine built them).
	introCheckpointsByTxid := make(map[string]string)
	for _, cpB64 := range introSignedCheckpoints {
		cp, err := psbt.NewFromRawBytes(strings.NewReader(cpB64), true)
		if err != nil {
			continue
		}
		// Only consider this checkpoint "intro-signed" if it has tapscript sigs
		hasSigs := false
		for _, inp := range cp.Inputs {
			if len(inp.TaprootScriptSpendSig) > 0 {
				hasSigs = true
				break
			}
		}
		if hasSigs {
			introCheckpointsByTxid[cp.UnsignedTx.TxHash().String()] = cpB64
		}
	}

	// Merge sigs and finalize checkpoints.
	// The swap VTXO checkpoint needs server + introspector sigs.
	// Taker VTXO checkpoints need server + taker sigs.
	finalCheckpoints := make([]string, 0, len(serverSignedCheckpoints))
	for _, serverCpB64 := range serverSignedCheckpoints {
		serverCp, err := psbt.NewFromRawBytes(strings.NewReader(serverCpB64), true)
		if err != nil {
			return nil, fmt.Errorf("failed to parse server checkpoint: %w", err)
		}
		serverTxid := serverCp.UnsignedTx.TxHash().String()

		// Check if the introspector signed this checkpoint (swap VTXO)
		introCpB64, hasIntroSig := introCheckpointsByTxid[serverTxid]
		if hasIntroSig {
			// Swap input: merge introspector sig into server checkpoint
			introCp, err := psbt.NewFromRawBytes(strings.NewReader(introCpB64), true)
			if err == nil {
				for j := range introCp.Inputs {
					if j < len(serverCp.Inputs) && len(introCp.Inputs[j].TaprootScriptSpendSig) > 0 {
						serverCp.Inputs[j].TaprootScriptSpendSig = append(
							serverCp.Inputs[j].TaprootScriptSpendSig,
							introCp.Inputs[j].TaprootScriptSpendSig...,
						)
					}
				}
			}

			cpB64, err := serverCp.B64Encode()
			if err != nil {
				return nil, fmt.Errorf("failed to encode merged checkpoint: %w", err)
			}
			finalCheckpoints = append(finalCheckpoints, cpB64)
		} else {
			// Taker input: sign with taker key
			signedCpB64, err := arkClient.SignTransaction(ctx, serverCpB64)
			if err != nil {
				return nil, fmt.Errorf("failed to sign checkpoint %s: %w", serverTxid, err)
			}
			finalCheckpoints = append(finalCheckpoints, signedCpB64)
		}
	}

	if err := transportClient.FinalizeTx(ctx, arkTxid, finalCheckpoints); err != nil {
		return nil, fmt.Errorf("finalization failed: %w", err)
	}

	return &FulfillResult{ArkTxid: arkTxid}, nil
}

// buildFulfillAssetPacket creates an asset packet for the fulfillment tx.
// Input layout:  0 = swap VTXO, 1..N = taker VTXOs.
// Output layout: 0 = maker, 1 = taker (receives swap assets + taker asset change + BTC change).
func buildFulfillAssetPacket(
	swapVtxo clientTypes.Vtxo,
	takerVtxos []clientTypes.VtxoWithTapTree,
	offer *BancoOffer,
) (asset.Packet, error) {
	type assetTransfer struct {
		inputs  []asset.AssetInput
		outputs []asset.AssetOutput
	}

	transfers := make(map[string]*assetTransfer)

	ensureTransfer := func(assetId string) *assetTransfer {
		if _, exists := transfers[assetId]; !exists {
			transfers[assetId] = &assetTransfer{}
		}
		return transfers[assetId]
	}

	// Register all asset inputs: swap VTXO (input 0) + taker VTXOs (inputs 1+)
	for _, a := range swapVtxo.Assets {
		t := ensureTransfer(a.AssetId)
		input, err := asset.NewAssetInput(0, a.Amount)
		if err != nil {
			return nil, err
		}
		t.inputs = append(t.inputs, *input)
	}

	for i, tv := range takerVtxos {
		inputIdx := uint16(i + 1)
		for _, a := range tv.Assets {
			t := ensureTransfer(a.AssetId)
			input, err := asset.NewAssetInput(inputIdx, a.Amount)
			if err != nil {
				return nil, err
			}
			t.inputs = append(t.inputs, *input)
		}
	}

	if len(transfers) == 0 {
		return nil, nil
	}

	// Route maker's wanted asset to output 0
	if offer.WantAsset != nil {
		wantAssetStr := offer.WantAsset.String()
		t, exists := transfers[wantAssetStr]
		if !exists {
			return nil, fmt.Errorf("asset %s not found in inputs", wantAssetStr)
		}
		output, err := asset.NewAssetOutput(0, offer.WantAmount)
		if err != nil {
			return nil, err
		}
		t.outputs = append(t.outputs, *output)
	}

	// All remaining asset balance goes to the taker output (output 1).
	for _, t := range transfers {
		var totalIn, totalOut uint64
		for _, in := range t.inputs {
			totalIn += in.Amount
		}
		for _, out := range t.outputs {
			totalOut += out.Amount
		}
		if totalIn > totalOut {
			output, err := asset.NewAssetOutput(1, totalIn-totalOut)
			if err != nil {
				return nil, err
			}
			t.outputs = append(t.outputs, *output)
		}
	}

	groups := make([]asset.AssetGroup, 0, len(transfers))

	// The wanted asset group MUST be at index 0 because the fulfill script
	// uses lookup_index=0 in OP_INSPECTOUTASSETLOOKUP.
	if offer.WantAsset != nil {
		wantAssetStr := offer.WantAsset.String()
		if t, exists := transfers[wantAssetStr]; exists && len(t.inputs) > 0 {
			group, err := asset.NewAssetGroup(offer.WantAsset, nil, t.inputs, t.outputs, nil)
			if err != nil {
				return nil, err
			}
			groups = append(groups, *group)
		}
	}

	for assetIdStr, t := range transfers {
		if len(t.inputs) == 0 {
			continue
		}
		// Skip the wanted asset since it was already added at index 0.
		if offer.WantAsset != nil && assetIdStr == offer.WantAsset.String() {
			continue
		}
		assetId, err := asset.NewAssetIdFromString(assetIdStr)
		if err != nil {
			return nil, err
		}
		group, err := asset.NewAssetGroup(assetId, nil, t.inputs, t.outputs, nil)
		if err != nil {
			return nil, err
		}
		groups = append(groups, *group)
	}

	if len(groups) == 0 {
		return nil, nil
	}

	return asset.NewPacket(groups)
}
