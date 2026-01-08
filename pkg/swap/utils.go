package swap

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ArkLabsHQ/fulmine/pkg/vhtlc"
	arklib "github.com/arkade-os/arkd/pkg/ark-lib"
	"github.com/arkade-os/arkd/pkg/ark-lib/intent"
	"github.com/arkade-os/arkd/pkg/ark-lib/script"
	"github.com/arkade-os/arkd/pkg/ark-lib/tree"
	"github.com/arkade-os/arkd/pkg/ark-lib/txutils"
	"github.com/arkade-os/go-sdk/client"
	"github.com/arkade-os/go-sdk/types"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/btcsuite/btcd/btcutil/psbt"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	"github.com/btcsuite/btcwallet/waddrmgr"
	"github.com/lightningnetwork/lnd/input"
	decodepay "github.com/nbd-wtf/ln-decodepay"
)

func checkpointExitScript(cfg types.Config) []byte {
	buf, _ := hex.DecodeString(cfg.CheckpointTapscript)
	return buf
}

// verifyInputSignatures checks that all inputs have a signature for the given pubkey
// and the signature is correct for the given tapscript leaf
func verifyInputSignatures(
	tx *psbt.Packet, pubkey *btcec.PublicKey, tapLeaves map[int]txscript.TapLeaf,
) error {
	xOnlyPubkey := schnorr.SerializePubKey(pubkey)

	prevouts := make(map[wire.OutPoint]*wire.TxOut)
	sigsToVerify := make(map[int]*psbt.TaprootScriptSpendSig)

	for inputIndex, input := range tx.Inputs {
		// collect previous outputs
		if input.WitnessUtxo == nil {
			return fmt.Errorf("input %d has no witness utxo, cannot verify signature", inputIndex)
		}

		outpoint := tx.UnsignedTx.TxIn[inputIndex].PreviousOutPoint
		prevouts[outpoint] = input.WitnessUtxo

		tapLeaf, ok := tapLeaves[inputIndex]
		if !ok {
			return fmt.Errorf(
				"input %d has no tapscript leaf, cannot verify signature", inputIndex,
			)
		}

		tapLeafHash := tapLeaf.TapHash()

		// check if pubkey has a tapscript sig
		hasSig := false
		for _, sig := range input.TaprootScriptSpendSig {
			if bytes.Equal(sig.XOnlyPubKey, xOnlyPubkey) &&
				bytes.Equal(sig.LeafHash, tapLeafHash[:]) {
				hasSig = true
				sigsToVerify[inputIndex] = sig
				break
			}
		}

		if !hasSig {
			return fmt.Errorf("input %d has no signature for pubkey %x", inputIndex, xOnlyPubkey)
		}
	}

	prevoutFetcher := txscript.NewMultiPrevOutFetcher(prevouts)
	txSigHashes := txscript.NewTxSigHashes(tx.UnsignedTx, prevoutFetcher)

	for inputIndex, sig := range sigsToVerify {
		msgHash, err := txscript.CalcTapscriptSignaturehash(
			txSigHashes,
			sig.SigHash,
			tx.UnsignedTx,
			inputIndex,
			prevoutFetcher,
			tapLeaves[inputIndex],
		)
		if err != nil {
			return fmt.Errorf("failed to calculate tapscript signature hash: %w", err)
		}

		signature, err := schnorr.ParseSignature(sig.Signature)
		if err != nil {
			return fmt.Errorf("failed to parse signature: %w", err)
		}

		if !signature.Verify(msgHash, pubkey) {
			return fmt.Errorf("input %d: invalid signature", inputIndex)
		}
	}

	return nil
}

// GetInputTapLeaves returns a map of input index to tapscript leaf
// if the input has no tapscript leaf, it is not included in the map
func getInputTapLeaves(tx *psbt.Packet) map[int]txscript.TapLeaf {
	tapLeaves := make(map[int]txscript.TapLeaf)
	for inputIndex, input := range tx.Inputs {
		if len(input.TaprootLeafScript) <= 0 {
			continue
		}
		tapLeaves[inputIndex] = txscript.NewBaseTapLeaf(input.TaprootLeafScript[0].Script)
	}
	return tapLeaves
}

