package banco

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/hex"
	"fmt"

	introclient "github.com/ArkLabsHQ/introspector/pkg/client"
	arklib "github.com/arkade-os/arkd/pkg/ark-lib"
	"github.com/arkade-os/arkd/pkg/ark-lib/asset"
	"github.com/arkade-os/arkd/pkg/ark-lib/extension"
	"github.com/arkade-os/arkd/pkg/ark-lib/offchain"
	"github.com/arkade-os/arkd/pkg/ark-lib/script"
	"github.com/arkade-os/arkd/pkg/client-lib/client"
	"github.com/arkade-os/arkd/pkg/client-lib/indexer"
	clientTypes "github.com/arkade-os/arkd/pkg/client-lib/types"
	"github.com/arkade-os/arkd/pkg/client-lib/wallet"
	arksdk "github.com/arkade-os/go-sdk"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/btcsuite/btcd/btcutil/psbt"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	"github.com/btcsuite/btcwallet/waddrmgr"
)

// CreateOfferParams holds parameters for creating a banco swap offer.
type CreateOfferParams struct {
	WantAmount  uint64 // sats the maker wants to receive
	WantAsset   string // "txid:vout" for asset swaps, empty for BTC
	CancelAt uint64  // 0 = no cancel, TODO support cancel tapscript
	ExitDelay *arklib.RelativeLocktime
}

// CreateOfferResult contains the result of creating an offer.
type CreateOfferResult struct {
	OfferHex    string
	Packet      extension.Packet
	SwapAddress string
}

// OfferStatus represents the status of a VTXO at a swap address.
type OfferStatus struct {
	Txid      string
	VOut      uint32
	Value     uint64
	Spendable bool
}

// CreateOffer creates a new banco swap offer.
// Matches ts-sdk/src/banco/maker.ts Maker.createOffer().
func CreateOffer(
	ctx context.Context,
	params CreateOfferParams,
	transportClient client.TransportClient,
	introClient introclient.TransportClient,
	arkClient arksdk.ArkClient,
) (*CreateOfferResult, error) {
	// TODO cancel and exit needs a way to get the wallet public key
	if params.CancelAt > 0 {
		return nil, fmt.Errorf("cancel path not supported")
	}
	if params.ExitDelay != nil {
		return nil, fmt.Errorf("exit not supported")
	}


	// Get introspector pubkey
	introInfo, err := introClient.GetInfo(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get introspector info: %w", err)
	}

	introPubKeyBytes, err := hex.DecodeString(introInfo.SignerPublicKey)
	if err != nil {
		return nil, fmt.Errorf("failed to decode introspector pubkey: %w", err)
	}
	introspectorPubkey, err := btcec.ParsePubKey(introPubKeyBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse introspector pubkey: %w", err)
	}

	// Get maker address
	makerAddr, err := arkClient.NewOffchainAddress(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get maker address: %w", err)
	}
	decodedAddr, err := arklib.DecodeAddressV0(makerAddr)
	if err != nil {
		return nil, fmt.Errorf("failed to decode maker address: %w", err)
	}
	makerPkScript, err := script.P2TRScript(decodedAddr.VtxoTapKey)
	if err != nil {
		return nil, fmt.Errorf("failed to build maker pkscript: %w", err)
	}

	cfg, err := arkClient.GetConfigData(ctx)
	if err != nil {
		return nil, err
	}

	offer := &BancoOffer{
		WantAmount:         params.WantAmount,
		WantAsset:          params.WantAsset,
		CancelAt:        		params.CancelAt,
		MakerPkScript:      makerPkScript,
	}

	// Compute swap address
	vtxoScript, err := offer.VtxoScript(introspectorPubkey, cfg.SignerPubKey)
	if err != nil {
		return nil, fmt.Errorf("failed to build vtxo script: %w", err)
	}
	taprootKey, _, err := vtxoScript.TapTree()
	if err != nil {
		return nil, fmt.Errorf("failed to build taptree: %w", err)
	}

	swapAddr := &arklib.Address{
		HRP:        cfg.Network.Addr,
		Signer:     cfg.SignerPubKey,
		VtxoTapKey: taprootKey,
	}
	swapAddress, err := swapAddr.EncodeV0()
	if err != nil {
		return nil, fmt.Errorf("failed to encode swap address: %w", err)
	}

	swapPkScript, err := swapAddr.GetPkScript()
	if err != nil {
		return nil, err
	}

	offer.SwapPkScript = swapPkScript

	encodedOffer, err := offer.Serialize()
	if err != nil {
		return nil, fmt.Errorf("failed to encode offer: %w", err)
	}

	packet, err := offer.ToPacket()
	if err != nil {
		return nil, fmt.Errorf("failed to create packet: %w", err)
	}

	return &CreateOfferResult{
		OfferHex:    hex.EncodeToString(encodedOffer),
		Packet:      packet,
		SwapAddress: swapAddress,
	}, nil
}

