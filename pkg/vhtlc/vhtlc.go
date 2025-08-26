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
