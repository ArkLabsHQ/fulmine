package handlers

import (
	"context"
	"fmt"

	pb "github.com/ArkLabsHQ/ark-node/api-spec/protobuf/gen/go/ark_node/v1"
	"github.com/ArkLabsHQ/ark-node/internal/core/application"
	"github.com/ArkLabsHQ/ark-node/utils"
	"github.com/tyler-smith/go-bip39"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type walletHandler struct {
	svc *application.Service
}

func NewWalletHandler(appSvc *application.Service) pb.WalletServiceServer {
	return &walletHandler{appSvc}
}

func (h *walletHandler) GenSeed(
	ctx context.Context, req *pb.GenSeedRequest,
) (*pb.GenSeedResponse, error) {
	entropy, err := bip39.NewEntropy(128)
	if err != nil {
		return nil, err
	}
	mnemonic, err := bip39.NewMnemonic(entropy)
	if err != nil {
		return nil, err
	}
	return &pb.GenSeedResponse{Mnemonic: mnemonic}, nil
}

// CreateWallet creates an HD Wallet based on signing seeds,
// encrypts them with the password and persists the encrypted seeds.
func (h *walletHandler) CreateWallet(
	ctx context.Context, req *pb.CreateWalletRequest,
) (*pb.CreateWalletResponse, error) {
	aspUrl, err := parseAspUrl(req.GetAspUrl())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	password, err := parsePassword(req.GetPassword())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	mnemonic, err := parseMnemonic(req.GetMnemonic())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	if err := h.svc.Setup(ctx, aspUrl, password, mnemonic); err != nil {
		return nil, err
	}
	return &pb.CreateWalletResponse{}, nil
}

// Unlock tries to unlock the HD Wallet using the given password.
func (h *walletHandler) Unlock(
	ctx context.Context, req *pb.UnlockRequest,
) (*pb.UnlockResponse, error) {
	password, err := parsePassword(req.GetPassword())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	if err := h.svc.UnlockNode(ctx, password); err != nil {
		return nil, err
	}
	return &pb.UnlockResponse{}, nil
}

// Lock locks the HD wallet.
func (h *walletHandler) Lock(
	ctx context.Context, req *pb.LockRequest,
) (*pb.LockResponse, error) {
	password, err := parsePassword(req.GetPassword())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	if err := h.svc.Lock(ctx, password); err != nil {
		return nil, err
	}
	return &pb.LockResponse{}, nil
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
	isInitialized := h.svc.IsReady()
	isSynced := isInitialized
	var isUnlocked bool
	if isInitialized {
		isUnlocked = !h.svc.IsLocked(ctx)
	}
	return &pb.StatusResponse{
		Initialized: isInitialized,
		Unlocked:    isUnlocked,
		Synced:      isSynced,
	}, nil
}

// Auth verifies whether the given password is valid without unlocking the wallet
func (h *walletHandler) Auth(
	ctx context.Context, req *pb.AuthRequest,
) (*pb.AuthResponse, error) {
	return nil, fmt.Errorf("not implemented")
}

func parseAspUrl(a string) (string, error) {
	if len(a) <= 0 {
		return "", fmt.Errorf("missing asp url")
	}
	if !utils.IsValidURL(a) {
		return "", fmt.Errorf("invalid asp url")
	}
	return a, nil
}

func parseMnemonic(m string) (string, error) {
	if len(m) <= 0 {
		return "", fmt.Errorf("missing mnemonic")
	}
	if err := utils.IsValidMnemonic(m); err != nil {
		return "", err
	}
	return m, nil
}

func parsePassword(p string) (string, error) {
	if len(p) <= 0 {
		return "", fmt.Errorf("missing password")
	}
	if err := utils.IsValidPassword(p); err != nil {
		return "", err
	}
	return p, nil
}
