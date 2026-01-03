package swap

import (
	"bytes"
	"crypto/sha256"
	"fmt"
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
	"github.com/btcsuite/btcd/btcutil/psbt"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	"github.com/btcsuite/btcwallet/waddrmgr"
	"github.com/lightningnetwork/lnd/input"
)

// validatePreimage validates a preimage against its expected hash.
// It checks both the length (must be 32 bytes) and that the hash matches.
//
// Returns an error if:
//   - preimage is not 32 bytes
//   - hash(preimage) does not match expectedHash
func validatePreimage(preimage, expectedHash []byte) error {
	if len(preimage) != 32 {
		return fmt.Errorf("preimage must be 32 bytes, got %d", len(preimage))
	}

	buf := sha256.Sum256(preimage)
	preimageHash := input.Ripemd160H(buf[:])
	if !bytes.Equal(preimageHash, expectedHash) {
		return fmt.Errorf("preimage hash mismatch: expected %x, got %x",
			expectedHash, preimageHash)
	}

	return nil
}

// buildEventTopics constructs the event topics for subscribing to batch session events.
//
// The topics include:
//   - intentID: The unique intent identifier
//   - VTXO outpoints: For each VTXO, topic format is "txid:vout"
//   - Signer pubkey: The ephemeral public key used for musig2 signing
//
// This function is used by both claim and refund settlement flows to subscribe
// to the correct event stream from arkd.
func buildEventTopics(intentID string, vtxos []types.Vtxo, signerPubkey string) []string {
	topics := []string{intentID}
	for _, vtxo := range vtxos {
		topics = append(topics, fmt.Sprintf("%s:%d", vtxo.Outpoint.Txid, vtxo.Outpoint.VOut))
	}
	topics = append(topics, signerPubkey)
	return topics
}

// selectForfeitClosure selects the appropriate forfeit closure based on settlement path.
//
// For claim path (isClaimPath=true):
//   - Returns ConditionMultisigClosure (no locktime, preimage-gated)
//
// For refund path (isClaimPath=false):
//   - If withReceiver=true: Returns RefundClosure (3-of-3 multisig: Sender+Receiver+Server)
//   - If withReceiver=false: Returns RefundWithoutReceiverClosure (2-of-2 multisig: Sender+Server, CLTV-gated)
//
// This function encapsulates the forfeit closure selection logic that appears in multiple
// places in batch_handler.go and swap.go.
func selectForfeitClosure(vhtlcScript *vhtlc.VHTLCScript, isClaimPath, withReceiver bool) script.Closure {
	if isClaimPath {
		// Claim path always uses ConditionMultisigClosure
		return vhtlcScript.ClaimClosure
	}

	// Refund path: select based on receiver participation
	if withReceiver {
		return vhtlcScript.RefundClosure
	}
	return vhtlcScript.RefundWithoutReceiverClosure
}

// extractLocktimeAndSequence extracts the locktime and sequence values from a closure.
//
// For closures with CLTV (time-locked):
//   - CLTVMultisigClosure: Returns (closure.Locktime, 0xFFFFFFFE)
//
// For closures without locktime:
//   - MultisigClosure: Returns (0, 0xFFFFFFFF)
//   - ConditionMultisigClosure: Returns (0, 0xFFFFFFFF)
//
// The sequence value must be 0xFFFFFFFE (MaxTxInSequenceNum-1) for CLTV validation to work.
// For non-CLTV closures, sequence is 0xFFFFFFFF (MaxTxInSequenceNum).
func extractLocktimeAndSequence(closure script.Closure) (arklib.AbsoluteLocktime, uint32) {
	if cltv, ok := closure.(*script.CLTVMultisigClosure); ok {
		return cltv.Locktime, wire.MaxTxInSequenceNum - 1
	}
	return arklib.AbsoluteLocktime(0), wire.MaxTxInSequenceNum
}

// selectForfeitClosureFromScript selects the appropriate forfeit closure from parsed VTXO script.
//
// For claim path (isClaimPath=true):
//   - Returns ConditionMultisigClosure (preimage-gated, no locktime)
//
// For refund path (isClaimPath=false):
//   - Returns CLTVMultisigClosure (CLTV-gated, has locktime)
//
// This is used during intent building to select the correct forfeit closure for the settlement type.
func selectForfeitClosureFromScript(forfeitClosures []script.Closure, isClaimPath bool) (script.Closure, error) {
	if isClaimPath {
		// Claim path: find ConditionMultisigClosure
		for _, fc := range forfeitClosures {
			if _, ok := fc.(*script.ConditionMultisigClosure); ok {
				return fc, nil
			}
		}
		return nil, fmt.Errorf("ConditionMultisigClosure not found for claim path")
	}

	// Refund path: find CLTVMultisigClosure (sweep closure)
	for _, fc := range forfeitClosures {
		if _, ok := fc.(*script.CLTVMultisigClosure); ok {
			return fc, nil
		}
	}
	return nil, fmt.Errorf("CLTVMultisigClosure not found for refund path")
}

