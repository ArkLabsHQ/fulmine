package application

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/ArkLabsHQ/fulmine/internal/core/domain"
	"github.com/ArkLabsHQ/fulmine/pkg/vhtlc"
	"github.com/arkade-os/arkd/pkg/ark-lib/txutils"
	"github.com/arkade-os/arkd/pkg/client-lib/indexer"
	clientTypes "github.com/arkade-os/arkd/pkg/client-lib/types"
	"github.com/btcsuite/btcd/btcutil/psbt"
	"github.com/btcsuite/btcd/wire"
	log "github.com/sirupsen/logrus"
)

type EventType int

const (
	EventTypeUnspecified EventType = iota
	EventTypeVhtlcCreated
	EventTypeVhtlcFunded
	EventTypeVhtlcClaimed
	EventTypeVhtlcRefunded
	EventTypeVhtlcSpent
)

// VhtlcEvent represents an event emitted by Fulmine for VHTLC lifecycle changes
type VhtlcEvent struct {
	ID        string
	Txid      string
	Preimage  string // hex-encoded, only for CLAIMED events
	Type      EventType
	Timestamp time.Time
}

type vhtlcSpendDetails struct {
	eventType EventType
	preimage  string
}

func (s *Service) emitEvent(event VhtlcEvent) {
	// Event delivery is best-effort. A slow or absent stream consumer must not
	// block wallet/indexer processing.
	select {
	case s.events <- event:
	default:
		log.Warnf("dropped vhtlc event %d for %s: no listener ready", event.Type, event.ID)
	}
}

