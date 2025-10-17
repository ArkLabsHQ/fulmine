package vhtlc

import (
	"encoding/hex"
	"fmt"

	arklib "github.com/arkade-os/arkd/pkg/ark-lib"
	"github.com/arkade-os/arkd/pkg/ark-lib/script"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcwallet/waddrmgr"
)

const (
	hash160Len              = 20
	minSecondsTimelock      = 512
	secondsTimelockMultiple = 512
)

type Opts struct {
	Sender                               *btcec.PublicKey
	Receiver                             *btcec.PublicKey
	Server                               *btcec.PublicKey
	PreimageHash                         []byte
	RefundLocktime                       arklib.AbsoluteLocktime
	UnilateralClaimDelay                 arklib.RelativeLocktime
	UnilateralRefundDelay                arklib.RelativeLocktime
	UnilateralRefundWithoutReceiverDelay arklib.RelativeLocktime
}

func (o Opts) validate() error {
	if o.Sender == nil || o.Receiver == nil || o.Server == nil {
		return fmt.Errorf("sender, receiver, and server are required")
	}

	if len(o.PreimageHash) != hash160Len {
		return fmt.Errorf("preimage hash must be %d bytes", hash160Len)
	}

	if o.RefundLocktime == 0 {
		return fmt.Errorf("refund locktime must be greater than 0")
	}

	if o.UnilateralClaimDelay.Value == 0 {
		return fmt.Errorf("unilateral claim delay must be greater than 0")
	}

	if o.UnilateralRefundDelay.Value == 0 {
		return fmt.Errorf("unilateral refund delay must be greater than 0")
	}

	if o.UnilateralRefundWithoutReceiverDelay.Value == 0 {
		return fmt.Errorf("unilateral refund without receiver delay must be greater than 0")
	}

	// Validate seconds timelock values
	if err := validateSecondsTimelock(o.UnilateralClaimDelay); err != nil {
		return fmt.Errorf("unilateral claim delay: %w", err)
	}

	if err := validateSecondsTimelock(o.UnilateralRefundDelay); err != nil {
		return fmt.Errorf("unilateral refund delay: %w", err)
	}

	if err := validateSecondsTimelock(o.UnilateralRefundWithoutReceiverDelay); err != nil {
		return fmt.Errorf("unilateral refund without receiver delay: %w", err)
	}

	return nil
}

// validateSecondsTimelock validates that seconds timelock values meet the requirements
func validateSecondsTimelock(locktime arklib.RelativeLocktime) error {
	if locktime.Type == arklib.LocktimeTypeSecond {
		if locktime.Value < minSecondsTimelock {
			return fmt.Errorf("seconds timelock must be greater or equal to %d", minSecondsTimelock)
		}
		if locktime.Value%secondsTimelockMultiple != 0 {
			return fmt.Errorf("seconds timelock must be multiple of %d", secondsTimelockMultiple)
		}
	}
	return nil
}

func (o Opts) claimClosure(preimageCondition []byte) *script.ConditionMultisigClosure {
	return &script.ConditionMultisigClosure{
		Condition: preimageCondition,
		MultisigClosure: script.MultisigClosure{
			PubKeys: []*btcec.PublicKey{o.Receiver, o.Server},
		},
	}
}

// refundClosure = (Sender + Receiver + Server)
func (o Opts) refundClosure() *script.MultisigClosure {
	return &script.MultisigClosure{
		PubKeys: []*btcec.PublicKey{o.Sender, o.Receiver, o.Server},
	}
}

// RefundWithoutReceiver = (Sender + Server) at RefundDelay
func (o Opts) refundWithoutReceiverClosure() *script.CLTVMultisigClosure {
	return &script.CLTVMultisigClosure{
		MultisigClosure: script.MultisigClosure{
			PubKeys: []*btcec.PublicKey{o.Sender, o.Server},
		},
		Locktime: o.RefundLocktime,
	}
}