// buildIntentInputs creates intent inputs with VHTLC tapscripts and merkle proofs.
//
// For each VTXO:
//   - Creates intent.Input with outpoint and witness UTXO
//   - Sets sequence based on locktime (0xFFFFFFFE for CLTV, 0xFFFFFFFF otherwise)
//   - Generates merkle proof for settlement path
//   - Encodes VHTLC tapscripts in custom PSBT field
//
// Returns:
//   - inputs: Intent inputs for the transaction
//   - tapLeaves: Merkle proofs for each input (for signing)
//   - arkFields: Custom PSBT fields with VHTLC tapscripts
func buildIntentInputs(
	vtxos []types.Vtxo,
	vhtlcScript *vhtlc.VHTLCScript,
	settlementTapscript *waddrmgr.Tapscript,
	inputSequence uint32,
) ([]intent.Input, []*arklib.TaprootMerkleProof, [][]*psbt.Unknown, error) {
	// Get VHTLC taproot key and tree
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

		// Get merkle proof for settlement path (claim or refund closure)
		settlementTapscriptLeaf := txscript.NewBaseTapLeaf(settlementTapscript.RevealedScript)
		merkleProof, err := vhtlcTapTree.GetTaprootMerkleProof(settlementTapscriptLeaf.TapHash())
		if err != nil {
			return nil, nil, nil, fmt.Errorf("failed to get taproot merkle proof: %w", err)
		}

		// Use INTERNAL taproot key directly (NOT tweaked)
		pkScript, err := script.P2TRScript(vhtlcTapKey)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("failed to create P2TR script: %w", err)
		}

		// Create intent.Input with VHTLC witness utxo and locktime-aware sequence
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

		// Add merkle proof for signing (exit path, not settlement path)
		tapLeaves = append(tapLeaves, merkleProof)

		// Encode VHTLC tapscripts in custom PSBT field
		vhtlcTapscripts := vhtlcScript.GetRevealedTapscripts()
		taptreeField, err := txutils.VtxoTaprootTreeField.Encode(vhtlcTapscripts)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("failed to encode tapscripts: %w", err)
		}
		arkFields = append(arkFields, []*psbt.Unknown{taptreeField})
	}

	return inputs, tapLeaves, arkFields, nil
}

