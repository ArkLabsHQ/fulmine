package handlers

import (
	"context"
	"strconv"
	"strings"

	delegatev1 "github.com/ArkLabsHQ/fulmine/api-spec/protobuf/gen/go/delegate/v1"
	"github.com/ArkLabsHQ/fulmine/internal/core/application"
	"github.com/arkade-os/arkd/pkg/ark-lib/intent"
	"github.com/btcsuite/btcd/btcutil/psbt"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type delegateHandler struct {
	svc *application.DelegateService
}

func NewDelegateHandler(svc *application.DelegateService) delegatev1.DelegateServiceServer {
	return &delegateHandler{svc}
}

func (h *delegateHandler) GetDelegateInfo(
	ctx context.Context, req *delegatev1.GetDelegateInfoRequest,
) (*delegatev1.GetDelegateInfoResponse, error) {
	info, err := h.svc.GetInfo(ctx)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	return &delegatev1.GetDelegateInfoResponse{
		Pubkey:           info.PubKey,
		Fee:              strconv.FormatUint(info.Fee, 10), // TODO: use CEL?
		DelegatorAddress: info.Address,
		DelegateAddress:  info.Address,
	}, nil
}

func (h *delegateHandler) Delegate(
	ctx context.Context, req *delegatev1.DelegateRequest,
) (*delegatev1.DelegateResponse, error) {
	delegateIntent := req.GetIntent()
	message := delegateIntent.GetMessage()
	proof := delegateIntent.GetProof()

	var intentMessage intent.RegisterMessage
	if err := intentMessage.Decode(message); err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	proofPtx, err := psbt.NewFromRawBytes(strings.NewReader(proof), true)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	forfeitTxs := make([]*psbt.Packet, 0, len(req.GetForfeitTxs()))
	for _, forfeit := range req.GetForfeitTxs() {
		forfeitPtx, err := psbt.NewFromRawBytes(strings.NewReader(forfeit), true)
		if err != nil {
			return nil, status.Error(codes.InvalidArgument, err.Error())
		}
		forfeitTxs = append(forfeitTxs, forfeitPtx)
	}

	intentProof := intent.Proof{Packet: *proofPtx}

	allowReplace := !req.GetRejectReplace()
	err = h.svc.Delegate(ctx, intentMessage, intentProof, forfeitTxs, allowReplace)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	return &delegatev1.DelegateResponse{}, nil
}
