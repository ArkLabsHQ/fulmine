package swap

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"math"

	"github.com/ArkLabsHQ/fulmine/pkg/boltz"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	"github.com/lightningnetwork/lnd/lntypes"
)

type ClaimTransactionParams struct {
	LockupTxid      string
	LockupVout      uint32
	LockupAmount    uint64
	DestinationAddr string
	Network         *chaincfg.Params
}

// constructClaimTransaction creates a transaction to claim BTC lockup
// This constructs a bare transaction skeleton that will be signed with MuSig2 (key path)
// or Schnorr signature (script path)
func constructClaimTransaction(
	explorerClient ExplorerClient,
	dustAmount uint64,
	params ClaimTransactionParams,
) (*wire.MsgTx, error) {
	lockupHash, err := chainhash.NewHashFromStr(params.LockupTxid)
	if err != nil {
		return nil, fmt.Errorf("invalid lockup txid: %w", err)
	}

	destAddr, err := btcutil.DecodeAddress(params.DestinationAddr, params.Network)
	if err != nil {
		return nil, fmt.Errorf("invalid destination address: %w", err)
	}

	tx := wire.NewMsgTx(2)

	tx.AddTxIn(&wire.TxIn{
		PreviousOutPoint: wire.OutPoint{
			Hash:  *lockupHash,
			Index: params.LockupVout,
		},
		Sequence: wire.MaxTxInSequenceNum,
	})

	pkScript, err := payToAddrScript(destAddr)
	if err != nil {
		return nil, fmt.Errorf("failed to create output script: %w", err)
	}

	tx.AddTxOut(&wire.TxOut{
		Value:    int64(params.LockupAmount),
		PkScript: pkScript,
	})

	vbytes := computeVSize(tx)

	feeRate, err := explorerClient.GetFeeRate()
	if err != nil {
		return nil, err
	}

	feeAmount := uint64(math.Ceil(float64(vbytes)*feeRate) + 100)
	if params.LockupAmount-feeAmount <= dustAmount {
		return nil, fmt.Errorf("not enough funds to cover network fees")
	}

	tx.TxOut[0].Value = int64(params.LockupAmount - feeAmount)

	return tx, nil
}

func computeVSize(tx *wire.MsgTx) lntypes.VByte {
	baseSize := tx.SerializeSizeStripped()
	totalSize := tx.SerializeSize() // including witness
	weight := totalSize + baseSize*3
	return lntypes.WeightUnit(uint64(weight)).ToVB()
}

func payToAddrScript(addr btcutil.Address) ([]byte, error) {
	switch addr.(type) {
	case *btcutil.AddressWitnessPubKeyHash,
		*btcutil.AddressWitnessScriptHash,
		*btcutil.AddressTaproot:
		// Witness addresses supported
		return txscript.PayToAddrScript(addr)
	default:
		return nil, fmt.Errorf("unsupported address type: %T", addr)
	}
}

func serializeTransaction(tx *wire.MsgTx) (string, error) {
	var buf bytes.Buffer
	if err := tx.Serialize(&buf); err != nil {
		return "", fmt.Errorf("failed to serialize transaction: %w", err)
	}
	return hex.EncodeToString(buf.Bytes()), nil
}

func deserializeTransaction(txHex string) (*wire.MsgTx, error) {
	txBytes, err := hex.DecodeString(txHex)
	if err != nil {
		return nil, fmt.Errorf("invalid hex: %w", err)
	}

	tx := wire.NewMsgTx(2)
	if err := tx.Deserialize(bytes.NewReader(txBytes)); err != nil {
		return nil, fmt.Errorf("failed to deserialize transaction: %w", err)
	}

	return tx, nil
}

