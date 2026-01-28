package handlers

import (
	"context"
	"strconv"
	"strings"

	pb "github.com/ArkLabsHQ/fulmine/api-spec/protobuf/gen/go/fulmine/v1"
	"github.com/ArkLabsHQ/fulmine/internal/core/application"
	"github.com/arkade-os/arkd/pkg/ark-lib/intent"
	"github.com/btcsuite/btcd/btcutil/psbt"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type delegatorHandler struct {
	svc *application.DelegatorService
}

func NewDelegatorHandler(svc *application.DelegatorService) pb.DelegatorServiceServer {
	return &delegatorHandler{svc}
}

func (h *delegatorHandler) GetDelegateInfo(
	ctx context.Context, req *pb.GetDelegateInfoRequest,
) (*pb.GetDelegateInfoResponse, error) {
	info, err := h.svc.GetDelegateInfo(ctx)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	return &pb.GetDelegateInfoResponse{
		Pubkey: info.DelegatorPublicKey,
		Fee: strconv.FormatUint(info.Fee, 10), // TODO: use CEL?
		DelegatorAddress: info.DelegatorAddress,
	}, nil
}

func (h *delegatorHandler) Delegate(
	ctx context.Context, req *pb.DelegateRequest,
) (*pb.DelegateResponse, error) {
	delegateIntent := req.GetIntent()
	message := delegateIntent.GetMessage()
	proof := delegateIntent.GetProof()
	forfeits := req.GetForfeits()

	var intentMessage intent.RegisterMessage
	if err := intentMessage.Decode(message); err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	proofPtx, err := psbt.NewFromRawBytes(strings.NewReader(proof), true)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	forfeitPtxs := make([]*psbt.Packet, 0, len(forfeits))
	for _, forfeit := range forfeits {
		forfeitPtx, err := psbt.NewFromRawBytes(strings.NewReader(forfeit), true)
		if err != nil {
			return nil, status.Error(codes.InvalidArgument, err.Error())
		}
		forfeitPtxs = append(forfeitPtxs, forfeitPtx)
	}

	intentProof := intent.Proof{Packet: *proofPtx}

	allowReplace := !req.GetRejectReplace()
	err = h.svc.Delegate(ctx, intentMessage, intentProof, forfeitPtxs, allowReplace)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	return &pb.DelegateResponse{}, nil
}
