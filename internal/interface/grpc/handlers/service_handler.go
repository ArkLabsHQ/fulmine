package handlers

import (
	"context"
	"encoding/hex"
	"fmt"
	"time"

	pb "github.com/ArkLabsHQ/ark-node/api-spec/protobuf/gen/go/ark_node/v1"
	"github.com/ArkLabsHQ/ark-node/internal/core/application"
	"github.com/ArkLabsHQ/ark-node/utils"
	"github.com/ark-network/ark/common/bitcointree"
	"github.com/ark-network/ark/common/tree"
	arksdk "github.com/ark-network/ark/pkg/client-sdk"
	"github.com/ark-network/ark/pkg/client-sdk/client"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type serviceHandler struct {
	svc *application.Service
}

func NewServiceHandler(svc *application.Service) pb.ServiceServer {
	return &serviceHandler{svc}
}

func (h *serviceHandler) BoltzClaimVHTLC(ctx context.Context, req *pb.BoltzClaimVHTLCRequest) (*pb.BoltzClaimVHTLCResponse, error) {
	preimage := req.GetPreimage()
	if len(preimage) <= 0 {
		return nil, status.Error(codes.InvalidArgument, "missing preimage")
	}

	preimageBytes, err := hex.DecodeString(preimage)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid preimage")
	}

	preimageHash := hex.EncodeToString(btcutil.Hash160(preimageBytes))

	vtxos, _, err := h.svc.ListVtxos(ctx)
	if err != nil {
		return nil, err
	}

	var vhtlc *client.Vtxo
	for _, vtxo := range vtxos {
		vtxoScript, err := bitcointree.ParseVtxoScript(vtxo.Descriptor)
		if err != nil {
			continue
		}

		if htlcVtxo, ok := vtxoScript.(*bitcointree.HTLCVtxoScript); ok {
			if htlcVtxo.PreimageHash == preimageHash {
				vhtlc = &vtxo
				break
			}
		}
	}

	if vhtlc == nil {
		return nil, status.Error(codes.NotFound, "vhtlc not found")
	}

	_, myAddress, _, err := h.svc.GetAddress(ctx, 0)
	if err != nil {
		return nil, err
	}

	preimageBytes, err = hex.DecodeString(preimage)
	if err != nil {
		return nil, err
	}

	preimageHash = hex.EncodeToString(btcutil.Hash160(preimageBytes))

	receivers := []arksdk.Receiver{
		arksdk.NewBitcoinReceiver(myAddress, vhtlc.Amount, preimageHash),
	}

	vhtlcOutpoint := client.Outpoint{Txid: vhtlc.Txid, VOut: vhtlc.VOut}

	redeemTx, err := h.svc.SendAsync(ctx, false, receivers, arksdk.Options{
		FilterOutpoints: []client.Outpoint{vhtlcOutpoint},
		HtlcPreimages:   map[client.Outpoint]string{vhtlcOutpoint: preimage},
	})
	if err != nil {
		return nil, err
	}

	return &pb.BoltzClaimVHTLCResponse{RedeemTx: redeemTx}, nil
}

func (h *serviceHandler) ListVHTLC(ctx context.Context, req *pb.ListVHTLCRequest) (*pb.ListVHTLCResponse, error) {
	preimageHashFilter := req.GetPreimageHashFilter()

	vtxos, _, err := h.svc.ListVtxos(ctx)
	if err != nil {
		return nil, err
	}

	vhtlcs := make([]*pb.Vtxo, 0, len(vtxos))
	for _, vtxo := range vtxos {
		vtxoScript, err := bitcointree.ParseVtxoScript(vtxo.Descriptor)
		if err != nil {
			continue
		}

		if htlcVtxo, ok := vtxoScript.(*bitcointree.HTLCVtxoScript); ok {

			if len(preimageHashFilter) > 0 && htlcVtxo.PreimageHash != preimageHashFilter {
				continue
			}

			vhtlcs = append(vhtlcs, &pb.Vtxo{
				Outpoint: &pb.Input{
					Txid: vtxo.Txid,
					Vout: vtxo.VOut,
				},
				Receiver: &pb.Output{
					Descriptor_: vtxo.Descriptor,
					Amount:      vtxo.Amount,
				},
				Spent:     len(vtxo.SpentBy) > 0,
				RoundTxid: vtxo.RoundTxid,
				SpentBy:   vtxo.SpentBy,
				ExpireAt:  vtxo.ExpiresAt.Unix(),
			})
		}
	}

	return &pb.ListVHTLCResponse{Vhtlcs: vhtlcs}, nil
}