func verifyAndSignCheckpoints(
	signedCheckpoints []string, myCheckpoints []*psbt.Packet,
	arkSigner *btcec.PublicKey, sign func(tx *psbt.Packet) (string, error),
) ([]string, error) {
	finalCheckpoints := make([]string, 0, len(signedCheckpoints))
	for _, checkpoint := range signedCheckpoints {
		signedCheckpointPtx, err := psbt.NewFromRawBytes(strings.NewReader(checkpoint), true)
		if err != nil {
			return nil, err
		}

		// search for the checkpoint tx we initially created
		var myCheckpointTx *psbt.Packet
		for _, chk := range myCheckpoints {
			if chk.UnsignedTx.TxID() == signedCheckpointPtx.UnsignedTx.TxID() {
				myCheckpointTx = chk
				break
			}
		}
		if myCheckpointTx == nil {
			return nil, fmt.Errorf("checkpoint tx not found")
		}

		// verify the server has signed the checkpoint tx
		if err := verifyInputSignatures(
			signedCheckpointPtx, arkSigner, getInputTapLeaves(myCheckpointTx),
		); err != nil {
			return nil, err
		}

		finalCheckpoint, err := sign(signedCheckpointPtx)
		if err != nil {
			return nil, fmt.Errorf("failed to sign checkpoint transaction: %w", err)
		}

		finalCheckpoints = append(finalCheckpoints, finalCheckpoint)
	}

	return finalCheckpoints, nil
}

func verifyFinalArkTx(
	finalArkTx string, arkSigner *btcec.PublicKey, expectedTapLeaves map[int]txscript.TapLeaf,
) error {
	finalArkPtx, err := psbt.NewFromRawBytes(strings.NewReader(finalArkTx), true)
	if err != nil {
		return err
	}

	// verify that the ark signer has signed the ark tx
	return verifyInputSignatures(finalArkPtx, arkSigner, expectedTapLeaves)
}

func offchainAddressPkScript(addr string) (string, error) {
	decodedAddress, err := arklib.DecodeAddressV0(addr)
	if err != nil {
		return "", fmt.Errorf("failed to decode address %s: %w", addr, err)
	}

	p2trScript, err := txscript.PayToTaprootScript(decodedAddress.VtxoTapKey)
	if err != nil {
		return "", fmt.Errorf("failed to parse address to p2tr script: %w", err)
	}
	return hex.EncodeToString(p2trScript), nil
}

func parseLocktime(locktime uint32) arklib.RelativeLocktime {
	if locktime >= 512 {
		return arklib.RelativeLocktime{Type: arklib.LocktimeTypeSecond, Value: locktime}
	}

	return arklib.RelativeLocktime{Type: arklib.LocktimeTypeBlock, Value: locktime}
}

func combineTapscripts(signedPackets []*psbt.Packet) (*psbt.Packet, error) {
	if len(signedPackets) <= 0 {
		return nil, errors.New("missing txs to combine")
	}
	if len(signedPackets) == 1 {
		return signedPackets[0], nil
	}

	finalCheckpoint := signedPackets[0]
	for i := range finalCheckpoint.Inputs {
		scriptSigs := make([]*psbt.TaprootScriptSpendSig, len(signedPackets))
		for j, signedCheckpointPsbt := range signedPackets {
			boltzIn := signedCheckpointPsbt.Inputs[i]
			scriptSigs[j] = boltzIn.TaprootScriptSpendSig[0]
		}
		finalCheckpoint.Inputs[i].TaprootScriptSpendSig = scriptSigs
	}
	return finalCheckpoint, nil
}

func verifySignatures(
	signedCheckpointTxs []*psbt.Packet, pubkeys []*btcec.PublicKey,
	expectedTapLeaves map[int]txscript.TapLeaf,
) error {
	for _, signedCheckpointTx := range signedCheckpointTxs {
		for _, signer := range pubkeys {
			// verify that the ark signer has signed the ark tx
			if err := verifyInputSignatures(
				signedCheckpointTx, signer, expectedTapLeaves,
			); err != nil {
				return err
			}
		}
	}
	return nil
}

