package application

import (
	"context"
	"encoding/hex"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/ArkLabsHQ/fulmine/internal/core/domain"
	"github.com/ArkLabsHQ/fulmine/internal/core/ports"
	"github.com/ArkLabsHQ/fulmine/internal/infrastructure/cln"
	"github.com/ArkLabsHQ/fulmine/pkg/boltz"
	"github.com/ArkLabsHQ/fulmine/pkg/vhtlc"
	"github.com/ArkLabsHQ/fulmine/utils"
	"github.com/ark-network/ark/common"
	"github.com/ark-network/ark/common/tree"
	arksdk "github.com/ark-network/ark/pkg/client-sdk"
	"github.com/ark-network/ark/pkg/client-sdk/client"
	grpcclient "github.com/ark-network/ark/pkg/client-sdk/client/grpc"
	"github.com/ark-network/ark/pkg/client-sdk/explorer"
	indexer "github.com/ark-network/ark/pkg/client-sdk/indexer"
	indexerTransport "github.com/ark-network/ark/pkg/client-sdk/indexer/grpc"
	"github.com/ark-network/ark/pkg/client-sdk/store"
	"github.com/ark-network/ark/pkg/client-sdk/types"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/btcutil/psbt"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	"github.com/btcsuite/btcwallet/waddrmgr"
	"github.com/ccoveille/go-safecast"
	"github.com/decred/dcrd/dcrec/secp256k1/v4"
	log "github.com/sirupsen/logrus"

	// nolint:staticcheck
	"golang.org/x/crypto/ripemd160"
)

const (
	WalletInit   = "init"
	WalletUnlock = "unlock"
	WalletReset  = "reset"
)

var boltzURLByNetwork = map[string]string{
	common.Bitcoin.Name:          "https://api.boltz.exchange",
	common.BitcoinTestNet.Name:   "https://api.testnet.boltz.exchange",
	common.BitcoinMutinyNet.Name: "https://api.boltz.mutinynet.arkade.sh",
	common.BitcoinRegTest.Name:   "http://localhost:9001",
}

type BuildInfo struct {
	Version string
	Commit  string
	Date    string
}

type WalletUpdate struct {
	Type     string
	Password string
}

type Service struct {
	BuildInfo BuildInfo

	arksdk.ArkClient
	storeCfg      store.Config
	storeRepo     types.Store
	dbSvc         ports.RepoManager
	grpcClient    client.TransportClient
	indexerClient indexer.Indexer
	schedulerSvc  ports.SchedulerService
	lnSvc         ports.LnService
	boltzSvc      *boltz.Api

	publicKey *secp256k1.PublicKey

	esploraUrl string
	boltzUrl   string
	boltzWSUrl string

	isReady bool

	subscriptionId   string
	subscriptionLock sync.RWMutex

	walletUpdates chan WalletUpdate

	// Notification channels
	notifications chan Notification

	stopBoardingEventListener chan struct{}
	closeInternalListener     func()
	closeAddressEventListener func()
}

type Notification struct {
	Addrs      []string
	NewVtxos   []indexer.Vtxo
	SpentVtxos []indexer.Vtxo
}

func NewService(
	buildInfo BuildInfo,
	storeCfg store.Config,
	storeSvc types.Store,
	dbSvc ports.RepoManager,
	schedulerSvc ports.SchedulerService,
	lnSvc ports.LnService,
	esploraUrl, boltzUrl, boltzWSUrl string,
) (*Service, error) {
	if arkClient, err := arksdk.LoadArkClient(storeSvc); err == nil {
		data, err := arkClient.GetConfigData(context.Background())
		if err != nil {
			return nil, err
		}

		grpcClient, err := grpcclient.NewClient(data.ServerUrl)
		if err != nil {
			return nil, err
		}

		indexerClient, err := indexerTransport.NewClient(data.ServerUrl)
		if err != nil {
			return nil, err
		}

		svc := &Service{
			BuildInfo:                 buildInfo,
			ArkClient:                 arkClient,
			storeCfg:                  storeCfg,
			storeRepo:                 storeSvc,
			dbSvc:                     dbSvc,
			grpcClient:                grpcClient,
			indexerClient:             indexerClient,
			schedulerSvc:              schedulerSvc,
			lnSvc:                     lnSvc,
			publicKey:                 nil,
			isReady:                   true,
			subscriptionLock:          sync.RWMutex{},
			notifications:             make(chan Notification),
			stopBoardingEventListener: make(chan struct{}),
			esploraUrl:                data.ExplorerURL,
			boltzUrl:                  boltzUrl,
			boltzWSUrl:                boltzWSUrl,
			walletUpdates:             make(chan WalletUpdate),
		}

		return svc, nil
	} else if !strings.Contains(err.Error(), "not initialized") {
		return nil, err
	}

	ctx := context.Background()
	settingsRepo := dbSvc.Settings()
	if _, err := settingsRepo.GetSettings(ctx); err != nil {
		if err := settingsRepo.AddDefaultSettings(ctx); err != nil {
			return nil, err
		}
	}

	arkClient, err := arksdk.NewArkClient(storeSvc)
	if err != nil {
		// nolint:all
		settingsRepo.CleanSettings(ctx)
		return nil, err
	}

	svc := &Service{
		BuildInfo:                 buildInfo,
		ArkClient:                 arkClient,
		storeCfg:                  storeCfg,
		storeRepo:                 storeSvc,
		dbSvc:                     dbSvc,
		grpcClient:                nil,
		schedulerSvc:              schedulerSvc,
		lnSvc:                     lnSvc,
		subscriptionLock:          sync.RWMutex{},
		notifications:             make(chan Notification),
		stopBoardingEventListener: make(chan struct{}),
		esploraUrl:                esploraUrl,
		boltzUrl:                  boltzUrl,
		boltzWSUrl:                boltzWSUrl,
		walletUpdates:             make(chan WalletUpdate),
	}

	return svc, nil
}

func (s *Service) IsReady() bool {
	return s.isReady
}

func (s *Service) GetWalletUpdates() <-chan WalletUpdate {
	return s.walletUpdates
}

func (s *Service) SetupFromMnemonic(ctx context.Context, serverUrl, password, mnemonic string) error {
	privateKey, err := utils.PrivateKeyFromMnemonic(mnemonic)
	if err != nil {
		return err
	}
	return s.Setup(ctx, serverUrl, password, privateKey)
}

