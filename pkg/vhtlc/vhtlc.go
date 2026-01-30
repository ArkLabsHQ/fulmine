package vhtlc

import (
	"encoding/hex"
	"fmt"

	arklib "github.com/arkade-os/arkd/pkg/ark-lib"
	"github.com/arkade-os/arkd/pkg/ark-lib/script"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcwallet/waddrmgr"
	"github.com/lightningnetwork/lnd/input"
)

const (
	hash160Len              = 20
	sha256Len               = 32
	minSecondsTimelock      = 512
	secondsTimelockMultiple = 512
)

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

// NewVHTLCScriptFromOpts creates a VHTLC VtxoScript from the given options.
func NewVHTLCScriptFromOpts(opts Opts) (*VHTLCScript, error) {
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

func NewVhtlcScript(
	preimageHash, claimLeaf, refundLeaf, refundWithoutReceiverLeaf, unilateralClaimLeaf,
	unilateralRefundLeaf, unilateralRefundWithoutReceiverLeaf string,
) (*VHTLCScript, error) {
	// ClaimPreimage hash
	decodedPreimageHash, err := hex.DecodeString(preimageHash)
	if err != nil {
		return nil, fmt.Errorf("failed to decode preimage hash: %w", err)
	}
	// If the sha256 hash is provided, convert it to hash160
	if len(decodedPreimageHash) == sha256Len {
		decodedPreimageHash = input.Ripemd160H(decodedPreimageHash)
	}
	if len(decodedPreimageHash) != hash160Len {
		return nil, fmt.Errorf(
			"invalid preimage hash length: expected %d, got %d",
			hash160Len, len(decodedPreimageHash),
		)
	}
	preimageCondition, err := makePreimageConditionScript(decodedPreimageHash)
	if err != nil {
		return nil, fmt.Errorf("failed to build condition script: %w", err)
	}

	// Claim path
	claimClosure, err := parseClaimClosure(claimLeaf)
	if err != nil {
		return nil, err
	}

	// Refund path
	refundClosure, err := parseRefundClosure(refundLeaf)
	if err != nil {
		return nil, err
	}

	// Refund without receiver path
	refundWithoutReceiverClosure, err := parseRefundWithoutReceiverClosure(refundWithoutReceiverLeaf)
	if err != nil {
		return nil, err
	}

	// Claim exit path
	unilateralClaimClosure, err := parseUnilateralClaimClosure(unilateralClaimLeaf)
	if err != nil {
		return nil, err
	}

	// Refund exit path
	unilateralRefundClosure, err := parseUnilateralRefundClosure(unilateralRefundLeaf)
	if err != nil {
		return nil, err
	}

	// Refund without receiver exit path
	unilateralRefundWithoutReceiverClosure, err := parseUnilateralRefundWithoutReceiverClosure(
		unilateralRefundWithoutReceiverLeaf,
	)
	if err != nil {
		return nil, err
	}

	// Extract keys from closures
	receiver := unilateralClaimClosure.PubKeys[0]
	sender := unilateralRefundWithoutReceiverClosure.PubKeys[0]
	var server *btcec.PublicKey
	for _, pk := range claimClosure.PubKeys {
		if pk.IsEqual(receiver) {
			continue
		}
		server = pk
	}

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
		Sender:                                 sender,
		Receiver:                               receiver,
		Server:                                 server,
		ClaimClosure:                           claimClosure,
		RefundClosure:                          refundClosure,
		RefundWithoutReceiverClosure:           refundWithoutReceiverClosure,
		UnilateralClaimClosure:                 unilateralClaimClosure,
		UnilateralRefundClosure:                unilateralRefundClosure,
		UnilateralRefundWithoutReceiverClosure: unilateralRefundWithoutReceiverClosure,
		preimageConditionScript:                preimageCondition,
	}, nil
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

func (v *VHTLCScript) Address(hrp string) (string, error) {
	tapKey, _, err := v.TapTree()
	if err != nil {
		return "", err
	}

	addr := &arklib.Address{
		HRP:        hrp,
		Signer:     v.Server,
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

func (v *VHTLCScript) Opts() Opts {
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