func decodeInvoice(invoice string) (uint64, []byte, error) {
	bolt11, err := decodepay.Decodepay(invoice)
	if err != nil {
		return 0, nil, err
	}

	amount := uint64(bolt11.MSatoshi / 1000)
	preimageHash, err := hex.DecodeString(bolt11.PaymentHash)
	if err != nil {
		return 0, nil, err
	}

	return amount, input.Ripemd160H(preimageHash), nil
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

func retry(
	ctx context.Context, interval time.Duration, fn func(ctx context.Context) (bool, error),
) error {
	for {
		select {
		case <-ctx.Done():
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				return fmt.Errorf("timed out")
			}
			return ctx.Err()
		default:
			done, err := fn(ctx)
			if err != nil {
				return err
			}
			if done {
				return nil
			}
			<-time.After(interval)
		}
	}
}

func validatePreimage(preimage, expectedHash []byte) error {
	if len(preimage) != 32 {
		return fmt.Errorf("preimage must be 32 bytes, got %d", len(preimage))
	}

	buf := sha256.Sum256(preimage)
	preimageHash := input.Ripemd160H(buf[:])
	if !bytes.Equal(preimageHash, expectedHash) {
		return fmt.Errorf(
			"preimage hash mismatch: expected %x, got %x", expectedHash, preimageHash,
		)
	}

	return nil
}

func getEventTopics(vtxos []client.TapscriptsVtxo, signerPubkey string) []string {
	topics := make([]string, 0, len(vtxos)+1)
	for _, vtxo := range vtxos {
		topics = append(topics, vtxo.Vtxo.Outpoint.String())
	}
	topics = append(topics, signerPubkey)
	return topics
}

func getClaimIntent(
	session *batchSessionArgs, preimage []byte,
) (string, string, error) {
	vtxoScript, err := script.ParseVtxoScript(session.vhtlcScript.GetRevealedTapscripts())
	if err != nil {
		return "", "", fmt.Errorf("failed to parse vtxo script: %w", err)
	}

	forfeitClosures := vtxoScript.ForfeitClosures()
	if len(forfeitClosures) <= 0 {
		return "", "", fmt.Errorf("no forfeit closures found")
	}

	forfeitClosure, err := getClaimClosure(forfeitClosures)
	if err != nil {
		return "", "", err
	}

	vtxoLocktime, inputSequence := extractLocktimeAndSequence(forfeitClosure)

	claimTapscript, err := session.vhtlcScript.ClaimTapscript()
	if err != nil {
		return "", "", fmt.Errorf("failed to get claim tapscript for intent: %w", err)
	}

	inputs, tapLeaves, arkFields, err := getIntentInputs(
		session.vtxos, session.vhtlcScript, claimTapscript, inputSequence,
	)
	if err != nil {
		return "", "", err
	}

	receivers := []types.Receiver{{To: session.destinationAddr, Amount: session.totalAmount}}

	intentMessage, err := getIntentMessage(session.signerSession)
	if err != nil {
		return "", "", err
	}

	outputs, err := getIntentOutputs(receivers)
	if err != nil {
		return "", "", err
	}

	proof, err := intent.New(intentMessage, inputs, outputs)
	if err != nil {
		return "", "", fmt.Errorf("failed to build intent proof: %w", err)
	}
	proof.UnsignedTx.LockTime = uint32(vtxoLocktime)

	if err := addForfeitLeafProof(proof, session.vhtlcScript, forfeitClosure); err != nil {
		return "", "", err
	}

	for i := range inputs {
		proof.Inputs[i+1].Unknowns = arkFields[i]
		if tapLeaves[i] != nil {
			proof.Inputs[i+1].TaprootLeafScript = []*psbt.TaprootTapLeafScript{
				{
					ControlBlock: tapLeaves[i].ControlBlock,
					Script:       tapLeaves[i].Script,
					LeafVersion:  txscript.BaseLeafVersion,
				},
			}
		}
	}

	if err := txutils.SetArkPsbtField(
		&proof.Packet, 1, txutils.ConditionWitnessField, wire.TxWitness{preimage},
	); err != nil {
		return "", "", fmt.Errorf("failed to inject preimage into intent proof: %w", err)
	}

	encodedProof, err := proof.B64Encode()
	if err != nil {
		return "", "", fmt.Errorf("failed to encode proof for signing: %w", err)
	}

	return encodedProof, intentMessage, nil
}

