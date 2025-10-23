package application

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"strings"

	arklib "github.com/arkade-os/arkd/pkg/ark-lib"
	"github.com/arkade-os/go-sdk/types"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/btcutil/psbt"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
)

func checkpointExitScript(cfg *types.Config) []byte {
	buf, _ := hex.DecodeString(cfg.CheckpointTapscript)
	return buf
}

func parsePubkey(pubkey string) (*btcec.PublicKey, error) {
	if len(pubkey) <= 0 {
		return nil, nil
	}

	dec, err := hex.DecodeString(pubkey)
	if err != nil {
		return nil, fmt.Errorf("invalid pubkey: %s", err)
	}

	pk, err := btcec.ParsePubKey(dec)
	if err != nil {
		return nil, fmt.Errorf("invalid pubkey: %s", err)
	}

	return pk, nil
}

// verifyInputSignatures checks that all inputs have a signature for the given pubkey
// and the signature is correct for the given tapscript leaf
func verifyInputSignatures(tx *psbt.Packet, pubkey *btcec.PublicKey, tapLeaves map[int]txscript.TapLeaf) error {
	xOnlyPubkey := schnorr.SerializePubKey(pubkey)

	prevouts := make(map[wire.OutPoint]*wire.TxOut)
	sigsToVerify := make(map[int]*psbt.TaprootScriptSpendSig)

	for inputIndex, input := range tx.Inputs {
		// collect previous outputs
		if input.WitnessUtxo == nil {
			return fmt.Errorf("input %d has no witness utxo, cannot verify signature", inputIndex)
		}

		outpoint := tx.UnsignedTx.TxIn[inputIndex].PreviousOutPoint
		prevouts[outpoint] = input.WitnessUtxo

		tapLeaf, ok := tapLeaves[inputIndex]
		if !ok {
			return fmt.Errorf("input %d has no tapscript leaf, cannot verify signature", inputIndex)
		}

		tapLeafHash := tapLeaf.TapHash()

		// check if pubkey has a tapscript sig
		hasSig := false
		for _, sig := range input.TaprootScriptSpendSig {
			if bytes.Equal(sig.XOnlyPubKey, xOnlyPubkey) && bytes.Equal(sig.LeafHash, tapLeafHash[:]) {
				hasSig = true
				sigsToVerify[inputIndex] = sig
				break
			}
		}

		if !hasSig {
			return fmt.Errorf("input %d has no signature for pubkey %x", inputIndex, xOnlyPubkey)
		}
	}

	prevoutFetcher := txscript.NewMultiPrevOutFetcher(prevouts)
	txSigHashes := txscript.NewTxSigHashes(tx.UnsignedTx, prevoutFetcher)

	for inputIndex, sig := range sigsToVerify {
		msgHash, err := txscript.CalcTapscriptSignaturehash(
			txSigHashes,
			sig.SigHash,
			tx.UnsignedTx,
			inputIndex,
			prevoutFetcher,
			tapLeaves[inputIndex],
		)
		if err != nil {
			return fmt.Errorf("failed to calculate tapscript signature hash: %w", err)
		}

		signature, err := schnorr.ParseSignature(sig.Signature)
		if err != nil {
			return fmt.Errorf("failed to parse signature: %w", err)
		}

		if !signature.Verify(msgHash, pubkey) {
			return fmt.Errorf("input %d: invalid signature", inputIndex)
		}
	}

	return nil
}

// GetInputTapLeaves returns a map of input index to tapscript leaf
// if the input has no tapscript leaf, it is not included in the map
func getInputTapLeaves(tx *psbt.Packet) map[int]txscript.TapLeaf {
	tapLeaves := make(map[int]txscript.TapLeaf)
	for inputIndex, input := range tx.Inputs {
		if input.TaprootLeafScript == nil {
			continue
		}
		tapLeaves[inputIndex] = txscript.NewBaseTapLeaf(input.TaprootLeafScript[0].Script)
	}
	return tapLeaves
}