func (s *Service) Setup(ctx context.Context, serverUrl, password, privateKey string) (err error) {
	privKeyBytes, err := hex.DecodeString(privateKey)
	if err != nil {
		return err
	}
	prvKey := secp256k1.PrivKeyFromBytes(privKeyBytes)

	client, err := grpcclient.NewClient(serverUrl)
	if err != nil {
		return err
	}

	indexerClient, err := indexerTransport.NewClient(serverUrl)
	if err != nil {
		return err
	}

	if err := s.Init(ctx, arksdk.InitArgs{
		WalletType:          arksdk.SingleKeyWallet,
		ClientType:          arksdk.GrpcClient,
		ServerUrl:           serverUrl,
		ExplorerURL:         s.esploraUrl,
		Password:            password,
		Seed:                privateKey,
		WithTransactionFeed: true,
	}); err != nil {
		return err
	}

	config, err := s.GetConfigData(ctx)
	if err != nil {
		return err
	}

	if err := s.dbSvc.Settings().UpdateSettings(
		ctx, domain.Settings{ServerUrl: config.ServerUrl, EsploraUrl: config.ExplorerURL},
	); err != nil {
		return err
	}

	s.esploraUrl = config.ExplorerURL
	s.publicKey = prvKey.PubKey()
	s.grpcClient = client
	s.indexerClient = indexerClient
	s.isReady = true

	go func() {
		s.walletUpdates <- WalletUpdate{Type: WalletInit, Password: password}
	}()

	return nil
}

func (s *Service) LockNode(ctx context.Context) error {
	if err := s.isInitializedAndUnlocked(ctx); err != nil {
		return err
	}

	err := s.Lock(ctx)
	if err != nil {
		return err
	}

	s.schedulerSvc.Stop()
	log.Info("scheduler stopped")

	// close all subscriptions
	s.subscriptionLock.Lock()
	defer s.subscriptionLock.Unlock()

	// close address subscriptions stream
	if s.closeAddressEventListener != nil {
		s.closeAddressEventListener()
		s.closeAddressEventListener = nil
	}

	// close boarding event listener
	s.stopBoardingEventListener <- struct{}{}
	close(s.stopBoardingEventListener)
	s.stopBoardingEventListener = make(chan struct{})

	// close internal address event listener
	s.closeInternalListener()
	s.closeInternalListener = nil

	go func() {
		s.walletUpdates <- WalletUpdate{Type: "lock"}
	}()

	return nil
}

func (s *Service) UnlockNode(ctx context.Context, password string) error {
	if !s.isReady {
		return fmt.Errorf("service not initialized")
	}

	if err := s.Unlock(ctx, password); err != nil {
		return err
	}

	s.schedulerSvc.Start()
	log.Info("scheduler started")

	data, err := s.GetConfigData(ctx)
	if err != nil {
		return err
	}

	nextExpiry, err := s.computeNextExpiry(ctx, data)
	if err != nil {
		log.WithError(err).Error("failed to compute next expiry")
	}

	if nextExpiry != nil {
		if err := s.scheduleNextSettlement(*nextExpiry, data); err != nil {
			log.WithError(err).Info("schedule next claim failed")
		}
	}

	prvkeyStr, err := s.Dump(ctx)
	if err != nil {
		return err
	}

	buf, err := hex.DecodeString(prvkeyStr)
	if err != nil {
		return err
	}

	_, pubkey := btcec.PrivKeyFromBytes(buf)
	s.publicKey = pubkey

	settings, err := s.dbSvc.Settings().GetSettings(ctx)
	if err != nil {
		log.WithError(err).Warn("failed to get settings")
		return err
	}
	if len(settings.LnUrl) > 0 {
		if strings.HasPrefix(settings.LnUrl, "clnconnect:") {
			s.lnSvc = cln.NewService()
		}
		if err := s.lnSvc.Connect(ctx, settings.LnUrl); err != nil {
			log.WithError(err).Warn("failed to connect to ln node")
		}
		url := s.boltzUrl
		wsUrl := s.boltzWSUrl
		if url == "" {
			url = boltzURLByNetwork[data.Network.Name]
		}
		if wsUrl == "" {
			wsUrl = boltzURLByNetwork[data.Network.Name]
		}
		s.boltzSvc = &boltz.Api{URL: url, WSURL: wsUrl}
	}

	offchainAddress, onchainAddress, err := s.Receive(ctx)
	if err != nil {
		log.WithError(err).Error("failed to get addresses")
		return err
	}

	eventsCh, closeFn, err := s.grpcClient.SubscribeForAddress(context.Background(), offchainAddress)
	if err != nil {
		log.WithError(err).Error("failed to subscribe for offchain address")
		return err
	}
	s.closeInternalListener = closeFn
	go s.handleInternalAddressEventChannel(eventsCh)
	if data.UtxoMaxAmount != 0 {
		go s.subscribeForBoardingEvent(ctx, onchainAddress, data)
	}

	// resubscribe to previously subscribed scripts
	scriptsToSubscribe, err := s.dbSvc.SubscribedScript().Get(ctx)
	if err != nil {
		log.WithError(err).Error("failed to get subscribed scripts")
		return err
	}

	if len(scriptsToSubscribe) > 0 {
		err := s.subscribeForScripts(context.Background(), scriptsToSubscribe)
		if err != nil {
			log.WithError(err).Error("failed to resubscribe for scripts")
			return err
		}
	}

	go func() {
		s.walletUpdates <- WalletUpdate{Type: WalletUnlock, Password: password}
	}()

	return nil
}

func (s *Service) ResetWallet(ctx context.Context) error {
	if err := s.dbSvc.Settings().CleanSettings(ctx); err != nil {
		return err
	}
	// reset wallet (cleans all repos)
	s.Reset(ctx)
	// TODO: Maybe drop?
	// nolint:all
	s.dbSvc.Settings().AddDefaultSettings(ctx)

	go func() {
		s.walletUpdates <- WalletUpdate{Type: WalletReset}
	}()
	return nil
}

func (s *Service) AddDefaultSettings(ctx context.Context) error {
	return s.dbSvc.Settings().AddDefaultSettings(ctx)
}

func (s *Service) GetSettings(ctx context.Context) (*domain.Settings, error) {
	sett, err := s.dbSvc.Settings().GetSettings(ctx)
	return sett, err
}

func (s *Service) NewSettings(ctx context.Context, settings domain.Settings) error {
	if err := s.isInitializedAndUnlocked(ctx); err != nil {
		return err
	}

	return s.dbSvc.Settings().AddSettings(ctx, settings)
}

func (s *Service) UpdateSettings(ctx context.Context, settings domain.Settings) error {
	if err := s.isInitializedAndUnlocked(ctx); err != nil {
		return err
	}

	return s.dbSvc.Settings().UpdateSettings(ctx, settings)
}

func (s *Service) GetAddress(ctx context.Context, sats uint64) (string, string, string, string, error) {
	if err := s.isInitializedAndUnlocked(ctx); err != nil {
		return "", "", "", "", err
	}

	offchainAddr, boardingAddr, err := s.Receive(ctx)
	if err != nil {
		return "", "", "", "", err
	}
	bip21Addr := fmt.Sprintf("bitcoin:%s?ark=%s", boardingAddr, offchainAddr)
	// add amount if passed
	if sats > 0 {
		btc := float64(sats) / 100000000.0
		amount := fmt.Sprintf("%.8f", btc)
		bip21Addr += fmt.Sprintf("&amount=%s", amount)
	}
	pubkey := hex.EncodeToString(s.publicKey.SerializeCompressed())
	return bip21Addr, offchainAddr, boardingAddr, pubkey, nil
}

