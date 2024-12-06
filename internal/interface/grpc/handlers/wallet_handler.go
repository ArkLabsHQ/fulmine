package handlers

import (
	"context"
	"fmt"
	"strings"

	pb "github.com/ArkLabsHQ/ark-node/api-spec/protobuf/gen/go/ark_node/v1"
	"github.com/ArkLabsHQ/ark-node/internal/core/application"
	"github.com/ArkLabsHQ/ark-node/utils"
	"github.com/nbd-wtf/go-nostr/nip19"
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
	hex := utils.GetNewPrivateKey()
	nsec, err := utils.SeedToNsec(hex)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	return &pb.GenSeedResponse{Hex: hex, Nsec: nsec}, nil
}

// CreateWallet creates an HD Wallet based on signing seeds,
// encrypts them with the password and persists the encrypted seeds.
func (h *walletHandler) CreateWallet(
	ctx context.Context, req *pb.CreateWalletRequest,
) (*pb.CreateWalletResponse, error) {
	serverUrl, err := parseServerUrl(req.GetServerUrl())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	password, err := parsePassword(req.GetPassword())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	privateKey, err := parsePrivateKey(req.GetPrivateKey())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	if err := h.svc.Setup(ctx, serverUrl, password, privateKey); err != nil {
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

func parseServerUrl(a string) (string, error) {
	if len(a) == 0 {
		return "", fmt.Errorf("missing server url")
	}
	if !utils.IsValidURL(a) {
		return "", fmt.Errorf("invalid server url")
	}
	return a, nil
}

func parsePassword(p string) (string, error) {
	if len(p) == 0 {
		return "", fmt.Errorf("missing password")
	}
	if err := utils.IsValidPassword(p); err != nil {
		return "", err
	}
	return p, nil
}

func parsePrivateKey(sk string) (string, error) {
	if len(sk) == 0 {
		return "", fmt.Errorf("missing private key")
	}
	if strings.HasPrefix(sk, "nsec") {
		_, seed, err := nip19.Decode(sk)
		if err != nil {
			return "", err
		}
		sk = fmt.Sprint(seed)
	}
	if err := utils.IsValidPrivateKey(sk); err != nil {
		return "", err
	}
	return sk, nil
}