func verifyAndSignCheckpoints(
	signedCheckpoints []string, myCheckpoints []*psbt.Packet,
	arkSigner *btcec.PublicKey, sign func(tx *psbt.Packet) (string, error),
) ([]string, error) {
	finalCheckpoints := make([]string, 0, len(signedCheckpoints))
	for _, checkpoint := range signedCheckpoints {
		signedCheckpointPtx, err := psbt.NewFromRawBytes(strings.NewReader(checkpoint), true)
		if err != nil {
			return nil, err
		}

		// search for the checkpoint tx we initially created
		var myCheckpointTx *psbt.Packet
		for _, chk := range myCheckpoints {
			if chk.UnsignedTx.TxID() == signedCheckpointPtx.UnsignedTx.TxID() {
				myCheckpointTx = chk
				break
			}
		}
		if myCheckpointTx == nil {
			return nil, fmt.Errorf("checkpoint tx not found")
		}

		// verify the server has signed the checkpoint tx
		err = verifyInputSignatures(signedCheckpointPtx, arkSigner, getInputTapLeaves(myCheckpointTx))
		if err != nil {
			return nil, err
		}

		finalCheckpoint, err := sign(signedCheckpointPtx)
		if err != nil {
			return nil, fmt.Errorf("failed to sign checkpoint transaction: %w", err)
		}

		finalCheckpoints = append(finalCheckpoints, finalCheckpoint)
	}

	return finalCheckpoints, nil
}

func verifyFinalArkTx(
	finalArkTx string, arkSigner *btcec.PublicKey, expectedTapLeaves map[int]txscript.TapLeaf,
) error {
	finalArkPtx, err := psbt.NewFromRawBytes(strings.NewReader(finalArkTx), true)
	if err != nil {
		return err
	}

	// verify that the ark signer has signed the ark tx
	err = verifyInputSignatures(finalArkPtx, arkSigner, expectedTapLeaves)
	if err != nil {
		return err
	}

	return nil
}

func offchainAddressesPkScripts(addresses []string) ([]string, error) {
	scripts := make([]string, 0, len(addresses))
	for _, addr := range addresses {
		decodedAddress, err := arklib.DecodeAddressV0(addr)
		if err != nil {
			return nil, fmt.Errorf("failed to decode address %s: %w", addr, err)
		}

		p2trScript, err := txscript.PayToTaprootScript(decodedAddress.VtxoTapKey)
		if err != nil {
			return nil, fmt.Errorf("failed to parse address to p2tr script: %w", err)
		}

		scripts = append(scripts, hex.EncodeToString(p2trScript))
	}
	return scripts, nil
}

func onchainAddressesPkScripts(addresses []string, network arklib.Network) ([]string, error) {
	scripts := make([]string, 0, len(addresses))
	for _, addr := range addresses {
		btcAddress, err := btcutil.DecodeAddress(addr, toBitcoinNetwork(network))
		if err != nil {
			return nil, fmt.Errorf("failed to decode address %s: %w", addr, err)
		}

		script, err := txscript.PayToAddrScript(btcAddress)
		if err != nil {
			return nil, fmt.Errorf("failed to parse address to p2tr script: %w", err)
		}
		scripts = append(scripts, hex.EncodeToString(script))
	}
	return scripts, nil
}

func toBitcoinNetwork(net arklib.Network) *chaincfg.Params {
	switch net.Name {
	case arklib.Bitcoin.Name:
		return &chaincfg.MainNetParams
	case arklib.BitcoinTestNet.Name:
		return &chaincfg.TestNet3Params
	//case arklib.BitcoinTestNet4.Name: //TODO uncomment once supported
	//	return chaincfg.TestNet4Params
	case arklib.BitcoinSigNet.Name:
		return &chaincfg.SigNetParams
	case arklib.BitcoinMutinyNet.Name:
		return &arklib.MutinyNetSigNetParams
	case arklib.BitcoinRegTest.Name:
		return &chaincfg.RegressionNetParams
	default:
		return &chaincfg.MainNetParams
	}
}

func deriveTimelock(timelock uint32) arklib.RelativeLocktime {
	if timelock >= 512 {
		return arklib.RelativeLocktime{Type: arklib.LocktimeTypeSecond, Value: timelock}
	}

	return arklib.RelativeLocktime{Type: arklib.LocktimeTypeBlock, Value: timelock}
}

type internalScriptsStore []string

func (s internalScriptsStore) Get(ctx context.Context) ([]string, error) {
	return s, nil
}

func (s internalScriptsStore) Add(ctx context.Context, scripts []string) (int, error) {
	return 0, fmt.Errorf("cannot add scripts to internal subscription")
}

func (s internalScriptsStore) Delete(ctx context.Context, scripts []string) (int, error) {
	return 0, fmt.Errorf("cannot delete scripts from internal subscription")
}
