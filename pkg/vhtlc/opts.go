package vhtlc

import (
	"fmt"

	arklib "github.com/arkade-os/arkd/pkg/ark-lib"
	"github.com/arkade-os/arkd/pkg/ark-lib/script"
	"github.com/btcsuite/btcd/btcec/v2"
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
	if o.Sender == nil {
		return fmt.Errorf("missing sender pubkey")
	}
	if o.Receiver == nil {
		return fmt.Errorf("missing receiver pubkey")
	}
	if o.Server == nil {
		return fmt.Errorf("missing server pubkey")
	}

	if len(o.PreimageHash) <= 0 {
		return fmt.Errorf("missing preimage hash")
	}
	if len(o.PreimageHash) != hash160Len {
		return fmt.Errorf("preimage hash must be %d bytes", hash160Len)
	}

	if o.RefundLocktime == 0 {
		return fmt.Errorf("refund locktime must be greater than 0")
	}

	if err := validateTimelock(o.UnilateralClaimDelay); err != nil {
		return fmt.Errorf("invalid unilateral claim delay: %w", err)
	}

	if err := validateTimelock(o.UnilateralRefundDelay); err != nil {
		return fmt.Errorf("invalid unilateral refund delay: %w", err)
	}

	if err := validateTimelock(o.UnilateralRefundWithoutReceiverDelay); err != nil {
		return fmt.Errorf("invalid unilateral refund without receiver delay: %w", err)
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
func (o Opts) unilateralClaimClosure(
	preimageCondition []byte,
) *script.ConditionCSVMultisigClosure {
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
