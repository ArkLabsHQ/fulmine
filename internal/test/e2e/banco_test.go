package e2e_test

import (
	"encoding/hex"
	"strings"
	"sync"
	"testing"
	"time"

	pb "github.com/ArkLabsHQ/fulmine/api-spec/protobuf/gen/go/fulmine/v1"
	"github.com/ArkLabsHQ/fulmine/pkg/banco"
	"github.com/arkade-os/arkd/pkg/ark-lib/asset"
	introclient "github.com/ArkLabsHQ/introspector/pkg/client"
	"github.com/arkade-os/arkd/pkg/client-lib/client"
	grpcclient "github.com/arkade-os/arkd/pkg/client-lib/client/grpc"
	"github.com/arkade-os/arkd/pkg/client-lib/indexer"
	grpcindexer "github.com/arkade-os/arkd/pkg/client-lib/indexer/grpc"
	clientTypes "github.com/arkade-os/arkd/pkg/client-lib/types"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

const introspectorAddr = "localhost:7073"

func newIntroClient(t *testing.T) introclient.TransportClient {
	t.Helper()
	conn, err := grpc.NewClient(introspectorAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	return introclient.NewGRPCClient(conn)
}

func arkTransportClient(t *testing.T) client.TransportClient {
	t.Helper()
	c, err := grpcclient.NewClient("localhost:7070")
	require.NoError(t, err)
	return c
}

func arkIndexerClient(t *testing.T) indexer.Indexer {
	t.Helper()
	c, err := grpcindexer.NewClient("localhost:7070")
	require.NoError(t, err)
	return c
}

func decodeOffer(t *testing.T, offerHex string) *banco.BancoOffer {
	t.Helper()
	data, err := hex.DecodeString(offerHex)
	require.NoError(t, err)
	offer, err := banco.DeserializeOffer(data)
	require.NoError(t, err)
	return offer
}

const coingeckoPriceFeed = "https://api.coingecko.com/api/v3/simple/price?ids=bitcoin&vs_currencies=usd"

// addBancoPair configures a pair on the taker bot and registers cleanup.
func addBancoPair(t *testing.T, fc pb.ServiceClient, pair, quoteAssetID, priceFeed string) {
	t.Helper()
	ctx := t.Context()
	_, err := fc.AddBancoPair(ctx, &pb.AddBancoPairRequest{
		Pair:         pair,
		QuoteAssetId: quoteAssetID,
		MinAmount:    1,
		MaxAmount:    100000000,
		PriceFeed:    priceFeed,
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = fc.RemoveBancoPair(ctx, &pb.RemoveBancoPairRequest{Pair: pair})
	})
}

// fundFulmineWithAsset issues an asset from a temp client and sends it to the
// fulmine wallet. Returns the asset ID.
func fundFulmineWithAsset(t *testing.T, supply uint64) string {
	t.Helper()
	ctx := t.Context()

	// Get fulmine's offchain ark address (extract from BIP21 URI).
	fc, err := newFulmineClient(clientFulmineURL)
	require.NoError(t, err)
	addrResp, err := fc.GetAddress(ctx, &pb.GetAddressRequest{})
	require.NoError(t, err)
	bip21 := addrResp.GetAddress()
	// BIP21 format: "bitcoin:<onchain>?ark=<offchain>"
	parts := strings.SplitN(bip21, "?ark=", 2)
	require.Len(t, parts, 2, "expected BIP21 address with ark= param")
	fulmineAddr := parts[1]

	// Create a temp client, fund it, issue asset.
	tempClient, _, _ := setupArkSDKwithPublicKey(t)
	faucetOffchain(t, tempClient, 0.0005)
	assetID := issueAsset(t, tempClient, supply)

	// Send asset to fulmine.
	_, err = tempClient.SendOffChain(ctx, []clientTypes.Receiver{{
		To:     fulmineAddr,
		Amount: 1000, // dust BTC for the VTXO
		Assets: []clientTypes.Asset{{AssetId: assetID, Amount: supply}},
	}})
	require.NoError(t, err)

	// Give fulmine time to see the incoming VTXO.
	time.Sleep(3 * time.Second)

	return assetID
}

// waitForBalanceDecrease polls fulmine's BTC balance until it drops below the
// given threshold, or fails after timeout.
func waitForBalanceDecrease(t *testing.T, fc pb.ServiceClient, before uint64, timeout time.Duration) uint64 {
	t.Helper()
	ctx := t.Context()
	deadline := time.Now().Add(timeout)
	var after uint64
	for time.Now().Before(deadline) {
		resp, err := fc.GetBalance(ctx, &pb.GetBalanceRequest{})
		require.NoError(t, err)
		after = resp.GetAmount()
		if after < before {
			return after
		}
		time.Sleep(2 * time.Second)
	}
	t.Fatalf("fulmine balance did not decrease within %s (before=%d, current=%d)", timeout, before, after)
	return 0
}

// TestBancoTakerBotAssetToBTC verifies the taker bot auto-fulfills an offer
// where the maker deposits an asset and wants BTC.
func TestBancoTakerBotAssetToBTC(t *testing.T) {
	ctx := t.Context()
	priceFeed := coingeckoPriceFeed

	fc, err := newFulmineClient(clientFulmineURL)
	require.NoError(t, err)

	// Maker creates and owns the asset.
	maker, _, _ := setupArkSDKwithPublicKey(t)
	faucetOffchain(t, maker, 0.0005)
	assetID := issueAsset(t, maker, 500)

	// Configure pair: offers wanting BTC (QuoteAssetID="").
	addBancoPair(t, fc, assetID+"/BTC", "", priceFeed)

	// Record fulmine BTC balance before.
	balanceBefore, err := fc.GetBalance(ctx, &pb.GetBalanceRequest{})
	require.NoError(t, err)

	// Maker creates offer: deposit asset, want 5000 sats BTC.
	transport := arkTransportClient(t)
	intro := newIntroClient(t)
	offerResult, err := banco.CreateOffer(ctx, banco.CreateOfferParams{
		WantAmount: 5000,
	}, transport, intro, maker)
	require.NoError(t, err)
	require.NotEmpty(t, offerResult.SwapAddress)

	// Fund the swap address with the asset, including the offer packet
	// in the extension output so the taker bot can discover it.
	wg := &sync.WaitGroup{}
	wg.Add(1)
	var incomingFunds []clientTypes.Vtxo
	var incomingErr error
	go func() {
		defer wg.Done()
		incomingFunds, incomingErr = maker.NotifyIncomingFunds(ctx, offerResult.SwapAddress)
	}()

	txid, err := banco.FundOffer(ctx, offerResult, 450,
		[]clientTypes.Asset{{AssetId: assetID, Amount: 500}},
		transport, maker,
	)
	t.Log(txid)
	require.NoError(t, err)
	wg.Wait()
	require.NoError(t, incomingErr)
	require.NotEmpty(t, incomingFunds)

	// Wait for the taker bot to detect and fulfill — fulmine BTC balance should decrease.
	balanceAfter := waitForBalanceDecrease(t, fc, balanceBefore.GetAmount(), 30*time.Second)

	spent := balanceBefore.GetAmount() - balanceAfter
	t.Logf("asset/btc: fulmine balance before=%d after=%d spent=%d",
		balanceBefore.GetAmount(), balanceAfter, spent)
}

// TestBancoTakerBotBTCToAsset verifies the taker bot auto-fulfills an offer
// where the maker deposits BTC and wants an asset.
func TestBancoTakerBotBTCToAsset(t *testing.T) {
	ctx := t.Context()
	priceFeed := coingeckoPriceFeed

	fc, err := newFulmineClient(clientFulmineURL)
	require.NoError(t, err)

	// Pre-fund fulmine with an asset.
	assetID := fundFulmineWithAsset(t, 1000)

	// Configure pair: offers wanting this asset (QuoteAssetID=assetID).
	addBancoPair(t, fc, "BTC/"+assetID, assetID, priceFeed)

	// Create maker, fund with BTC.
	maker, _, _ := setupArkSDKwithPublicKey(t)
	faucetOffchain(t, maker, 0.0005)

	// Maker creates offer: deposit BTC, want 500 of asset.
	transport := arkTransportClient(t)
	intro := newIntroClient(t)
	wantAssetID, err := asset.NewAssetIdFromString(assetID)
	require.NoError(t, err)
	offerResult, err := banco.CreateOffer(ctx, banco.CreateOfferParams{
		WantAmount: 500,
		WantAsset:  wantAssetID,
	}, transport, intro, maker)
	require.NoError(t, err)
	require.NotEmpty(t, offerResult.SwapAddress)

	// Fund the swap address with BTC, including the offer packet
	// in the extension output so the taker bot can discover it.
	wg := &sync.WaitGroup{}
	wg.Add(1)
	var incomingFunds []clientTypes.Vtxo
	var incomingErr error
	go func() {
		defer wg.Done()
		incomingFunds, incomingErr = maker.NotifyIncomingFunds(ctx, offerResult.SwapAddress)
	}()

	_, err = banco.FundOffer(ctx, offerResult, 10000, nil, transport, maker)
	require.NoError(t, err)
	wg.Wait()
	require.NoError(t, incomingErr)
	require.NotEmpty(t, incomingFunds)

	// Wait for the taker bot to fulfill — maker should receive the asset.
	// Poll maker's VTXOs for up to 30s.
	deadline := time.Now().Add(30 * time.Second)
	var makerAssetVtxos []clientTypes.Vtxo
	for time.Now().Before(deadline) {
		makerAssetVtxos = listVtxosWithAsset(t, maker, assetID)
		if len(makerAssetVtxos) > 0 {
			break
		}
		time.Sleep(2 * time.Second)
	}
	require.NotEmpty(t, makerAssetVtxos, "maker should have received asset from taker bot")
	t.Logf("btc/asset: maker received asset %s", assetID)
}

// TestBancoTakerBotAssetToAsset verifies the taker bot auto-fulfills an offer
// where the maker deposits assetA and wants assetB.
func TestBancoTakerBotAssetToAsset(t *testing.T) {
	ctx := t.Context()
	priceFeed := coingeckoPriceFeed

	fc, err := newFulmineClient(clientFulmineURL)
	require.NoError(t, err)

	// Create maker, fund with BTC, issue assetA.
	maker, _, _ := setupArkSDKwithPublicKey(t)
	faucetOffchain(t, maker, 0.0005)
	assetA := issueAsset(t, maker, 500)

	// Pre-fund fulmine with assetB.
	assetB := fundFulmineWithAsset(t, 1000)

	// Configure pair: offers wanting assetB.
	addBancoPair(t, fc, assetA+"/"+assetB, assetB, priceFeed)

	// Maker creates offer: deposit assetA, want 500 of assetB.
	transport := arkTransportClient(t)
	intro := newIntroClient(t)
	wantAssetID, err := asset.NewAssetIdFromString(assetB)
	require.NoError(t, err)
	offerResult, err := banco.CreateOffer(ctx, banco.CreateOfferParams{
		WantAmount: 500,
		WantAsset:  wantAssetID,
	}, transport, intro, maker)
	require.NoError(t, err)
	require.NotEmpty(t, offerResult.SwapAddress)

	// Fund the swap address with assetA, including the offer packet
	// in the extension output so the taker bot can discover it.
	wg := &sync.WaitGroup{}
	wg.Add(1)
	var incomingFunds []clientTypes.Vtxo
	var incomingErr error
	go func() {
		defer wg.Done()
		incomingFunds, incomingErr = maker.NotifyIncomingFunds(ctx, offerResult.SwapAddress)
	}()

	_, err = banco.FundOffer(ctx, offerResult, 450,
		[]clientTypes.Asset{{AssetId: assetA, Amount: 500}},
		transport, maker,
	)
	require.NoError(t, err)
	wg.Wait()
	require.NoError(t, incomingErr)
	require.NotEmpty(t, incomingFunds)

	// Wait for the taker bot to fulfill — maker should receive assetB.
	deadline := time.Now().Add(30 * time.Second)
	var makerAssetVtxos []clientTypes.Vtxo
	for time.Now().Before(deadline) {
		makerAssetVtxos = listVtxosWithAsset(t, maker, assetB)
		if len(makerAssetVtxos) > 0 {
			break
		}
		time.Sleep(2 * time.Second)
	}
	require.NotEmpty(t, makerAssetVtxos, "maker should have received assetB from taker bot")
	t.Logf("asset/asset: maker received assetB %s", assetB)
}