func findOutputForAddress(tx *wire.MsgTx, address string, network *chaincfg.Params) (uint32, uint64, error) {
	if tx == nil {
		return 0, 0, fmt.Errorf("tx is nil")
	}
	if address == "" {
		return 0, 0, fmt.Errorf("address is empty")
	}
	if network == nil {
		return 0, 0, fmt.Errorf("network is nil")
	}

	addr, err := btcutil.DecodeAddress(address, network)
	if err != nil {
		return 0, 0, fmt.Errorf("decode address: %w", err)
	}

	expectedPkScript, err := payToAddrScript(addr)
	if err != nil {
		return 0, 0, fmt.Errorf("address script: %w", err)
	}

	for i, out := range tx.TxOut {
		if bytes.Equal(out.PkScript, expectedPkScript) {
			if out.Value <= 0 {
				return 0, 0, fmt.Errorf("matched output %d has non-positive value %d", i, out.Value)
			}
			return uint32(i), uint64(out.Value), nil
		}
	}

	return 0, 0, fmt.Errorf("address output not found in tx")
}

func computeSwapTreeMerkleRoot(tree boltz.SwapTree) ([]byte, error) {
	claimScript, err := hex.DecodeString(tree.ClaimLeaf.Output)
	if err != nil {
		return nil, fmt.Errorf("decode claim leaf script: %w", err)
	}
	refundScript, err := hex.DecodeString(tree.RefundLeaf.Output)
	if err != nil {
		return nil, fmt.Errorf("decode refund leaf script: %w", err)
	}

	claimLeafHash := tapLeafHash(tree.ClaimLeaf.Version, claimScript)
	refundLeafHash := tapLeafHash(tree.RefundLeaf.Version, refundScript)

	h := computeMerkleRoot(claimLeafHash[:], refundLeafHash[:])
	return h[:], nil
}

func computeMerkleRoot(claimLeafHash, refundLeafHash []byte) []byte {
	left, right := claimLeafHash[:], refundLeafHash[:]
	if bytes.Compare(left, right) > 0 {
		left, right = right, left
	}

	branch := append(append([]byte{}, left...), right...)
	h := chainhash.TaggedHash(chainhash.TagTapBranch, branch)
	return h[:]
}

func tapLeafHash(leafVersion uint8, script []byte) [32]byte {
	var b bytes.Buffer
	b.WriteByte(leafVersion)
	_ = wire.WriteVarInt(&b, 0, uint64(len(script)))
	b.Write(script)
	sum := chainhash.TaggedHash(chainhash.TagTapLeaf, b.Bytes())

	return *sum
}

func createControlBlockFromSwapTree(
	internalKey *btcec.PublicKey,
	swapTree boltz.SwapTree,
	isClaimPath bool,
) ([]byte, error) {
	claimScript, err := hex.DecodeString(swapTree.ClaimLeaf.Output)
	if err != nil {
		return nil, fmt.Errorf("failed to decode claim script: %w", err)
	}

	refundScript, err := hex.DecodeString(swapTree.RefundLeaf.Output)
	if err != nil {
		return nil, fmt.Errorf("failed to decode refund script: %w", err)
	}

	claimLeaf := txscript.NewBaseTapLeaf(claimScript)
	refundLeaf := txscript.NewBaseTapLeaf(refundScript)

	var siblingLeaf txscript.TapLeaf
	if isClaimPath {
		siblingLeaf = refundLeaf
	} else {
		siblingLeaf = claimLeaf
	}

	merkleRoot, err := computeSwapTreeMerkleRoot(swapTree)
	if err != nil {
		return nil, fmt.Errorf("failed to compute merkle root: %w", err)
	}

	tweakedKey := txscript.ComputeTaprootOutputKey(internalKey, merkleRoot)
	parity := tweakedKey.SerializeCompressed()[0] & 0x01
	internalKeyBytes := internalKey.SerializeCompressed()[1:]
	siblingHash := siblingLeaf.TapHash()
	controlBlock := make([]byte, 0, 1+32+32)
	leafVersionByte := byte(txscript.BaseLeafVersion) | parity
	controlBlock = append(controlBlock, leafVersionByte)
	controlBlock = append(controlBlock, internalKeyBytes...)
	controlBlock = append(controlBlock, siblingHash[:]...)

	return controlBlock, nil
}
