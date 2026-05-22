package swap

import (
	"context"
	"encoding/hex"
	"fmt"
	"strconv"

	"github.com/ArkLabsHQ/fulmine/pkg/vhtlc"
	arklib "github.com/arkade-os/arkd/pkg/ark-lib"
	"github.com/arkade-os/arkd/pkg/client-lib/identity"
	"github.com/arkade-os/go-sdk/contract/handlers"
	"github.com/arkade-os/go-sdk/types"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/btcsuite/btcd/txscript"
)

// VHTLCContractType is the contract type fulmine registers VHTLC scripts under
// in the go-sdk's contract manager. It is intentionally distinct from
// types.ContractTypeDefault / ContractTypeBoarding so the manager dispatches
// to the VHTLC handler rather than the built-in default handler.
const VHTLCContractType types.ContractType = "vhtlc"

const (
	paramOwnerKeyId  = "vhtlc_owner_key_id"
	paramOwnerKey    = "vhtlc_owner_key"
	paramSender      = "vhtlc_sender"
	paramReceiver    = "vhtlc_receiver"
	paramServer      = "vhtlc_server"
	paramPreimageHsh = "vhtlc_preimage_hash"
	paramRefundLock  = "vhtlc_refund_locktime"
	paramUCType      = "vhtlc_uc_delay_type"
	paramUCValue     = "vhtlc_uc_delay_value"
	paramURType      = "vhtlc_ur_delay_type"
	paramURValue     = "vhtlc_ur_delay_value"
	paramURNRType    = "vhtlc_urnr_delay_type"
	paramURNRValue   = "vhtlc_urnr_delay_value"
	paramNICRecvPk   = "vhtlc_nic_recv_pkscript"
	paramNICIntrosPk = "vhtlc_nic_introspector_pubkey"

	locktimeTypeBlock  = "block"
	locktimeTypeSecond = "second"
)

// VHTLCHandler is the go-sdk contract handler for VHTLC scripts. Registered
// via arksdk.WithContractHandlers, it lets wallet.SignTransaction recognise
// VHTLC inputs and route signing through the wallet's identity — replacing
// the historical signLocalTapscriptInputs workaround.
//
// The handler is stateless: each VHTLC contract carries the full Opts in its
// Params map, so the script can be rebuilt deterministically from any
// persisted contract row without consulting an external store.
type VHTLCHandler struct{}

// NewVHTLCHandler returns a handler ready to be passed to
// arksdk.WithContractHandlers under VHTLCContractType.
func NewVHTLCHandler() *VHTLCHandler {
	return &VHTLCHandler{}
}

// NewContract is invoked by the SDK's gap-limit scan at unlock. VHTLC
// contracts cannot be derived from a key alone — they require peer pubkeys,
// a preimage hash, and timelocks supplied by the swap flow — so callers
// create them via NewVHTLCContract and persist them directly through
// wallet.Store().ContractStore().AddContract.
//
// The stub returned here is a syntactically valid P2TR pkScript derived
// from the owner pubkey: the arkd indexer accepts the GetVtxos request,
// finds no matches, and the gap-limit scan short-circuits without
// persisting anything. We cannot simply return an error because
// ScanContracts propagates handler errors and would prevent the wallet's
// onchain listener from starting, leaving boarding UTXOs invisible to
// Settle.
func (h *VHTLCHandler) NewContract(
	_ context.Context, keyRef identity.KeyRef,
) (*types.Contract, error) {
	stubScript := "5120" + hex.EncodeToString(schnorr.SerializePubKey(keyRef.PubKey))
	return &types.Contract{
		Type:   VHTLCContractType,
		Script: stubScript,
		Params: map[string]string{paramOwnerKeyId: keyRef.Id},
		State:  types.ContractStateInactive,
	}, nil
}

func (h *VHTLCHandler) GetKeyRefs(c types.Contract) (map[string]string, error) {
	keyId, err := requireParam(c, paramOwnerKeyId)
	if err != nil {
		return nil, err
	}
	return map[string]string{c.Script: keyId}, nil
}

func (h *VHTLCHandler) GetKeyRef(c types.Contract) (*identity.KeyRef, error) {
	keyId, err := requireParam(c, paramOwnerKeyId)
	if err != nil {
		return nil, err
	}
	pubHex, err := requireParam(c, paramOwnerKey)
	if err != nil {
		return nil, err
	}
	buf, err := hex.DecodeString(pubHex)
	if err != nil {
		return nil, fmt.Errorf("vhtlc contract %s: invalid owner key hex: %w", c.Script, err)
	}
	pub, err := schnorr.ParsePubKey(buf)
	if err != nil {
		return nil, fmt.Errorf("vhtlc contract %s: invalid owner key: %w", c.Script, err)
	}
	return &identity.KeyRef{Id: keyId, PubKey: pub}, nil
}

