package vhtlc

import (
	"encoding/hex"
	"errors"

	"github.com/ark-network/ark/common"
	"github.com/ark-network/ark/common/bitcointree"
	"github.com/ark-network/ark/common/tree"
	"github.com/btcsuite/btcd/txscript"
	"github.com/decred/dcrd/dcrec/secp256k1/v4"
)

const (
	hash160Len = 20
)

type Opts struct {
	Sender                               *secp256k1.PublicKey
	Receiver                             *secp256k1.PublicKey
	Server                               *secp256k1.PublicKey
	PreimageHash                         []byte
	RefundLocktime                       common.AbsoluteLocktime
	UnilateralClaimDelay                 common.RelativeLocktime
	UnilateralRefundDelay                common.RelativeLocktime
	UnilateralRefundWithoutReceiverDelay common.RelativeLocktime
}

func (o Opts) validate() error {
	if o.Sender == nil || o.Receiver == nil || o.Server == nil {
		return errors.New("sender, receiver, and server are required")
	}

	if len(o.PreimageHash) != hash160Len {
		return errors.New("preimage hash must be 20 bytes")
	}

	return nil
}

func (o Opts) claimClosure(preimageCondition []byte) *tree.ConditionMultisigClosure {
	return &tree.ConditionMultisigClosure{
		Condition: preimageCondition,
		MultisigClosure: tree.MultisigClosure{
			PubKeys: []*secp256k1.PublicKey{o.Receiver, o.Server},
		},
	}
}

// refundClosure = (Sender + Receiver + Server)
func (o Opts) refundClosure() *tree.MultisigClosure {
	return &tree.MultisigClosure{
		PubKeys: []*secp256k1.PublicKey{o.Sender, o.Receiver, o.Server},
	}
}

// RefundWithoutReceiver = (Sender + Server) at RefundDelay
func (o Opts) refundWithoutReceiverClosure() *tree.CLTVMultisigClosure {
	return &tree.CLTVMultisigClosure{
		MultisigClosure: tree.MultisigClosure{
			PubKeys: []*secp256k1.PublicKey{o.Sender, o.Server},
		},
		Locktime: o.RefundLocktime,
	}
}

// unilateralClaimClosure = (Receiver + Preimage) at UnilateralClaimDelay
func (o Opts) unilateralClaimClosure(preimageCondition []byte) *tree.ConditionCSVMultisigClosure {
	// TODO: update deps and add condition
	return &tree.ConditionCSVMultisigClosure{
		CSVMultisigClosure: tree.CSVMultisigClosure{
			MultisigClosure: tree.MultisigClosure{
				PubKeys: []*secp256k1.PublicKey{o.Receiver},
			},
			Locktime: o.UnilateralClaimDelay,
		},
		Condition: preimageCondition,
	}
}

// unilateralRefundClosure = (Sender + Receiver) at UnilateralRefundDelay
func (o Opts) unilateralRefundClosure() *tree.CSVMultisigClosure {
	return &tree.CSVMultisigClosure{
		MultisigClosure: tree.MultisigClosure{
			PubKeys: []*secp256k1.PublicKey{o.Sender, o.Receiver},
		},
		Locktime: o.UnilateralRefundDelay,
	}
}

// unilateralRefundWithoutReceiverClosure = (Sender) at UnilateralRefundWithoutReceiverDelay
func (o Opts) unilateralRefundWithoutReceiverClosure() *tree.CSVMultisigClosure {
	return &tree.CSVMultisigClosure{
		MultisigClosure: tree.MultisigClosure{
			PubKeys: []*secp256k1.PublicKey{o.Sender},
		},
		Locktime: o.UnilateralRefundWithoutReceiverDelay,
	}
}

type VHTLCScript struct {
	bitcointree.TapscriptsVtxoScript

	Sender                                 *secp256k1.PublicKey
	Receiver                               *secp256k1.PublicKey
	Server                                 *secp256k1.PublicKey
	ClaimClosure                           *tree.ConditionMultisigClosure
	RefundClosure                          *tree.MultisigClosure
	RefundWithoutReceiverClosure           *tree.CLTVMultisigClosure
	UnilateralClaimClosure                 *tree.ConditionCSVMultisigClosure
	UnilateralRefundClosure                *tree.CSVMultisigClosure
	UnilateralRefundWithoutReceiverClosure *tree.CSVMultisigClosure

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
		TapscriptsVtxoScript: bitcointree.TapscriptsVtxoScript{
			TapscriptsVtxoScript: tree.TapscriptsVtxoScript{
				Closures: []tree.Closure{
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
	for _, closure := range []tree.Closure{
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
