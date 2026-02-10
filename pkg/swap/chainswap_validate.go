package swap

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"

	"github.com/ArkLabsHQ/fulmine/pkg/boltz"
	"github.com/ArkLabsHQ/fulmine/pkg/vhtlc"
	arklib "github.com/arkade-os/arkd/pkg/ark-lib"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/btcsuite/btcd/btcec/v2/schnorr/musig2"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/txscript"
)

func validateVHTLC(
	ctx context.Context,
	h *SwapHandler,
	isArkToBtc bool,
	swapResp *boltz.CreateChainSwapResponse,
	preimageHashHASH160 []byte,
) (*vhtlc.Opts, error) {
	var (
		vhtlcAddr                                                                         string
		receiverKey, senderKey                                                            *btcec.PublicKey
		refundLocktime                                                                    arklib.AbsoluteLocktime
		unilateralClaimDelay, unilateralRefundDelay, unilateralRefundWithoutReceiverDelay arklib.RelativeLocktime
	)

	if isArkToBtc {
		vhtlcAddr = swapResp.LockupDetails.LockupAddress
		boltzReceiverKey, err := parsePubkey(swapResp.LockupDetails.ServerPublicKey)
		if err != nil {
			return nil, fmt.Errorf("invalid Boltz claim public key: %w", err)
		}

		receiverKey = boltzReceiverKey
		refundLocktime = arklib.AbsoluteLocktime(swapResp.LockupDetails.Timeouts.Refund)
		unilateralClaimDelay = parseLocktime(uint32(swapResp.LockupDetails.Timeouts.UnilateralClaim))
		unilateralRefundDelay = parseLocktime(uint32(swapResp.LockupDetails.Timeouts.UnilateralRefund))
		unilateralRefundWithoutReceiverDelay = parseLocktime(uint32(swapResp.LockupDetails.Timeouts.UnilateralRefundWithoutReceiver))
	} else {
		vhtlcAddr = swapResp.ClaimDetails.LockupAddress
		boltzSenderKey, err := parsePubkey(swapResp.ClaimDetails.ServerPublicKey)
		if err != nil {
			return nil, fmt.Errorf("invalid Boltz claim public key: %w", err)
		}

		senderKey = boltzSenderKey
		receiverKey = nil
		refundLocktime = arklib.AbsoluteLocktime(swapResp.ClaimDetails.Timeouts.Refund)
		unilateralClaimDelay = parseLocktime(uint32(swapResp.ClaimDetails.Timeouts.UnilateralClaim))
		unilateralRefundDelay = parseLocktime(uint32(swapResp.ClaimDetails.Timeouts.UnilateralRefund))
		unilateralRefundWithoutReceiverDelay = parseLocktime(uint32(swapResp.ClaimDetails.Timeouts.UnilateralRefundWithoutReceiver))
	}

	vhtlcAddress, _, vhtlcOpts, err := h.getVHTLC(
		ctx,
		receiverKey,
		senderKey,
		preimageHashHASH160[:],
		refundLocktime,
		unilateralClaimDelay,
		unilateralRefundDelay,
		unilateralRefundWithoutReceiverDelay,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to compute VHTLC: %w", err)
	}

	if vhtlcAddr != vhtlcAddress {
		return nil, fmt.Errorf(
			"VHTLC address mismatch - potential scam!\nExpected: %s\nGot: %s",
			vhtlcAddress,
			vhtlcAddr,
		)
	}
	return vhtlcOpts, nil
}

// HTLCComponents contains the parsed components from a Boltz HTLC script
type HTLCComponents struct {
	PreimageHash [20]byte // HASH160 of the preimage (20 bytes)
	ClaimPubKey  [32]byte // X-only public key for claim (32 bytes)
}

// RefundHTLCComponents contains the parsed components from a Boltz HTLC refund leaf script
type RefundHTLCComponents struct {
	RefundPubKey [32]byte // X-only public key for refund (32 bytes)
	Timeout      uint32   // CSV timeout in blocks
}

