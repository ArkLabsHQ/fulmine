package application

import (
	"context"
	"fmt"
	"time"

	"github.com/ArkLabsHQ/ark-node/internal/core/domain"
	"github.com/ArkLabsHQ/ark-node/internal/core/ports"
	"github.com/ArkLabsHQ/ark-node/utils"
	arksdk "github.com/ark-network/ark/pkg/client-sdk"
	"github.com/ark-network/ark/pkg/client-sdk/client"
	grpcclient "github.com/ark-network/ark/pkg/client-sdk/client/grpc"
	store "github.com/ark-network/ark/pkg/client-sdk/store"
	"github.com/sirupsen/logrus"
)

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
	schedulerSvc ports.SchedulerService

	isReady bool
}

func NewService(
	buildInfo BuildInfo,
	storeSvc store.ConfigStore,
	settingsRepo domain.SettingsRepository,
	schedulerSvc ports.SchedulerService,
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
			buildInfo, arkClient, storeSvc, settingsRepo, client, schedulerSvc, true,
		}, nil
	}

	ctx := context.Background()
	if _, err := settingsRepo.GetSettings(ctx); err != nil {
		if err := settingsRepo.AddDefaultSettings(ctx); err != nil {
			return nil, err
		}
	}
	arkClient, err := arksdk.NewCovenantlessClient(storeSvc)
	if err != nil {
		// nolint:all
		settingsRepo.CleanSettings(ctx)
		return nil, err
	}

	return &Service{buildInfo, arkClient, storeSvc, settingsRepo, nil, schedulerSvc, false}, nil
}

func (s *Service) IsReady() bool {
	return s.isReady
}

func (s *Service) SetupFromMnemonic(ctx context.Context, aspURL, password, mnemonic string) error {
	privateKey, err := utils.PrivateKeyFromMnemonic(mnemonic)
	if err != nil {
		return err
	}
	return s.Setup(ctx, aspURL, password, privateKey)
}

func (s *Service) Setup(
	ctx context.Context, aspURL, password, privateKey string,
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

func (s *Service) LockNode(ctx context.Context, password string) error {
	err := s.Lock(ctx, password)
	if err != nil {
		return err
	}
	s.schedulerSvc.Stop()
	logrus.Info("scheduler stopped")
	return nil
}

func (s *Service) UnlockNode(ctx context.Context, password string) error {
	err := s.Unlock(ctx, password)
	if err != nil {
		return err
	}

	s.schedulerSvc.Start()
	logrus.Info("scheduler started")

	err = s.ScheduleClaims(ctx)
	if err != nil {
		logrus.WithError(err).Info("schedule next claim failed")
	}

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

func (s *Service) AddDefaultSettings(ctx context.Context) error {
	return s.settingsRepo.AddDefaultSettings(ctx)
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
		btc := float64(sats) / 100000000.0
		amount := fmt.Sprintf("%.8f", btc)
		bip21Addr += fmt.Sprintf("&amount=%s", amount)
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
		return nil, fmt.Errorf("service not initialized")
	}
	return s.grpcClient.GetRoundByID(ctx, roundId)
}

func (s *Service) ClaimPending(ctx context.Context) (string, error) {
	roundTxid, err := s.ArkClient.Claim(ctx)
	if err == nil {
		err := s.ScheduleClaims(ctx)
		if err != nil {
			logrus.WithError(err).Warn("error scheduling next claims")
		}
	}
	return roundTxid, err
}

func (s *Service) ScheduleClaims(ctx context.Context) error {
	if !s.isReady {
		return fmt.Errorf("service not initialized")
	}

	txHistory, err := s.ArkClient.GetTransactionHistory(ctx)
	if err != nil {
		return err
	}

	data, err := s.GetConfigData(ctx)
	if err != nil {
		return err
	}

	task := func() {
		logrus.Infof("running auto claim at %s", time.Now())
		_, err := s.ClaimPending(ctx)
		if err != nil {
			logrus.WithError(err).Warn("failed to auto claim")
		}
	}

	return s.schedulerSvc.ScheduleNextClaim(txHistory, data, task)
}

func (s *Service) WhenNextClaim(ctx context.Context) time.Time {
	return s.schedulerSvc.WhenNextClaim()
}