func (s *Service) GetTotalBalance(ctx context.Context) (uint64, error) {
	if err := s.isInitializedAndUnlocked(ctx); err != nil {
		return 0, err
	}

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
	return s.grpcClient.GetRoundByID(ctx, roundId)
}

func (s *Service) Settle(ctx context.Context) (string, error) {
	if err := s.isInitializedAndUnlocked(ctx); err != nil {
		return "", err
	}

	return s.ArkClient.Settle(ctx)
}

func (s *Service) scheduleNextSettlement(at time.Time, data *types.Config) error {
	task := func() {
		_, err := s.Settle(context.Background())
		if err != nil {
			log.WithError(err).Warn("failed to auto claim")
		}
	}

	// if market hour is set, schedule the task at the best market hour = market hour closer to `at` timestamp

	marketHourStartTime := time.Unix(data.MarketHourStartTime, 0)
	if !marketHourStartTime.IsZero() && data.MarketHourPeriod > 0 && at.After(marketHourStartTime) {
		cycles := math.Floor(at.Sub(marketHourStartTime).Seconds() / float64(data.MarketHourPeriod))
		at = marketHourStartTime.Add(time.Duration(cycles) * time.Duration(data.MarketHourPeriod) * time.Second)
	}

	roundInterval := time.Duration(data.RoundInterval) * time.Second
	at = at.Add(-2 * roundInterval) // schedule 2 rounds before the expiry

	return s.schedulerSvc.ScheduleNextSettlement(at, task)
}

func (s *Service) WhenNextSettlement(ctx context.Context) time.Time {
	return s.schedulerSvc.WhenNextSettlement()
}

func (s *Service) ConnectLN(ctx context.Context, connectUrl string) error {
	data, err := s.GetConfigData(ctx)
	if err != nil {
		return err
	}

	if strings.HasPrefix(connectUrl, "clnconnect:") {
		s.lnSvc = cln.NewService()
	}
	if err := s.lnSvc.Connect(ctx, connectUrl); err != nil {
		return err
	}

	url := s.boltzUrl
	wsUrl := s.boltzWSUrl
	if url == "" {
		url = boltzURLByNetwork[data.Network.Name]
	}
	if wsUrl == "" {
		wsUrl = boltzURLByNetwork[data.Network.Name]
	}
	s.boltzSvc = &boltz.Api{URL: url, WSURL: wsUrl}
	return nil
}

func (s *Service) DisconnectLN() {
	s.lnSvc.Disconnect()
}

func (s *Service) IsConnectedLN() bool {
	return s.lnSvc.IsConnected()
}

func (s *Service) GetVHTLC(
	ctx context.Context,
	receiverPubkey, senderPubkey *secp256k1.PublicKey,
	preimageHash []byte,
	refundLocktimeParam *common.AbsoluteLocktime,
	unilateralClaimDelayParam *common.RelativeLocktime,
	unilateralRefundDelayParam *common.RelativeLocktime,
	unilateralRefundWithoutReceiverDelayParam *common.RelativeLocktime,
	persistToDb bool,
) (string, *vhtlc.VHTLCScript, *vhtlc.Opts, error) {
	if err := s.isInitializedAndUnlocked(ctx); err != nil {
		return "", nil, nil, err
	}

	receiverPubkeySet := receiverPubkey != nil
	senderPubkeySet := senderPubkey != nil
	if receiverPubkeySet == senderPubkeySet {
		return "", nil, nil, fmt.Errorf("only one of receiver and sender pubkey must be set")
	}
	if !receiverPubkeySet {
		receiverPubkey = s.publicKey
	}
	if !senderPubkeySet {
		senderPubkey = s.publicKey
	}

	offchainAddr, _, err := s.Receive(ctx)
	if err != nil {
		return "", nil, nil, err
	}

	decodedAddr, err := common.DecodeAddress(offchainAddr)
	if err != nil {
		return "", nil, nil, err
	}

	// Default values if not provided
	refundLocktime := common.AbsoluteLocktime(80 * 600) // 80 blocks
	if refundLocktimeParam != nil {
		refundLocktime = *refundLocktimeParam
	}

	unilateralClaimDelay := common.RelativeLocktime{
		Type:  common.LocktimeTypeSecond,
		Value: 512, //60 * 12, // 12 hours
	}
	if unilateralClaimDelayParam != nil {
		unilateralClaimDelay = *unilateralClaimDelayParam
	}

	unilateralRefundDelay := common.RelativeLocktime{
		Type:  common.LocktimeTypeSecond,
		Value: 1024, //60 * 24, // 24 hours
	}
	if unilateralRefundDelayParam != nil {
		unilateralRefundDelay = *unilateralRefundDelayParam
	}

	unilateralRefundWithoutReceiverDelay := common.RelativeLocktime{
		Type:  common.LocktimeTypeBlock,
		Value: 224, // 224 blocks
	}
	if unilateralRefundWithoutReceiverDelayParam != nil {
		unilateralRefundWithoutReceiverDelay = *unilateralRefundWithoutReceiverDelayParam
	}

	opts := vhtlc.Opts{
		Sender:                               senderPubkey,
		Receiver:                             receiverPubkey,
		Server:                               decodedAddr.Server,
		PreimageHash:                         preimageHash,
		RefundLocktime:                       refundLocktime,
		UnilateralClaimDelay:                 unilateralClaimDelay,
		UnilateralRefundDelay:                unilateralRefundDelay,
		UnilateralRefundWithoutReceiverDelay: unilateralRefundWithoutReceiverDelay,
	}
	vHTLC, err := vhtlc.NewVHTLCScript(opts)
	if err != nil {
		return "", nil, nil, err
	}

	// nolint
	cfg, _ := s.GetConfigData(ctx)

	encodedAddr, err := vHTLC.Address(cfg.Network.Addr, cfg.ServerPubKey)
	if err != nil {
		return "", nil, nil, err
	}

	// store the vhtlc options for future use
	if persistToDb {
		if err := s.dbSvc.VHTLC().Add(ctx, opts); err != nil {
			return "", nil, nil, err
		}
	}

	return encodedAddr, vHTLC, &opts, nil
}

func (s *Service) ListVHTLC(ctx context.Context, preimageHashFilter string) ([]client.Vtxo, []vhtlc.Opts, error) {
	if err := s.isInitializedAndUnlocked(ctx); err != nil {
		return nil, nil, err
	}

	// Get VHTLC options based on filter
	var vhtlcOpts []vhtlc.Opts
	vhtlcRepo := s.dbSvc.VHTLC()

	if preimageHashFilter != "" {
		opt, err := vhtlcRepo.Get(ctx, preimageHashFilter)
		if err != nil {
			return nil, nil, err
		}
		vhtlcOpts = []vhtlc.Opts{*opt}
	} else {
		var err error
		vhtlcOpts, err = vhtlcRepo.GetAll(ctx)
		if err != nil {
			return nil, nil, err
		}
	}

	vtxos, err := s.getVHTLCFunds(ctx, vhtlcOpts)
	if err != nil {
		return nil, nil, err
	}

	return vtxos, vhtlcOpts, nil
}