// createIntentMessage creates a RegisterMessage for intent registration.
//
// The message includes:
//   - Type: IntentMessageTypeRegister
//   - ExpireAt: 5 minutes from now
//   - ValidAt: Current time
//   - CosignersPublicKeys: Ephemeral signer pubkey for musig2
//
// Returns the encoded intent message ready for proof construction.
func createIntentMessage(signerSession tree.SignerSession) (string, error) {
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

// buildIntentOutputs converts receivers to TxOut format for intent proof.
//
// For each receiver:
//   - Decodes the Ark offchain address
//   - Extracts the VtxoTapKey
//   - Creates P2TR pkScript
//   - Creates wire.TxOut with amount and pkScript
//
// Returns the list of outputs ready for intent proof construction.
func buildIntentOutputs(receivers []types.Receiver) ([]*wire.TxOut, error) {
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

// addForfeitLeafProof adds the forfeit closure merkle proof to the intent proof.
//
// This function:
//   - Generates the forfeit tapscript from the closure
//   - Computes the tapscript leaf
//   - Gets the merkle proof from the VHTLC tap tree
//   - Adds the proof to input 0 (the BIP-322 toSpend input)
//
// The forfeit proof is needed for the server to construct the forfeit transaction
// during the batch session.
func addForfeitLeafProof(
	proof *intent.Proof,
	vhtlcScript *vhtlc.VHTLCScript,
	forfeitClosure script.Closure,
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

// ForfeitBuilder is a strategy interface for building forfeits.
// Different implementations handle claim vs refund forfeit building logic.
type ForfeitBuilder interface {
	// BuildForfeit prepares a PSBT forfeit transaction before signing.
	// It may inject additional witness data (e.g., preimage).
	BuildForfeit(forfeitPtx *psbt.Packet) error

	// GetSettlementClosure returns the closure to use for the settlement path.
	GetSettlementClosure(vhtlcScript *vhtlc.VHTLCScript) script.Closure
}

// ClaimForfeitBuilder implements ForfeitBuilder for VHTLC claim path.
// It injects the preimage into the forfeit witness before signing.
type ClaimForfeitBuilder struct {
	preimage []byte
}

// BuildForfeit injects the preimage into the forfeit transaction.
func (b *ClaimForfeitBuilder) BuildForfeit(forfeitPtx *psbt.Packet) error {
	// CRITICAL: Inject preimage BEFORE signing (claim path requirement)
	if err := txutils.SetArkPsbtField(
		forfeitPtx, 0, txutils.ConditionWitnessField, wire.TxWitness{b.preimage},
	); err != nil {
		return fmt.Errorf("failed to inject preimage: %w", err)
	}
	return nil
}

// GetSettlementClosure returns the claim closure for claim path.
func (b *ClaimForfeitBuilder) GetSettlementClosure(vhtlcScript *vhtlc.VHTLCScript) script.Closure {
	return vhtlcScript.ClaimClosure
}

// RefundForfeitBuilder implements ForfeitBuilder for VHTLC refund path.
// It selects the appropriate refund closure (with or without receiver).
type RefundForfeitBuilder struct {
	withReceiver bool
}

// BuildForfeit does nothing for refund path (no preimage injection needed).
func (b *RefundForfeitBuilder) BuildForfeit(_ *psbt.Packet) error {
	// No preimage injection for refund path
	return nil
}

// GetSettlementClosure returns the appropriate refund closure based on receiver participation.
func (b *RefundForfeitBuilder) GetSettlementClosure(vhtlcScript *vhtlc.VHTLCScript) script.Closure {
	if b.withReceiver {
		return vhtlcScript.RefundClosure
	}
	return vhtlcScript.RefundWithoutReceiverClosure
}

// extractConnector extracts the connector output from a connector PSBT.
// It skips anchor outputs and returns the first non-anchor output.
//
// Returns:
//   - connector: The connector TxOut
//   - connectorOutpoint: The outpoint referencing the connector
//   - error: If no connector is found
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

// buildForfeitTransaction creates a PSBT forfeit transaction with the given parameters.
//
// This helper encapsulates the common logic for building forfeit transactions,
// including:
//   - Creating VTXO input and prevout
//   - Building forfeit transaction using tree.BuildForfeitTx
//   - Setting the settlement tapscript for the VTXO input
//
// Returns the constructed PSBT forfeit transaction ready for strategy-specific modifications.
func buildForfeitTransaction(
	vtxo client.TapscriptsVtxo,
	vtxoTapKey *btcec.PublicKey,
	settlementTapscript *psbt.TaprootTapLeafScript,
	connector *wire.TxOut,
	connectorOutpoint *wire.OutPoint,
	vtxoLocktime arklib.AbsoluteLocktime,
	vtxoSequence uint32,
	forfeitPkScript []byte,
) (*psbt.Packet, error) {
	// Build VTXO output script
	vtxoOutputScript, err := script.P2TRScript(vtxoTapKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create P2TR script: %w", err)
	}

	// Parse VTXO txid
	vtxoTxHash, err := chainhash.NewHashFromStr(vtxo.Txid)
	if err != nil {
		return nil, fmt.Errorf("invalid vtxo txid: %w", err)
	}

	// Create VTXO input
	vtxoInput := &wire.OutPoint{
		Hash:  *vtxoTxHash,
		Index: vtxo.VOut,
	}

	vtxoPrevout := &wire.TxOut{
		Value:    int64(vtxo.Amount),
		PkScript: vtxoOutputScript,
	}

	// Build forfeit transaction using tree.BuildForfeitTx
	forfeitPtx, err := tree.BuildForfeitTx(
		[]*wire.OutPoint{vtxoInput, connectorOutpoint},
		[]uint32{vtxoSequence, wire.MaxTxInSequenceNum},
		[]*wire.TxOut{vtxoPrevout, connector},
		forfeitPkScript,
		uint32(vtxoLocktime),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to build forfeit tx: %w", err)
	}

	// Set tapscript for VTXO input (settlement path)
	forfeitPtx.Inputs[0].TaprootLeafScript = []*psbt.TaprootTapLeafScript{settlementTapscript}

	return forfeitPtx, nil
}