// handleVhtlcScriptEvent translates indexer script events into VHTLC lifecycle
// events. A VHTLC is recognized by matching the script from the indexer event
// against the locking script derived from the stored VHTLC options.
func (s *Service) handleVhtlcScriptEvent(event indexer.ScriptEvent) {
	ctx := context.Background()

	if event.Data == nil {
		log.Debug("received nil or empty script event")
		return
	}

	data := event.Data

	log.Debugf(
		"received vhtlc script event with %d spent vtxos and %d new vtxos in tx %s",
		len(data.SpentVtxos), len(data.NewVtxos), data.Txid,
	)

	now := time.Now()

	// new VTXOs.
	//
	// If the indexer reports a new VTXO whose script matches a tracked VHTLC
	// script, the VHTLC has been funded. The same indexer event can contain
	// several VTXOs and several matching scripts, so we collapse scripts and
	// VHTLC records before emitting one FUNDED event per VHTLC id.
	fundedScripts := uniqueVhtlcScripts(data.NewVtxos)
	if len(fundedScripts) > 0 {
		vhtlcs, err := s.dbSvc.VHTLC().GetByScripts(ctx, fundedScripts)
		if err != nil {
			log.WithError(err).Debug("failed to get funded vhtlcs for scripts")
		} else {
			for _, v := range uniqueVhtlcs(vhtlcs) {
				s.emitEvent(VhtlcEvent{
					ID:        v.Id,
					Txid:      data.Txid,
					Type:      EventTypeVhtlcFunded,
					Timestamp: now,
				})
			}
		}
	}

	// spent VTXOs.
	//
	// If no tracked VHTLC script was spent, there is no terminal VHTLC event to
	// publish and no tracking state to change. A funding-only event is handled
	// above and returns here.
	spentScripts := uniqueVhtlcScripts(data.SpentVtxos)
	if len(spentScripts) == 0 {
		return
	}

	// Resolve spent scripts back to stored VHTLC records. From this point on,
	// each matching VHTLC is terminal: we will emit CLAIMED, REFUNDED, or SPENT
	// and then stop tracking the script.
	spentVhtlcs, err := s.dbSvc.VHTLC().GetByScripts(ctx, spentScripts)
	if err != nil {
		log.WithError(err).Debug("failed to get spent vhtlcs for scripts")
		return
	}

	for _, v := range uniqueVhtlcs(spentVhtlcs) {
		// direct spend.
		//
		// Direct claim/refund transactions carry the executed VHTLC leaf either
		// in PSBT fields or in the finalized witness. That leaf tells us whether
		// the spend used a claim or refund closure.
		details, err := classifyVhtlcSpend(data.Tx, data.SpentVtxos, v)

		// settled batch spend.
		//
		// Ark settlement batches can spend the VHTLC through a commitment tx, so
		// the script event's tx may not contain the VHTLC leaf itself. When the
		// indexer marks a candidate VTXO as SettledBy, fetch the batch forfeit
		// transactions and classify the leaf from the forfeit PSBT instead.
		if err != nil && hasSettledVhtlcCandidate(data.SpentVtxos) {
			details, err = s.classifySettledVhtlcSpend(ctx, data.SpentVtxos, v)
		}
		if err != nil {
			log.WithError(err).Warnf("failed to classify vhtlc spend for %s", v.Id)
			// unknown terminal spend.
			//
			// We know the tracked VHTLC script was spent, but we could not decode
			// which VHTLC leaf executed. Emit SPENT instead of dropping the event
			// so consumers still learn that the VHTLC is terminal.
			details = vhtlcSpendDetails{eventType: EventTypeVhtlcSpent}
		}

		s.emitEvent(VhtlcEvent{
			ID:        v.Id,
			Txid:      data.Txid,
			Preimage:  details.preimage,
			Type:      details.eventType,
			Timestamp: now,
		})
	}

	// Final state transition: stop tracking terminal VHTLC scripts.
	//
	// The persistent model is intentionally minimal: new VHTLC records start
	// with tracked=true, and the first spent-script event flips them to
	// tracked=false. On restart, only records still marked tracked are
	// resubscribed with the indexer.
	if s.vhtlcSubscription != nil {
		scriptsToUnsubscribe := vhtlcLockingScripts(spentVhtlcs)
		if len(scriptsToUnsubscribe) > 0 {
			if err := s.dbSvc.VHTLC().UntrackByScripts(ctx, scriptsToUnsubscribe); err != nil {
				log.WithError(err).Warn("failed to mark terminal vhtlc scripts untracked")
			}
			if err := s.vhtlcSubscription.unsubscribe(ctx, scriptsToUnsubscribe); err != nil {
				log.WithError(err).Warn("failed to unsubscribe terminal vhtlc scripts")
			}
		}
	}
}

func uniqueVhtlcScripts(vtxos []clientTypes.Vtxo) []string {
	seen := make(map[string]struct{}, len(vtxos))
	scripts := make([]string, 0, len(vtxos))

	for _, vtxo := range vtxos {
		if vtxo.Script == "" {
			continue
		}
		if _, ok := seen[vtxo.Script]; ok {
			continue
		}
		seen[vtxo.Script] = struct{}{}
		scripts = append(scripts, vtxo.Script)
	}

	return scripts
}

// uniqueVhtlcs avoids duplicate lifecycle events when an indexer event includes
// multiple VTXOs for the same VHTLC script.
func uniqueVhtlcs(vhtlcs []domain.Vhtlc) []domain.Vhtlc {
	seen := make(map[string]struct{}, len(vhtlcs))
	out := make([]domain.Vhtlc, 0, len(vhtlcs))

	for _, v := range vhtlcs {
		if _, ok := seen[v.Id]; ok {
			continue
		}
		seen[v.Id] = struct{}{}
		out = append(out, v)
	}

	return out
}

// vhtlcLockingScripts derives the tracked script set from VHTLC options. It is
// used when removing terminal VHTLCs from persistence and from the live indexer
// subscription.
func vhtlcLockingScripts(vhtlcs []domain.Vhtlc) []string {
	seen := make(map[string]struct{}, len(vhtlcs))
	scripts := make([]string, 0, len(vhtlcs))

	for _, v := range uniqueVhtlcs(vhtlcs) {
		scriptHex, err := vhtlc.LockingScriptHexFromOpts(v.Opts)
		if err != nil {
			log.WithError(err).Warnf("failed to derive locking script for vhtlc %s", v.Id)
			continue
		}
		if _, ok := seen[scriptHex]; ok {
			continue
		}
		seen[scriptHex] = struct{}{}
		scripts = append(scripts, scriptHex)
	}

	return scripts
}