func getRefundIntent(session *batchSessionArgs) (string, string, error) {
	vtxoScript, err := script.ParseVtxoScript(session.vhtlcScript.GetRevealedTapscripts())
	if err != nil {
		return "", "", fmt.Errorf("failed to parse vtxo script: %w", err)
	}

	forfeitClosures := vtxoScript.ForfeitClosures()
	if len(forfeitClosures) <= 0 {
		return "", "", fmt.Errorf("no forfeit closures found")
	}

	forfeitClosure, err := getRefundClosure(forfeitClosures)
	if err != nil {
		return "", "", err
	}

	vtxoLocktime, inputSequence := extractLocktimeAndSequence(forfeitClosure)

	refundTapscript, err := session.vhtlcScript.RefundTapscript(false)
	if err != nil {
		return "", "", fmt.Errorf("failed to get refund tapscript for intent: %w", err)
	}

	inputs, tapLeaves, arkFields, err := getIntentInputs(
		session.vtxos, session.vhtlcScript, refundTapscript, inputSequence,
	)
	if err != nil {
		return "", "", err
	}

	receivers := []types.Receiver{{To: session.destinationAddr, Amount: session.totalAmount}}

	intentMessage, err := getIntentMessage(session.signerSession)
	if err != nil {
		return "", "", err
	}

	outputs, err := getIntentOutputs(receivers)
	if err != nil {
		return "", "", err
	}

	proof, err := intent.New(intentMessage, inputs, outputs)
	if err != nil {
		return "", "", fmt.Errorf("failed to build intent proof: %w", err)
	}
	proof.UnsignedTx.LockTime = uint32(vtxoLocktime)

	if err := addForfeitLeafProof(proof, session.vhtlcScript, forfeitClosure); err != nil {
		return "", "", err
	}

	for i := range inputs {
		proof.Inputs[i+1].Unknowns = arkFields[i]
		if tapLeaves[i] != nil {
			proof.Inputs[i+1].TaprootLeafScript = []*psbt.TaprootTapLeafScript{
				{
					ControlBlock: tapLeaves[i].ControlBlock,
					Script:       tapLeaves[i].Script,
					LeafVersion:  txscript.BaseLeafVersion,
				},
			}
		}
	}

	encodedProof, err := proof.B64Encode()
	if err != nil {
		return "", "", fmt.Errorf("failed to encode proof for signing: %w", err)
	}

	return encodedProof, intentMessage, nil
}

func extractLocktimeAndSequence(closure script.Closure) (arklib.AbsoluteLocktime, uint32) {
	if cltv, ok := closure.(*script.CLTVMultisigClosure); ok {
		return cltv.Locktime, wire.MaxTxInSequenceNum - 1
	}
	return arklib.AbsoluteLocktime(0), wire.MaxTxInSequenceNum
}

// getClaimClosure returns the ConditionMultisigClosure from the list of closures
func getClaimClosure(forfeitClosures []script.Closure) (script.Closure, error) {
	for _, fc := range forfeitClosures {
		if _, ok := fc.(*script.ConditionMultisigClosure); ok {
			return fc, nil
		}
	}
	return nil, fmt.Errorf("ConditionMultisigClosure not found for claim path")
}

func getRefundClosure(forfeitClosures []script.Closure) (script.Closure, error) {
	// Refund path: find CLTVMultisigClosure (sweep closure)
	for _, fc := range forfeitClosures {
		if _, ok := fc.(*script.CLTVMultisigClosure); ok {
			return fc, nil
		}
	}
	return nil, fmt.Errorf("CLTVMultisigClosure not found for refund path")
}