// validateClaimLeafScript parses and validates a Boltz HTLC claim leaf script.
// The expected structure follows Boltz's HTLC format:
//
//	OP_SIZE
//	OP_PUSHBYTES_1 0x20
//	OP_EQUALVERIFY
//	OP_HASH160
//	OP_PUSHBYTES_20 <preimage_hash>
//	OP_EQUALVERIFY
//	OP_PUSHBYTES_32 <claim_pubkey>
//	OP_CHECKSIG
//
// Example:
//
//	output: "82012088a914608bc8a727928e8aa18c7a2489c003deb47ff08388207599756afc49ebf5a6f3ac5848ef0afe934edd7b669bca02029acf10cc7f83acac"
//	-> preimageHash: 608bc8a727928e8aa18c7a2489c003deb47ff083
//	-> claimPubKey: 7599756afc49ebf5a6f3ac5848ef0afe934edd7b669bca02029acf10cc7f83ac
func validateClaimLeafScript(outputHex string) (*HTLCComponents, error) {
	scriptBytes, err := hex.DecodeString(outputHex)
	if err != nil {
		return nil, fmt.Errorf("failed to decode claim leaf output hex: %w", err)
	}

	components, err := parseHTLCScriptManually(scriptBytes)
	if err != nil {
		return nil, err
	}

	return components, nil
}

// script components (opcodes, push operations, fixed-length data).
type scriptParser struct {
	buf *bytes.Reader
}

// newScriptParser creates a new scriptParser for the given script bytes.
func newScriptParser(script []byte) *scriptParser {
	return &scriptParser{buf: bytes.NewReader(script)}
}

// expectOpcode reads a byte and verifies it matches the expected opcode.
// Returns an error if reading fails or the byte doesn't match.
func (p *scriptParser) expectOpcode(expected byte, name string) error {
	got, err := p.buf.ReadByte()
	if err != nil {
		return fmt.Errorf("failed to read %s: %w", name, err)
	}
	if got != expected {
		return fmt.Errorf("expected %s (0x%x), got 0x%x", name, expected, got)
	}
	return nil
}

// expectPush reads a byte and verifies it matches the expected push length.
// Returns an error if reading fails or the push length doesn't match.
func (p *scriptParser) expectPush(expectedLen byte, name string) error {
	got, err := p.buf.ReadByte()
	if err != nil {
		return fmt.Errorf("failed to read push length for %s: %w", name, err)
	}
	if got != expectedLen {
		return fmt.Errorf("expected push length 0x%x for %s, got 0x%x", expectedLen, name, got)
	}
	return nil
}

// readFixedBytes reads exactly n bytes and returns them.
// Returns an error if reading fails or fewer than n bytes are available.
func (p *scriptParser) readFixedBytes(n int, name string) ([]byte, error) {
	data := make([]byte, n)
	read, err := p.buf.Read(data)
	if err != nil {
		return nil, fmt.Errorf("failed to read %s: %w", name, err)
	}
	if read != n {
		return nil, fmt.Errorf("expected %d bytes for %s, got %d", n, name, read)
	}
	return data, nil
}

// expectNoMoreBytes verifies that no bytes remain in the buffer.
// Returns an error if extra bytes are present.
func (p *scriptParser) expectNoMoreBytes() error {
	if p.buf.Len() != 0 {
		return fmt.Errorf("unexpected extra bytes at end of script: %d bytes remaining", p.buf.Len())
	}
	return nil
}

// parseHTLCScriptManually manually parses the HTLC script byte by byte
// to extract preimage hash and claim pubkey with full validation.
func parseHTLCScriptManually(script []byte) (*HTLCComponents, error) {
	if len(script) < 57 {
		// Minimum: 1 (OP_SIZE) + 2 (push 0x20) + 1 (OP_EQUALVERIFY) + 1 (OP_HASH160) + 21 (push 20 bytes) + 1 (OP_EQUALVERIFY) + 33 (push 32 bytes) + 1 (OP_CHECKSIG) = 61 bytes
		// But actual encoding might be slightly shorter, let's check minimum reasonable size
		return nil, fmt.Errorf("script too short: expected at least 57 bytes, got %d", len(script))
	}

	p := newScriptParser(script)

	// Read OP_SIZE (0x82)
	if err := p.expectOpcode(txscript.OP_SIZE, "OP_SIZE"); err != nil {
		return nil, err
	}

	// Read push of 0x20 (1 byte indicating 32)
	if err := p.expectPush(0x01, "preimage size length"); err != nil {
		return nil, err
	}

	preimageSize, err := p.readFixedBytes(1, "preimage size")
	if err != nil {
		return nil, err
	}
	if preimageSize[0] != 0x20 {
		return nil, fmt.Errorf("expected preimage size 0x20 (32 bytes), got 0x%x", preimageSize[0])
	}

	// Read OP_EQUALVERIFY (0x88)
	if err := p.expectOpcode(txscript.OP_EQUALVERIFY, "OP_EQUALVERIFY"); err != nil {
		return nil, err
	}

	// Read OP_HASH160 (0xa9)
	if err := p.expectOpcode(txscript.OP_HASH160, "OP_HASH160"); err != nil {
		return nil, err
	}

	// Read push of 20 bytes (preimage hash)
	if err := p.expectPush(0x14, "preimage hash"); err != nil {
		return nil, err
	}

	preimageHashBytes, err := p.readFixedBytes(20, "preimage hash")
	if err != nil {
		return nil, err
	}

	// Read second OP_EQUALVERIFY (0x88)
	if err := p.expectOpcode(txscript.OP_EQUALVERIFY, "second OP_EQUALVERIFY"); err != nil {
		return nil, err
	}

	// Read push of 32 bytes (claim pubkey)
	if err := p.expectPush(0x20, "claim pubkey"); err != nil {
		return nil, err
	}

	claimPubKeyBytes, err := p.readFixedBytes(32, "claim pubkey")
	if err != nil {
		return nil, err
	}

	// Read OP_CHECKSIG (0xac)
	if err := p.expectOpcode(txscript.OP_CHECKSIG, "OP_CHECKSIG"); err != nil {
		return nil, err
	}

	// Verify no extra bytes
	if err := p.expectNoMoreBytes(); err != nil {
		return nil, err
	}

	var preimageHash [20]byte
	copy(preimageHash[:], preimageHashBytes)

	var claimPubKey [32]byte
	copy(claimPubKey[:], claimPubKeyBytes)

	return &HTLCComponents{
		PreimageHash: preimageHash,
		ClaimPubKey:  claimPubKey,
	}, nil
}