// GetOffers queries VTXOs at a swap address to check offer status.
// Matches ts-sdk/src/banco/maker.ts Maker.getOffers().
func GetOffers(
	ctx context.Context,
	swapAddress string,
	indexerClient indexer.Indexer,
) ([]OfferStatus, error) {
	decoded, err := arklib.DecodeAddressV0(swapAddress)
	if err != nil {
		return nil, fmt.Errorf("failed to decode swap address: %w", err)
	}
	pkScript, err := decoded.GetPkScript()
	if err != nil {
		return nil, fmt.Errorf("failed to get pkscript: %w", err)
	}

	vtxosResp, err := indexerClient.GetVtxos(ctx,
		indexer.WithScripts([]string{hex.EncodeToString(pkScript)}),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get vtxos: %w", err)
	}

	statuses := make([]OfferStatus, 0, len(vtxosResp.Vtxos))
	for _, v := range vtxosResp.Vtxos {
		statuses = append(statuses, OfferStatus{
			Txid:      v.Txid,
			VOut:      v.VOut,
			Value:     v.Amount,
			Spendable: !v.Spent,
		})
	}
	return statuses, nil
}

// walletProvider is satisfied by the go-sdk ArkClient concrete type,
// which embeds the client-lib ArkClient (providing the Wallet method).
type walletProvider interface {
	Wallet() wallet.WalletService
}