func getIntentInputs(
	vtxos []client.TapscriptsVtxo, vhtlcScript *vhtlc.VHTLCScript,
	settlementTapscript *waddrmgr.Tapscript, inputSequence uint32,
) ([]intent.Input, []*arklib.TaprootMerkleProof, [][]*psbt.Unknown, error) {
	vhtlcTapKey, vhtlcTapTree, err := vhtlcScript.TapTree()
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to get VHTLC tap tree: %w", err)
	}

	inputs := make([]intent.Input, 0, len(vtxos))
	tapLeaves := make([]*arklib.TaprootMerkleProof, 0, len(vtxos))
	arkFields := make([][]*psbt.Unknown, 0, len(vtxos))

	for _, vtxo := range vtxos {
		vtxoTxHash, err := chainhash.NewHashFromStr(vtxo.Txid)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("invalid vtxo txid %s: %w", vtxo.Txid, err)
		}

		settlementTapscriptLeaf := txscript.NewBaseTapLeaf(settlementTapscript.RevealedScript)
		merkleProof, err := vhtlcTapTree.GetTaprootMerkleProof(settlementTapscriptLeaf.TapHash())
		if err != nil {
			return nil, nil, nil, fmt.Errorf("failed to get taproot merkle proof: %w", err)
		}

		pkScript, err := script.P2TRScript(vhtlcTapKey)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("failed to create P2TR script: %w", err)
		}

		inputs = append(inputs, intent.Input{
			OutPoint: &wire.OutPoint{
				Hash:  *vtxoTxHash,
				Index: vtxo.VOut,
			},
			Sequence: inputSequence,
			WitnessUtxo: &wire.TxOut{
				Value:    int64(vtxo.Amount),
				PkScript: pkScript,
			},
		})

		tapLeaves = append(tapLeaves, merkleProof)
		vhtlcTapscripts := vhtlcScript.GetRevealedTapscripts()
		taptreeField, err := txutils.VtxoTaprootTreeField.Encode(vhtlcTapscripts)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("failed to encode tapscripts: %w", err)
		}
		arkFields = append(arkFields, []*psbt.Unknown{taptreeField})
	}

	return inputs, tapLeaves, arkFields, nil
}

func getIntentOutputs(receivers []types.Receiver) ([]*wire.TxOut, error) {
	outputs := make([]*wire.TxOut, 0, len(receivers))
	for _, receiver := range receivers {
		decodedAddr, err := arklib.DecodeAddressV0(receiver.To)
		if err != nil {
			return nil, fmt.Errorf("failed to decode receiver address: %w", err)
		}

		pkScript, err := script.P2TRScript(decodedAddr.VtxoTapKey)
		if err != nil {
			return nil, fmt.Errorf("failed to create receiver pkScript: %w", err)
		}

		outputs = append(outputs, &wire.TxOut{
			Value:    int64(receiver.Amount),
			PkScript: pkScript,
		})
	}
	return outputs, nil
}

func getIntentMessage(signerSession tree.SignerSession) (string, error) {
	validAt := time.Now()
	intentMessage, err := intent.RegisterMessage{
		BaseMessage: intent.BaseMessage{
			Type: intent.IntentMessageTypeRegister,
		},
		ExpireAt:            validAt.Add(5 * time.Minute).Unix(),
		ValidAt:             validAt.Unix(),
		CosignersPublicKeys: []string{signerSession.GetPublicKey()},
	}.Encode()
	if err != nil {
		return "", fmt.Errorf("failed to encode intent message: %w", err)
	}
	return intentMessage, nil
}

func addForfeitLeafProof(
	proof *intent.Proof, vhtlcScript *vhtlc.VHTLCScript, forfeitClosure script.Closure,
) error {
	_, vhtlcTapTree, err := vhtlcScript.TapTree()
	if err != nil {
		return fmt.Errorf("failed to get VHTLC tap tree: %w", err)
	}

	forfeitScript, err := forfeitClosure.Script()
	if err != nil {
		return fmt.Errorf("failed to get forfeit script: %w", err)
	}

	forfeitLeaf := txscript.NewBaseTapLeaf(forfeitScript)
	leafProof, err := vhtlcTapTree.GetTaprootMerkleProof(forfeitLeaf.TapHash())
	if err != nil {
		return fmt.Errorf("failed to get forfeit merkle proof: %w", err)
	}

	if leafProof != nil {
		proof.Packet.Inputs[0].TaprootLeafScript = []*psbt.TaprootTapLeafScript{
			{
				ControlBlock: leafProof.ControlBlock,
				Script:       leafProof.Script,
				LeafVersion:  txscript.BaseLeafVersion,
			},
		}
	}

	return nil
}
