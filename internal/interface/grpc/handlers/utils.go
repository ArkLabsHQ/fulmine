package handlers

import (
	"encoding/hex"
	"fmt"
	"strings"

	pb "github.com/ArkLabsHQ/fulmine/api-spec/protobuf/gen/go/fulmine/v1"
	"github.com/ArkLabsHQ/fulmine/internal/core/application"
	"github.com/ArkLabsHQ/fulmine/pkg/vhtlc"
	"github.com/ArkLabsHQ/fulmine/utils"
	arklib "github.com/arkade-os/arkd/pkg/ark-lib"
	"github.com/arkade-os/go-sdk/types"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcutil/psbt"
	"github.com/nbd-wtf/go-nostr/nip19"
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

func parsePrivateKey(sk string) (string, error) {
	if len(sk) == 0 {
		return "", fmt.Errorf("missing private key")
	}
	if strings.HasPrefix(sk, "nsec") {
		_, seed, err := nip19.Decode(sk)
		if err != nil {
			return "", err
		}
		sk = fmt.Sprint(seed)
	}
	if err := utils.IsValidPrivateKey(sk); err != nil {
		return "", err
	}
	return sk, nil
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
	if !utils.IsValidArkAddress(a) {
		return "", fmt.Errorf("invalid address")
	}
	return a, nil
}

func parseAddress(a string) (string, error) {
	if len(a) <= 0 {
		return "", fmt.Errorf("missing address")
	}
	if !utils.IsValidArkAddress(a) && !utils.IsValidBtcAddress(a) {
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
	if !utils.IsValidArkNote(n) {
		return "", fmt.Errorf("invalid note")
	}
	return n, nil
}

func parseRoundId(id string) (string, error) {
	if len(id) <= 0 {
		return "", fmt.Errorf("missing round id")
	}
	return id, nil
}

func parseInvoice(invoice string) (string, error) {
	if len(invoice) <= 0 {
		return "", fmt.Errorf("missing invoice")
	}
	return invoice, nil
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

func parseRelativeLocktime(locktime *pb.RelativeLocktime) *arklib.RelativeLocktime {
	if locktime == nil {
		return nil
	}
	return &arklib.RelativeLocktime{
		Type:  parseRelativeLocktimeType(locktime.Type),
		Value: locktime.Value,
	}
}

func parseRelativeLocktimeType(locktimeType pb.RelativeLocktime_LocktimeType) arklib.RelativeLocktimeType {
	switch locktimeType {
	case pb.RelativeLocktime_LOCKTIME_TYPE_BLOCK:
		return arklib.LocktimeTypeBlock
	case pb.RelativeLocktime_LOCKTIME_TYPE_SECOND:
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

func parsePreimageHash(hash string) (string, error) {
	if len(hash) <= 0 {
		return "", fmt.Errorf("missing preimage hash")
	}
	buf, err := hex.DecodeString(hash)
	if err != nil {
		return "", fmt.Errorf("invalid preimage hash")
	}
	if len(buf) != 20 {
		return "", fmt.Errorf("invalid preimage hash length")
	}
	return hash, nil
}

func toNetworkProto(net string) pb.GetInfoResponse_Network {
	switch net {
	case "regtest":
		return pb.GetInfoResponse_NETWORK_REGTEST
	case "testnet":
		return pb.GetInfoResponse_NETWORK_TESTNET
	case "mainnet":
		return pb.GetInfoResponse_NETWORK_MAINNET
	default:
		return pb.GetInfoResponse_NETWORK_UNSPECIFIED
	}
}

func toTxTypeProto(txType types.TxType) pb.TxType {
	switch txType {
	case types.TxSent:
		return pb.TxType_TX_TYPE_SENT
	case types.TxReceived:
		return pb.TxType_TX_TYPE_RECEIVED
	default:
		return pb.TxType_TX_TYPE_UNSPECIFIED
	}
}

func toSwapTreeProto(tree *vhtlc.VHTLCScript) *pb.TaprootTree {
	claimScript, _ := tree.ClaimClosure.Script()
	refundScript, _ := tree.RefundClosure.Script()
	refundWithoutBoltzScript, _ := tree.RefundWithoutReceiverClosure.Script()
	unilateralClaimScript, _ := tree.UnilateralClaimClosure.Script()
	unilateralRefundScript, _ := tree.UnilateralRefundClosure.Script()
	unilateralRefundWithoutBoltzScript, _ := tree.UnilateralRefundWithoutReceiverClosure.Script()
	return &pb.TaprootTree{
		ClaimLeaf: &pb.TaprootLeaf{
			Version: 0,
			Output:  hex.EncodeToString(claimScript),
		},
		RefundLeaf: &pb.TaprootLeaf{
			Version: 0,
			Output:  hex.EncodeToString(refundScript),
		},
		RefundWithoutBoltzLeaf: &pb.TaprootLeaf{
			Version: 0,
			Output:  hex.EncodeToString(refundWithoutBoltzScript),
		},
		UnilateralClaimLeaf: &pb.TaprootLeaf{
			Version: 0,
			Output:  hex.EncodeToString(unilateralClaimScript),
		},
		UnilateralRefundLeaf: &pb.TaprootLeaf{
			Version: 0,
			Output:  hex.EncodeToString(unilateralRefundScript),
		},
		UnilateralRefundWithoutBoltzLeaf: &pb.TaprootLeaf{
			Version: 0,
			Output:  hex.EncodeToString(unilateralRefundWithoutBoltzScript),
		},
	}
}

func toNotificationProto(n application.Notification) *pb.Notification {
	notification := &pb.Notification{
		Addresses:  n.Addrs,
		NewVtxos:   toVtxosProto(n.NewVtxos),
		SpentVtxos: toVtxosProto(n.SpentVtxos),
		Txid:       n.Txid,
		Tx:         n.Tx,
	}
	if len(n.Checkpoints) > 0 {
		notification.Checkpoints = make(map[string]*pb.TxData, len(n.Checkpoints))
		for k, v := range n.Checkpoints {
			notification.Checkpoints[k] = &pb.TxData{
				Tx:   v.Tx,
				Txid: v.Txid,
			}
		}
	}
	return notification
}

// Todo: Verify that the script is not Taproot Script
func toVtxosProto(vtxos []types.Vtxo) []*pb.Vtxo {
	list := make([]*pb.Vtxo, 0, len(vtxos))
	for _, vtxo := range vtxos {
		list = append(list, &pb.Vtxo{
			Outpoint:        toInputProto(vtxo.Outpoint),
			Script:          vtxo.Script,
			Amount:          vtxo.Amount,
			SpentBy:         vtxo.SpentBy,
			ExpiresAt:       vtxo.ExpiresAt.Unix(),
			CommitmentTxids: vtxo.CommitmentTxids,
			ArkTxid:         vtxo.ArkTxid,
		})
	}
	return list
}

func toInputProto(outpoint types.Outpoint) *pb.Input {
	return &pb.Input{
		Txid: outpoint.Txid,
		Vout: outpoint.VOut,
	}
}
