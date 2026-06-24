package vhtlc

import (
	"encoding/hex"
	"fmt"

	arklib "github.com/arkade-os/arkd/pkg/ark-lib"
	"github.com/arkade-os/arkd/pkg/ark-lib/script"
	"github.com/btcsuite/btcd/txscript"
)

func parseClaimClosure(leaf string) (*script.ConditionMultisigClosure, error) {
	buf, err := hex.DecodeString(leaf)
	if err != nil {
		return nil, fmt.Errorf("failed to decode claim closure: %w", err)
	}
	closure := script.ConditionMultisigClosure{}
	ok, err := closure.Decode(buf)
	if err != nil {
		return nil, fmt.Errorf("failed to parse claim closure: %w", err)
	}
	if !ok {
		return nil, fmt.Errorf("invalid claim closure %s", leaf)
	}
	if len(closure.PubKeys) != 2 {
		return nil, fmt.Errorf(
			"invalid claim closure: expected 2 pubkeys, got %d", len(closure.PubKeys),
		)
	}
	return &closure, nil
}

func parseRefundClosure(leaf string) (*script.MultisigClosure, error) {
	buf, err := hex.DecodeString(leaf)
	if err != nil {
		return nil, fmt.Errorf("failed to decode refund closure: %w", err)
	}
	closure := script.MultisigClosure{}
	ok, err := closure.Decode(buf)
	if err != nil {
		return nil, fmt.Errorf("failed to parse refund closure: %w", err)
	}
	if !ok {
		return nil, fmt.Errorf("invalid refund closure %s", leaf)
	}
	if len(closure.PubKeys) != 3 {
		return nil, fmt.Errorf(
			"invalid refund closure: expected 3 pubkeys, got %d", len(closure.PubKeys),
		)
	}
	return &closure, nil
}

func parseRefundWithoutReceiverClosure(leaf string) (*script.CLTVMultisigClosure, error) {
	buf, err := hex.DecodeString(leaf)
	if err != nil {
		return nil, fmt.Errorf("failed to decode refund without receiver closure: %w", err)
	}
	closure := script.CLTVMultisigClosure{}
	ok, err := closure.Decode(buf)
	if err != nil {
		return nil, fmt.Errorf("failed to parse refund without receiver closure: %w", err)
	}
	if !ok {
		return nil, fmt.Errorf("invalid refund without receiver closure %s", leaf)
	}
	if len(closure.PubKeys) != 2 {
		return nil, fmt.Errorf(
			"invalid refund without receiver closure: expected 2 pubkeys, got %d",
			len(closure.PubKeys),
		)
	}
	return &closure, nil
}

func parseUnilateralClaimClosure(leaf string) (*script.ConditionCSVMultisigClosure, error) {
	buf, err := hex.DecodeString(leaf)
	if err != nil {
		return nil, fmt.Errorf("failed to decode unilateral claim closure: %w", err)
	}
	closure := script.ConditionCSVMultisigClosure{}
	ok, err := closure.Decode(buf)
	if err != nil {
		return nil, fmt.Errorf("failed to parse unilateral claim closure: %w", err)
	}
	if !ok {
		return nil, fmt.Errorf("invalid unilateral claim closure")
	}
	if len(closure.PubKeys) != 1 {
		return nil, fmt.Errorf(
			"invalid unilateral claim closure: expected 1 pubkey, got %d", len(closure.PubKeys),
		)
	}
	return &closure, nil
}

func parseUnilateralRefundClosure(leaf string) (*script.CSVMultisigClosure, error) {
	buf, err := hex.DecodeString(leaf)
	if err != nil {
		return nil, fmt.Errorf("failed to decode unilateral refund closure: %w", err)
	}
	closure := script.CSVMultisigClosure{}
	ok, err := closure.Decode(buf)
	if err != nil {
		return nil, fmt.Errorf("failed to parse unilateral refund closure: %w", err)
	}
	if !ok {
		return nil, fmt.Errorf("invalid unilateral refund closure")
	}
	if len(closure.PubKeys) != 2 {
		return nil, fmt.Errorf(
			"invalid unilateral refund closure: expected 2 pubkeys, got %d", len(closure.PubKeys),
		)
	}
	return &closure, nil
}

func parseUnilateralRefundWithoutReceiverClosure(leaf string) (*script.CSVMultisigClosure, error) {
	buf, err := hex.DecodeString(leaf)
	if err != nil {
		return nil, fmt.Errorf(
			"failed to decode unilateral refund without receiver closure: %w", err,
		)
	}
	closure := script.CSVMultisigClosure{}
	ok, err := closure.Decode(buf)
	if err != nil {
		return nil, fmt.Errorf(
			"failed to parse unilateral refund without receiver closure: %w", err,
		)
	}
	if !ok {
		return nil, fmt.Errorf("invalid unilateral refund without receiver closure")
	}
	if len(closure.PubKeys) != 1 {
		return nil, fmt.Errorf(
			"invalid unilateral refund without receiver closure: expected 1 pubkey, got %d",
			len(closure.PubKeys),
		)
	}
	return &closure, nil
}

func makePreimageConditionScript(preimageHash []byte) ([]byte, error) {
	return txscript.NewScriptBuilder().
		AddOp(txscript.OP_HASH160).
		AddData(preimageHash).
		AddOp(txscript.OP_EQUAL).
		Script()
}

func validateTimelock(locktime arklib.RelativeLocktime) error {
	if locktime.Value == 0 {
		return fmt.Errorf("value must be greater than 0")
	}
	if locktime.Type == arklib.LocktimeTypeSecond {
		if locktime.Value < minSecondsTimelock {
			return fmt.Errorf("value in seconds must be at least %d", minSecondsTimelock)
		}
		if locktime.Value%secondsTimelockMultiple != 0 {
			return fmt.Errorf("value in seconds must be a multiple of %d", secondsTimelockMultiple)
		}
	}
	return nil
}