// FundOffer sends funds to the swap address with the offer packet embedded
// in the extension output. This is required so the taker bot can discover
// the offer by inspecting the transaction stream.
// TODO FundOffer should disapear once sdk SendOffchain supports custom extension packets
func FundOffer(
	ctx context.Context,
	offerResult *CreateOfferResult,
	btcAmount uint64,
	assets []clientTypes.Asset,
	transportClient client.TransportClient,
	arkClient arksdk.ArkClient,
) (string, error) {
	// Get server info
	info, err := transportClient.GetInfo(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to get server info: %w", err)
	}

	checkpointTapscriptBytes, err := hex.DecodeString(info.CheckpointTapscript)
	if err != nil {
		return "", fmt.Errorf("failed to decode checkpoint tapscript: %w", err)
	}

	serverPubKeyBytes, err := hex.DecodeString(info.SignerPubKey)
	if err != nil {
		return "", fmt.Errorf("failed to decode signer pubkey: %w", err)
	}
	if len(serverPubKeyBytes) == 33 {
		serverPubKeyBytes = serverPubKeyBytes[1:]
	}
	serverPubKey, err := schnorr.ParsePubKey(serverPubKeyBytes)
	if err != nil {
		return "", fmt.Errorf("failed to parse signer pubkey: %w", err)
	}

	// Get swap address pkScript
	decoded, err := arklib.DecodeAddressV0(offerResult.SwapAddress)
	if err != nil {
		return "", fmt.Errorf("failed to decode swap address: %w", err)
	}
	swapPkScript, err := decoded.GetPkScript()
	if err != nil {
		return "", fmt.Errorf("failed to get swap pkscript: %w", err)
	}

	// Resolve spendable VTXOs with their tapscripts.
	// The Vtxo.Script field is a pkScript; the actual tapscript closures
	// come from the wallet's offchain addresses (same pattern as client-lib SendOffChain).
	wp, ok := arkClient.(walletProvider)
	if !ok {
		return "", fmt.Errorf("arkClient does not provide wallet access")
	}

	spendableVtxos, err := arkClient.ListSpendableVtxos(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to list spendable vtxos: %w", err)
	}
	if len(spendableVtxos) == 0 {
		return "", fmt.Errorf("no spendable VTXOs available")
	}

	_, offchainAddrs, _, _, err := wp.Wallet().GetAddresses(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to get offchain addresses: %w", err)
	}

	network := networkFromName(info.Network)

	// Match VTXOs to their offchain addresses to get tapscripts
	selectedVtxos := make([]clientTypes.VtxoWithTapTree, 0, len(spendableVtxos))
	for _, addr := range offchainAddrs {
		for _, v := range spendableVtxos {
			vtxoAddr, err := v.Address(serverPubKey, network)
			if err != nil {
				continue
			}
			if vtxoAddr == addr.Address {
				selectedVtxos = append(selectedVtxos, clientTypes.VtxoWithTapTree{
					Vtxo:       v,
					Tapscripts: addr.Tapscripts,
				})
			}
		}
	}
	if len(selectedVtxos) == 0 {
		return "", fmt.Errorf("no VTXOs matched to offchain addresses")
	}

	var totalBtc uint64
	for _, v := range selectedVtxos {
		totalBtc += v.Amount
	}
	if totalBtc < btcAmount {
		return "", fmt.Errorf("insufficient BTC: have %d, need %d", totalBtc, btcAmount)
	}

	// Get maker's change address
	changeAddr, err := arkClient.NewOffchainAddress(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to get change address: %w", err)
	}
	changeDecodedAddr, err := arklib.DecodeAddressV0(changeAddr)
	if err != nil {
		return "", fmt.Errorf("failed to decode change address: %w", err)
	}
	changePkScript, err := script.P2TRScript(changeDecodedAddr.VtxoTapKey)
	if err != nil {
		return "", fmt.Errorf("failed to build change pkscript: %w", err)
	}

	// Build outputs
	outputs := []*wire.TxOut{
		{Value: int64(btcAmount), PkScript: swapPkScript},
	}

	btcChange := int64(totalBtc) - int64(btcAmount)
	if btcChange > 0 {
		outputs = append(outputs, &wire.TxOut{Value: btcChange, PkScript: changePkScript})
	}

	// Build extension with the offer packet (and asset packet if assets involved)
	ext := extension.Extension{offerResult.Packet}

	if len(assets) > 0 {
		assetPacket, err := buildAssetPacket(selectedVtxos, assets, btcChange > 0)
		if err != nil {
			return "", fmt.Errorf("failed to build asset packet: %w", err)
		}
		if assetPacket != nil {
			ext = append(ext, assetPacket)
		}
	}

	extTxOut, err := ext.TxOut()
	if err != nil {
		return "", fmt.Errorf("failed to build extension output: %w", err)
	}
	outputs = append(outputs, extTxOut)

	// Build inputs from VTXOs using tapscripts for forfeit closures
	vtxoInputs := make([]offchain.VtxoInput, 0, len(selectedVtxos))
	for _, v := range selectedVtxos {
		vHash, err := chainhash.NewHashFromStr(v.Txid)
		if err != nil {
			return "", fmt.Errorf("failed to parse vtxo txid: %w", err)
		}

		vtxoScripts, err := script.ParseVtxoScript(v.Tapscripts)
		if err != nil {
			return "", fmt.Errorf("failed to parse vtxo script: %w", err)
		}

		tapscriptScript, ok := vtxoScripts.(*script.TapscriptsVtxoScript)
		if !ok {
			return "", fmt.Errorf("unexpected vtxo script type")
		}

		forfeitClosures := tapscriptScript.ForfeitClosures()
		if len(forfeitClosures) == 0 {
			return "", fmt.Errorf("vtxo has no forfeit closures")
		}

		forfeitClosure := forfeitClosures[0]
		forfeitScript, err := forfeitClosure.Script()
		if err != nil {
			return "", fmt.Errorf("failed to build forfeit script: %w", err)
		}

		_, tapTree, err := tapscriptScript.TapTree()
		if err != nil {
			return "", fmt.Errorf("failed to build taptree: %w", err)
		}

		forfeitLeafHash := txscript.NewBaseTapLeaf(forfeitScript).TapHash()
		forfeitMerkleProof, err := tapTree.GetTaprootMerkleProof(forfeitLeafHash)
		if err != nil {
			return "", fmt.Errorf("failed to get forfeit merkle proof: %w", err)
		}
		forfeitControlBlock, err := txscript.ParseControlBlock(forfeitMerkleProof.ControlBlock)
		if err != nil {
			return "", fmt.Errorf("failed to parse forfeit control block: %w", err)
		}

		revealedScripts, err := tapscriptScript.Encode()
		if err != nil {
			return "", fmt.Errorf("failed to encode vtxo tapscripts: %w", err)
		}

		vtxoInputs = append(vtxoInputs, offchain.VtxoInput{
			Outpoint:           &wire.OutPoint{Hash: *vHash, Index: v.VOut},
			Amount:             int64(v.Amount),
			Tapscript:          &waddrmgr.Tapscript{ControlBlock: forfeitControlBlock, RevealedScript: forfeitMerkleProof.Script},
			RevealedTapscripts: revealedScripts,
		})
	}

	// Build offchain tx
	arkTx, checkpoints, err := offchain.BuildTxs(vtxoInputs, outputs, checkpointTapscriptBytes)
	if err != nil {
		return "", fmt.Errorf("failed to build offchain txs: %w", err)
	}

	// Sign
	signedArkTxB64, err := arkTx.B64Encode()
	if err != nil {
		return "", fmt.Errorf("failed to encode ark tx: %w", err)
	}
	signedArkTxB64, err = arkClient.SignTransaction(ctx, signedArkTxB64)
	if err != nil {
		return "", fmt.Errorf("failed to sign ark tx: %w", err)
	}

	checkpointB64s := make([]string, 0, len(checkpoints))
	for _, cp := range checkpoints {
		cpB64, err := cp.B64Encode()
		if err != nil {
			return "", fmt.Errorf("failed to encode checkpoint: %w", err)
		}
		checkpointB64s = append(checkpointB64s, cpB64)
	}

	// Submit to server
	arkTxid, _, serverSignedCheckpoints, err := transportClient.SubmitTx(ctx, signedArkTxB64, checkpointB64s)
	if err != nil {
		return "", fmt.Errorf("ark server submission failed: %w", err)
	}

	// Counter-sign checkpoints
	finalCheckpoints := make([]string, 0, len(serverSignedCheckpoints))
	for i, serverCpB64 := range serverSignedCheckpoints {
		serverCpBytes, err := base64.StdEncoding.DecodeString(serverCpB64)
		if err != nil {
			return "", fmt.Errorf("failed to decode server checkpoint %d: %w", i, err)
		}
		serverCp, err := psbt.NewFromRawBytes(bytes.NewReader(serverCpBytes), false)
		if err != nil {
			return "", fmt.Errorf("failed to parse server checkpoint %d: %w", i, err)
		}
		cpB64, err := serverCp.B64Encode()
		if err != nil {
			return "", fmt.Errorf("failed to encode checkpoint %d: %w", i, err)
		}
		signedCpB64, err := arkClient.SignTransaction(ctx, cpB64)
		if err != nil {
			return "", fmt.Errorf("failed to sign checkpoint %d: %w", i, err)
		}
		finalCheckpoints = append(finalCheckpoints, signedCpB64)
	}

	// Finalize
	if err := transportClient.FinalizeTx(ctx, arkTxid, finalCheckpoints); err != nil {
		return "", fmt.Errorf("finalization failed: %w", err)
	}

	return arkTxid, nil
}