// classifyVhtlcSpend determines the terminal event type for one VHTLC.
//
// Recognition rules:
//   - find spent VTXOs whose script equals this VHTLC locking script;
//   - extract the spent tapscript leaf from the PSBT/raw tx payload;
//   - compare that leaf with the VHTLC claim/refund closure scripts.
func classifyVhtlcSpend(
	tx string, spentVtxos []clientTypes.Vtxo, v domain.Vhtlc,
) (vhtlcSpendDetails, error) {
	vhtlcScript, err := vhtlc.NewVHTLCScriptFromOpts(v.Opts)
	if err != nil {
		return vhtlcSpendDetails{}, fmt.Errorf("failed to rebuild vhtlc script: %w", err)
	}

	lockingScript, err := vhtlc.LockingScriptHexFromOpts(v.Opts)
	if err != nil {
		return vhtlcSpendDetails{}, fmt.Errorf("failed to derive vhtlc locking script: %w", err)
	}

	candidates := vhtlcSpendCandidates(spentVtxos, lockingScript)

	if len(candidates) == 0 {
		return vhtlcSpendDetails{}, fmt.Errorf("no spent vtxos found for %s", v.Id)
	}

	leafScript, preimageCandidate, err := extractVhtlcSpendLeaf(tx, candidates)
	if err != nil {
		return vhtlcSpendDetails{}, err
	}

	eventType, err := classifyVhtlcSpendScript(vhtlcScript, leafScript)
	if err != nil {
		return vhtlcSpendDetails{}, err
	}

	details := vhtlcSpendDetails{eventType: eventType}
	if eventType == EventTypeVhtlcClaimed {
		details.preimage = preimageCandidate
	}

	return details, nil
}

// classifySettledVhtlcSpend classifies VHTLCs consumed through Ark batch
// settlement. The subscription event identifies the spent VHTLC and the
// commitment txid in SettledBy, but the event tx often does not contain the
// VHTLC claim/refund tap leaf. To recover the real spend path, fetch the
// round's forfeit txs from the indexer and classify the matching forfeit PSBT.
func (s *Service) classifySettledVhtlcSpend(
	ctx context.Context, spentVtxos []clientTypes.Vtxo, v domain.Vhtlc,
) (vhtlcSpendDetails, error) {
	lockingScript, err := vhtlc.LockingScriptHexFromOpts(v.Opts)
	if err != nil {
		return vhtlcSpendDetails{}, fmt.Errorf("failed to derive vhtlc locking script: %w", err)
	}

	candidates := vhtlcSpendCandidates(spentVtxos, lockingScript)
	if len(candidates) == 0 {
		return vhtlcSpendDetails{}, fmt.Errorf("no settled vhtlc candidates found for %s", v.Id)
	}

	if s.ArkClient == nil || s.Indexer() == nil {
		return vhtlcSpendDetails{}, fmt.Errorf("missing ark indexer")
	}

	txids := make([]string, 0)
	seen := make(map[string]struct{})
	for _, candidate := range candidates {
		if candidate.SettledBy == "" {
			continue
		}

		// SettledBy is the commitment txid for the round that consumed the
		// VHTLC. Ark indexes the signed forfeit txs under that commitment txid;
		// those forfeits are the transactions that spend the original VHTLC path.
		forfeits, err := s.Indexer().GetForfeitTxs(ctx, candidate.SettledBy)
		if err != nil {
			return vhtlcSpendDetails{}, fmt.Errorf("failed to get forfeit txs for %s: %w", candidate.SettledBy, err)
		}
		for _, txid := range forfeits.Txids {
			if _, ok := seen[txid]; ok {
				continue
			}
			seen[txid] = struct{}{}
			txids = append(txids, txid)
		}
	}

	if len(txids) == 0 {
		return vhtlcSpendDetails{}, fmt.Errorf("no forfeit txids found for settled vhtlc %s", v.Id)
	}

	// GetForfeitTxs returns only txids. Fetch the virtual tx bodies so we can
	// parse the PSBT inputs and compare their tap leaf with the VHTLC claim and
	// refund closure scripts.
	virtualTxs, err := s.Indexer().GetVirtualTxs(ctx, txids)
	if err != nil {
		return vhtlcSpendDetails{}, fmt.Errorf("failed to get forfeit tx bodies: %w", err)
	}

	return classifyVhtlcSpendFromForfeitTxs(virtualTxs.Txs, candidates, v)
}