// validateBtcClaimOrRefundPossible validates the Bitcoin HTLC (swap tree) parameters for a chain swap.
// It supports both claim path and refund path validation based on the arkToBtc parameter.
//
// When arkToBtc is true (Ark → BTC swap):
//   - Validates the claim path: user can claim BTC using preimage
//   - Requires: serverPubKeyHex, claimPubKey, preimageHash
//   - refundPubKey and expectedTimeout are ignored
//
// When arkToBtc is false (BTC → Ark swap):
//   - Validates the refund path: user can refund BTC after timeout
//   - Requires: refundPubKey, expectedTimeout
//   - serverPubKeyHex, claimPubKey, and preimageHash are ignored
func validateBtcClaimOrRefundPossible(
	swapTree boltz.SwapTree,
	arkToBtc bool,
	serverPubKeyHex string,
	claimPubKey *btcec.PublicKey,
	preimageHash []byte,
	refundPubKey *btcec.PublicKey,
	expectedTimeout uint32,
) error {
	if arkToBtc {
		// Validate claim path - user claims BTC from Boltz
		return validateClaimPath(swapTree, serverPubKeyHex, claimPubKey, preimageHash)
	}
	return validateRefundPath(swapTree, refundPubKey, expectedTimeout)
}

// validateClaimPath validates the Bitcoin HTLC claim path.
// This validates the taproot script structure, public keys, and script validity for the claim leaf.
func validateClaimPath(
	swapTree boltz.SwapTree,
	serverPubKeyHex string,
	claimPubKey *btcec.PublicKey,
	preimageHash []byte,
) error {
	if err := validateSwapTree(swapTree); err != nil {
		return fmt.Errorf("invalid swap tree: %w", err)
	}

	components, err := validateClaimLeafScript(swapTree.ClaimLeaf.Output)
	if err != nil {
		return fmt.Errorf("invalid claim leaf HTLC script: %w", err)
	}

	if serverPubKeyHex == "" {
		return fmt.Errorf("server public key is empty")
	}
	serverPubKeyBytes, err := hex.DecodeString(serverPubKeyHex)
	if err != nil {
		return fmt.Errorf("invalid server public key hex: %w", err)
	}
	serverPubKey, err := btcec.ParsePubKey(serverPubKeyBytes)
	if err != nil {
		return fmt.Errorf("invalid server public key: %w", err)
	}

	if len(serverPubKey.SerializeCompressed()) != 33 {
		return fmt.Errorf("server public key must be 33 bytes compressed, got %d", len(serverPubKey.SerializeCompressed()))
	}

	if claimPubKey == nil {
		return fmt.Errorf("claim public key is nil")
	}
	if len(claimPubKey.SerializeCompressed()) != 33 {
		return fmt.Errorf("claim public key must be 33 bytes compressed, got %d", len(claimPubKey.SerializeCompressed()))
	}

	claimPubKeyXOnly := claimPubKey.SerializeCompressed()[1:]
	if !bytes.Equal(claimPubKeyXOnly, components.ClaimPubKey[:]) {
		return fmt.Errorf(
			"claim pubkey mismatch: expected %x, got %x in script",
			claimPubKeyXOnly,
			components.ClaimPubKey[:],
		)
	}

	if len(preimageHash) != 20 {
		return fmt.Errorf("preimage hash must be 20, got %d", len(preimageHash))
	}

	if !bytes.Equal(preimageHash, components.PreimageHash[:]) {
		return fmt.Errorf(
			"preimage hash mismatch: expected %x, got %x in script",
			preimageHash,
			components.PreimageHash[:],
		)
	}

	return nil
}

