package handlers

import (
	"context"
	"fmt"

	pb "github.com/ArkLabsHQ/ark-node/api-spec/protobuf/gen/go/ark_node/v1"
)

type walletHandler struct{}

func NewWalletHandler() pb.WalletServiceServer {
	return &walletHandler{}
}

func (h *walletHandler) GenSeed(
	ctx context.Context, req *pb.GenSeedRequest,
) (*pb.GenSeedResponse, error) {
	return nil, fmt.Errorf("not implemented")
}

// CreateWallet creates an HD Wallet based on signing seeds,
// encrypts them with the password and persists the encrypted seeds.
func (h *walletHandler) CreateWallet(
	ctx context.Context, req *pb.CreateWalletRequest,
) (*pb.CreateWalletResponse, error) {
	return nil, fmt.Errorf("not implemented")
}

// Unlock tries to unlock the HD Wallet using the given password.
func (h *walletHandler) Unlock(
	ctx context.Context, req *pb.UnlockRequest,
) (*pb.UnlockResponse, error) {
	return nil, fmt.Errorf("not implemented")
}

// Lock locks the HD wallet.
func (h *walletHandler) Lock(
	ctx context.Context, req *pb.LockRequest,
) (*pb.LockResponse, error) {
	return nil, fmt.Errorf("not implemented")
}

// ChangePassword changes the password used to encrypt/decrypt the HD seeds.
// It requires the wallet to be locked.
func (h *walletHandler) ChangePassword(
	ctx context.Context, req *pb.ChangePasswordRequest,
) (*pb.ChangePasswordResponse, error) {
	return nil, fmt.Errorf("not implemented")
}

func (h *walletHandler) RestoreWallet(
	req *pb.RestoreWalletRequest, stream pb.WalletService_RestoreWalletServer,
) error {
	return fmt.Errorf("not implemented")
}

// Status returns info about the status of the wallet.
func (h *walletHandler) Status(
	ctx context.Context, req *pb.StatusRequest,
) (*pb.StatusResponse, error) {
	return nil, fmt.Errorf("not implemented")
}

// GetWalletInfo returns info about the HD wallet.
func (h *walletHandler) GetWalletInfo(
	ctx context.Context, req *pb.GetWalletInfoRequest,
) (*pb.GetWalletInfoResponse, error) {
	return nil, fmt.Errorf("not implemented")
}

// Auth verifies whether the given password is valid without unlocking the wallet
func (h *walletHandler) Auth(
	ctx context.Context, req *pb.AuthRequest,
) (*pb.AuthResponse, error) {
	return nil, fmt.Errorf("not implemented")
}
