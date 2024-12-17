package handlers

import (
	"context"
	"encoding/hex"
	"fmt"
	"time"

	pb "github.com/ArkLabsHQ/ark-node/api-spec/protobuf/gen/go/ark_node/v1"
	"github.com/ArkLabsHQ/ark-node/internal/core/application"
	"github.com/ArkLabsHQ/ark-node/utils"
	"github.com/ark-network/ark/common/tree"
	arksdk "github.com/ark-network/ark/pkg/client-sdk"
	"github.com/ark-network/ark/pkg/client-sdk/types"
	"github.com/decred/dcrd/dcrec/secp256k1/v4"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type serviceHandler struct {
	svc *application.Service
}

func NewServiceHandler(svc *application.Service) pb.ServiceServer {
	return &serviceHandler{svc}
}

func (h *serviceHandler) ClaimVHTLC(ctx context.Context, req *pb.ClaimVHTLCRequest) (*pb.ClaimVHTLCResponse, error) {
	preimage := req.GetPreimage()
	if len(preimage) <= 0 {
		return nil, status.Error(codes.InvalidArgument, "missing preimage")
	}

	preimageBytes, err := hex.DecodeString(preimage)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid preimage")
	}

	redeemTxid, err := h.svc.ClaimVHTLC(ctx, preimageBytes)
	if err != nil {
		return nil, err
	}

	return &pb.ClaimVHTLCResponse{RedeemTxid: redeemTxid}, nil
}

func (h *serviceHandler) ListVHTLC(ctx context.Context, req *pb.ListVHTLCRequest) (*pb.ListVHTLCResponse, error) {
	vtxos, _, err := h.svc.ListVHTLC(ctx, req.GetPreimageHashFilter())
	if err != nil {
		return nil, err
	}

	vhtlcs := make([]*pb.Vtxo, 0, len(vtxos))
	for _, vtxo := range vtxos {
		vhtlcs = append(vhtlcs, &pb.Vtxo{
			Outpoint: &pb.Input{
				Txid: vtxo.Txid,
				Vout: vtxo.VOut,
			},
			Receiver: &pb.Output{
				Pubkey: vtxo.PubKey,
				Amount: vtxo.Amount,
			},
			SpentBy:   vtxo.SpentBy,
			RoundTxid: vtxo.RoundTxid,
			ExpireAt:  vtxo.ExpiresAt.Unix(),
		})
	}

	return &pb.ListVHTLCResponse{Vhtlcs: vhtlcs}, nil
}

func (h *serviceHandler) CreateVHTLC(ctx context.Context, req *pb.CreateVHTLCRequest) (*pb.CreateVHTLCResponse, error) {
	pubkeyBytes, err := hex.DecodeString(req.GetPubkey())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid pubkey")
	}

	pubkey, err := secp256k1.ParsePubKey(pubkeyBytes)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid pubkey")
	}

	preimageHashBytes, err := hex.DecodeString(req.GetPreimageHash())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid preimage hash")
	}

	addr, tapscripts, err := h.svc.GetVHTLC(ctx, pubkey, preimageHashBytes)
	if err != nil {
		return nil, err
	}
	return &pb.CreateVHTLCResponse{
		Address:    addr,
		Tapscripts: tapscripts,
	}, nil
}

func (h *serviceHandler) GetAddress(
	ctx context.Context, req *pb.GetAddressRequest,
) (*pb.GetAddressResponse, error) {
	bip21Addr, _, _, err := h.svc.GetAddress(ctx, 0)
	if err != nil {
		return nil, err
	}
	return &pb.GetAddressResponse{Address: bip21Addr}, nil
}

func (h *serviceHandler) GetBalance(
	ctx context.Context, req *pb.GetBalanceRequest,
) (*pb.GetBalanceResponse, error) {
	balance, err := h.svc.GetTotalBalance(ctx)
	if err != nil {
		return nil, err
	}
	return &pb.GetBalanceResponse{Amount: balance}, nil
}

func (h *serviceHandler) GetInfo(
	ctx context.Context, req *pb.GetInfoRequest,
) (*pb.GetInfoResponse, error) {
	data, err := h.svc.GetConfigData(ctx)
	if err != nil {
		return nil, err
	}

	return &pb.GetInfoResponse{
		Network: toNetworkProto(data.Network.Name),
		BuildInfo: &pb.BuildInfo{
			Version: h.svc.BuildInfo.Version,
			Commit:  h.svc.BuildInfo.Commit,
			Date:    h.svc.BuildInfo.Date,
		},
	}, nil
}