func (s *Service) ClaimVHTLC(ctx context.Context, preimage []byte) (string, error) {
	if err := s.isInitializedAndUnlocked(ctx); err != nil {
		return "", err
	}

	preimageHash := hex.EncodeToString(btcutil.Hash160(preimage))
	vhtlcOpts, err := s.dbSvc.VHTLC().Get(ctx, preimageHash)
	if err != nil {
		return "", err
	}

	return s.claimVHTLC(ctx, preimage, *vhtlcOpts)
}

func (s *Service) RefundVHTLC(ctx context.Context, swapId, preimageHash string, withReceiver bool) (string, error) {
	if err := s.isInitializedAndUnlocked(ctx); err != nil {
		return "", err
	}

	vhtlcOpts, err := s.dbSvc.VHTLC().Get(ctx, preimageHash)
	if err != nil {
		return "", err
	}

	return s.refundVHTLC(ctx, swapId, withReceiver, *vhtlcOpts)
}

func (s *Service) GetInvoice(ctx context.Context, amount uint64, memo, preimage string) (string, string, error) {
	if err := s.isInitializedAndUnlocked(ctx); err != nil {
		return "", "", err
	}

	if !s.lnSvc.IsConnected() {
		return "", "", fmt.Errorf("not connected to LN")
	}

	return s.lnSvc.GetInvoice(ctx, amount, memo, preimage)
}

func (s *Service) DecodeInvoice(ctx context.Context, invoice string) (uint64, []byte, error) {
	if err := s.isInitializedAndUnlocked(ctx); err != nil {
		return 0, nil, err
	}

	return s.lnSvc.DecodeInvoice(ctx, invoice)
}

func (s *Service) PayInvoice(ctx context.Context, invoice string) (string, error) {
	if err := s.isInitializedAndUnlocked(ctx); err != nil {
		return "", err
	}

	if !s.lnSvc.IsConnected() {
		return "", fmt.Errorf("not connected to LN")
	}

	return s.lnSvc.PayInvoice(ctx, invoice)
}

func (s *Service) IsInvoiceSettled(ctx context.Context, invoice string) (bool, error) {
	if err := s.isInitializedAndUnlocked(ctx); err != nil {
		return false, err
	}

	if !s.lnSvc.IsConnected() {
		return false, fmt.Errorf("not connected to LN")
	}

	return s.lnSvc.IsInvoiceSettled(ctx, invoice)
}

func (s *Service) GetBalanceLN(ctx context.Context) (msats uint64, err error) {
	if err := s.isInitializedAndUnlocked(ctx); err != nil {
		return 0, err
	}

	if !s.lnSvc.IsConnected() {
		return 0, fmt.Errorf("not connected to LN")
	}

	return s.lnSvc.GetBalance(ctx)
}

// ln -> ark (reverse submarine swap)
func (s *Service) IncreaseInboundCapacity(ctx context.Context, amount uint64) (string, error) {
	if err := s.isInitializedAndUnlocked(ctx); err != nil {
		return "", err
	}

	// get our pubkey
	_, _, _, pk, err := s.GetAddress(ctx, 0)
	if err != nil {
		return "", fmt.Errorf("failed to get address: %s", err)
	}

	myPubkey, _ := hex.DecodeString(pk)

	// make swap
	swap, err := s.boltzSvc.CreateReverseSwap(boltz.CreateReverseSwapRequest{
		From:           boltz.CurrencyBtc,
		To:             boltz.CurrencyArk,
		InvoiceAmount:  amount,
		ClaimPublicKey: hex.EncodeToString(myPubkey),
	})
	if err != nil {
		return "", fmt.Errorf("failed to make reverse submarine swap: %v", err)
	}

	// verify vHTLC
	senderPubkey, err := parsePubkey(swap.RefundPublicKey)
	if err != nil {
		return "", fmt.Errorf("invalid refund pubkey: %v", err)
	}

	// TODO: verify amount
	_, preimageHash, err := s.DecodeInvoice(ctx, swap.Invoice)
	if err != nil {
		return "", fmt.Errorf("failed to decode invoice: %v", err)
	}

	ripe := ripemd160.New()
	ripe.Write(preimageHash)
	preimageHashRipemd160 := ripe.Sum(nil)

	vhtlcAddress, _, opts, err := s.GetVHTLC(
		ctx,
		nil,
		senderPubkey,
		preimageHashRipemd160,
		nil,
		&common.RelativeLocktime{Type: common.LocktimeTypeBlock, Value: swap.TimeoutBlockHeights.UnilateralClaim},
		&common.RelativeLocktime{Type: common.LocktimeTypeBlock, Value: swap.TimeoutBlockHeights.UnilateralRefund},
		&common.RelativeLocktime{Type: common.LocktimeTypeBlock, Value: swap.TimeoutBlockHeights.UnilateralRefundWithoutReceiver},
		false,
	)
	if err != nil {
		return "", fmt.Errorf("failed to verify vHTLC: %v", err)
	}

	if swap.LockupAddress != vhtlcAddress {
		return "", fmt.Errorf("boltz is trying to scam us, vHTLCs do not match")
	}

	// pay the invoice
	preimage, err := s.PayInvoice(ctx, swap.Invoice)
	if err != nil {
		return "", fmt.Errorf("failed to pay invoice: %v", err)
	}

	decodedPreimage, err := hex.DecodeString(preimage)
	if err != nil {
		return "", fmt.Errorf("invalid preimage: %v", err)
	}

	txid, err := s.claimVHTLC(ctx, decodedPreimage, *opts)
	if err != nil {
		return "", fmt.Errorf("failed to claim vHTLC: %v", err)
	}

	go func() {
		if err := s.dbSvc.Swap().Add(context.Background(), domain.Swap{
			Id:         swap.Id,
			Amount:     amount,
			Timestamp:  time.Now().Unix(),
			Invoice:    swap.Invoice,
			To:         boltz.CurrencyArk,
			From:       boltz.CurrencyBtc,
			Status:     domain.SwapSuccess,
			VhtlcOpts:  *opts,
			RedeemTxId: txid,
		}); err != nil {
			log.WithError(err).Fatal("failed to store swap")
		}
		log.Debugf("added new swap %s", swap.Id)
	}()

	return txid, err

}