// ValidateRefundLeafScript parses and validates a Boltz HTLC refund leaf script.
// The expected structure follows Boltz's refund format:
//
//	OP_PUSHBYTES_32 <refund_pubkey>
//	OP_CHECKSIGVERIFY
//	OP_PUSHBYTES_2 <timeout>
//	OP_CHECKLOCKTIMEVERIFY (absolute block height)
//
// Note: Boltz uses CLTV (absolute timelock) not CSV (relative timelock) for refund paths.
//
// Example:
//
//	input: "207599756afc49ebf5a6f3ac5848ef0afe934edd7b669bca02029acf10cc7f83acad02f802b1"
//	-> refundPubKey: 7599756afc49ebf5a6f3ac5848ef0afe934edd7b669bca02029acf10cc7f83ac
//	-> timeout: 760 blocks (0x02f8 little-endian)
func ValidateRefundLeafScript(outputHex string) (*RefundHTLCComponents, error) {
	scriptBytes, err := hex.DecodeString(outputHex)
	if err != nil {
		return nil, fmt.Errorf("failed to decode refund leaf output hex: %w", err)
	}

	components, err := parseRefundHTLCScriptManually(scriptBytes)
	if err != nil {
		return nil, err
	}

	return components, nil
}

// parseRefundHTLCScriptManually manually parses the refund HTLC script byte by byte
// to extract refund pubkey and timeout with full validation.
func parseRefundHTLCScriptManually(script []byte) (*RefundHTLCComponents, error) {
	// Minimum: 1 (push 32) + 32 (pubkey) + 1 (OP_CHECKSIGVERIFY) + 1 (push len) + 2 (timeout) + 1 (OP_CLTV) = 38 bytes
	if len(script) < 38 {
		return nil, fmt.Errorf("refund script too short: expected at least 38 bytes, got %d", len(script))
	}

	p := newScriptParser(script)

	// Read push of 32 bytes (refund pubkey)
	if err := p.expectPush(0x20, "refund pubkey"); err != nil {
		return nil, err
	}

	refundPubKeyBytes, err := p.readFixedBytes(32, "refund pubkey")
	if err != nil {
		return nil, err
	}

	// Read OP_CHECKSIGVERIFY (0xad)
	if err := p.expectOpcode(txscript.OP_CHECKSIGVERIFY, "OP_CHECKSIGVERIFY"); err != nil {
		return nil, err
	}

	// Read push length for timeout (typically 2-3 bytes)
	pushLen, err := p.buf.ReadByte()
	if err != nil {
		return nil, fmt.Errorf("failed to read push length for timeout: %w", err)
	}
	if pushLen < 0x01 || pushLen > 0x04 {
		return nil, fmt.Errorf("expected timeout push length 1-4 bytes, got 0x%x", pushLen)
	}

	// Read timeout bytes (little-endian)
	timeoutBytes, err := p.readFixedBytes(int(pushLen), "timeout")
	if err != nil {
		return nil, err
	}

	// Decode little-endian timeout
	var timeout uint32
	for i := 0; i < len(timeoutBytes); i++ {
		timeout |= uint32(timeoutBytes[i]) << (8 * i)
	}

	// Read OP_CHECKLOCKTIMEVERIFY (0xb1) - Boltz uses absolute timelock, not relative
	if err := p.expectOpcode(txscript.OP_CHECKLOCKTIMEVERIFY, "OP_CHECKLOCKTIMEVERIFY"); err != nil {
		return nil, err
	}

	// Verify no extra bytes
	if err := p.expectNoMoreBytes(); err != nil {
		return nil, err
	}

	var refundPubKey [32]byte
	copy(refundPubKey[:], refundPubKeyBytes)

	return &RefundHTLCComponents{
		RefundPubKey: refundPubKey,
		Timeout:      timeout,
	}, nil
}

