package application

import (
	"context"
	"encoding/hex"
	"fmt"

	"github.com/ArkLabsHQ/ark-node/internal/core/domain"
	arksdk "github.com/ark-network/ark/pkg/client-sdk"
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

type Service struct {
	arksdk.ArkClient
	storeRepo    store.ConfigStore
	settingsRepo domain.SettingsRepository

	isReady bool
}

func NewService(
	storeSvc store.ConfigStore, settingsRepo domain.SettingsRepository,
) (*Service, error) {
	if arkClient, err := arksdk.LoadCovenantlessClient(storeSvc); err == nil {
		return &Service{arkClient, storeSvc, settingsRepo, true}, nil
	}

	ctx := context.Background()
	if err := settingsRepo.AddSettings(
		ctx, defaultSettings,
	); err != nil {
		return nil, err
	}
	arkClient, err := arksdk.NewCovenantlessClient(storeSvc)
	if err != nil {
		//nolint:all
		settingsRepo.CleanSettings(ctx)
		return nil, err
	}

	return &Service{arkClient, storeSvc, settingsRepo, false}, nil
}

func (s *Service) IsReady() bool {
	return s.isReady
}

func (s *Service) Setup(ctx context.Context, aspURL, password, mnemonic string) error {
	fmt.Println(aspURL, password, mnemonic)
	if err := s.settingsRepo.UpdateSettings(
		ctx, domain.Settings{AspUrl: aspURL},
	); err != nil {
		return err
	}

	privateKey, err := privateKeyFromMnemonic(mnemonic)
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
		//nolint:all
		s.settingsRepo.UpdateSettings(ctx, domain.Settings{AspUrl: ""})
		return err
	}

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
		//nolint:all
		s.settingsRepo.AddSettings(ctx, *backup)
		return err
	}
	return nil
}

func (s *Service) GetSettings(ctx context.Context) (*domain.Settings, error) {
	sett, err := s.settingsRepo.GetSettings(ctx)
	fmt.Printf("%+v\n", sett)
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
