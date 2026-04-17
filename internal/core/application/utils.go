package application

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	arklib "github.com/arkade-os/arkd/pkg/ark-lib"
	"github.com/arkade-os/arkd/pkg/ark-lib/tree"
	"github.com/arkade-os/arkd/pkg/ark-lib/txutils"
	"github.com/arkade-os/arkd/pkg/client-lib/client"
	clientTypes "github.com/arkade-os/arkd/pkg/client-lib/types"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/btcsuite/btcd/btcutil/psbt"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	log "github.com/sirupsen/logrus"
)

func offchainAddressesPkScripts(addresses []string) ([]string, error) {
	scripts := make([]string, 0, len(addresses))
	for _, addr := range addresses {
		decodedAddress, err := arklib.DecodeAddressV0(addr)
		if err != nil {
			return nil, fmt.Errorf("failed to decode address %s: %w", addr, err)
		}

		p2trScript, err := txscript.PayToTaprootScript(decodedAddress.VtxoTapKey)
		if err != nil {
			return nil, fmt.Errorf("failed to parse address to p2tr script: %w", err)
		}

		scripts = append(scripts, hex.EncodeToString(p2trScript))
	}
	return scripts, nil
}

func parseLocktime(locktime uint32) arklib.RelativeLocktime {
	if locktime >= 512 {
		return arklib.RelativeLocktime{Type: arklib.LocktimeTypeSecond, Value: locktime}
	}

	return arklib.RelativeLocktime{Type: arklib.LocktimeTypeBlock, Value: locktime}
}

func parsePubkey(pubkey string) (*btcec.PublicKey, error) {
	if len(pubkey) <= 0 {
		return nil, nil
	}

	dec, err := hex.DecodeString(pubkey)
	if err != nil {
		return nil, fmt.Errorf("invalid pubkey: %s", err)
	}

	pk, err := btcec.ParsePubKey(dec)
	if err != nil {
		return nil, fmt.Errorf("invalid pubkey: %s", err)
	}

	return pk, nil
}

func signVtxoTree(
	event client.TreeSignatureEvent, txTree *tree.TxTree,
) error {
	if event.BatchIndex != 0 {
		return fmt.Errorf("batch index %d is not 0", event.BatchIndex)
	}

	decodedSig, err := hex.DecodeString(event.Signature)
	if err != nil {
		return fmt.Errorf("failed to decode signature: %s", err)
	}

	sig, err := schnorr.ParseSignature(decodedSig)
	if err != nil {
		return fmt.Errorf("failed to parse signature: %s", err)
	}

	return txTree.Apply(func(g *tree.TxTree) (bool, error) {
		if g.Root.UnsignedTx.TxID() != event.Txid {
			return true, nil
		}

		g.Root.Inputs[0].TaprootKeySpendSig = sig.Serialize()
		return false, nil
	})
}

func extractConnector(connectorTx *psbt.Packet) (*wire.TxOut, *wire.OutPoint, error) {
	for outIndex, output := range connectorTx.UnsignedTx.TxOut {
		if bytes.Equal(txutils.ANCHOR_PKSCRIPT, output.PkScript) {
			continue
		}

		return output, &wire.OutPoint{
			Hash:  connectorTx.UnsignedTx.TxHash(),
			Index: uint32(outIndex),
		}, nil
	}

	return nil, nil, fmt.Errorf("connector output not found")
}

// a wrapper around delegate task id
type registeredIntent struct {
	taskID   string
	intentID string
	inputs   []wire.OutPoint
}

func (i registeredIntent) intentIDHash() string {
	buf := sha256.Sum256([]byte(i.intentID))
	return hex.EncodeToString(buf[:])
}

func getSpentVtxosFromTransactionEvent(event client.TransactionEvent) []wire.OutPoint {
	spentVtxos := make([]clientTypes.Vtxo, 0)

	if event.CommitmentTx != nil {
		spentVtxos = append(spentVtxos, event.CommitmentTx.SpentVtxos...)
	}

	if event.ArkTx != nil {
		spentVtxos = append(spentVtxos, event.ArkTx.SpentVtxos...)
	}

	outpoints := make([]wire.OutPoint, 0, len(spentVtxos))
	for _, vtxo := range spentVtxos {
		hash, err := chainhash.NewHashFromStr(vtxo.Txid)
		if err != nil {
			log.WithError(err).Warnf("failed to parse vtxo txid %s", vtxo.Txid)
			continue
		}

		outpoints = append(outpoints, wire.OutPoint{
			Hash:  *hash,
			Index: vtxo.VOut,
		})
	}

	return outpoints
}