// ark -> ln (submarine swap)
func (s *Service) IncreaseOutboundCapacity(ctx context.Context, amount uint64) (string, error) {
	if err := s.isInitializedAndUnlocked(ctx); err != nil {
		return "", err
	}

	// get our pubkey
	_, _, _, pk, err := s.GetAddress(ctx, 0)
	if err != nil {
		return "", fmt.Errorf("failed to get address: %v", err)
	}

	myPubkey, _ := hex.DecodeString(pk)

	// generate invoice where to receive funds
	invoice, preimageHash, err := s.GetInvoice(ctx, amount, "increase outbound capacity", "")
	if err != nil {
		return "", fmt.Errorf("failed to create invoice: %w", err)
	}

	// make swap
	swap, err := s.boltzSvc.CreateSwap(boltz.CreateSwapRequest{
		From:            boltz.CurrencyArk,
		To:              boltz.CurrencyBtc,
		Invoice:         invoice,
		RefundPublicKey: hex.EncodeToString(myPubkey),
	})
	if err != nil {
		return "", fmt.Errorf("failed to make submarine swap: %v", err)
	}

	preimageHashBytes, err := hex.DecodeString(preimageHash)
	if err != nil {
		return "", fmt.Errorf("invalid preimage hash: %v", err)
	}

	receiverPubkey, err := parsePubkey(swap.ClaimPublicKey)
	if err != nil {
		return "", fmt.Errorf("invalid claim pubkey: %v", err)
	}

	address, _, opts, err := s.GetVHTLC(
		ctx,
		receiverPubkey,
		nil,
		preimageHashBytes,
		nil,
		&common.RelativeLocktime{Type: common.LocktimeTypeBlock, Value: swap.TimeoutBlockHeights.UnilateralClaim},
		&common.RelativeLocktime{Type: common.LocktimeTypeBlock, Value: swap.TimeoutBlockHeights.UnilateralRefund},
		&common.RelativeLocktime{Type: common.LocktimeTypeBlock, Value: swap.TimeoutBlockHeights.UnilateralRefundWithoutReceiver},
		false,
	)
	if err != nil {
		return "", fmt.Errorf("failed to verify vHTLC: %v", err)
	}
	if swap.Address != address {
		return "", fmt.Errorf("boltz is trying to scam us, vHTLCs do not match")
	}

	// pay to vHTLC address
	receivers := []arksdk.Receiver{arksdk.NewBitcoinReceiver(swap.Address, amount)}
	txid, err := s.SendOffChain(ctx, false, receivers, true)
	if err != nil {
		return "", fmt.Errorf("failed to pay to vHTLC address: %v", err)
	}

	// TODO workaround to connect ws endpoint on a different port for regtest
	wsClient := s.boltzSvc
	if s.boltzSvc.URL == boltzURLByNetwork[common.BitcoinRegTest.Name] {
		wsClient = &boltz.Api{WSURL: "ws://localhost:9004"}
	}

	ws := wsClient.NewWebsocket()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	err = ws.Connect()
	for err != nil {
		log.WithError(err).Warn("failed to connect to boltz websocket")
		time.Sleep(time.Second)
		log.Debug("reconnecting...")
		err = ws.Connect()
		if ctx.Err() != nil {
			return "", fmt.Errorf("timeout while connecting to websocket: %v", ctx.Err())
		}
	}

	err = ws.Subscribe([]string{swap.Id})
	for err != nil {
		log.WithError(err).Warn("failed to subscribe for swap events")
		time.Sleep(time.Second)
		log.Debug("retrying...")
		err = ws.Subscribe([]string{swap.Id})
	}

	for update := range ws.Updates {
		parsedStatus := boltz.ParseEvent(update.Status)

		switch parsedStatus {
		// TODO: ensure these are the right events to react to in case the vhtlc needs to be refunded.
		case boltz.TransactionLockupFailed, boltz.InvoiceFailedToPay:
			withReceiver := true

			refundTxid, err := s.refundVHTLC(context.Background(), swap.Id, withReceiver, *opts)
			if err != nil {
				return "", fmt.Errorf("failed to refund vHTLC: %s", err)
			}

			go func() {
				if err := s.dbSvc.Swap().Add(context.Background(), domain.Swap{
					Id:          swap.Id,
					Amount:      amount,
					Timestamp:   time.Now().Unix(),
					Status:      domain.SwapFailed,
					Invoice:     invoice,
					FundingTxId: txid,
					RedeemTxId:  refundTxid,
					To:          boltz.CurrencyBtc,
					From:        boltz.CurrencyArk,
					VhtlcOpts:   *opts,
				}); err != nil {
					log.WithError(err).Fatal("failed to store swap")
				}
				log.Debugf("added new refunded swap %s", swap.Id)
			}()

			return "", fmt.Errorf("something went wrong, the vhtlc was refunded %s", txid)
		case boltz.TransactionClaimed:
			go func() {
				if err := s.dbSvc.Swap().Add(context.Background(), domain.Swap{
					Id:          swap.Id,
					Amount:      amount,
					Timestamp:   time.Now().Unix(),
					Status:      domain.SwapSuccess,
					Invoice:     invoice,
					FundingTxId: txid,
					To:          boltz.CurrencyBtc,
					From:        boltz.CurrencyArk,
					VhtlcOpts:   *opts,
				}); err != nil {
					log.WithError(err).Fatal("failed to store swap")
				}
				log.Debugf("added new swap %s", swap.Id)
			}()

			return txid, nil
		}
	}

	return "", fmt.Errorf("something went wrong")
}

func (s *Service) subscribeForScripts(ctx context.Context, scripts []string) error {
	if s.subscriptionId == "" {
		subscriptionId, err := s.indexerClient.SubscribeForScripts(ctx, "", scripts)
		if err != nil {
			return fmt.Errorf("failed to subscribe for scripts: %w", err)
		}

		subscriptionChannel, closeFn, err := s.indexerClient.GetSubscription(ctx, subscriptionId)
		if err != nil {
			return fmt.Errorf("failed to get subscription for scripts: %w", err)
		}

		config, err := s.GetConfigData(context.Background())
		if err != nil {
			return fmt.Errorf("failed to get config data: %w", err)
		}
		go s.handleAddressEventChannel(subscriptionChannel, config)
		s.subscriptionId = subscriptionId
		s.closeAddressEventListener = func() {
			s.subscriptionId = ""
			closeFn()
		}

	} else {
		_, err := s.indexerClient.SubscribeForScripts(ctx, s.subscriptionId, scripts)
		if err != nil {
			return fmt.Errorf("failed to update subscription for scripts: %w", err)
		}
	}

	log.Debugf("restored watching %d scripts", len(scripts))

	return nil
}

