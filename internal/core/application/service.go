package application

import (
	"context"
	"encoding/hex"
	"fmt"

	"github.com/ArkLabsHQ/ark-node/internal/core/domain"
	arksdk "github.com/ark-network/ark/pkg/client-sdk"
	"github.com/ark-network/ark/pkg/client-sdk/client"
	grpcclient "github.com/ark-network/ark/pkg/client-sdk/client/grpc"
	store "github.com/ark-network/ark/pkg/client-sdk/store"
	"github.com/tyler-smith/go-bip32"
	"github.com/tyler-smith/go-bip39"
)

var defaultSettings = domain.Settings{
	ApiRoot:     "https://fulmine.io/api/D9D90N192031",
	AspUrl:      "http://localhost:7000",
	Currency:    "usd",
	EventServer: "http://arklabs.to/node/jupiter29",
	FullNode:    "http://arklabs.to/node/213908123",
	LnConnect:   false,
	LnUrl:       "lndconnect://192.168.1.4:10009",
	Unit:        "sat",
}

type BuildInfo struct {
	Version string
	Commit  string
	Date    string
}

type Service struct {
	BuildInfo BuildInfo

	arksdk.ArkClient
	storeRepo    store.ConfigStore
	settingsRepo domain.SettingsRepository
	grpcClient   client.ASPClient

	isReady bool
}

func NewService(
	buildInfo BuildInfo,
	storeSvc store.ConfigStore, settingsRepo domain.SettingsRepository,
) (*Service, error) {
	if arkClient, err := arksdk.LoadCovenantlessClient(storeSvc); err == nil {
		data, err := arkClient.GetConfigData(context.Background())
		if err != nil {
			return nil, err
		}
		client, err := grpcclient.NewClient(data.AspUrl)
		if err != nil {
			return nil, err
		}
		return &Service{
			buildInfo, arkClient, storeSvc, settingsRepo, client, true,
		}, nil
	}

	ctx := context.Background()
	if _, err := settingsRepo.GetSettings(ctx); err != nil {
		if err := settingsRepo.AddSettings(
			ctx, defaultSettings,
		); err != nil {
			return nil, err
		}
	}
	arkClient, err := arksdk.NewCovenantlessClient(storeSvc)
	if err != nil {
		// nolint:all
		settingsRepo.CleanSettings(ctx)
		return nil, err
	}

	return &Service{buildInfo, arkClient, storeSvc, settingsRepo, nil, false}, nil
}

func (s *Service) IsReady() bool {
	return s.isReady
}

func (s *Service) Setup(
	ctx context.Context, aspURL, password, mnemonic string,
) (err error) {
	if err := s.settingsRepo.UpdateSettings(
		ctx, domain.Settings{AspUrl: aspURL},
	); err != nil {
		return err
	}

	defer func() {
		if err != nil {
			// nolint:all
			s.settingsRepo.UpdateSettings(ctx, domain.Settings{AspUrl: ""})
		}
	}()

	privateKey, err := privateKeyFromMnemonic(mnemonic)
	if err != nil {
		return err
	}

	client, err := grpcclient.NewClient(aspURL)
	if err != nil {
		return err
	}

	if err := s.Init(ctx, arksdk.InitArgs{
		WalletType: arksdk.SingleKeyWallet,
		ClientType: arksdk.GrpcClient,
		AspUrl:     aspURL,
		Password:   password,
		Seed:       privateKey,
	}); err != nil {
		return err
	}

	s.grpcClient = client
	s.isReady = true
	return nil
}

func (s *Service) Reset(ctx context.Context) error {
	backup, err := s.settingsRepo.GetSettings(ctx)
	if err != nil {
		return err
	}
	if err := s.settingsRepo.CleanSettings(ctx); err != nil {
		return err
	}
	if err := s.storeRepo.CleanData(ctx); err != nil {
		// nolint:all
		s.settingsRepo.AddSettings(ctx, *backup)
		return err
	}
	return nil
}

func (s *Service) GetSettings(ctx context.Context) (*domain.Settings, error) {
	sett, err := s.settingsRepo.GetSettings(ctx)
	return sett, err
}

func (s *Service) NewSettings(
	ctx context.Context, settings domain.Settings,
) error {
	return s.settingsRepo.AddSettings(ctx, settings)
}

func (s *Service) UpdateSettings(
	ctx context.Context, settings domain.Settings,
) error {
	return s.settingsRepo.UpdateSettings(ctx, settings)
}

func (s *Service) GetAddress(
	ctx context.Context, sats uint64,
) (bip21Addr, offchainAddr, boardingAddr string, err error) {
	offchainAddr, boardingAddr, err = s.Receive(ctx)
	if err != nil {
		return
	}
	bip21Addr = fmt.Sprintf("bitcoin:%s?ark=%s", boardingAddr, offchainAddr)
	// add amount if passed
	if sats > 0 {
		amount := fmt.Sprintf("&amount=%d", sats)
		bip21Addr += amount
	}
	return
}

func (s *Service) GetTotalBalance(ctx context.Context) (uint64, error) {
	balance, err := s.Balance(ctx, false)
	if err != nil {
		return 0, err
	}
	onchainBalance := balance.OnchainBalance.SpendableAmount
	for _, amount := range balance.OnchainBalance.LockedAmount {
		onchainBalance += amount.Amount
	}
	return balance.OffchainBalance.Total + onchainBalance, nil
}

func (s *Service) GetRound(ctx context.Context, roundId string) (*client.Round, error) {
	if !s.isReady {
		return nil, fmt.Errorf("service not iniitialized")
	}
	return s.grpcClient.GetRoundByID(ctx, roundId)
}

func privateKeyFromMnemonic(mnemonic string) (string, error) {
	seed := bip39.NewSeed(mnemonic, "")
	key, err := bip32.NewMasterKey(seed)
	if err != nil {
		return "", err
	}

	// TODO: validate this path
	derivationPath := []uint32{
		bip32.FirstHardenedChild + 44,
		bip32.FirstHardenedChild + 1237,
		bip32.FirstHardenedChild + 0,
		0,
		0,
	}

	next := key
	for _, idx := range derivationPath {
		var err error
		if next, err = next.NewChildKey(idx); err != nil {
			return "", err
		}
	}

	return hex.EncodeToString(next.Key), nil
}
