package handlers

import (
	"context"
	"encoding/hex"
	"time"

	pb "github.com/ArkLabsHQ/ark-node/api-spec/protobuf/gen/go/ark_node/v1"
	"github.com/ArkLabsHQ/ark-node/internal/core/application"
	"github.com/ark-network/ark/common"
	arksdk "github.com/ark-network/ark/pkg/client-sdk"
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
	receiverPubkey, err := parsePubkey(req.GetReceiverPubkey())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid pubkey")
	}
	senderPubkey, err := parsePubkey(req.GetSenderPubkey())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid pubkey")
	}

	receiverPubkeySet := receiverPubkey != nil
	senderPubkeySet := senderPubkey != nil
	if receiverPubkeySet == senderPubkeySet {
		return nil, status.Error(codes.InvalidArgument, "only one of receiver or sender public keys must be set")
	}

	preimageHashBytes, err := hex.DecodeString(req.GetPreimageHash())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid preimage hash")
	}

	// Parse optional locktime values
	var refundLocktime *common.AbsoluteLocktime
	if req.RefundLocktime > 0 {
		locktime := common.AbsoluteLocktime(req.RefundLocktime)
		refundLocktime = &locktime
	}

	// Parse unilateral claim delay
	var unilateralClaimDelay *common.RelativeLocktime
	if req.UnilateralClaimDelay != nil {
		delay := common.RelativeLocktime{
			Type:  toCommonLocktimeType(req.UnilateralClaimDelay.Type),
			Value: req.UnilateralClaimDelay.Value,
		}
		unilateralClaimDelay = &delay
	}

	// Parse unilateral refund delay
	var unilateralRefundDelay *common.RelativeLocktime
	if req.UnilateralRefundDelay != nil {
		delay := common.RelativeLocktime{
			Type:  toCommonLocktimeType(req.UnilateralRefundDelay.Type),
			Value: req.UnilateralRefundDelay.Value,
		}
		unilateralRefundDelay = &delay
	}

	// Parse unilateral refund without receiver delay
	var unilateralRefundWithoutReceiverDelay *common.RelativeLocktime
	if req.UnilateralRefundWithoutReceiverDelay != nil {
		delay := common.RelativeLocktime{
			Type:  toCommonLocktimeType(req.UnilateralRefundWithoutReceiverDelay.Type),
			Value: req.UnilateralRefundWithoutReceiverDelay.Value,
		}
		unilateralRefundWithoutReceiverDelay = &delay
	}

	addr, swapTree, err := h.svc.GetVHTLC(
		ctx,
		receiverPubkey,
		senderPubkey,
		preimageHashBytes,
		refundLocktime,
		unilateralClaimDelay,
		unilateralRefundDelay,
		unilateralRefundWithoutReceiverDelay,
	)
	if err != nil {
		return nil, err
	}
	return &pb.CreateVHTLCResponse{
		Address:                              addr,
		ClaimPubkey:                          hex.EncodeToString(swapTree.Receiver.SerializeCompressed()[1:]),
		RefundPubkey:                         hex.EncodeToString(swapTree.Sender.SerializeCompressed()[1:]),
		ServerPubkey:                         hex.EncodeToString(swapTree.Server.SerializeCompressed()[1:]),
		SwapTree:                             toSwapTreeProto(swapTree),
		RefundLocktime:                       int64(req.RefundLocktime),
		UnilateralClaimDelay:                 int64(req.UnilateralClaimDelay.Value),
		UnilateralRefundDelay:                int64(req.UnilateralRefundDelay.Value),
		UnilateralRefundWithoutReceiverDelay: int64(req.UnilateralRefundWithoutReceiverDelay.Value),
	}, nil
}

func toCommonLocktimeType(pbType pb.RelativeLocktime_LocktimeType) common.RelativeLocktimeType {
	switch pbType {
	case pb.RelativeLocktime_LOCKTIME_TYPE_BLOCK:
		return common.LocktimeTypeBlock
	case pb.RelativeLocktime_LOCKTIME_TYPE_SECOND:
		return common.LocktimeTypeSecond
	default:
		return common.LocktimeTypeBlock
	}
}