func (s *Service) SubscribeForAddresses(ctx context.Context, addresses []string) error {
	if err := s.isInitializedAndUnlocked(ctx); err != nil {
		return err
	}

	s.subscriptionLock.Lock()
	defer s.subscriptionLock.Unlock()

	subscribedScripts, err := s.dbSvc.SubscribedScript().Get(ctx)
	if err != nil {
		return fmt.Errorf("failed to get subscribed scripts from db: %w", err)
	}
	subscribedScriptsMap := make(map[string]struct{}, len(subscribedScripts))
	for _, script := range subscribedScripts {
		subscribedScriptsMap[script] = struct{}{}
	}

	addressScripts := make([]string, 0, len(addresses))
	for _, addr := range addresses {
		if addr == "" {
			return fmt.Errorf("empty address provided")
		}

		decoded_address, err := common.DecodeAddress(addr)
		if err != nil {
			return fmt.Errorf("failed to decode address %s: %w", addr, err)
		}
		serialised_script := hex.EncodeToString(schnorr.SerializePubKey(decoded_address.VtxoTapKey))

		if _, ok := subscribedScriptsMap[serialised_script]; ok {
			log.Warnf("address %s already subscribed, skipping", addr)
			continue
		}

		addressScripts = append(addressScripts, serialised_script)
	}

	if len(addressScripts) == 0 {
		return nil
	}

	err = s.subscribeForScripts(context.Background(), addressScripts)
	if err != nil {
		return fmt.Errorf("failed to subscribe for address scripts: %w", err)
	}

	// store in db
	count, err := s.dbSvc.SubscribedScript().Add(ctx, addressScripts)
	if err != nil {
		return fmt.Errorf("failed to store subscribed scripts in db: %w", err)
	}

	if count > 0 {
		log.Infof("subscribed to %d address scripts", count)
	}

	return nil
}

func (s *Service) UnsubscribeForAddresses(ctx context.Context, addresses []string) error {
	if err := s.isInitializedAndUnlocked(ctx); err != nil {
		return err
	}

	s.subscriptionLock.Lock()
	defer s.subscriptionLock.Unlock()

	addressScripts := make([]string, 0, len(addresses))

	subscribedScripts, err := s.dbSvc.SubscribedScript().Get(ctx)
	if err != nil {
		return fmt.Errorf("failed to get subscribed scripts from db: %w", err)
	}
	subscribedScriptsMap := make(map[string]struct{}, len(subscribedScripts))
	for _, script := range subscribedScripts {
		subscribedScriptsMap[script] = struct{}{}
	}

	for _, addr := range addresses {
		decoded_address, err := common.DecodeAddress(addr)
		if err != nil {
			return fmt.Errorf("failed to decode address %s: %w", addr, err)
		}
		serialised_script := hex.EncodeToString(schnorr.SerializePubKey(decoded_address.VtxoTapKey))

		_, ok := subscribedScriptsMap[serialised_script]
		if !ok {
			continue
		}

		addressScripts = append(addressScripts, serialised_script)
	}

	err = s.indexerClient.UnsubscribeForScripts(ctx, s.subscriptionId, addressScripts)
	if err != nil {
		return fmt.Errorf("failed to unsubscribe for address scripts: %w", err)
	}

	// remove scripts from db
	count, err := s.dbSvc.SubscribedScript().Delete(ctx, addressScripts)
	if err != nil {
		return fmt.Errorf("failed to remove subscribed scripts from db: %w", err)
	}

	log.Infof("unsubscribed from %d address scripts", count)

	return nil
}

func (s *Service) GetVtxoNotifications(ctx context.Context) <-chan Notification {
	return s.notifications
}

func (s *Service) GetDelegatePublicKey(ctx context.Context) (string, error) {
	if err := s.isInitializedAndUnlocked(ctx); err != nil {
		return "", err
	}

	if s.publicKey == nil {
		return "", fmt.Errorf("service not initialized")
	}

	return hex.EncodeToString(s.publicKey.SerializeCompressed()), nil
}

func (s *Service) WatchAddressForRollover(ctx context.Context, address, destinationAddress string, taprootTree []string) error {
	if err := s.isInitializedAndUnlocked(ctx); err != nil {
		return err
	}

	if address == "" {
		return fmt.Errorf("missing address")
	}
	if len(taprootTree) == 0 {
		return fmt.Errorf("missing taproot tree")
	}
	if destinationAddress == "" {
		return fmt.Errorf("missing destination address")
	}

	target := domain.VtxoRolloverTarget{
		Address:            address,
		TaprootTree:        taprootTree,
		DestinationAddress: destinationAddress,
	}

	return s.dbSvc.VtxoRollover().AddTarget(ctx, target)
}

func (s *Service) UnwatchAddress(ctx context.Context, address string) error {
	if err := s.isInitializedAndUnlocked(ctx); err != nil {
		return err
	}

	if address == "" {
		return fmt.Errorf("missing address")
	}

	return s.dbSvc.VtxoRollover().DeleteTarget(ctx, address)
}

func (s *Service) ListWatchedAddresses(ctx context.Context) ([]domain.VtxoRolloverTarget, error) {
	if err := s.isInitializedAndUnlocked(ctx); err != nil {
		return nil, err
	}

	return s.dbSvc.VtxoRollover().GetAllTargets(ctx)
}

func (s *Service) IsLocked(ctx context.Context) bool {
	if s.ArkClient == nil {
		return false
	}

	return s.ArkClient.IsLocked(ctx)
}

func (s *Service) isInitializedAndUnlocked(ctx context.Context) error {
	if !s.isReady {
		return fmt.Errorf("service not initialized")
	}

	if s.IsLocked(ctx) {
		return fmt.Errorf("service is locked")
	}

	return nil
}

func (s *Service) boltzRefundSwap(swapId, refundTx string) (string, error) {
	tx, err := s.boltzSvc.RefundSubmarine(swapId, boltz.RefundSwapRequest{
		Transaction: refundTx,
	})
	if err != nil {
		return "", err
	}

	return tx.Transaction, nil
}

func (s *Service) computeNextExpiry(ctx context.Context, data *types.Config) (*time.Time, error) {
	spendableVtxos, _, err := s.ListVtxos(ctx)
	if err != nil {
		return nil, err
	}

	var expiry *time.Time

	if len(spendableVtxos) > 0 {
		nextExpiry := spendableVtxos[0].ExpiresAt
		for _, vtxo := range spendableVtxos[1:] {
			if vtxo.ExpiresAt.Before(nextExpiry) {
				nextExpiry = vtxo.ExpiresAt
			}
		}
		expiry = &nextExpiry
	}

	txs, err := s.GetTransactionHistory(ctx)
	if err != nil {
		return nil, err
	}

	// check for unsettled boarding UTXOs
	for _, tx := range txs {
		if len(tx.BoardingTxid) > 0 && !tx.Settled {
			// TODO replace by boardingExitDelay https://github.com/ark-network/ark/pull/501
			boardingExpiry := tx.CreatedAt.Add(time.Duration(data.UnilateralExitDelay.Seconds()*2) * time.Second)
			if expiry == nil || boardingExpiry.Before(*expiry) {
				expiry = &boardingExpiry
			}
		}
	}

	return expiry, nil
}

