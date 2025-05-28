package handlers

import (
	"context"
	"encoding/hex"
	"time"

	pb "github.com/ArkLabsHQ/fulmine/api-spec/protobuf/gen/go/fulmine/v1"
	"github.com/ArkLabsHQ/fulmine/internal/core/application"
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

func (h *serviceHandler) GetAddress(
	ctx context.Context, req *pb.GetAddressRequest,
) (*pb.GetAddressResponse, error) {
	bip21Addr, _, _, _, pubkey, err := h.svc.GetAddress(ctx, 0)
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
	_, _, _, _, pubkey, err := h.svc.GetAddress(ctx, 0)
	if err != nil {
		return nil, err
	}

	response := &pb.GetInfoResponse{
		BuildInfo: &pb.BuildInfo{
			Version: h.svc.BuildInfo.Version,
			Commit:  h.svc.BuildInfo.Commit,
			Date:    h.svc.BuildInfo.Date,
		},
		Pubkey: pubkey,
	}

	// Try to get network info, but don't fail if wallet is not initialized
	data, err := h.svc.GetConfigData(ctx)
	if err == nil && data != nil {
		// Only set Network field if we successfully got config data
		response.Network = toNetworkProto(data.Network.Name)
		response.AddrPrefix = data.Network.Addr
		response.ServerUrl = data.ServerUrl
	}

	return response, nil
}

func (h *serviceHandler) GetOnboardAddress(
	ctx context.Context, req *pb.GetOnboardAddressRequest,
) (*pb.GetOnboardAddressResponse, error) {
	_, _, addr, _, _, err := h.svc.GetAddress(ctx, 0)
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

func (h *serviceHandler) Settle(
	ctx context.Context, req *pb.SettleRequest,
) (*pb.SettleResponse, error) {
	txid, err := h.svc.Settle(ctx)
	if err != nil {
		return nil, err
	}
	return &pb.SettleResponse{Txid: txid}, nil
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

func (h *serviceHandler) SignTransaction(
	ctx context.Context, req *pb.SignTransactionRequest,
) (*pb.SignTransactionResponse, error) {
	tx, err := parseTransaction(req.GetTx())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	signedTx, err := h.svc.SignTransaction(ctx, tx)
	if err != nil {
		return nil, err
	}
	return &pb.SignTransactionResponse{SignedTx: signedTx}, nil
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

func (h *serviceHandler) RefundVHTLCWithoutReceiver(ctx context.Context, req *pb.RefundVHTLCWithoutReceiverRequest) (*pb.RefundVHTLCWithoutReceiverResponse, error) {
	preimageHash, err := parsePreimageHash(req.GetPreimageHash())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	withReceiver := true
	withoutReceiver := !withReceiver

	redeemTxid, err := h.svc.RefundVHTLC(ctx, "", preimageHash, withoutReceiver)
	if err != nil {
		return nil, err
	}

	return &pb.RefundVHTLCWithoutReceiverResponse{RedeemTxid: redeemTxid}, nil
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
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request cannot be nil")
	}

	if req.GetPreimageHash() == "" {
		return nil, status.Error(codes.InvalidArgument, "preimage hash is required")
	}

	receiverPubkey, err := parsePubkey(req.GetReceiverPubkey())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid receiver pubkey")
	}
	senderPubkey, err := parsePubkey(req.GetSenderPubkey())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid sender pubkey")
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
	refundLocktime := parseAbsoluteLocktime(req.GetRefundLocktime())
	unilateralClaimDelay := parseRelativeLocktime(req.GetUnilateralClaimDelay())
	unilateralRefundDelay := parseRelativeLocktime(req.GetUnilateralRefundDelay())
	unilateralRefundWithoutReceiverDelay := parseRelativeLocktime(req.GetUnilateralRefundWithoutReceiverDelay())

	addr, vhtlcScript, _, err := h.svc.GetVHTLC(
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
		ClaimPubkey:                          hex.EncodeToString(vhtlcScript.Receiver.SerializeCompressed()[1:]),
		RefundPubkey:                         hex.EncodeToString(vhtlcScript.Sender.SerializeCompressed()[1:]),
		ServerPubkey:                         hex.EncodeToString(vhtlcScript.Server.SerializeCompressed()[1:]),
		SwapTree:                             toSwapTreeProto(vhtlcScript),
		RefundLocktime:                       int64(vhtlcScript.RefundWithoutReceiverClosure.Locktime),
		UnilateralClaimDelay:                 int64(vhtlcScript.UnilateralClaimClosure.Locktime.Value),
		UnilateralRefundDelay:                int64(vhtlcScript.UnilateralRefundClosure.Locktime.Value),
		UnilateralRefundWithoutReceiverDelay: int64(vhtlcScript.UnilateralRefundWithoutReceiverClosure.Locktime.Value),
	}, nil
}

func (h *serviceHandler) GetInvoice(
	ctx context.Context, req *pb.GetInvoiceRequest,
) (*pb.GetInvoiceResponse, error) {
	amount, err := parseAmount(req.GetAmount())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	invoice, err := h.svc.GetInvoice(ctx, amount)
	if err != nil {
		return nil, err
	}

	return &pb.GetInvoiceResponse{
		Invoice: invoice,
	}, nil
}

func (h *serviceHandler) PayInvoice(
	ctx context.Context, req *pb.PayInvoiceRequest,
) (*pb.PayInvoiceResponse, error) {
	invoice, err := parseInvoice(req.GetInvoice())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	txid, err := h.svc.PayInvoice(ctx, invoice)
	if err != nil {
		return nil, err
	}

	return &pb.PayInvoiceResponse{Txid: txid}, nil
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
