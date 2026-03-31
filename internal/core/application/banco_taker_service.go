package application

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/ArkLabsHQ/fulmine/internal/core/domain"
	"github.com/ArkLabsHQ/fulmine/internal/core/ports"
	"github.com/ArkLabsHQ/fulmine/pkg/banco"
	introclient "github.com/ArkLabsHQ/introspector/pkg/client"
	"github.com/arkade-os/arkd/pkg/ark-lib/extension"
	"github.com/arkade-os/arkd/pkg/client-lib/client"
	"github.com/btcsuite/btcd/btcutil/psbt"
	log "github.com/sirupsen/logrus"
)

// TODO : move to config
const priceCacheTTL = 5 * time.Minute


type TakerStatus struct {
	Running bool
}

// BancoTakerService watches the arkd transaction stream for banco swap offers
// and automatically fulfills matching BTC offers.
type BancoTakerService struct {
	svc         *Service
	pairRepo    domain.BancoPairRepository
	introClient introclient.TransportClient
	priceFeed   ports.PriceFeed

	mu     sync.Mutex
	pairs  []domain.BancoPair
	prices map[string]cachedPrice // pair -> cached price with timestamp

	ctx      context.Context
	cancelFn context.CancelFunc
	running  bool
}

func newBancoTakerService(svc *Service, pairRepo domain.BancoPairRepository, introClient introclient.TransportClient, priceFeed ports.PriceFeed) *BancoTakerService {
	return &BancoTakerService{
		svc:         svc,
		pairRepo:    pairRepo,
		introClient: introClient,
		priceFeed:   priceFeed,
		prices:      make(map[string]cachedPrice),
	}
}

func (s *BancoTakerService) start() {
	ctx, cancel := context.WithCancel(context.Background())
	s.ctx = ctx
	s.cancelFn = cancel
	s.running = true

	pairs, err := s.pairRepo.List(ctx)
	if err != nil {
		log.WithError(err).Error("banco taker: failed to load pairs from db")
	} else {
		s.pairs = pairs
	}

	log.WithField("pairs", len(s.pairs)).Info("banco taker service started")

	go s.monitorStream(ctx)
}

// reloadPairs replaces the in-memory pair slice from the database.
// Must be called with s.mu held.
func (s *BancoTakerService) reloadPairs(ctx context.Context) error {
	pairs, err := s.pairRepo.List(ctx)
	if err != nil {
		return fmt.Errorf("failed to reload pairs: %w", err)
	}
	s.pairs = pairs
	return nil
}