// subscribeForBoardingEvent aims to update the scheduled settlement
// by checking for spent and new vtxos on the given boarding address
func (s *Service) subscribeForBoardingEvent(ctx context.Context, address string, cfg *types.Config) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	// TODO: use boardingExitDelay https://github.com/ark-network/ark/pull/501
	boardingTimelock := common.RelativeLocktime{Type: cfg.UnilateralExitDelay.Type, Value: cfg.UnilateralExitDelay.Value * 2}

	expl := explorer.NewExplorer(s.esploraUrl, cfg.Network)

	currentSet := make(map[string]types.Utxo)
	utxos, err := expl.GetUtxos(address)
	if err != nil {
		log.WithError(err).Error("failed to get utxos")
		return
	}
	for _, utxo := range utxos {
		key := fmt.Sprintf("%s:%d", utxo.Txid, utxo.Vout)
		currentSet[key] = utxo.ToUtxo(boardingTimelock, []string{})
	}

	for {
		select {
		case <-s.stopBoardingEventListener:
			return
		case <-ticker.C:
			utxos, err := expl.GetUtxos(address)
			if err != nil {
				log.WithError(err).Error("failed to get utxos")
				continue
			}

			if len(utxos) == 0 {
				continue
			}

			newSet := make(map[string]types.Utxo)
			for _, utxo := range utxos {
				key := fmt.Sprintf("%s:%d", utxo.Txid, utxo.Vout)
				newSet[key] = utxo.ToUtxo(boardingTimelock, []string{})
			}

			// find new utxos
			newUtxos := make([]types.Utxo, 0)
			for key, newUtxo := range newSet {
				if _, exists := currentSet[key]; !exists {
					newUtxos = append(newUtxos, newUtxo)
				}
			}

			if len(newUtxos) > 0 {
				log.Infof("boarding event detected: %d new utxos", len(newUtxos))
			}

			// if expiry is before the next scheduled settlement, we need to schedule a new one
			if len(newUtxos) > 0 {
				nextScheduledSettlement := s.WhenNextSettlement(ctx)

				needSchedule := false

				for _, vtxo := range newUtxos {
					if nextScheduledSettlement.IsZero() || vtxo.SpendableAt.Before(nextScheduledSettlement) {
						nextScheduledSettlement = vtxo.SpendableAt
						needSchedule = true
					}
				}

				if needSchedule {
					if err := s.scheduleNextSettlement(nextScheduledSettlement, cfg); err != nil {
						log.WithError(err).Info("schedule next claim failed")
					}
				}
			}

			// update current set
			currentSet = newSet
		}
	}
}

// handleAddressEventChannel is used to forward address events to the notifications channel
func (s *Service) handleAddressEventChannel(eventsCh <-chan *indexer.ScriptEvent, config *types.Config) {
	log.Infof("starting address event handler")
	for event := range eventsCh {
		if event == nil {
			log.Warn("Received nil event from event channel")
			continue
		}

		if event.Err != nil {
			log.WithError(event.Err).Error("AddressEvent subscription error")
			continue
		}

		log.Infof("received address event(%d spent vtxos, %d new vtxos)", len(event.SpentVtxos), len(event.NewVtxos))

		// convert scripts to addresses
		addresses := make([]string, 0, len(event.Scripts))
		for _, script := range event.Scripts {
			decodedPubKey, err := hex.DecodeString(script)
			if err != nil {
				log.WithError(err).Errorf("failed to decode script %s", script)
				continue
			}
			vtxoTapPubkey, err := schnorr.ParsePubKey(decodedPubKey)
			if err != nil {
				log.WithError(err).Errorf("failed to parse pubkey %s", script)
				continue
			}

			vtxoAddress := common.Address{
				VtxoTapKey: vtxoTapPubkey,
				Server:     config.ServerPubKey,
				HRP:        config.Network.Addr,
			}

			encodedAddress, err := vtxoAddress.Encode()
			if err != nil {
				log.WithError(err).Errorf("failed to encode address %s", script)
				continue
			}
			addresses = append(addresses, encodedAddress)

		}
		// non-blocking forward to notifications channel
		go func(evt *indexer.ScriptEvent) {
			s.notifications <- Notification{
				Addrs:      addresses,
				NewVtxos:   event.NewVtxos,
				SpentVtxos: event.SpentVtxos,
			}
		}(event)
	}
}

// handleInternalAddressEventChannel is used to handle address events from the internal address event channel
// it is used to schedule next settlement when a VTXO is spent or created
func (s *Service) handleInternalAddressEventChannel(eventsCh <-chan client.AddressEvent) {
	for event := range eventsCh {
		if event.Err != nil {
			log.WithError(event.Err).Error("AddressEvent subscription error")
			continue
		}

		ctx := context.Background()

		data, err := s.GetConfigData(ctx)
		if err != nil {
			log.WithError(err).Error("failed to get config data")
			return
		}

		log.Infof("received internal address event (%d spent vtxos, %d new vtxos)", len(event.SpentVtxos), len(event.NewVtxos))

		// if some vtxos were spent, schedule a settlement to soonest expiry among new vtxos / boarding UTXOs set
		if len(event.SpentVtxos) > 0 {
			nextExpiry, err := s.computeNextExpiry(ctx, data)
			if err != nil {
				log.WithError(err).Error("failed to compute next expiry")
				return
			}

			if nextExpiry != nil {
				if err := s.scheduleNextSettlement(*nextExpiry, data); err != nil {
					log.WithError(err).Info("schedule next claim failed")
				}
			}

			return
		}

		// if some vtxos were created, schedule a settlement to the soonest expiry among new vtxos
		if len(event.NewVtxos) > 0 {
			nextScheduledSettlement := s.WhenNextSettlement(ctx)

			needSchedule := false

			for _, vtxo := range event.NewVtxos {
				log.Infof("new vtxo: %s, expires at: %s", vtxo.Txid, vtxo.ExpiresAt.Format(time.RFC3339))
				if nextScheduledSettlement.IsZero() || vtxo.ExpiresAt.Before(nextScheduledSettlement) {
					nextScheduledSettlement = vtxo.ExpiresAt
					needSchedule = true
				}
			}

			if needSchedule {
				if err := s.scheduleNextSettlement(nextScheduledSettlement, data); err != nil {
					log.WithError(err).Info("schedule next claim failed")
				}
			}
		}
	}
}

func (s *Service) getVHTLCFunds(ctx context.Context, vhtlcOpts []vhtlc.Opts) ([]client.Vtxo, error) {
	cfg, err := s.GetConfigData(ctx)
	if err != nil {
		return nil, err
	}

	var allVtxos []client.Vtxo
	for _, opt := range vhtlcOpts {
		vHTLC, err := vhtlc.NewVHTLCScript(opt)
		if err != nil {
			return nil, err
		}

		addrStr, err := vHTLC.Address(cfg.Network.Addr, cfg.ServerPubKey)
		if err != nil {
			return nil, err
		}

		// Get vtxos for this address
		vtxos, _, err := s.grpcClient.ListVtxos(ctx, addrStr)
		if err != nil {
			return nil, err
		}
		allVtxos = append(allVtxos, vtxos...)
	}

	return allVtxos, nil
}

