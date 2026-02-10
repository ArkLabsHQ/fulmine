package swap

import (
	"encoding/hex"
	"fmt"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/btcsuite/btcd/btcec/v2/schnorr/musig2"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
)

// MuSigContext holds just what we need for the Boltz cooperative (2-of-2) MuSig2 flow.
// We intentionally DO NOT use musig2.Session API here to avoid tweak/sort/session mismatch
// and to match the proven working pattern from your tree signer code:
//
// - GenNonces (keep SecNonce + PubNonce)
// - send PubNonce to Boltz
// - receive their PubNonce + their partial sig (S-only 32B scalar)
// - AggregateNonces
// - musig2.Sign(... WithTaprootSignTweak(merkleRoot) ...)
// - musig2.CombineSigs(... WithTaprootTweakedCombine(...))
type MuSigContext struct {
	privateKey     *btcec.PrivateKey
	publicKey      *btcec.PublicKey
	theirPublicKey *btcec.PublicKey

	ourNonces *musig2.Nonces
}

// NewMuSigContext creates a MuSig2 context for 2-of-2 signing.
// IMPORTANT: ordering must match Boltz expectation. We keep "their key first" in Keys().
func NewMuSigContext(ourPriv *btcec.PrivateKey, theirPub *btcec.PublicKey) (*MuSigContext, error) {
	if ourPriv == nil {
		return nil, fmt.Errorf("nil private key")
	}
	if theirPub == nil {
		return nil, fmt.Errorf("nil their public key")
	}

	return &MuSigContext{
		privateKey:     ourPriv,
		publicKey:      ourPriv.PubKey(),
		theirPublicKey: theirPub,
	}, nil
}

// Keys returns the signer keyset in the canonical order used in this swap flow.
// We intentionally return [their, ours] because you noted Boltz expects server first.
func (c *MuSigContext) Keys() []*btcec.PublicKey {
	return []*btcec.PublicKey{c.theirPublicKey, c.publicKey}
}

// GenerateNonce generates a fresh MuSig2 nonce pair (secret+public).
// WARNING: Never reuse c.ourNonces.SecNonce across different messages/txs.
func (c *MuSigContext) GenerateNonce() ([66]byte, error) {
	nonces, err := musig2.GenNonces(
		musig2.WithPublicKey(c.publicKey),
	)
	if err != nil {
		return [66]byte{}, fmt.Errorf("musig2.GenNonces: %w", err)
	}

	c.ourNonces = nonces
	return nonces.PubNonce, nil
}

// AggregateNonces aggregates our pubnonce and their pubnonce.
func (c *MuSigContext) AggregateNonces(theirNonce [66]byte) ([66]byte, error) {
	if c.ourNonces == nil {
		return [66]byte{}, fmt.Errorf("nonce not generated")
	}

	combined, err := musig2.AggregateNonces([][66]byte{
		c.ourNonces.PubNonce,
		theirNonce,
	})
	if err != nil {
		return [66]byte{}, fmt.Errorf("musig2.AggregateNonces: %w", err)
	}

	return combined, nil
}

// TaprootMessage computes the BIP341 sighash (32 bytes) for key-path signing.
func TaprootMessage(
	tx *wire.MsgTx,
	inputIndex int,
	prevOutFetcher txscript.PrevOutputFetcher,
) ([32]byte, error) {
	if tx == nil {
		return [32]byte{}, fmt.Errorf("nil tx")
	}
	if prevOutFetcher == nil {
		return [32]byte{}, fmt.Errorf("nil prevOutFetcher")
	}
	if inputIndex < 0 || inputIndex >= len(tx.TxIn) {
		return [32]byte{}, fmt.Errorf("inputIndex out of range")
	}

	sigHashes := txscript.NewTxSigHashes(tx, prevOutFetcher)
	msg32, err := txscript.CalcTaprootSignatureHash(
		sigHashes,
		txscript.SigHashDefault,
		tx,
		inputIndex,
		prevOutFetcher,
	)
	if err != nil {
		return [32]byte{}, fmt.Errorf("CalcTaprootSignatureHash: %w", err)
	}

	var msg [32]byte
	copy(msg[:], msg32)
	return msg, nil
}

// OurPartialSign produces our partial signature for the given message,
// using the aggregated nonce and Taproot tweak (merkleRoot/scriptRoot).
//
// merkleRoot MUST be 32 bytes (taproot script root).
func (c *MuSigContext) OurPartialSign(
	combinedNonce [66]byte,
	keys []*btcec.PublicKey,
	msg [32]byte,
	merkleRoot []byte,
) (*musig2.PartialSignature, error) {
	if c.ourNonces == nil {
		return nil, fmt.Errorf("nonce not generated")
	}
	if len(keys) == 0 {
		return nil, fmt.Errorf("empty key set")
	}
	if len(merkleRoot) != 32 {
		return nil, fmt.Errorf("invalid merkleRoot len: got %d want 32", len(merkleRoot))
	}

	// This mirrors your working tree code.
	ps, err := musig2.Sign(
		c.ourNonces.SecNonce,
		c.privateKey,
		combinedNonce,
		keys,
		msg,
		musig2.WithTaprootSignTweak(merkleRoot), // critical: must match address/output key tweak
		musig2.WithFastSign(),
	)
	if err != nil {
		return nil, fmt.Errorf("musig2.Sign: %w", err)
	}

	return ps, nil
}

