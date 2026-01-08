package handlers

import (
	"context"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	pb "github.com/ArkLabsHQ/fulmine/api-spec/protobuf/gen/go/fulmine/v1"
	"github.com/ArkLabsHQ/fulmine/internal/core/application"
	"github.com/ArkLabsHQ/fulmine/pkg/swap"
	"github.com/ArkLabsHQ/fulmine/utils"
	"github.com/arkade-os/arkd/pkg/ark-lib/intent"
	"github.com/arkade-os/go-sdk/types"
	"github.com/btcsuite/btcd/btcutil/psbt"
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
	config, err := h.svc.GetConfigData(ctx)
	if err != nil {
		return nil, err
	}

	if h.svc.IsLocked(ctx) {
		return nil, status.Error(codes.FailedPrecondition, "wallet is locked")
	}

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
		Pubkey:       pubkey,
		SignerPubkey: hex.EncodeToString(config.SignerPubKey.SerializeCompressed()),
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

	return &pb.GetRoundInfoResponse{
		Round: &pb.Round{
			Id:    roundId,
			Start: round.StartedAt,
			End:   round.EndedAt,
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
			Date:           tx.CreatedAt.Format(time.RFC3339),
			Amount:         tx.Amount,
			CommitmentTxid: tx.CommitmentTxid,
			ArkTxid:        tx.ArkTxid,
			BoardingTxid:   tx.BoardingTxid,
			Type:           toTxTypeProto(tx.Type),
			Settled:        tx.Settled,
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

	receivers := []types.Receiver{{To: address, Amount: amount}}
	var arkTxid string
	for range 3 {
		arkTxid, err = h.svc.SendOffChain(ctx, receivers)
		if err != nil {
			if strings.Contains(strings.ToLower(err.Error()), "vtxo_already_spent") {
				continue
			}
			return nil, err
		}
		break
	}
	if err != nil {
		return nil, err
	}
	return &pb.SendOffChainResponse{Txid: arkTxid}, nil
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
	txid, err := h.svc.SendOnChain(ctx, address, amount)
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

func (h *serviceHandler) ClaimVHTLC(
	ctx context.Context, req *pb.ClaimVHTLCRequest,
) (*pb.ClaimVHTLCResponse, error) {
	preimage := req.GetPreimage()
	if len(preimage) <= 0 {
		return nil, status.Error(codes.InvalidArgument, "missing preimage")
	}

	preimageBytes, err := hex.DecodeString(preimage)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid preimage")
	}

	vhtlcId := req.GetVhtlcId()
	if vhtlcId == "" {
		return nil, status.Error(codes.InvalidArgument, "missing vhtlc id")
	}

	redeemTxid, err := h.svc.ClaimVHTLC(ctx, preimageBytes, vhtlcId)
	if err != nil {
		return nil, err
	}

	return &pb.ClaimVHTLCResponse{RedeemTxid: redeemTxid}, nil
}

func (h *serviceHandler) RefundVHTLCWithoutReceiver(
	ctx context.Context, req *pb.RefundVHTLCWithoutReceiverRequest,
) (*pb.RefundVHTLCWithoutReceiverResponse, error) {
	vhtlcId := req.GetVhtlcId()
	if vhtlcId == "" {
		return nil, status.Error(codes.InvalidArgument, "missing vhtlc id")
	}
	withReceiver := true
	withoutReceiver := !withReceiver

	redeemTxid, err := h.svc.RefundVHTLC(ctx, "", vhtlcId, withoutReceiver)
	if err != nil {
		return nil, err
	}

	return &pb.RefundVHTLCWithoutReceiverResponse{RedeemTxid: redeemTxid}, nil
}

func (h *serviceHandler) SettleVHTLC(
	ctx context.Context, req *pb.SettleVHTLCRequest,
) (*pb.SettleVHTLCResponse, error) {
	vhtlcId := req.GetVhtlcId()
	if vhtlcId == "" {
		return nil, status.Error(codes.InvalidArgument, "missing vhtlc id")
	}

	var txid string
	var err error

	switch settlement := req.GetSettlementType().(type) {
	case *pb.SettleVHTLCRequest_Claim:
		preimage := settlement.Claim.GetPreimage()
		if len(preimage) == 0 {
			return nil, status.Error(codes.InvalidArgument, "missing preimage")
		}

		preimageBytes, err := hex.DecodeString(preimage)
		if err != nil {
			return nil, status.Error(
				codes.InvalidArgument, fmt.Sprintf("invalid preimage: %v", err),
			)
		}

		txid, err = h.svc.SettleVHTLCWithClaimPath(ctx, vhtlcId, preimageBytes)
		if err != nil {
			return nil, err
		}
	case *pb.SettleVHTLCRequest_Refund:
		refund := settlement.Refund

		if params := refund.GetDelegateParams(); params != nil {
			message := params.GetIntentMessage()
			proof := params.GetSignedIntentProof()
			forfeitTx := params.GetPartialForfeitTx()

			var buf intent.RegisterMessage
			if err := buf.Decode(params.GetIntentMessage()); err != nil {
				return nil, status.Error(codes.InvalidArgument, err.Error())
			}
			if _, err := psbt.NewFromRawBytes(strings.NewReader(proof), true); err != nil {
				return nil, status.Error(
					codes.InvalidArgument, fmt.Sprintf("invalid signed intent proof: %v", err),
				)
			}
			if _, err := psbt.NewFromRawBytes(strings.NewReader(forfeitTx), true); err != nil {
				return nil, status.Error(
					codes.InvalidArgument, fmt.Sprintf("invalid forfeit tx: %v", err),
				)
			}

			txid, err = h.svc.SettleVHTLCWithCollaborativeRefundPath(
				ctx, vhtlcId, proof, message, forfeitTx,
			)
			if err != nil {
				return nil, err
			}
		} else {
			txid, err = h.svc.SettleVHTLCWithRefundPath(ctx, vhtlcId)
			if err != nil {
				return nil, err
			}
		}
	default:
		return nil, status.Error(codes.InvalidArgument, "unknown settlement_type")
	}

	return &pb.SettleVHTLCResponse{Txid: txid}, nil
}

func (h *serviceHandler) ListVHTLC(ctx context.Context, req *pb.ListVHTLCRequest) (*pb.ListVHTLCResponse, error) {
	vtxos, _, err := h.svc.ListVHTLC(ctx, req.GetVhtlcId())
	if err != nil {
		return nil, err
	}

	return &pb.ListVHTLCResponse{Vhtlcs: toVtxosProto(vtxos)}, nil
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

	addr, vhtlc_id, vhtlcScript, err := h.svc.GetSwapVHTLC(
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
		Id:                                   vhtlc_id,
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

	resp, err := h.svc.GetInvoice(ctx, amount)
	if err != nil {
		return nil, err
	}

	return &pb.GetInvoiceResponse{
		Invoice: resp.Invoice,
	}, nil
}

func (h *serviceHandler) PayInvoice(
	ctx context.Context, req *pb.PayInvoiceRequest,
) (*pb.PayInvoiceResponse, error) {
	invoice, err := parseInvoice(req.GetInvoice())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	// Handle BOLT11 and BOLT12
	if utils.IsValidInvoice(invoice) {
		resp, err := h.svc.PayInvoice(ctx, invoice)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "paying invoice failed: %v", err)
		}
		return &pb.PayInvoiceResponse{Txid: resp.TxId}, nil
	}

	if swap.IsBolt12Offer(invoice) {
		resp, err := h.svc.PayOffer(ctx, invoice)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "paying offer failed: %v", err)
		}
		return &pb.PayInvoiceResponse{Txid: resp.TxId}, nil
	}

	return nil, status.Error(codes.InvalidArgument, "invoice string is neither valid BOLT11 nor BOLT12 offer")
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

func (h *serviceHandler) GetVirtualTxs(
	ctx context.Context, req *pb.GetVirtualTxsRequest,
) (*pb.GetVirtualTxsResponse, error) {
	txids := req.GetTxids()

	// Filter out empty strings
	filteredTxids := make([]string, 0, len(txids))
	for _, txid := range txids {
		if txid != "" {
			filteredTxids = append(filteredTxids, txid)
		}
	}

	// If no valid txids, return empty list
	if len(filteredTxids) == 0 {
		return &pb.GetVirtualTxsResponse{
			Txs: []string{},
		}, nil
	}

	txs, err := h.svc.GetVirtualTxs(ctx, filteredTxids)
	if err != nil {
		return nil, err
	}

	return &pb.GetVirtualTxsResponse{
		Txs: txs,
	}, nil
}
