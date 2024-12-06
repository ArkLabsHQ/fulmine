package vhtlc

import (
	"errors"

	"github.com/ark-network/ark/common"
	"github.com/ark-network/ark/common/tree"
	"github.com/btcsuite/btcd/txscript"
	"github.com/decred/dcrd/dcrec/secp256k1/v4"
)

const (
	hash160Len = 20
)

type Opts struct {
	Sender                 *secp256k1.PublicKey
	Receiver               *secp256k1.PublicKey
	Server                 *secp256k1.PublicKey
	ReceiverRefundLocktime common.Locktime // absolute locktime
	SenderReclaimLocktime  common.Locktime // absolute locktime
	SenderReclaimDelay     common.Locktime // relative locktime
	ClaimDelay             common.Locktime // relative locktime
	PreimageHash           []byte
}

func (o Opts) Validate() error {
	if o.Sender == nil || o.Receiver == nil || o.Server == nil {
		return errors.New("sender, receiver, and server are required")
	}

	if len(o.PreimageHash) != hash160Len {
		return errors.New("preimage hash must be 20 bytes")
	}

	return nil
}

// Refund = (Sender + Receiver + Server)
// offchain path
func (o Opts) Refund() *tree.MultisigClosure {
	return &tree.MultisigClosure{
		PubKeys: []*secp256k1.PublicKey{o.Sender, o.Receiver, o.Server},
	}
}

// RefundWithoutSender = (Receiver + Server) at ReceiverRefundLocktime
// offchain path
func (o Opts) RefundWithoutSender() *tree.CLTVMultisigClosure {
	return &tree.CLTVMultisigClosure{
		MultisigClosure: tree.MultisigClosure{
			PubKeys: []*secp256k1.PublicKey{o.Receiver, o.Server},
		},
		Locktime: o.ReceiverRefundLocktime,
	}
}

// CollaborativeReclaim = (Sender + Server) at SenderReclaimLocktime
// offchain path
func (o Opts) CollaborativeReclaim() *tree.CLTVMultisigClosure {
	return &tree.CLTVMultisigClosure{
		MultisigClosure: tree.MultisigClosure{
			PubKeys: []*secp256k1.PublicKey{o.Sender, o.Server},
		},
		Locktime: o.SenderReclaimLocktime,
	}
}

// UnilateralReclaim = (Sender) after SenderReclaimAloneDelay
// onchain path
func (o Opts) UnilateralReclaim() *tree.CSVSigClosure {
	return &tree.CSVSigClosure{
		MultisigClosure: tree.MultisigClosure{
			PubKeys: []*secp256k1.PublicKey{o.Sender},
		},
		Locktime: o.SenderReclaimDelay,
	}
}

// Claim = (receiver + server + preimage)
func (o Opts) Claim() (*tree.ConditionMultisigClosure, error) {
	preimageConditionScript, err := makePreimageConditionScript(o.PreimageHash)
	if err != nil {
		return nil, err
	}

	return &tree.ConditionMultisigClosure{
		Condition: preimageConditionScript,
		MultisigClosure: tree.MultisigClosure{
			PubKeys: []*secp256k1.PublicKey{o.Receiver, o.Server},
		},
	}, nil
}

// TODO: implement ConditionCSVClosure (ark)
// UnilateralClaim = (receiver + preimage) after ClaimDelay
// onchain path
// func (o Opts) UnilateralClaim() (*tree.CSVConditionClosure, error) {
// claimAloneClosure := &tree.ConditionCSVClosure{
// 	Condition: preimageConditionScript,
// 	CSVSigClosure: tree.CSVSigClosure{
// 		MultisigClosure: tree.MultisigClosure{
// 			PubKeys: []*secp256k1.PublicKey{opts.Receiver},
// 		},
// 		Locktime: opts.ReceiverExitDelay,
// 	},
// }
// }

// NewVHTLC creates a VHTLC VtxoScript from the given options.
func NewVHTLC(opts Opts) (*tree.TapscriptsVtxoScript, error) {
	if err := opts.Validate(); err != nil {
		return nil, err
	}

	refundClosure := opts.Refund()
	reclaimWithASPClosure := opts.CollaborativeReclaim()
	reclaimAloneClosure := opts.UnilateralReclaim()
	refundWithASPClosure := opts.RefundWithoutSender()
	claimClosure, err := opts.Claim()
	if err != nil {
		return nil, err
	}

	return &tree.TapscriptsVtxoScript{
		Closures: []tree.Closure{
			claimClosure,
			// claimAloneClosure,
			reclaimWithASPClosure,
			reclaimAloneClosure,
			refundClosure,
			refundWithASPClosure,
		},
	}, nil
}

func makePreimageConditionScript(preimageHash []byte) ([]byte, error) {
	return txscript.NewScriptBuilder().
		AddOp(txscript.OP_HASH160).
		AddData(preimageHash).
		AddOp(txscript.OP_EQUAL).
		Script()
}
