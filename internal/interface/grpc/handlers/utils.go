package handlers

import (
	"encoding/hex"
	"fmt"
	"strings"

	pb "github.com/ArkLabsHQ/ark-node/api-spec/protobuf/gen/go/ark_node/v1"
	"github.com/ArkLabsHQ/ark-node/internal/core/application"
	"github.com/ArkLabsHQ/ark-node/pkg/vhtlc"
	"github.com/ArkLabsHQ/ark-node/utils"
	"github.com/ark-network/ark/common/tree"
	"github.com/ark-network/ark/pkg/client-sdk/client"
	"github.com/ark-network/ark/pkg/client-sdk/types"
	"github.com/decred/dcrd/dcrec/secp256k1/v4"
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

func parsePubkey(pubkey string) (*secp256k1.PublicKey, error) {
	if len(pubkey) <= 0 {
		return nil, nil
	}

	buf, err := hex.DecodeString(pubkey)
	if err != nil {
		return nil, fmt.Errorf("pubkey must be encoded in hex format")
	}

	pk, err := secp256k1.ParsePubKey(buf)
	if err != nil {
		return nil, fmt.Errorf("invalid pubkey: %s", err)
	}

	return pk, nil
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

func toTreeProto(tree tree.TxTree) *pb.Tree {
	levels := make([]*pb.TreeLevel, 0, len(tree))
	for _, treeLevel := range tree {
		nodes := make([]*pb.Node, 0, len(treeLevel))
		for _, node := range treeLevel {
			nodes = append(nodes, &pb.Node{
				Txid:       node.Txid,
				Tx:         node.Tx,
				ParentTxid: node.ParentTxid,
			})
		}
		levels = append(levels, &pb.TreeLevel{Nodes: nodes})
	}
	return &pb.Tree{Levels: levels}
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
	return &pb.Notification{
		Address:    n.Address,
		NewVtxos:   toVtxosProto(n.NewVtxos),
		SpentVtxos: toVtxosProto(n.SpentVtxos),
	}
}

func toVtxosProto(vtxos []client.Vtxo) []*pb.Vtxo {
	list := make([]*pb.Vtxo, 0, len(vtxos))
	for _, vtxo := range vtxos {
		list = append(list, &pb.Vtxo{
			Outpoint: toInputProto(vtxo.Outpoint),
			Receiver: &pb.Output{
				Pubkey: vtxo.PubKey,
				Amount: vtxo.Amount,
			},
			SpentBy:   vtxo.SpentBy,
			RoundTxid: vtxo.RoundTxid,
			ExpireAt:  vtxo.ExpiresAt.Unix(),
		})
	}
	return list
}

func toInputProto(outpoint client.Outpoint) *pb.Input {
	return &pb.Input{
		Txid: outpoint.Txid,
		Vout: outpoint.VOut,
	}
}