func classifyVhtlcSpendFromForfeitTxs(
	txs []string, candidates []clientTypes.Vtxo, v domain.Vhtlc,
) (vhtlcSpendDetails, error) {
	var lastErr error
	for _, tx := range txs {
		details, err := classifyVhtlcSpend(tx, candidates, v)
		if err == nil {
			return details, nil
		}
		lastErr = err
	}

	if lastErr != nil {
		return vhtlcSpendDetails{}, lastErr
	}
	return vhtlcSpendDetails{}, fmt.Errorf("no forfeit txs found")
}

func vhtlcSpendCandidates(
	spentVtxos []clientTypes.Vtxo, lockingScript string,
) []clientTypes.Vtxo {
	candidates := make([]clientTypes.Vtxo, 0, len(spentVtxos))
	for _, spentVtxo := range spentVtxos {
		if spentVtxo.Script == lockingScript {
			candidates = append(candidates, spentVtxo)
		}
	}
	return candidates
}

// classifyVhtlcSpendScript compares the executed tapscript leaf with all known
// VHTLC closures. Claim closures map to CLAIMED; every refund closure maps to
// REFUNDED. Unknown leaves are treated as classification errors by the caller.
func classifyVhtlcSpendScript(vhtlcScript *vhtlc.VHTLCScript, leafScript []byte) (EventType, error) {
	claimScript, err := vhtlcScript.ClaimClosure.Script()
	if err != nil {
		return EventTypeUnspecified, fmt.Errorf("failed to derive claim script: %w", err)
	}
	unilateralClaimScript, err := vhtlcScript.UnilateralClaimClosure.Script()
	if err != nil {
		return EventTypeUnspecified, fmt.Errorf("failed to derive unilateral claim script: %w", err)
	}
	refundScript, err := vhtlcScript.RefundClosure.Script()
	if err != nil {
		return EventTypeUnspecified, fmt.Errorf("failed to derive refund script: %w", err)
	}
	refundWithoutReceiverScript, err := vhtlcScript.RefundWithoutReceiverClosure.Script()
	if err != nil {
		return EventTypeUnspecified, fmt.Errorf("failed to derive refund-without-receiver script: %w", err)
	}
	unilateralRefundScript, err := vhtlcScript.UnilateralRefundClosure.Script()
	if err != nil {
		return EventTypeUnspecified, fmt.Errorf("failed to derive unilateral refund script: %w", err)
	}
	unilateralRefundWithoutReceiverScript, err := vhtlcScript.UnilateralRefundWithoutReceiverClosure.Script()
	if err != nil {
		return EventTypeUnspecified, fmt.Errorf("failed to derive unilateral refund-without-receiver script: %w", err)
	}

	switch {
	case bytes.Equal(leafScript, claimScript), bytes.Equal(leafScript, unilateralClaimScript):
		return EventTypeVhtlcClaimed, nil
	case bytes.Equal(leafScript, refundScript),
		bytes.Equal(leafScript, refundWithoutReceiverScript),
		bytes.Equal(leafScript, unilateralRefundScript),
		bytes.Equal(leafScript, unilateralRefundWithoutReceiverScript):
		return EventTypeVhtlcRefunded, nil
	default:
		return EventTypeUnspecified, fmt.Errorf("unknown vhtlc spend path")
	}
}