func (s *BancoTakerService) AddPair(ctx context.Context, pair domain.BancoPair) error {
	if pair.Pair == "" {
		return fmt.Errorf("pair name is required")
	}
	if pair.MinAmount >= pair.MaxAmount {
		return fmt.Errorf("min_amount must be less than max_amount")
	}
	if pair.PriceFeed == "" {
		return fmt.Errorf("price_feed is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.pairRepo.Add(ctx, pair); err != nil {
		return err
	}
	return s.reloadPairs(ctx)
}

func (s *BancoTakerService) UpdatePair(ctx context.Context, pair domain.BancoPair) error {
	if pair.Pair == "" {
		return fmt.Errorf("pair name is required")
	}
	if pair.MinAmount >= pair.MaxAmount {
		return fmt.Errorf("min_amount must be less than max_amount")
	}
	if pair.PriceFeed == "" {
		return fmt.Errorf("price_feed is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.pairRepo.Update(ctx, pair); err != nil {
		return err
	}
	return s.reloadPairs(ctx)
}

func (s *BancoTakerService) RemovePair(ctx context.Context, pairName string) error {
	if pairName == "" {
		return fmt.Errorf("pair name is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.pairRepo.Remove(ctx, pairName); err != nil {
		return err
	}
	return s.reloadPairs(ctx)
}

func (s *BancoTakerService) ListPairs(ctx context.Context) ([]domain.BancoPair, error) {
	return s.pairRepo.List(ctx)
}

func (s *BancoTakerService) Stop() {
	if s.cancelFn != nil {
		s.cancelFn()
		s.cancelFn = nil
		s.running = false
		log.Info("banco taker service stopped")
	}
}

// Status returns the current state of the taker service.
func (s *BancoTakerService) Status() TakerStatus {
	s.mu.Lock()
	defer s.mu.Unlock()

	return TakerStatus{s.running}
}

// getPrice returns the cached price for a pair, refreshing from the feed if
// the cached value is older than priceCacheTTL (5 minutes).
// Must be called with s.mu held.
func (s *BancoTakerService) getPrice(pair *domain.BancoPair) (float64, error) {
	now := time.Now()

	cached, ok := s.prices[pair.Pair]

	if ok && now.Sub(cached.fetchedAt) < priceCacheTTL {
		return cached.price, nil
	}

	// Unlock while making the HTTP call to avoid blocking other goroutines.
	s.mu.Unlock()
	price, err := s.priceFeed.FetchPrice(s.ctx, pair.PriceFeed)
	s.mu.Lock()

	if err != nil {
		// If we have a stale price, return it with a warning rather than failing
		if ok {
			log.WithError(err).WithField("pair", pair.Pair).Warn("taker: failed to refresh price, using stale cache")
			return cached.price, nil
		}
		return 0, fmt.Errorf("failed to fetch price for %s: %w", pair.Pair, err)
	}

	s.prices[pair.Pair] = cachedPrice{price: price, fetchedAt: now}

	log.WithFields(log.Fields{
		"pair":  pair.Pair,
		"price": price,
	}).Debug("taker: price updated")

	return price, nil
}

// monitorStream subscribes to the arkd transaction stream and processes ArkTx events.
// Follows the same pattern as DelegatorService.monitorVtxosSpent:
// returns on stream close, logs and continues on event errors.
func (s *BancoTakerService) monitorStream(ctx context.Context) {
	log.Debug("taker: starting transaction stream monitor")

	var eventsCh <-chan client.TransactionEvent
	var stop func()
	var err error

	eventsCh, stop, err = s.svc.Client().GetTransactionsStream(ctx)
	if err != nil {
		log.WithError(err).Error("taker: failed to establish initial connection to transaction stream")
		return
	}

	for {
		select {
		case <-ctx.Done():
			if stop != nil {
				stop()
			}
			return
		case event, ok := <-eventsCh:
			if !ok {
				log.Debug("taker: tx stream closed")
				return
			}
			if event.Err != nil {
				log.WithError(event.Err).Error("taker: error received from transaction stream")
				continue
			}

			// Only process ArkTx events; skip CommitmentTx and SweepTx.
			if event.ArkTx == nil {
				continue
			}

			s.processArkTx(ctx, event.ArkTx)
		}
	}
}

// processArkTx handles a single ArkTx event from the transaction stream.
func (s *BancoTakerService) processArkTx(ctx context.Context, notification *client.TxNotification) {
	tx, err := psbt.NewFromRawBytes(strings.NewReader(notification.Tx), true)
	if err != nil {
		log.WithError(err).Warn("taker: failed to decode psbt")
		return
	}

	// Look for an extension output in the transaction
	ext, err := extension.NewExtensionFromTx(tx.UnsignedTx)
	if err != nil {
		log.WithError(err).WithField("txid", notification.Txid).Debug("taker: no extension output in tx")
		return
	}

	// Look for a banco offer in the extension
	offer, err := banco.FindBancoOffer(ext)
	if err != nil {
		log.WithError(err).Warn("taker: failed to decode banco offer from extension")
		return
	}
	if offer == nil {
		log.WithField("txid", notification.Txid).Debug("taker: extension has no banco offer packet")
		return
	}

	txid := notification.Txid
	// Compute the deposited amount from the tx itself by finding the output
	// that matches the offer's swap address pkScript.

	var swapOutputValue int64
	var swapOutputIndex int
	for i, out := range tx.UnsignedTx.TxOut {
		log.WithFields(log.Fields{
			"txid":      txid,
			"outIndex":  i,
			"value":     out.Value,
			"scriptLen": len(out.PkScript),
		}).Debug("taker: inspecting tx output")
		if bytes.Equal(out.PkScript, offer.SwapPkScript) {
			swapOutputValue = out.Value
			swapOutputIndex = i
			log.WithFields(log.Fields{
				"txid":     txid,
				"outIndex": i,
				"value":    out.Value,
			}).Debug("taker: found matching swap output")
			break
		}
	}
	if swapOutputValue <= 0 {
		log.WithField("txid", txid).Debug("taker: no matching swap output in tx")
		return
	}

	// Determine the deposit (base) asset from the extension's asset packet.
	// If an asset packet is present, the deposited asset is the first group's ID.
	// If no asset packet, the deposit is BTC.
	depositAsset := "BTC"
	if assetPacket := ext.GetAssetPacket(); len(assetPacket) > 0 {
		for _, asst := range assetPacket {
			for _, out := range asst.Outputs {
				if out.Vout == uint16(swapOutputIndex) {
					if depositAsset != "BTC" {
						log.WithField("txid", txid).Debug("taker: swap address contains more than 1 asset")
					}

					if asst.AssetId == nil {
						log.WithField("txid", txid).Debug("taker: asset output is an issuance")
						return
					}
					depositAsset = asst.AssetId.String()
					break
				}
			}
		}
	}

	// we lock to sync processing of different orders
	s.mu.Lock()
	defer s.mu.Unlock()

	// Find a matching pair (must match both base and quote)
	pair := s.findMatchingPair(depositAsset, offer.WantAsset)
	if pair == nil {
		return
	}

	// Check amount bounds
	if offer.WantAmount < pair.MinAmount {
		return
	}
	if offer.WantAmount > pair.MaxAmount {
		return
	}

	// Pre-check BTC balance for BTC offers.
	// For asset offers the balance check happens inside FulfillOffer during coin selection.
	if offer.WantAsset == "" {
		balance, err := s.svc.Balance(ctx)
		if err != nil {
			log.WithError(err).Warn("taker: failed to check balance")
			return
		}
		log.WithFields(log.Fields{
			"txid":       txid,
			"balance":    balance.OffchainBalance.Total,
			"wantAmount": offer.WantAmount,
		}).Debug("taker: BTC balance check")
		if balance.OffchainBalance.Total < offer.WantAmount {
			log.WithFields(log.Fields{
				"txid":       txid,
				"balance":    balance.OffchainBalance.Total,
				"wantAmount": offer.WantAmount,
			}).Debug("taker: insufficient BTC balance")
			return
		}
	}


	log.WithFields(log.Fields{
		"txid":      txid,
		"pair":      pair.Pair,
		"priceFeed": pair.PriceFeed,
	}).Debug("taker: fetching price from feed")

	feedPrice, err := s.getPrice(pair)
	if err != nil {
		log.WithError(err).WithField("pair", pair.Pair).Warn("taker: failed to get price, skipping offer")
		return
	}

	log.WithFields(log.Fields{
		"txid":      txid,
		"feedPrice": feedPrice,
		"pair":      pair.Pair,
	}).Debug("taker: fetched feed price")



	// offerPrice = what the maker wants / what the maker deposited
	offerPrice := float64(offer.WantAmount) / float64(swapOutputValue)

	log.WithFields(log.Fields{
		"txid":       txid,
		"offerPrice": offerPrice,
		"feedPrice":  feedPrice,
		"wantAmount": offer.WantAmount,
		"swapAmount": swapOutputValue,
	}).Debug("taker: computed offer price")

	if offerPrice > feedPrice {
		log.WithFields(log.Fields{
			"txid":       txid,
			"offerPrice": offerPrice,
			"feedPrice":  feedPrice,
		}).Debug("taker: offer price exceeds feed price, skipping")
		return
	}

	log.WithFields(log.Fields{
		"txid":       txid,
		"offerPrice": offerPrice,
		"feedPrice":  feedPrice,
	}).Debug("taker: offer price acceptable, proceeding to fulfill")

	// Fulfill the offer
	log.WithFields(log.Fields{
		"txid":       txid,
		"wantAmount": offer.WantAmount,
		"wantAsset":  offer.WantAsset,
	}).Info("taker: attempting to fulfill banco offer")

	result, err := banco.FulfillOffer(
		ctx,
		offer,
		s.svc.Client(),
		s.svc.Indexer(),
		s.svc.ArkClient,
		s.introClient,
	)
	if err != nil {
		log.WithError(err).WithField("txid", txid).Warn("taker: fulfillment failed")
		return
	}

	log.WithFields(log.Fields{
		"offerTxid":  txid,
		"arkTxid":    result.ArkTxid,
		"wantAmount": offer.WantAmount,
	}).Info("taker: banco offer fulfilled successfully")
}

// findMatchingPair returns the first configured pair whose base and quote
// both match the offer. The pair format is "{base}/{quote}" where each side
// is either "BTC" or an asset ID. depositAsset is "BTC" or the hex asset ID
// the maker deposited; wantAsset is "" (for BTC) or the hex asset ID.
func (s *BancoTakerService) findMatchingPair(depositAsset, wantAsset string) *domain.BancoPair {
	for i := range s.pairs {
		pair := &s.pairs[i]
		if pair.Base() != depositAsset {
			continue
		}
		// Quote side: pair.QuoteAssetID is "" for BTC, asset ID otherwise.
		if pair.QuoteAssetID != wantAsset {
			continue
		}
		return pair
	}
	return nil
}

// cachedPrice holds a price and the time it was fetched.
type cachedPrice struct {
	price     float64
	fetchedAt time.Time
}