func networkFromName(name string) arklib.Network {
	switch name {
	case "bitcoin", "mainnet":
		return arklib.Bitcoin
	case "testnet":
		return arklib.BitcoinTestNet
	case "regtest":
		return arklib.BitcoinRegTest
	case "signet":
		return arklib.BitcoinSigNet
	case "mutinynet":
		return arklib.BitcoinMutinyNet
	default:
		return arklib.BitcoinRegTest
	}
}

// buildAssetPacket creates an extension asset packet for assets being sent from
// the maker's VTXOs to the swap address (output index 0).
// hasChange indicates whether there's a BTC change output at index 1.
func buildAssetPacket(
	vtxos []clientTypes.VtxoWithTapTree,
	sendAssets []clientTypes.Asset,
	hasChange bool,
) (asset.Packet, error) {
	type assetTransfer struct {
		inputs  []asset.AssetInput
		outputs []asset.AssetOutput
	}

	transfers := make(map[string]*assetTransfer)

	// Build inputs from VTXOs that contain assets
	for inputIdx, v := range vtxos {
		for _, a := range v.Assets {
			if _, exists := transfers[a.AssetId]; !exists {
				transfers[a.AssetId] = &assetTransfer{}
			}
			input, err := asset.NewAssetInput(uint16(inputIdx), a.Amount)
			if err != nil {
				return nil, err
			}
			transfers[a.AssetId].inputs = append(transfers[a.AssetId].inputs, *input)
		}
	}

	// Build outputs: assets go to output 0 (swap address)
	for _, a := range sendAssets {
		if _, exists := transfers[a.AssetId]; !exists {
			return nil, fmt.Errorf("asset %s not found in inputs", a.AssetId)
		}
		output, err := asset.NewAssetOutput(0, a.Amount)
		if err != nil {
			return nil, err
		}
		transfers[a.AssetId].outputs = append(transfers[a.AssetId].outputs, *output)
	}

	// Asset change goes to the change output (index 1) if present
	if hasChange {
		for assetId, t := range transfers {
			var totalIn, totalOut uint64
			for _, in := range t.inputs {
				totalIn += in.Amount
			}
			for _, out := range t.outputs {
				totalOut += out.Amount
			}
			if totalIn > totalOut {
				changeOutput, err := asset.NewAssetOutput(1, totalIn-totalOut)
				if err != nil {
					return nil, err
				}
				transfers[assetId].outputs = append(transfers[assetId].outputs, *changeOutput)
			}
		}
	}

	groups := make([]asset.AssetGroup, 0, len(transfers))
	for assetIdStr, t := range transfers {
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