func (s *Service) claimVHTLC(
	ctx context.Context, preimage []byte, vhtlcOpts vhtlc.Opts,
) (string, error) {
	vtxos, err := s.getVHTLCFunds(ctx, []vhtlc.Opts{vhtlcOpts})
	if err != nil {
		return "", err
	}
	vtxo := &vtxos[0]

	vtxoTxHash, err := chainhash.NewHashFromStr(vtxo.Txid)
	if err != nil {
		return "", err
	}

	vtxoOutpoint := &wire.OutPoint{
		Hash:  *vtxoTxHash,
		Index: vtxo.VOut,
	}

	vtxoScript, err := vhtlc.NewVHTLCScript(vhtlcOpts)
	if err != nil {
		return "", err
	}

	claimClosure := vtxoScript.ClaimClosure
	claimWitnessSize := claimClosure.WitnessSize(len(preimage))
	claimScript, err := claimClosure.Script()
	if err != nil {
		return "", err
	}

	_, tapTree, err := vtxoScript.TapTree()
	if err != nil {
		return "", err
	}

	claimLeafProof, err := tapTree.GetTaprootMerkleProof(
		txscript.NewBaseTapLeaf(claimScript).TapHash(),
	)
	if err != nil {
		return "", err
	}

	ctrlBlock, err := txscript.ParseControlBlock(claimLeafProof.ControlBlock)
	if err != nil {
		return "", err
	}

	// self send output
	_, myAddr, _, _, err := s.GetAddress(ctx, 0)
	if err != nil {
		return "", err
	}

	decodedAddr, err := common.DecodeAddress(myAddr)
	if err != nil {
		return "", err
	}

	pkScript, err := common.P2TRScript(decodedAddr.VtxoTapKey)
	if err != nil {
		return "", err
	}

	amount, err := safecast.ToInt64(vtxo.Amount)
	if err != nil {
		return "", err
	}

	redeemTx, err := tree.BuildRedeemTx(
		[]common.VtxoInput{
			{
				RevealedTapscripts: vtxoScript.GetRevealedTapscripts(),
				Outpoint:           vtxoOutpoint,
				Amount:             amount,
				WitnessSize:        claimWitnessSize,
				Tapscript: &waddrmgr.Tapscript{
					ControlBlock:   ctrlBlock,
					RevealedScript: claimScript,
				},
			},
		},
		[]*wire.TxOut{
			{
				Value:    amount,
				PkScript: pkScript,
			},
		},
	)
	if err != nil {
		return "", err
	}

	redeemPtx, err := psbt.NewFromRawBytes(strings.NewReader(redeemTx), true)
	if err != nil {
		return "", err
	}

	if err := tree.AddConditionWitness(0, redeemPtx, wire.TxWitness{preimage}); err != nil {
		return "", err
	}

	reemdemTxId := redeemPtx.UnsignedTx.TxHash().String()

	redeemTx, err = redeemPtx.B64Encode()
	if err != nil {
		return "", err
	}

	signedRedeemTx, err := s.SignTransaction(ctx, redeemTx)
	if err != nil {
		return "", err
	}

	if _, _, err := s.grpcClient.SubmitRedeemTx(ctx, signedRedeemTx); err != nil {
		return "", err
	}

	return reemdemTxId, nil
}

func (s *Service) refundVHTLC(
	ctx context.Context, swapId string, withReceiver bool, vhtlcOpts vhtlc.Opts,
) (string, error) {
	vtxos, err := s.getVHTLCFunds(ctx, []vhtlc.Opts{vhtlcOpts})
	if err != nil {
		return "", err
	}
	vtxo := vtxos[0]

	vtxoTxHash, err := chainhash.NewHashFromStr(vtxo.Txid)
	if err != nil {
		return "", err
	}

	vtxoOutpoint := &wire.OutPoint{
		Hash:  *vtxoTxHash,
		Index: vtxo.VOut,
	}

	vtxoScript, err := vhtlc.NewVHTLCScript(vhtlcOpts)
	if err != nil {
		return "", err
	}

	var refundClosure tree.Closure
	refundClosure = vtxoScript.RefundWithoutReceiverClosure
	if withReceiver {
		refundClosure = vtxoScript.RefundClosure
	}
	refundWitnessSize := refundClosure.WitnessSize()
	refundScript, err := refundClosure.Script()
	if err != nil {
		return "", err
	}

	_, tapTree, err := vtxoScript.TapTree()
	if err != nil {
		return "", err
	}

	refundLeafProof, err := tapTree.GetTaprootMerkleProof(
		txscript.NewBaseTapLeaf(refundScript).TapHash(),
	)
	if err != nil {
		return "", err
	}

	ctrlBlock, err := txscript.ParseControlBlock(refundLeafProof.ControlBlock)
	if err != nil {
		return "", err
	}

	dest, err := txscript.PayToTaprootScript(vhtlcOpts.Sender)
	if err != nil {
		return "", err
	}

	amount, err := safecast.ToInt64(vtxo.Amount)
	if err != nil {
		return "", err
	}

	refundTx, err := tree.BuildRedeemTx(
		[]common.VtxoInput{
			{
				RevealedTapscripts: vtxoScript.GetRevealedTapscripts(),
				Outpoint:           vtxoOutpoint,
				Amount:             amount,
				WitnessSize:        refundWitnessSize,
				Tapscript: &waddrmgr.Tapscript{
					ControlBlock:   ctrlBlock,
					RevealedScript: refundScript,
				},
			},
		},
		[]*wire.TxOut{
			{
				Value:    amount,
				PkScript: dest,
			},
		},
	)
	if err != nil {
		return "", err
	}

	refundPtx, err := psbt.NewFromRawBytes(strings.NewReader(refundTx), true)
	if err != nil {
		return "", err
	}

	txid := refundPtx.UnsignedTx.TxHash().String()

	refundTx, err = refundPtx.B64Encode()
	if err != nil {
		return "", err
	}

	signedRefundTx, err := s.SignTransaction(ctx, refundTx)
	if err != nil {
		return "", err
	}

	if withReceiver {
		signedRefundTx, err = s.boltzRefundSwap(swapId, signedRefundTx)
		if err != nil {
			return "", err
		}
	}

	if _, _, err := s.grpcClient.SubmitRedeemTx(ctx, signedRefundTx); err != nil {
		return "", err
	}

	return txid, nil
}

func parsePubkey(pubkey string) (*secp256k1.PublicKey, error) {
	if len(pubkey) <= 0 {
		return nil, nil
	}

	dec, err := hex.DecodeString(pubkey)
	if err != nil {
		return nil, fmt.Errorf("invalid pubkey: %s", err)
	}

	pk, err := secp256k1.ParsePubKey(dec)
	if err != nil {
		return nil, fmt.Errorf("invalid pubkey: %s", err)
	}

	return pk, nil
}

func (s *Service) GetSwapHistory(ctx context.Context) ([]domain.Swap, error) {
	all, err := s.dbSvc.Swap().GetAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get swap history: %w", err)
	}
	if len(all) == 0 {
		return all, nil
	}
	// sort swaps by timestamp descending
	sort.Slice(all, func(i, j int) bool {
		return all[i].Timestamp > all[j].Timestamp
	})
	return all, nil
}