func (h *VHTLCHandler) GetSignerKey(c types.Contract) (*btcec.PublicKey, error) {
	return parseCompressedParam(c, paramServer)
}

// GetExitDelay returns the unilateral refund delay — the closure that controls
// when the VHTLC becomes unilaterally claimable for the funder.
func (h *VHTLCHandler) GetExitDelay(c types.Contract) (*arklib.RelativeLocktime, error) {
	delay, err := parseRelativeLocktime(c, paramURType, paramURValue)
	if err != nil {
		return nil, err
	}
	return &delay, nil
}

func (h *VHTLCHandler) GetTapscripts(c types.Contract) ([]string, error) {
	opts, err := optsFromContract(c)
	if err != nil {
		return nil, err
	}
	vhtlcScript, err := vhtlc.NewVHTLCScriptFromOpts(opts)
	if err != nil {
		return nil, fmt.Errorf("vhtlc contract %s: rebuild script: %w", c.Script, err)
	}
	return vhtlcScript.Encode()
}

// NewVHTLCContract builds a contract entry suitable for persisting in
// wallet.Store().ContractStore() — i.e. the row the VHTLCHandler resolves at
// sign time. The owner key is the wallet identity key whose pubkey appears in
// at least one of the VHTLC closures; for the singlekey identity used today
// that's the singleton "m" keyRef.
func NewVHTLCContract(
	opts vhtlc.Opts,
	ownerKeyRef identity.KeyRef,
	network arklib.Network,
) (*types.Contract, error) {
	vhtlcScript, err := vhtlc.NewVHTLCScriptFromOpts(opts)
	if err != nil {
		return nil, fmt.Errorf("build vhtlc script: %w", err)
	}
	tapKey, _, err := vhtlcScript.TapTree()
	if err != nil {
		return nil, fmt.Errorf("compute vhtlc tap tree: %w", err)
	}
	pkScript, err := txscript.PayToTaprootScript(tapKey)
	if err != nil {
		return nil, fmt.Errorf("compute vhtlc pkScript: %w", err)
	}
	address, err := vhtlcScript.Address(network.Addr)
	if err != nil {
		return nil, fmt.Errorf("encode vhtlc address: %w", err)
	}

	params := map[string]string{
		paramOwnerKeyId:  ownerKeyRef.Id,
		paramOwnerKey:    hex.EncodeToString(schnorr.SerializePubKey(ownerKeyRef.PubKey)),
		paramSender:      hex.EncodeToString(opts.Sender.SerializeCompressed()),
		paramReceiver:    hex.EncodeToString(opts.Receiver.SerializeCompressed()),
		paramServer:      hex.EncodeToString(opts.Server.SerializeCompressed()),
		paramPreimageHsh: hex.EncodeToString(opts.PreimageHash),
		paramRefundLock:  strconv.FormatUint(uint64(opts.RefundLocktime), 10),
		paramUCType:      locktimeTypeName(opts.UnilateralClaimDelay.Type),
		paramUCValue:     strconv.FormatUint(uint64(opts.UnilateralClaimDelay.Value), 10),
		paramURType:      locktimeTypeName(opts.UnilateralRefundDelay.Type),
		paramURValue:     strconv.FormatUint(uint64(opts.UnilateralRefundDelay.Value), 10),
		paramURNRType:    locktimeTypeName(opts.UnilateralRefundWithoutReceiverDelay.Type),
		paramURNRValue: strconv.FormatUint(
			uint64(opts.UnilateralRefundWithoutReceiverDelay.Value), 10,
		),
	}
	if opts.NonInteractiveClaim != nil {
		params[paramNICRecvPk] = hex.EncodeToString(opts.NonInteractiveClaim.ReceiverPkScript)
		params[paramNICIntrosPk] = hex.EncodeToString(
			opts.NonInteractiveClaim.IntrospectorPubKey.SerializeCompressed(),
		)
	}

	return &types.Contract{
		Type:    VHTLCContractType,
		Script:  hex.EncodeToString(pkScript),
		Address: address,
		Params:  params,
		State:   types.ContractStateActive,
	}, nil
}

// Compile-time check the handler satisfies the SDK interface.
var _ handlers.Handler = (*VHTLCHandler)(nil)