func (h *serviceHandler) GetOnboardAddress(
	ctx context.Context, req *pb.GetOnboardAddressRequest,
) (*pb.GetOnboardAddressResponse, error) {
	_, _, addr, err := h.svc.GetAddress(ctx, 0)
	if err != nil {
		return nil, err
	}
	return &pb.GetOnboardAddressResponse{Address: addr}, nil
}

func (h *serviceHandler) SendOffChain(
	ctx context.Context, req *pb.SendOffChainRequest,
) (*pb.SendOffChainResponse, error) {
	address, err := parseAddress(req.GetAddress())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	amount, err := parseAmount(req.GetAmount())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	receivers := []arksdk.Receiver{
		arksdk.NewBitcoinReceiver(address, amount),
	}
	roundId, err := h.svc.SendOffChain(ctx, false, receivers)
	if err != nil {
		return nil, err
	}
	return &pb.SendOffChainResponse{RoundId: roundId}, nil
}

func (h *serviceHandler) SendOnChain(
	ctx context.Context, req *pb.SendOnChainRequest,
) (*pb.SendOnChainResponse, error) {
	address, err := parseAddress(req.GetAddress())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	amount, err := parseAmount(req.GetAmount())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	receivers := []arksdk.Receiver{
		arksdk.NewBitcoinReceiver(address, amount),
	}
	txid, err := h.svc.SendOnChain(ctx, receivers)
	if err != nil {
		return nil, err
	}
	return &pb.SendOnChainResponse{Txid: txid}, nil
}

func (h *serviceHandler) GetRoundInfo(
	ctx context.Context, req *pb.GetRoundInfoRequest,
) (*pb.GetRoundInfoResponse, error) {
	roundId, err := parseRoundId(req.GetRoundId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	round, err := h.svc.GetRound(ctx, roundId)
	if err != nil {
		return nil, err
	}
	endedAt := int64(0)
	if round.EndedAt != nil {
		endedAt = round.EndedAt.Unix()
	}
	return &pb.GetRoundInfoResponse{
		Round: &pb.Round{
			Id:             round.ID,
			Start:          round.StartedAt.Unix(),
			End:            endedAt,
			RoundTx:        round.Tx,
			CongestionTree: toTreeProto(round.Tree),
			ForfeitTxs:     round.ForfeitTxs,
		},
	}, nil
}

func (h *serviceHandler) GetTransactionHistory(
	ctx context.Context, req *pb.GetTransactionHistoryRequest,
) (*pb.GetTransactionHistoryResponse, error) {
	txHistory, err := h.svc.GetTransactionHistory(ctx)
	if err != nil {
		return nil, err
	}
	txs := make([]*pb.TransactionInfo, 0, len(txHistory))
	for _, tx := range txHistory {
		txs = append(txs, &pb.TransactionInfo{
			Date:         tx.CreatedAt.Format(time.RFC3339),
			Amount:       tx.Amount,
			RoundTxid:    tx.RoundTxid,
			RedeemTxid:   tx.RedeemTxid,
			BoardingTxid: tx.BoardingTxid,
			Type:         toTxTypeProto(tx.Type),
			Settled:      tx.Settled,
		})
	}

	return &pb.GetTransactionHistoryResponse{Transactions: txs}, nil
}

func (h *serviceHandler) CreateInvoice(
	ctx context.Context, req *pb.CreateInvoiceRequest,
) (*pb.CreateInvoiceResponse, error) {
	amount, err := parseAmount(req.GetAmount())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	memo := req.GetMemo()

	invoice, preimageHash, err := h.svc.GetInvoice(ctx, amount, memo)
	if err != nil {
		return nil, err
	}

	return &pb.CreateInvoiceResponse{
		Invoice:      invoice,
		PreimageHash: preimageHash,
	}, nil
}

func (h *serviceHandler) PayInvoice(
	ctx context.Context, req *pb.PayInvoiceRequest,
) (*pb.PayInvoiceResponse, error) {
	invoice, err := parseInvoice(req.GetInvoice())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	preimage, err := h.svc.PayInvoice(ctx, invoice)
	if err != nil {
		return nil, err
	}

	return &pb.PayInvoiceResponse{Preimage: preimage}, nil
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

func toTreeProto(tree tree.VtxoTree) *pb.Tree {
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