// CombineFinalSig combines two partial signatures into a final Schnorr signature.
// Uses taproot tweaked combine so verification is against the tweaked output key.
func CombineFinalSig(
	combinedNoncePoint *btcec.PublicKey,
	allSigs []*musig2.PartialSignature,
	keys []*btcec.PublicKey,
	msg [32]byte,
	merkleRoot []byte,
) (*schnorr.Signature, error) {
	if combinedNoncePoint == nil {
		return nil, fmt.Errorf("nil combined nonce point")
	}
	if len(allSigs) == 0 {
		return nil, fmt.Errorf("no partial sigs")
	}
	if len(keys) == 0 {
		return nil, fmt.Errorf("empty key set")
	}
	if len(merkleRoot) != 32 {
		return nil, fmt.Errorf("invalid merkleRoot len: got %d want 32", len(merkleRoot))
	}

	// Mirrors your tree combine code.
	sig := musig2.CombineSigs(
		combinedNoncePoint,
		allSigs,
		musig2.WithTaprootTweakedCombine(msg, keys, merkleRoot, false),
	)

	if sig == nil {
		return nil, fmt.Errorf("CombineSigs returned nil")
	}

	return sig, nil
}

// ComputeTweakedOutputKey computes the P2TR output key for {keys, merkleRoot}.
// Useful for debug verification before broadcast.
func ComputeTweakedOutputKey(keys []*btcec.PublicKey, merkleRoot []byte) (*btcec.PublicKey, error) {
	if len(keys) == 0 {
		return nil, fmt.Errorf("empty key set")
	}
	if len(merkleRoot) != 32 {
		return nil, fmt.Errorf("invalid merkleRoot len: got %d want 32", len(merkleRoot))
	}

	agg, _, _, err := musig2.AggregateKeys(keys, false)
	if err != nil {
		return nil, fmt.Errorf("AggregateKeys: %w", err)
	}

	// This matches how your lockup output key is derived:
	// outputKey = internalKey + H_tapTweak(internalKey || merkleRoot)*G
	outKey := txscript.ComputeTaprootOutputKey(agg.FinalKey, merkleRoot)
	return outKey, nil
}

// VerifyFinalSig verifies against the tweaked output key (recommended debug gate).
func VerifyFinalSig(msg [32]byte, finalSig *schnorr.Signature, tweakedOutputKey *btcec.PublicKey) error {
	if finalSig == nil {
		return fmt.Errorf("nil finalSig")
	}
	if tweakedOutputKey == nil {
		return fmt.Errorf("nil tweakedOutputKey")
	}
	if !finalSig.Verify(msg[:], tweakedOutputKey) {
		return fmt.Errorf("final signature verify failed")
	}
	return nil
}

// --- Nonce helpers ---

func ParsePubNonce(nonceHex string) ([66]byte, error) {
	if len(nonceHex) != 132 { // 66 bytes * 2
		return [66]byte{}, fmt.Errorf("invalid nonce length: got %d want 132 hex chars", len(nonceHex))
	}
	b, err := hex.DecodeString(nonceHex)
	if err != nil {
		return [66]byte{}, fmt.Errorf("decode nonce hex: %w", err)
	}
	var n [66]byte
	copy(n[:], b)
	return n, nil
}

func SerializePubNonce(nonce [66]byte) string {
	return hex.EncodeToString(nonce[:])
}

// --- Partial signature helpers (Boltz format) ---

// ParsePartialSignatureScalar32 parses Boltz partial sig format: 32-byte scalar S (hex).
// This is NOT musig2.PartialSignature encoding; do not call sig.Decode for this format.
func ParsePartialSignatureScalar32(sigHex string) (*musig2.PartialSignature, error) {
	b, err := hex.DecodeString(sigHex)
	if err != nil {
		return nil, fmt.Errorf("decode partial sig hex: %w", err)
	}
	if len(b) != 32 {
		return nil, fmt.Errorf("invalid partial sig len: got %d want 32", len(b))
	}

	ps := &musig2.PartialSignature{
		S: new(btcec.ModNScalar),
		// R is nil; that's fine (combined nonce point comes from ourPartial.R)
	}

	if overflow := ps.S.SetByteSlice(b); overflow {
		return nil, fmt.Errorf("partial sig scalar overflow")
	}

	return ps, nil
}

func NewPrevOutputFetcher(prevOut *wire.TxOut, prevOutPoint wire.OutPoint) txscript.PrevOutputFetcher {
	return txscript.NewMultiPrevOutFetcher(map[wire.OutPoint]*wire.TxOut{
		prevOutPoint: prevOut,
	})
}