// extractVhtlcSpendLeaf parses the indexer transaction payload. Direct VHTLC
// spends arrive as PSBTs, while settlement/batch spends can arrive as
// raw transactions.
func extractVhtlcSpendLeaf(tx string, spentVtxos []clientTypes.Vtxo) ([]byte, string, error) {
	if tx == "" {
		return nil, "", fmt.Errorf("missing spend transaction")
	}

	if packet, err := psbt.NewFromRawBytes(strings.NewReader(tx), true); err == nil {
		return extractVhtlcSpendLeafFromPSBT(packet, spentVtxos)
	}

	var rawTx wire.MsgTx
	if err := rawTx.Deserialize(hex.NewDecoder(strings.NewReader(tx))); err == nil {
		return extractVhtlcSpendLeafFromWireTx(&rawTx, spentVtxos)
	}

	return nil, "", fmt.Errorf("failed to parse spend transaction")
}

// extractVhtlcSpendLeafFromPSBT returns the VHTLC leaf script from the PSBT
// input that spends the candidate VTXO. The claim preimage is read from Ark's
// condition-witness proprietary field when present, otherwise from the final
// witness argument.
func extractVhtlcSpendLeafFromPSBT(
	packet *psbt.Packet, spentVtxos []clientTypes.Vtxo,
) ([]byte, string, error) {
	inputIndex, err := findSpentVtxoInputIndex(packet.UnsignedTx.TxIn, spentVtxos)
	if err != nil {
		return nil, "", err
	}
	if len(packet.Inputs) <= inputIndex {
		return nil, "", fmt.Errorf("missing psbt input %d", inputIndex)
	}

	input := packet.Inputs[inputIndex]
	preimage := extractVhtlcConditionWitness(packet, inputIndex)

	if len(input.TaprootLeafScript) > 0 {
		if preimage == "" && len(input.FinalScriptWitness) > 0 {
			_, witnessArg, err := extractLeafScriptFromWitness(input.FinalScriptWitness)
			if err == nil && len(witnessArg) > 0 {
				preimage = hex.EncodeToString(witnessArg)
			}
		}
		return input.TaprootLeafScript[0].Script, preimage, nil
	}

	if len(input.FinalScriptWitness) == 0 {
		return nil, "", fmt.Errorf("psbt input %d has no tapscript data", inputIndex)
	}

	leafScript, witnessArg, err := extractLeafScriptFromWitness(input.FinalScriptWitness)
	if err != nil {
		return nil, "", err
	}
	if preimage == "" && len(witnessArg) > 0 {
		preimage = hex.EncodeToString(witnessArg)
	}

	return leafScript, preimage, nil
}

// extractVhtlcSpendLeafFromWireTx is used for raw tx payloads. It can classify
// spends only when the raw input witness contains the tapscript leaf.
func extractVhtlcSpendLeafFromWireTx(
	tx *wire.MsgTx, spentVtxos []clientTypes.Vtxo,
) ([]byte, string, error) {
	inputIndex, err := findSpentVtxoInputIndex(tx.TxIn, spentVtxos)
	if err != nil {
		return nil, "", err
	}

	leafScript, witnessArg, err := extractLeafScriptFromTxWitness(tx.TxIn[inputIndex].Witness)
	if err != nil {
		return nil, "", err
	}

	preimage := ""
	if len(witnessArg) > 0 {
		preimage = hex.EncodeToString(witnessArg)
	}

	return leafScript, preimage, nil
}