// validateRefundPath validates the Bitcoin HTLC refund path for a chain swap.
// This ensures the user can refund their BTC if Boltz fails to deliver VTXOs.
func validateRefundPath(
	swapTree boltz.SwapTree,
	refundPubKey *btcec.PublicKey,
	expectedTimeout uint32,
) error {
	if err := validateSwapTree(swapTree); err != nil {
		return fmt.Errorf("invalid swap tree: %w", err)
	}

	components, err := ValidateRefundLeafScript(swapTree.RefundLeaf.Output)
	if err != nil {
		return fmt.Errorf("invalid refund leaf HTLC script: %w", err)
	}

	// Validate refund pubkey
	if refundPubKey == nil {
		return fmt.Errorf("refund public key is nil")
	}
	refundPubKeyXOnly := refundPubKey.SerializeCompressed()[1:]
	if !bytes.Equal(refundPubKeyXOnly, components.RefundPubKey[:]) {
		return fmt.Errorf(
			"refund pubkey mismatch: expected %x, got %x in script",
			refundPubKeyXOnly,
			components.RefundPubKey[:],
		)
	}

	if components.Timeout != expectedTimeout {
		return fmt.Errorf(
			"timeout mismatch: expected %d blocks, got %d blocks in script",
			expectedTimeout,
			components.Timeout,
		)
	}

	const MinRefundTimeout = 144
	if components.Timeout < MinRefundTimeout {
		return fmt.Errorf(
			"timeout too short: got %d blocks, minimum safe timeout is %d blocks",
			components.Timeout,
			MinRefundTimeout,
		)
	}

	return nil
}

func validateBtcLockupAddress(
	network *chaincfg.Params,
	expectedAddr string,
	serverPubKeyHex string,
	clientPubKey *btcec.PublicKey,
	swapTree boltz.SwapTree,
) error {
	serverPubKeyBytes, err := hex.DecodeString(serverPubKeyHex)
	if err != nil {
		return fmt.Errorf("decode server pubkey hex: %w", err)
	}
	serverPubKey, err := btcec.ParsePubKey(serverPubKeyBytes)
	if err != nil {
		return fmt.Errorf("parse server pubkey: %w", err)
	}

	merkleRoot, err := computeSwapTreeMerkleRoot(swapTree)
	if err != nil {
		return fmt.Errorf("compute merkle root: %w", err)
	}

	agg, _, _, err := musig2.AggregateKeys(
		[]*btcec.PublicKey{serverPubKey, clientPubKey},
		false,
	)
	if err != nil {
		return fmt.Errorf("musig2 aggregate keys: %w", err)
	}

	tweakedKey := txscript.ComputeTaprootOutputKey(agg.FinalKey, merkleRoot)

	xonly := schnorr.SerializePubKey(tweakedKey)
	addr, err := encodeP2TRAddress(network, xonly)
	if err != nil {
		return fmt.Errorf("encode p2tr address: %w", err)
	}

	if addr != expectedAddr {
		return fmt.Errorf("btc lockup address mismatch: expected=%s derived=%s", expectedAddr, addr)
	}

	return nil
}

func validateSwapTree(swapTree boltz.SwapTree) error {
	if swapTree.ClaimLeaf.Output == "" {
		return fmt.Errorf("claim leaf output is empty")
	}

	if swapTree.ClaimLeaf.Version != 0xc0 {
		return fmt.Errorf("invalid claim leaf version: expected 0xc0, got 0x%x", swapTree.ClaimLeaf.Version)
	}

	if swapTree.RefundLeaf.Output == "" {
		return fmt.Errorf("refund leaf output is empty")
	}

	if swapTree.RefundLeaf.Version != 0xc0 {
		return fmt.Errorf("invalid refund leaf version: expected 0xc0, got 0x%x", swapTree.RefundLeaf.Version)
	}

	if _, err := hex.DecodeString(swapTree.ClaimLeaf.Output); err != nil {
		return fmt.Errorf("claim leaf script is not valid hex: %w", err)
	}

	if _, err := hex.DecodeString(swapTree.RefundLeaf.Output); err != nil {
		return fmt.Errorf("refund leaf script is not valid hex: %w", err)
	}

	return nil
}

func encodeP2TRAddress(net *chaincfg.Params, xonlyPubKey []byte) (string, error) {
	if len(xonlyPubKey) != 32 {
		return "", fmt.Errorf("x-only pubkey must be 32 bytes, got %d", len(xonlyPubKey))
	}

	tapAddr, err := btcutil.NewAddressTaproot(xonlyPubKey, net)
	if err != nil {
		return "", err
	}
	return tapAddr.EncodeAddress(), nil
}