func (h *serviceHandler) GetAddress(
	ctx context.Context, req *pb.GetAddressRequest,
) (*pb.GetAddressResponse, error) {
	bip21Addr, _, _, pubkey, err := h.svc.GetAddress(ctx, 0)
	if err != nil {
		return nil, err
	}
	return &pb.GetAddressResponse{
		Address: bip21Addr,
		Pubkey:  pubkey,
	}, nil
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
	_, _, addr, _, err := h.svc.GetAddress(ctx, 0)
	if err != nil {
		return nil, err
	}
	return &pb.GetOnboardAddressResponse{Address: addr}, nil
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

func (h *serviceHandler) RedeemNote(
	ctx context.Context, req *pb.RedeemNoteRequest,
) (*pb.RedeemNoteResponse, error) {
	note, err := parseNote(req.GetNote())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	txid, err := h.svc.RedeemNotes(ctx, []string{note})
	if err != nil {
		return nil, err
	}
	return &pb.RedeemNoteResponse{Txid: txid}, nil
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
	roundId, err := h.svc.SendOffChain(ctx, false, receivers, true)
	if err != nil {
		return nil, err
	}
	return &pb.SendOffChainResponse{Txid: roundId}, nil
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
	txid, err := h.svc.CollaborativeExit(ctx, address, amount, false)
	if err != nil {
		return nil, err
	}
	return &pb.SendOnChainResponse{Txid: txid}, nil
}

func (h *serviceHandler) CreateInvoice(
	ctx context.Context, req *pb.CreateInvoiceRequest,
) (*pb.CreateInvoiceResponse, error) {
	amount, err := parseAmount(req.GetAmount())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	memo := req.GetMemo()
	preimage := req.GetPreimage()

	invoice, preimageHash, err := h.svc.GetInvoice(ctx, amount, memo, preimage)
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

func (h *serviceHandler) IsInvoiceSettled(
	ctx context.Context, req *pb.IsInvoiceSettledRequest,
) (*pb.IsInvoiceSettledResponse, error) {
	invoice, err := parseInvoice(req.GetInvoice())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	settled, err := h.svc.IsInvoiceSettled(ctx, invoice)
	if err != nil {
		return nil, err
	}

	return &pb.IsInvoiceSettledResponse{Settled: settled}, nil
}

func (h *serviceHandler) GetDelegatePublicKey(
	ctx context.Context, req *pb.GetDelegatePublicKeyRequest,
) (*pb.GetDelegatePublicKeyResponse, error) {
	pubKey, err := h.svc.GetDelegatePublicKey(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get delegate public key: %v", err)
	}

	return &pb.GetDelegatePublicKeyResponse{
		PublicKey: pubKey,
	}, nil
}

func (h *serviceHandler) WatchAddressForRollover(
	ctx context.Context,
	req *pb.WatchAddressForRolloverRequest,
) (*pb.WatchAddressForRolloverResponse, error) {
	err := h.svc.WatchAddressForRollover(
		ctx, req.RolloverAddress.Address,
		req.RolloverAddress.DestinationAddress,
		req.RolloverAddress.TaprootTree.Scripts,
	)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to watch address: %v", err)
	}

	return &pb.WatchAddressForRolloverResponse{}, nil
}

func (h *serviceHandler) UnwatchAddress(
	ctx context.Context, req *pb.UnwatchAddressRequest,
) (*pb.UnwatchAddressResponse, error) {
	err := h.svc.UnwatchAddress(ctx, req.Address)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to unwatch address: %v", err)
	}

	return &pb.UnwatchAddressResponse{}, nil
}

func (h *serviceHandler) ListWatchedAddresses(
	ctx context.Context, req *pb.ListWatchedAddressesRequest,
) (*pb.ListWatchedAddressesResponse, error) {
	targets, err := h.svc.ListWatchedAddresses(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list watched addresses: %v", err)
	}

	rolloverAddresses := make([]*pb.RolloverAddress, 0, len(targets))
	for _, target := range targets {
		rolloverAddresses = append(rolloverAddresses, &pb.RolloverAddress{
			Address: target.Address,
			TaprootTree: &pb.Tapscripts{
				Scripts: target.TaprootTree,
			},
			DestinationAddress: target.DestinationAddress,
		})
	}

	return &pb.ListWatchedAddressesResponse{
		Addresses: rolloverAddresses,
	}, nil
}