// extractVhtlcConditionWitness extracts the claim preimage stored by Ark in the
// PSBT proprietary condition-witness field.
func extractVhtlcConditionWitness(packet *psbt.Packet, inputIndex int) string {
	witnesses, err := txutils.GetArkPsbtFields(packet, inputIndex, txutils.ConditionWitnessField)
	if err != nil || len(witnesses) == 0 || len(witnesses[0]) == 0 || len(witnesses[0][0]) == 0 {
		return ""
	}

	return hex.EncodeToString(witnesses[0][0])
}

// findSpentVtxoInputIndex maps an indexer spent VTXO to the spending
// transaction input. For direct Bitcoin spends the input prevout can be the
// VTXO outpoint. For Ark VTXO spends the input often references the intermediate
// transaction recorded by the indexer as SpentBy, so both forms are accepted.
func findSpentVtxoInputIndex(inputs []*wire.TxIn, spentVtxos []clientTypes.Vtxo) (int, error) {
	for inputIndex, txIn := range inputs {
		for _, spentVtxo := range spentVtxos {
			if txIn.PreviousOutPoint.Hash.String() == spentVtxo.Txid &&
				txIn.PreviousOutPoint.Index == spentVtxo.VOut {
				return inputIndex, nil
			}
			if spentVtxo.SpentBy != "" &&
				txIn.PreviousOutPoint.Hash.String() == spentVtxo.SpentBy {
				return inputIndex, nil
			}
		}
	}

	return 0, fmt.Errorf("spent vtxo input not found")
}

// hasSettledVhtlcCandidate reports whether the indexer already identified the
// VHTLC VTXO as consumed by an Ark settlement/batch transaction.
func hasSettledVhtlcCandidate(vtxos []clientTypes.Vtxo) bool {
	for _, vtxo := range vtxos {
		if vtxo.SettledBy != "" {
			return true
		}
	}
	return false
}

// extractLeafScriptFromWitness decodes a serialized final witness from PSBT.
func extractLeafScriptFromWitness(serialized []byte) ([]byte, []byte, error) {
	witness, err := txutils.ReadTxWitness(serialized)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read final witness: %w", err)
	}

	return extractLeafScriptFromTxWitness(witness)
}

// extractLeafScriptFromTxWitness reads the executed leaf from a finalized
// Taproot script-path witness.
//
// Ark builds non-condition closure witnesses as:
//
//	[signatures..., leaf_script, control_block]
//
// VHTLC refund leaves are non-condition closures. Because Ark appends
// signatures in reverse closure pubkey order, the concrete VHTLC refund
// witnesses are:
//
//	collaborative refund:          [server_sig, receiver_sig, sender_sig, refund_leaf, control_block]
//	refund without receiver:       [server_sig, sender_sig, refund_without_receiver_leaf, control_block]
//	unilateral refund:             [receiver_sig, sender_sig, unilateral_refund_leaf, control_block]
//	unilateral refund no receiver: [sender_sig, unilateral_refund_without_receiver_leaf, control_block]
//
// Ark builds condition closure witnesses as:
//
//	[signatures..., condition_witness..., leaf_script, control_block]
//
// Fulmine sets condition_witness to wire.TxWitness{preimage} on VHTLC claim
// paths. The concrete claim witnesses are:
//
//	collaborative claim: [server_sig, receiver_sig, preimage, claim_leaf, control_block]
//	unilateral claim:   [receiver_sig, preimage, unilateral_claim_leaf, control_block]
//
// This function returns witness[len-2] as the leaf and witness[len-3] as the
// last argument before the leaf. That argument is the preimage only for claim
// leaves; for refund leaves it is a signature and the caller must ignore it as
// a preimage.
func extractLeafScriptFromTxWitness(witness wire.TxWitness) ([]byte, []byte, error) {
	if len(witness) < 2 {
		return nil, nil, fmt.Errorf("invalid tapscript witness")
	}

	var witnessArg []byte
	if len(witness) >= 3 {
		// Last item is the control block, second-last is the executed leaf,
		// so the third-last item is the last script argument before the leaf.
		witnessArg = witness[len(witness)-3]
	}

	return witness[len(witness)-2], witnessArg, nil
}