// unilateralClaimClosure = (Receiver + Preimage) at UnilateralClaimDelay
func (o Opts) unilateralClaimClosure(preimageCondition []byte) *script.ConditionCSVMultisigClosure {
	// TODO: update deps and add condition
	return &script.ConditionCSVMultisigClosure{
		CSVMultisigClosure: script.CSVMultisigClosure{
			MultisigClosure: script.MultisigClosure{
				PubKeys: []*btcec.PublicKey{o.Receiver},
			},
			Locktime: o.UnilateralClaimDelay,
		},
		Condition: preimageCondition,
	}
}

// unilateralRefundClosure = (Sender + Receiver) at UnilateralRefundDelay
func (o Opts) unilateralRefundClosure() *script.CSVMultisigClosure {
	return &script.CSVMultisigClosure{
		MultisigClosure: script.MultisigClosure{
			PubKeys: []*btcec.PublicKey{o.Sender, o.Receiver},
		},
		Locktime: o.UnilateralRefundDelay,
	}
}

// unilateralRefundWithoutReceiverClosure = (Sender) at UnilateralRefundWithoutReceiverDelay
func (o Opts) unilateralRefundWithoutReceiverClosure() *script.CSVMultisigClosure {
	return &script.CSVMultisigClosure{
		MultisigClosure: script.MultisigClosure{
			PubKeys: []*btcec.PublicKey{o.Sender},
		},
		Locktime: o.UnilateralRefundWithoutReceiverDelay,
	}
}

type VHTLCScript struct {
	script.TapscriptsVtxoScript

	Sender                                 *btcec.PublicKey
	Receiver                               *btcec.PublicKey
	Server                                 *btcec.PublicKey
	ClaimClosure                           *script.ConditionMultisigClosure
	RefundClosure                          *script.MultisigClosure
	RefundWithoutReceiverClosure           *script.CLTVMultisigClosure
	UnilateralClaimClosure                 *script.ConditionCSVMultisigClosure
	UnilateralRefundClosure                *script.CSVMultisigClosure
	UnilateralRefundWithoutReceiverClosure *script.CSVMultisigClosure

	preimageConditionScript []byte
}

// NewVHTLCScript creates a VHTLC VtxoScript from the given options.
func NewVHTLCScript(opts Opts) (*VHTLCScript, error) {
	if err := opts.validate(); err != nil {
		return nil, err
	}

	preimageCondition, err := makePreimageConditionScript(opts.PreimageHash)
	if err != nil {
		return nil, err
	}

	claimClosure := opts.claimClosure(preimageCondition)
	refundClosure := opts.refundClosure()
	refundWithoutReceiverClosure := opts.refundWithoutReceiverClosure()

	unilateralClaimClosure := opts.unilateralClaimClosure(preimageCondition)
	unilateralRefundClosure := opts.unilateralRefundClosure()
	unilateralRefundWithoutReceiverClosure := opts.unilateralRefundWithoutReceiverClosure()

	return &VHTLCScript{
		TapscriptsVtxoScript: script.TapscriptsVtxoScript{
			Closures: []script.Closure{
				// Collaborative paths
				claimClosure,
				refundClosure,
				refundWithoutReceiverClosure,
				// Exit paths
				unilateralClaimClosure,
				unilateralRefundClosure,
				unilateralRefundWithoutReceiverClosure,
			},
		},
		Sender:                                 opts.Sender,
		Receiver:                               opts.Receiver,
		Server:                                 opts.Server,
		ClaimClosure:                           claimClosure,
		RefundClosure:                          refundClosure,
		RefundWithoutReceiverClosure:           refundWithoutReceiverClosure,
		UnilateralClaimClosure:                 unilateralClaimClosure,
		UnilateralRefundClosure:                unilateralRefundClosure,
		UnilateralRefundWithoutReceiverClosure: unilateralRefundWithoutReceiverClosure,
		preimageConditionScript:                preimageCondition,
	}, nil
}

func makePreimageConditionScript(preimageHash []byte) ([]byte, error) {
	return txscript.NewScriptBuilder().
		AddOp(txscript.OP_HASH160).
		AddData(preimageHash).
		AddOp(txscript.OP_EQUAL).
		Script()
}

