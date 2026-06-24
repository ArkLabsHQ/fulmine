package handlers

import (
	"context"
	"fmt"

	pb "github.com/ArkLabsHQ/fulmine/api-spec/protobuf/gen/go/fulmine/v1"
	"github.com/ArkLabsHQ/fulmine/internal/core/application"
	"github.com/ArkLabsHQ/fulmine/internal/core/ports"
	"github.com/ArkLabsHQ/fulmine/utils"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type walletHandler struct {
	svc      *application.Service
	unlocker ports.Unlocker
}

func NewWalletHandler(appSvc *application.Service, unlocker ports.Unlocker) pb.WalletServiceServer {
	return &walletHandler{svc: appSvc, unlocker: unlocker}
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
	var password string
	if autoPassword, ok := h.autoUnlockPassword(ctx); ok && !h.svc.IsInitialized() {
		// An unlocker is configured: create the wallet with its password so the
		// daemon can auto-unlock afterwards. The unlocker password is
		// authoritative, so any submitted value is ignored (mirrors the web
		// onboarding flow).
		password = autoPassword
	} else {
		password, err = parsePassword(req.GetPassword())
		if err != nil {
			return nil, status.Error(codes.InvalidArgument, err.Error())
		}
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

// autoUnlockPassword returns the password from the configured unlocker (env or
// file based) and whether one is configured, so CreateWallet can create the
// wallet with the same password the daemon uses to auto-unlock — matching the
// web onboarding flow.
func (h *walletHandler) autoUnlockPassword(ctx context.Context) (string, bool) {
	if h.unlocker == nil {
		return "", false
	}
	password, err := h.unlocker.GetPassword(ctx)
	if err != nil {
		log.WithError(err).Warn("failed to get password from unlocker")
		return "", false
	}
	if password == "" {
		// The file-based unlocker can return an empty password when its file is
		// empty or whitespace-only. Treat that as "no unlocker password" so the
		// request-password validation still applies instead of creating the
		// wallet with an empty password.
		log.Warn("unlocker returned an empty password; falling back to the standard password flow")
		return "", false
	}
	return password, true
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
	if err := h.svc.LockNode(ctx); err != nil {
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
	isInitialized := h.svc.IsInitialized()
	// nolint
	isSynced, _ := h.svc.IsSynced()
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