func optsFromContract(c types.Contract) (vhtlc.Opts, error) {
	sender, err := parseCompressedParam(c, paramSender)
	if err != nil {
		return vhtlc.Opts{}, err
	}
	receiver, err := parseCompressedParam(c, paramReceiver)
	if err != nil {
		return vhtlc.Opts{}, err
	}
	server, err := parseCompressedParam(c, paramServer)
	if err != nil {
		return vhtlc.Opts{}, err
	}
	preimageHash, err := hex.DecodeString(c.Params[paramPreimageHsh])
	if err != nil {
		return vhtlc.Opts{}, fmt.Errorf(
			"vhtlc contract %s: invalid preimage hash hex: %w", c.Script, err,
		)
	}
	refundLockStr, err := requireParam(c, paramRefundLock)
	if err != nil {
		return vhtlc.Opts{}, err
	}
	refundLock, err := strconv.ParseUint(refundLockStr, 10, 32)
	if err != nil {
		return vhtlc.Opts{}, fmt.Errorf(
			"vhtlc contract %s: invalid refund locktime: %w", c.Script, err,
		)
	}
	uc, err := parseRelativeLocktime(c, paramUCType, paramUCValue)
	if err != nil {
		return vhtlc.Opts{}, err
	}
	ur, err := parseRelativeLocktime(c, paramURType, paramURValue)
	if err != nil {
		return vhtlc.Opts{}, err
	}
	urnr, err := parseRelativeLocktime(c, paramURNRType, paramURNRValue)
	if err != nil {
		return vhtlc.Opts{}, err
	}

	opts := vhtlc.Opts{
		Sender:                               sender,
		Receiver:                             receiver,
		Server:                               server,
		PreimageHash:                         preimageHash,
		RefundLocktime:                       arklib.AbsoluteLocktime(refundLock),
		UnilateralClaimDelay:                 uc,
		UnilateralRefundDelay:                ur,
		UnilateralRefundWithoutReceiverDelay: urnr,
	}

	if recvHex, ok := c.Params[paramNICRecvPk]; ok && recvHex != "" {
		recv, err := hex.DecodeString(recvHex)
		if err != nil {
			return vhtlc.Opts{}, fmt.Errorf(
				"vhtlc contract %s: invalid NIC receiver pkScript hex: %w", c.Script, err,
			)
		}
		introspector, err := parseCompressedParam(c, paramNICIntrosPk)
		if err != nil {
			return vhtlc.Opts{}, err
		}
		opts.NonInteractiveClaim = &vhtlc.NonInteractiveClaimOpts{
			ReceiverPkScript:   recv,
			IntrospectorPubKey: introspector,
		}
	}

	return opts, nil
}

func requireParam(c types.Contract, key string) (string, error) {
	if c.Params == nil {
		return "", fmt.Errorf("vhtlc contract %s: no params", c.Script)
	}
	v, ok := c.Params[key]
	if !ok || v == "" {
		return "", fmt.Errorf("vhtlc contract %s: missing param %q", c.Script, key)
	}
	return v, nil
}

func parseCompressedParam(c types.Contract, key string) (*btcec.PublicKey, error) {
	raw, err := requireParam(c, key)
	if err != nil {
		return nil, err
	}
	buf, err := hex.DecodeString(raw)
	if err != nil {
		return nil, fmt.Errorf("vhtlc contract %s: invalid %s hex: %w", c.Script, key, err)
	}
	pub, err := btcec.ParsePubKey(buf)
	if err != nil {
		return nil, fmt.Errorf("vhtlc contract %s: invalid %s: %w", c.Script, key, err)
	}
	return pub, nil
}

func parseRelativeLocktime(
	c types.Contract, typeKey, valueKey string,
) (arklib.RelativeLocktime, error) {
	typeStr, err := requireParam(c, typeKey)
	if err != nil {
		return arklib.RelativeLocktime{}, err
	}
	valueStr, err := requireParam(c, valueKey)
	if err != nil {
		return arklib.RelativeLocktime{}, err
	}
	value, err := strconv.ParseUint(valueStr, 10, 32)
	if err != nil {
		return arklib.RelativeLocktime{}, fmt.Errorf(
			"vhtlc contract %s: invalid %s: %w", c.Script, valueKey, err,
		)
	}
	var locktimeType arklib.RelativeLocktimeType
	switch typeStr {
	case locktimeTypeBlock:
		locktimeType = arklib.LocktimeTypeBlock
	case locktimeTypeSecond:
		locktimeType = arklib.LocktimeTypeSecond
	default:
		return arklib.RelativeLocktime{}, fmt.Errorf(
			"vhtlc contract %s: unknown %s %q", c.Script, typeKey, typeStr,
		)
	}
	return arklib.RelativeLocktime{Type: locktimeType, Value: uint32(value)}, nil
}

func locktimeTypeName(t arklib.RelativeLocktimeType) string {
	switch t {
	case arklib.LocktimeTypeBlock:
		return locktimeTypeBlock
	case arklib.LocktimeTypeSecond:
		return locktimeTypeSecond
	default:
		return ""
	}
}