// GetRevealedTapscripts returns all available scripts as hex-encoded strings
func (v *VHTLCScript) GetRevealedTapscripts() []string {
	var scripts []string
	for _, closure := range []script.Closure{
		v.ClaimClosure,
		v.RefundClosure,
		v.RefundWithoutReceiverClosure,
		v.UnilateralClaimClosure,
		v.UnilateralRefundClosure,
		v.UnilateralRefundWithoutReceiverClosure,
	} {
		if script, err := closure.Script(); err == nil {
			scripts = append(scripts, hex.EncodeToString(script))
		}
	}
	return scripts
}

func (v *VHTLCScript) Address(hrp string, serverPubkey *btcec.PublicKey) (string, error) {
	tapKey, _, err := v.TapTree()
	if err != nil {
		return "", err
	}

	addr := &arklib.Address{
		HRP:        hrp,
		Signer:     serverPubkey,
		VtxoTapKey: tapKey,
	}

	return addr.EncodeV0()
}

// ClaimTapscript computes the necessary script and control block to spend the claim closure
func (v *VHTLCScript) ClaimTapscript() (*waddrmgr.Tapscript, error) {
	claimClosure := v.ClaimClosure
	claimScript, err := claimClosure.Script()
	if err != nil {
		return nil, err
	}

	_, tapTree, err := v.TapTree()
	if err != nil {
		return nil, err
	}

	leafProof, err := tapTree.GetTaprootMerkleProof(
		txscript.NewBaseTapLeaf(claimScript).TapHash(),
	)
	if err != nil {
		return nil, err
	}

	ctrlBlock, err := txscript.ParseControlBlock(leafProof.ControlBlock)
	if err != nil {
		return nil, err
	}

	return &waddrmgr.Tapscript{
		RevealedScript: leafProof.Script,
		ControlBlock:   ctrlBlock,
	}, nil
}

// RefundTapscript computes the necessary script and control block to spend the refund closure,
// it does not return any checkpoint output script.
func (v *VHTLCScript) RefundTapscript(withReceiver bool) (*waddrmgr.Tapscript, error) {
	var refundClosure script.Closure
	refundClosure = v.RefundWithoutReceiverClosure
	if withReceiver {
		refundClosure = v.RefundClosure
	}
	refundScript, err := refundClosure.Script()
	if err != nil {
		return nil, err
	}

	_, tapTree, err := v.TapTree()
	if err != nil {
		return nil, err
	}

	refundLeafProof, err := tapTree.GetTaprootMerkleProof(
		txscript.NewBaseTapLeaf(refundScript).TapHash(),
	)
	if err != nil {
		return nil, err
	}

	ctrlBlock, err := txscript.ParseControlBlock(refundLeafProof.ControlBlock)
	if err != nil {
		return nil, err
	}

	return &waddrmgr.Tapscript{
		RevealedScript: refundLeafProof.Script,
		ControlBlock:   ctrlBlock,
	}, nil
}

func (v *VHTLCScript) DeriveOpts() Opts {
	return Opts{
		Sender:                               v.Sender,
		Receiver:                             v.Receiver,
		Server:                               v.Server,
		PreimageHash:                         v.preimageConditionScript[2 : 2+hash160Len],
		RefundLocktime:                       v.RefundWithoutReceiverClosure.Locktime,
		UnilateralClaimDelay:                 v.UnilateralClaimClosure.Locktime,
		UnilateralRefundDelay:                v.UnilateralRefundClosure.Locktime,
		UnilateralRefundWithoutReceiverDelay: v.UnilateralRefundWithoutReceiverClosure.Locktime,
	}
}

