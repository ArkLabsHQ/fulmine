package handlers

import (
	"context"
	"fmt"

	pb "github.com/ArkLabsHQ/ark-wallet/api-spec/protobuf/gen/go/ark_wallet/v1"
)

type serviceHandler struct{}

func NewServiceHandler() pb.ServiceServer {
	return &serviceHandler{}
}

func (h *serviceHandler) GetAddress(
	ctx context.Context, req *pb.GetAddressRequest,
) (*pb.GetAddressResponse, error) {
	return nil, fmt.Errorf("not implemented")
}

func (h *serviceHandler) GetBalance(
	ctx context.Context, req *pb.GetBalanceRequest,
) (*pb.GetBalanceResponse, error) {
	return nil, fmt.Errorf("not implemented")
}

func (h *serviceHandler) GetInfo(
	ctx context.Context, req *pb.GetInfoRequest,
) (*pb.GetInfoResponse, error) {
	return nil, fmt.Errorf("not implemented")
}

func (h *serviceHandler) GetOnboardAddress(
	ctx context.Context, req *pb.GetOnboardAddressRequest,
) (*pb.GetOnboardAddressResponse, error) {
	return nil, fmt.Errorf("not implemented")
}

func (h *serviceHandler) Send(
	ctx context.Context, req *pb.SendRequest,
) (*pb.SendResponse, error) {
	return nil, fmt.Errorf("not implemented")
}

func (h *serviceHandler) SendOnchain(
	ctx context.Context, req *pb.SendOnchainRequest,
) (*pb.SendOnchainResponse, error) {
	return nil, fmt.Errorf("not implemented")
}

func (h *serviceHandler) GetSendOnchainFees(
	ctx context.Context, req *pb.GetSendOnchainFeesRequest,
) (*pb.GetSendOnchainFeesResponse, error) {
	return nil, fmt.Errorf("not implemented")
}

func (h *serviceHandler) GetRoundInfo(
	ctx context.Context, req *pb.GetRoundInfoRequest,
) (*pb.GetRoundInfoResponse, error) {
	return nil, fmt.Errorf("not implemented")
}

func (h *serviceHandler) GetTransactions(
	ctx context.Context, req *pb.GetTransactionsRequest,
) (*pb.GetTransactionsResponse, error) {
	return nil, fmt.Errorf("not implemented")
}
