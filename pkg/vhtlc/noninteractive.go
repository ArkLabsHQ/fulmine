package vhtlc

import (
	"fmt"

	"github.com/ArkLabsHQ/introspector/pkg/arkade"
	"github.com/arkade-os/arkd/pkg/ark-lib/script"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/txscript"
)

const p2trPkScriptLen = 34

// NonInteractiveClaimOpts enables the non-interactive claim covenant closure
// on a VHTLC. When set, NewVHTLCScriptFromOpts appends a ConditionMultisigClosure
// that lets a solver bot claim the VHTLC on the receiver's behalf
// by revealing the preimage. The solver does not need any signature from the
// receiver — it satisfies the introspector-tweaked multisig via the
// EnforcePayTo arkade script. 
// Thus, the receiver knows that the output script of the claim is going to its wallet
type NonInteractiveClaimOpts struct {
	// ReceiverPkScript is the 34-byte P2TR pkScript of the VHTLC receiver.
	ReceiverPkScript []byte
	// IntrospectorPubKey is the solver's introspector signing key (compressed).
	// it will be tweaked with the non interactive claim arkade script.
	IntrospectorPubKey *btcec.PublicKey
}

func (o NonInteractiveClaimOpts) validate() error {
	if len(o.ReceiverPkScript) != p2trPkScriptLen {
		return fmt.Errorf(
			"non-interactive claim: receiver pkScript must be %d bytes", p2trPkScriptLen,
		)
	}
	if o.ReceiverPkScript[0] != txscript.OP_1 || o.ReceiverPkScript[1] != txscript.OP_DATA_32 {
		return fmt.Errorf("non-interactive claim: receiver pkScript is not P2TR")
	}
	if o.IntrospectorPubKey == nil {
		return fmt.Errorf("non-interactive claim: introspector pubkey must not be nil")
	}
	return nil
}

// enforcePayTo builds the arkade enforcement script pinning output[i] to
// receiverPkScript with output[i].value >= input[i].value.
func enforcePayTo(receiverPkScript []byte) ([]byte, error) {
	if len(receiverPkScript) != p2trPkScriptLen {
		return nil, fmt.Errorf(
			"expected %d-byte P2TR pkScript, got %d", p2trPkScriptLen, len(receiverPkScript),
		)
	}
	witnessProgram := receiverPkScript[2:]
	b := txscript.NewScriptBuilder()
	b.AddOp(arkade.OP_PUSHCURRENTINPUTINDEX)
	b.AddOp(arkade.OP_DUP)
	b.AddOp(arkade.OP_INSPECTOUTPUTSCRIPTPUBKEY)
	b.AddOp(arkade.OP_1)
	b.AddOp(arkade.OP_EQUALVERIFY)
	b.AddData(witnessProgram)
	b.AddOp(arkade.OP_EQUALVERIFY)
	b.AddOp(arkade.OP_INSPECTOUTPUTVALUE)
	b.AddOp(arkade.OP_PUSHCURRENTINPUTINDEX)
	b.AddOp(arkade.OP_INSPECTINPUTVALUE)
	b.AddOp(arkade.OP_GREATERTHANOREQUAL)
	return b.Script()
}

// introspectorTweakedKey returns the introspector pubkey tweaked by the
// arkade script hash. It is the second pubkey in the non-interactive claim
// multisig.
func introspectorTweakedKey(
	arkadeScript []byte, introspectorPubKey *btcec.PublicKey,
) *btcec.PublicKey {
	return arkade.ComputeArkadeScriptPublicKey(
		introspectorPubKey, arkade.ArkadeScriptHash(arkadeScript),
	)
}

// nonInteractiveClaimClosure builds the ConditionMultisigClosure used by the
// solver to claim the VHTLC unilaterally with the preimage.
func nonInteractiveClaimClosure(
	preimageCondition []byte,
	opts NonInteractiveClaimOpts,
	serverPubKey *btcec.PublicKey,
) (*script.ConditionMultisigClosure, error) {
	enforcement, err := enforcePayTo(opts.ReceiverPkScript)
	if err != nil {
		return nil, err
	}
	tweaked := introspectorTweakedKey(enforcement, opts.IntrospectorPubKey)
	return &script.ConditionMultisigClosure{
		MultisigClosure: script.MultisigClosure{
			PubKeys: []*btcec.PublicKey{serverPubKey, tweaked},
		},
		Condition: preimageCondition,
	}, nil
}