func GetVhtlcScript(server *btcec.PublicKey, preimageHash, claimLeaf, refundLeaf, refundWithoutReceiverLeaf, unilateralClaimLeaf, unilateralRefundLeaf, unilateralRefundWithoutReceiverLeaf string) (*VHTLCScript, error) {
	decodedPreimagehash, err := hex.DecodeString(preimageHash)
	if err != nil {
		return nil, err
	}

	preimageCondition, err := makePreimageConditionScript(decodedPreimagehash)
	if err != nil {
		return nil, err
	}

	decodedClaimLeaf, err := hex.DecodeString(claimLeaf)
	if err != nil {
		return nil, err
	}

	claimClosure := script.ConditionMultisigClosure{}
	isDecoded, err := claimClosure.Decode(decodedClaimLeaf)
	if err != nil {
		return nil, err
	}

	if !isDecoded {
		return nil, fmt.Errorf("failed to decoded Claim Script")
	}

	decodedRefundLeaf, err := hex.DecodeString(refundLeaf)
	if err != nil {
		return nil, err
	}

	refundClosure := script.MultisigClosure{}
	isDecoded, err = refundClosure.Decode(decodedRefundLeaf)
	if err != nil {
		return nil, err
	}

	if !isDecoded {
		return nil, fmt.Errorf("failed to decode Refund Script")
	}

	decodedRefundWithoutReceiverLeaf, err := hex.DecodeString(refundWithoutReceiverLeaf)
	if err != nil {
		return nil, err
	}

	refundWithoutReceiverClosure := script.CLTVMultisigClosure{}
	isDecoded, err = refundWithoutReceiverClosure.Decode(decodedRefundWithoutReceiverLeaf)

	if err != nil {
		return nil, err
	}

	if !isDecoded {
		return nil, fmt.Errorf("failed to decode refundWithoutReceiverClosure")
	}

	decodedUnilateralClaimLeaf, err := hex.DecodeString(unilateralClaimLeaf)
	if err != nil {
		return nil, err
	}

	unilateralClaimClosure := script.ConditionCSVMultisigClosure{}
	isDecoded, err = unilateralClaimClosure.Decode(decodedUnilateralClaimLeaf)

	if err != nil {
		return nil, err
	}

	if !isDecoded {
		return nil, fmt.Errorf("failed to decode Unilateral Claim Script")
	}

	decodedUnilateralRefundClosure, err := hex.DecodeString(unilateralRefundLeaf)
	if err != nil {
		return nil, err
	}

	unilateralRefundClosure := script.CSVMultisigClosure{}
	isDecoded, err = unilateralRefundClosure.Decode(decodedUnilateralRefundClosure)

	if err != nil {
		return nil, err
	}

	if !isDecoded {
		return nil, fmt.Errorf("failed to decode Unilateral Refund Script")
	}

	decodedUnilateralRefundWithoutReceiverClosure, err := hex.DecodeString(unilateralRefundWithoutReceiverLeaf)
	if err != nil {
		return nil, err
	}

	unilateralRefundWithoutReceiverClosure := script.CSVMultisigClosure{}
	isDecoded, err = unilateralRefundWithoutReceiverClosure.Decode(decodedUnilateralRefundWithoutReceiverClosure)

	if err != nil {
		return nil, err
	}

	if !isDecoded {
		return nil, fmt.Errorf("failed to decode Unilateral Refund Without Receiver Script")
	}

	var receiver *btcec.PublicKey
	var sender *btcec.PublicKey

	for _, pk := range claimClosure.PubKeys {
		if pk.IsEqual(server) {
			continue
		}
		receiver = pk
	}

	for _, pk := range refundWithoutReceiverClosure.PubKeys {
		if pk.IsEqual(server) {
			continue
		}
		sender = pk
	}

	vhtlc := &VHTLCScript{
		TapscriptsVtxoScript: script.TapscriptsVtxoScript{
			Closures: []script.Closure{
				// Collaborative paths
				&claimClosure,
				&refundClosure,
				&refundWithoutReceiverClosure,
				// Exit paths
				&unilateralClaimClosure,
				&unilateralRefundClosure,
				&unilateralRefundWithoutReceiverClosure,
			},
		},
		preimageConditionScript:                preimageCondition,
		Receiver:                               receiver,
		Sender:                                 sender,
		Server:                                 server,
		ClaimClosure:                           &claimClosure,
		RefundClosure:                          &refundClosure,
		RefundWithoutReceiverClosure:           &refundWithoutReceiverClosure,
		UnilateralClaimClosure:                 &unilateralClaimClosure,
		UnilateralRefundClosure:                &unilateralRefundClosure,
		UnilateralRefundWithoutReceiverClosure: &unilateralRefundWithoutReceiverClosure,
	}

	return vhtlc, nil
}
