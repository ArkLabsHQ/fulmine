package application

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	arklib "github.com/arkade-os/arkd/pkg/ark-lib"
	"github.com/arkade-os/arkd/pkg/ark-lib/intent"
	"github.com/arkade-os/arkd/pkg/ark-lib/script"
	"github.com/arkade-os/arkd/pkg/ark-lib/tree"
	"github.com/arkade-os/arkd/pkg/ark-lib/txutils"
	"github.com/arkade-os/arkd/pkg/client-lib/client"
	clientTypes "github.com/arkade-os/arkd/pkg/client-lib/types"
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

type pendingTxIntentInput struct {
	Vtxo             clientTypes.VtxoWithTapTree
	Closure          script.Closure
	Sequence         uint32
	ConditionWitness wire.TxWitness
}

func getPendingTxIntent(inputsData []pendingTxIntentInput, locktime uint32) (string, string, error) {
	if len(inputsData) == 0 {
		return "", "", fmt.Errorf("missing pending vtxos")
	}

	inputs := make([]intent.Input, 0, len(inputsData))
	leafProofs := make([]*arklib.TaprootMerkleProof, 0, len(inputsData))
	arkFields := make([][]*psbt.Unknown, 0, len(inputsData))

	for _, inputData := range inputsData {
		hash, err := chainhash.NewHashFromStr(inputData.Vtxo.Txid)
		if err != nil {
			return "", "", err
		}

		pkScript, leafProof, err := extractTaprootLeaf(inputData.Vtxo.Tapscripts, inputData.Closure)
		if err != nil {
			return "", "", err
		}

		taptreeField, err := txutils.VtxoTaprootTreeField.Encode(inputData.Vtxo.Tapscripts)
		if err != nil {
			return "", "", err
		}

		inputs = append(inputs, intent.Input{
			OutPoint: wire.NewOutPoint(hash, inputData.Vtxo.VOut),
			Sequence: inputData.Sequence,
			WitnessUtxo: &wire.TxOut{
				Value:    int64(inputData.Vtxo.Amount),
				PkScript: pkScript,
			},
		})
		leafProofs = append(leafProofs, leafProof)
		arkFields = append(arkFields, []*psbt.Unknown{taptreeField})
	}

	message, err := intent.GetPendingTxMessage{
		BaseMessage: intent.BaseMessage{
			Type: intent.IntentMessageTypeGetPendingTx,
		},
		ExpireAt: time.Now().Add(10 * time.Minute).Unix(),
	}.Encode()
	if err != nil {
		return "", "", err
	}

	proof, err := intent.New(message, inputs, nil)
	if err != nil {
		return "", "", err
	}

	proof.UnsignedTx.LockTime = locktime

	for i, input := range proof.Inputs {
		var leafProof *arklib.TaprootMerkleProof
		if i == 0 {
			leafProof = leafProofs[0]
		} else {
			leafProof = leafProofs[i-1]
			input.Unknowns = arkFields[i-1]
		}

		input.TaprootLeafScript = []*psbt.TaprootTapLeafScript{{
			ControlBlock: leafProof.ControlBlock,
			Script:       leafProof.Script,
			LeafVersion:  txscript.BaseLeafVersion,
		}}
		proof.Inputs[i] = input
	}

	for i, inputData := range inputsData {
		if len(inputData.ConditionWitness) == 0 {
			continue
		}

		if err := txutils.SetArkPsbtField(
			&proof.Packet, i+1, txutils.ConditionWitnessField, inputData.ConditionWitness,
		); err != nil {
			return "", "", err
		}
	}

	encodedProof, err := proof.B64Encode()
	if err != nil {
		return "", "", err
	}

	return encodedProof, message, nil
}

func extractTaprootLeaf(
	tapscripts []string, closure script.Closure,
) ([]byte, *arklib.TaprootMerkleProof, error) {
	vtxoScript, err := script.ParseVtxoScript(tapscripts)
	if err != nil {
		return nil, nil, err
	}

	leafScript, err := closure.Script()
	if err != nil {
		return nil, nil, err
	}

	taprootKey, taprootTree, err := vtxoScript.TapTree()
	if err != nil {
		return nil, nil, err
	}

	tapLeaf := txscript.NewBaseTapLeaf(leafScript)
	leafProof, err := taprootTree.GetTaprootMerkleProof(tapLeaf.TapHash())
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get taproot merkle proof: %w", err)
	}

	pkScript, err := script.P2TRScript(taprootKey)
	if err != nil {
		return nil, nil, err
	}

	return pkScript, leafProof, nil
}