func (h *serviceHandler) BotlzFundVHTLC(ctx context.Context, req *pb.BotlzFundVHTLCRequest) (*pb.BotlzFundVHTLCResponse, error) {
	preimageHash := req.GetPreimageHash()
	amount := req.GetAmount()
	address := req.GetAddress()

	if len(preimageHash) <= 0 {
		return nil, status.Error(codes.InvalidArgument, "missing preimage hash")
	}

	if len(preimageHash) != 40 {
		return nil, status.Error(codes.InvalidArgument, "invalid preimage hash")
	}

	if amount <= 0 {
		return nil, status.Error(codes.InvalidArgument, "missing amount")
	}

	if len(address) <= 0 {
		return nil, status.Error(codes.InvalidArgument, "missing address")
	}

	logrus.Infof("Funding vHTLC with preimage hash %s, amount %d, address %s", preimageHash, amount, address)
	redeemTx, err := h.svc.ArkClient.SendAsync(ctx, false, []arksdk.Receiver{
		arksdk.NewBitcoinReceiver(address, amount, preimageHash),
	}, arksdk.Options{})
	if err != nil {
		return nil, err
	}

	return &pb.BotlzFundVHTLCResponse{RedeemTx: redeemTx}, nil
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

func (h *serviceHandler) Send(
	ctx context.Context, req *pb.SendRequest,
) (*pb.SendResponse, error) {
	address, err := parseAddress(req.GetAddress())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	amount, err := parseAmount(req.GetAmount())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	receivers := []arksdk.Receiver{
		arksdk.NewBitcoinReceiver(address, amount, ""),
	}
	roundId, err := h.svc.SendOffChain(ctx, false, receivers, arksdk.Options{})
	if err != nil {
		return nil, err
	}
	return &pb.SendResponse{RoundId: roundId}, nil
}

func (h *serviceHandler) SendAsync(
	ctx context.Context, req *pb.SendAsyncRequest,
) (*pb.SendAsyncResponse, error) {
	address, err := parseAddress(req.GetAddress())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	amount, err := parseAmount(req.GetAmount())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	receivers := []arksdk.Receiver{
		arksdk.NewBitcoinReceiver(address, amount, ""),
	}
	redeemTx, err := h.svc.SendAsync(ctx, false, receivers, arksdk.Options{})
	if err != nil {
		return nil, err
	}
	return &pb.SendAsyncResponse{RedeemTx: redeemTx}, nil
}

func (h *serviceHandler) SendOnchain(
	ctx context.Context, req *pb.SendOnchainRequest,
) (*pb.SendOnchainResponse, error) {
	address, err := parseAddress(req.GetAddress())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	amount, err := parseAmount(req.GetAmount())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	receivers := []arksdk.Receiver{
		arksdk.NewBitcoinReceiver(address, amount, ""),
	}
	txid, err := h.svc.SendOnChain(ctx, receivers)
	if err != nil {
		return nil, err
	}
	return &pb.SendOnchainResponse{Txid: txid}, nil
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
			Pending:      tx.IsPending,
		})
	}

	return &pb.GetTransactionHistoryResponse{Transactions: txs}, nil
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

func toTreeProto(tree tree.CongestionTree) *pb.Tree {
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

func toTxTypeProto(txType arksdk.TxType) pb.TxType {
	switch txType {
	case arksdk.TxSent:
		return pb.TxType_TX_TYPE_SENT
	case arksdk.TxReceived:
		return pb.TxType_TX_TYPE_RECEIVED
	default:
		return pb.TxType_TX_TYPE_UNSPECIFIED
	}
}
