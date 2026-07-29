package handlers

import (
	"encoding/hex"
	"fmt"
	"strings"

	delegatev1 "github.com/ArkLabsHQ/fulmine/api-spec/protobuf/gen/go/delegate/v1"
	fulminev1 "github.com/ArkLabsHQ/fulmine/api-spec/protobuf/gen/go/fulmine/v1"
	"github.com/ArkLabsHQ/fulmine/internal/core/application"
	"github.com/ArkLabsHQ/fulmine/internal/core/domain"
	"github.com/ArkLabsHQ/fulmine/utils"
	arklib "github.com/arkade-os/arkd/pkg/ark-lib"
	clientTypes "github.com/arkade-os/arkd/pkg/client-lib/types"
	"github.com/arkade-os/go-sdk/vhtlc"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcutil/psbt"
	"github.com/btcsuite/btcd/wire"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func parseServerUrl(a string) (string, error) {
	if len(a) == 0 {
		return "", fmt.Errorf("missing server url")
	}
	if !utils.IsValidURL(a) {
		return "", fmt.Errorf("invalid server url")
	}
	return a, nil
}

func parsePassword(p string) (string, error) {
	if len(p) == 0 {
		return "", fmt.Errorf("missing password")
	}
	if err := utils.IsValidPassword(p); err != nil {
		return "", err
	}
	return p, nil
}

func parseMnemonic(mnemonic string) (string, error) {
	if len(mnemonic) <= 0 {
		return "", fmt.Errorf("missing mnemonic")
	}
	if err := utils.IsValidMnemonic(mnemonic); err != nil {
		return "", err
	}
	return mnemonic, nil
}

func parseAddresses(addresses []string) ([]string, error) {
	if len(addresses) == 0 {
		return nil, status.Error(codes.InvalidArgument, "no addresses provided")
	}
	for _, addr := range addresses {
		if _, err := parseArkAddress(addr); err != nil {
			return nil, status.Error(codes.InvalidArgument, fmt.Sprintf("invalid address %s: %v", addr, err))
		}
	}
	return addresses, nil
}

func parseArkAddress(a string) (string, error) {
	if len(a) <= 0 {
		return "", fmt.Errorf("missing address")
	}
	if !utils.IsValidOffchainAddress(a) {
		return "", fmt.Errorf("invalid address")
	}
	return a, nil
}

func parseAddress(a string) (string, error) {
	if len(a) <= 0 {
		return "", fmt.Errorf("missing address")
	}
	if !utils.IsValidOffchainAddress(a) && !utils.IsValidBtcAddress(a) {
		return "", fmt.Errorf("invalid address")
	}
	return a, nil
}

func parseAmount(a uint64) (uint64, error) {
	if a == 0 {
		return 0, fmt.Errorf("missing amount")
	}
	return a, nil
}

func parseNote(n string) (string, error) {
	if len(n) == 0 {
		return "", fmt.Errorf("missing note")
	}
	if !utils.IsValidNote(n) {
		return "", fmt.Errorf("invalid note")
	}
	return n, nil
}

func parsePubkey(pubkey string) (*btcec.PublicKey, error) {
	if len(pubkey) <= 0 {
		return nil, nil
	}

	buf, err := hex.DecodeString(pubkey)
	if err != nil {
		return nil, fmt.Errorf("pubkey must be encoded in hex format")
	}

	pk, err := btcec.ParsePubKey(buf)
	if err != nil {
		return nil, fmt.Errorf("invalid pubkey: %s", err)
	}

	return pk, nil
}

func parseAbsoluteLocktime(locktime uint32) *arklib.AbsoluteLocktime {
	if locktime == 0 {
		return nil
	}
	lt := arklib.AbsoluteLocktime(locktime)
	return &lt
}

func parseRelativeLocktime(locktime *fulminev1.RelativeLocktime) *arklib.RelativeLocktime {
	if locktime == nil {
		return nil
	}
	return &arklib.RelativeLocktime{
		Type:  parseRelativeLocktimeType(locktime.Type),
		Value: locktime.Value,
	}
}

func parseRelativeLocktimeType(locktimeType fulminev1.RelativeLocktime_LocktimeType) arklib.RelativeLocktimeType {
	switch locktimeType {
	case fulminev1.RelativeLocktime_LOCKTIME_TYPE_BLOCK:
		return arklib.LocktimeTypeBlock
	case fulminev1.RelativeLocktime_LOCKTIME_TYPE_SECOND:
		return arklib.LocktimeTypeSecond
	default:
		return arklib.LocktimeTypeBlock
	}
}

func parseTransaction(tx string) (string, error) {
	if len(tx) <= 0 {
		return "", fmt.Errorf("missing transaction")
	}
	if _, err := psbt.NewFromRawBytes(strings.NewReader(tx), true); err != nil {
		return "", fmt.Errorf("invalid transaction: %s", err)
	}
	return tx, nil
}

func parseNonInteractiveClaim(nic *fulminev1.NonInteractiveClaim) (*arklib.Address, error) {
	if nic == nil {
		return nil, nil // non interactive claim path is optional
	}
	claimAddr := nic.GetClaimAddress()
	if len(claimAddr) == 0 {
		return nil, fmt.Errorf("claim_address is required")
	}
	return arklib.DecodeAddressV0(claimAddr)
}

func toNetworkProto(net string) fulminev1.Network {
	switch net {
	case "regtest":
		return fulminev1.Network_NETWORK_REGTEST
	case "testnet":
		return fulminev1.Network_NETWORK_TESTNET
	case "mainnet":
		return fulminev1.Network_NETWORK_MAINNET
	default:
		return fulminev1.Network_NETWORK_UNSPECIFIED
	}
}

func toTxTypeProto(txType clientTypes.TxType) fulminev1.TxType {
	switch txType {
	case clientTypes.TxSent:
		return fulminev1.TxType_TX_TYPE_SENT
	case clientTypes.TxReceived:
		return fulminev1.TxType_TX_TYPE_RECEIVED
	default:
		return fulminev1.TxType_TX_TYPE_UNSPECIFIED
	}
}

func toSwapTreeProto(tree *vhtlc.VHTLCScript) *fulminev1.TaprootTree {
	claimScript, _ := tree.ClaimClosure.Script()
	refundScript, _ := tree.RefundClosure.Script()
	refundWithoutBoltzScript, _ := tree.RefundWithoutReceiverClosure.Script()
	unilateralClaimScript, _ := tree.UnilateralClaimClosure.Script()
	unilateralRefundScript, _ := tree.UnilateralRefundClosure.Script()
	unilateralRefundWithoutBoltzScript, _ := tree.UnilateralRefundWithoutReceiverClosure.Script()

	taptree := &fulminev1.TaprootTree{
		ClaimLeaf: &fulminev1.TaprootLeaf{
			Version: 0,
			Output:  hex.EncodeToString(claimScript),
		},
		RefundLeaf: &fulminev1.TaprootLeaf{
			Version: 0,
			Output:  hex.EncodeToString(refundScript),
		},
		RefundWithoutBoltzLeaf: &fulminev1.TaprootLeaf{
			Version: 0,
			Output:  hex.EncodeToString(refundWithoutBoltzScript),
		},
		UnilateralClaimLeaf: &fulminev1.TaprootLeaf{
			Version: 0,
			Output:  hex.EncodeToString(unilateralClaimScript),
		},
		UnilateralRefundLeaf: &fulminev1.TaprootLeaf{
			Version: 0,
			Output:  hex.EncodeToString(unilateralRefundScript),
		},
		UnilateralRefundWithoutBoltzLeaf: &fulminev1.TaprootLeaf{
			Version: 0,
			Output:  hex.EncodeToString(unilateralRefundWithoutBoltzScript),
		},
	}

	if tree.NonInteractiveClaimClosure != nil {
		nonInteractiveClaimScript, _ := tree.NonInteractiveClaimClosure.Script()

		taptree.NonInteractiveClaimLeaf = &fulminev1.TaprootLeaf{
			Version: 0,
			Output:  hex.EncodeToString(nonInteractiveClaimScript),
		}
	}

	return taptree
}

func toNotificationProto(n application.Notification) *fulminev1.Notification {
	notification := &fulminev1.Notification{
		Addresses:  n.Addrs,
		NewVtxos:   toVtxosProto(n.NewVtxos),
		SpentVtxos: toVtxosProto(n.SpentVtxos),
		Txid:       n.Txid,
		Tx:         n.Tx,
	}
	if len(n.Checkpoints) > 0 {
		notification.Checkpoints = make(map[string]*fulminev1.TxData, len(n.Checkpoints))
		for k, v := range n.Checkpoints {
			notification.Checkpoints[k] = &fulminev1.TxData{
				Tx:   v.Tx,
				Txid: v.Txid,
			}
		}
	}
	return notification
}

// Todo: Verify that the script is not Taproot Script
func toVtxosProto(vtxos []clientTypes.Vtxo) []*fulminev1.Vtxo {
	list := make([]*fulminev1.Vtxo, 0, len(vtxos))
	for _, vtxo := range vtxos {
		list = append(list, &fulminev1.Vtxo{
			Outpoint:        toInputProto(vtxo.Outpoint),
			Script:          vtxo.Script,
			Amount:          vtxo.Amount,
			SpentBy:         vtxo.SpentBy,
			ExpiresAt:       vtxo.ExpiresAt.Unix(),
			CommitmentTxids: vtxo.CommitmentTxids,
			ArkTxid:         vtxo.ArkTxid,
			CreatedAt:       vtxo.CreatedAt.Unix(),
			IsPreconfirmed:  vtxo.Preconfirmed,
			IsSwept:         vtxo.Swept,
			IsUnrolled:      vtxo.Unrolled,
			IsSpent:         vtxo.Spent,
			SettledBy:       vtxo.SettledBy,
		})
	}
	return list
}

func toInputProto(outpoint clientTypes.Outpoint) *fulminev1.Input {
	return &fulminev1.Input{
		Txid: outpoint.Txid,
		Vout: outpoint.VOut,
	}
}

func toDelegateProtoInput(outpoint wire.OutPoint) *delegatev1.Input {
	return &delegatev1.Input{
		Txid: outpoint.Hash.String(),
		Vout: outpoint.Index,
	}
}

func toDelegateProto(delegate domain.DelegateTask) *delegatev1.Delegate {
	intent := &delegatev1.DelegateIntent{
		Txid:    delegate.Intent.Txid,
		Message: delegate.Intent.Message,
		Proof:   delegate.Intent.Proof,
		Inputs:  make([]*delegatev1.Input, 0, len(delegate.Intent.Inputs)),
	}
	for _, input := range delegate.Intent.Inputs {
		intent.Inputs = append(intent.Inputs, toDelegateProtoInput(input))
	}

	forfeitTxs := make([]*delegatev1.DelegateForfeitTx, 0, len(delegate.ForfeitTxs))
	for outpoint, forfeitTx := range delegate.ForfeitTxs {
		forfeitTxs = append(forfeitTxs, &delegatev1.DelegateForfeitTx{
			Input:     toDelegateProtoInput(outpoint),
			ForfeitTx: forfeitTx,
		})
	}

	return &delegatev1.Delegate{
		Id:                delegate.ID,
		Intent:            intent,
		ForfeitTxs:        forfeitTxs,
		Fee:               delegate.Fee,
		DelegatePublicKey: delegate.DelegatePublicKey,
		ScheduledAt:       delegate.ScheduledAt.Unix(),
		Status:            delegate.Status.String(),
		FailReason:        delegate.FailReason,
		CommitmentTxid:    delegate.CommitmentTxid,
	}
}

func toDelegatesProto(delegates []domain.DelegateTask) []*delegatev1.Delegate {
	list := make([]*delegatev1.Delegate, 0, len(delegates))
	for _, delegate := range delegates {
		list = append(list, toDelegateProto(delegate))
	}
	return list
}
