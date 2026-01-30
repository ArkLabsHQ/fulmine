package swap

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/ArkLabsHQ/fulmine/pkg/boltz"
	"github.com/ArkLabsHQ/fulmine/pkg/vhtlc"
	"github.com/lightningnetwork/lnd/input"

	"github.com/btcsuite/btcd/chaincfg"
	log "github.com/sirupsen/logrus"
)

type ChainSwapStatus int

const (
	// Pending states
	ChainSwapPending ChainSwapStatus = iota
	ChainSwapUserLocked
	ChainSwapServerLocked

	// Success states
	ChainSwapClaimed

	// Failed states
	ChainSwapUserLockedFailed
	ChainSwapFailed
	ChainSwapRefundFailed
	ChainSwapRefunded
)

type ChainSwap struct {
	Id       string
	Amount   uint64
	Preimage []byte

	UserBtcLockupAddress string

	VhtlcOpts vhtlc.Opts

	UserLockTxid   string
	ServerLockTxid string
	ClaimTxid      string
	RefundTxid     string

	Timestamp int64
	Status    ChainSwapStatus
	Error     string

	// onEvent is called when swap state transitions occur
	// Emits typed events that the service layer can handle
	onEvent ChainSwapEventCallback
}

func NewChainSwap(id string, amount uint64, preimage []byte, vhtlcOpts *vhtlc.Opts, eventCallback ChainSwapEventCallback) (*ChainSwap, error) {
	if id == "" {
		return nil, errors.New("id cannot be empty")
	}

	if amount == 0 {
		return nil, errors.New("amount cannot be 0")
	}

	if vhtlcOpts == nil {
		return nil, errors.New("vhtlcOpts cannot be nil")
	}

	if preimage == nil {
		return nil, errors.New("preimage cannot be nil")
	}

	return &ChainSwap{
		Id:        id,
		Timestamp: time.Now().Unix(),
		Status:    ChainSwapPending,
		Amount:    amount,
		Preimage:  preimage,
		VhtlcOpts: *vhtlcOpts,
		onEvent:   eventCallback,
	}, nil
}

func (s *ChainSwap) UserLock(txid string) {
	s.UserLockTxid = txid
	s.Status = ChainSwapUserLocked

	// Emit typed event
	if s.onEvent != nil {
		s.onEvent(UserLockEvent{
			SwapID: s.Id,
			TxID:   txid,
		})
	}
}

func (s *ChainSwap) ServerLock(txid string) {
	s.ServerLockTxid = txid
	s.Status = ChainSwapServerLocked

	// Emit typed event
	if s.onEvent != nil {
		s.onEvent(ServerLockEvent{
			SwapID: s.Id,
			TxID:   txid,
		})
	}
}

func (s *ChainSwap) Claim(txid string) {
	s.ClaimTxid = txid
	s.Status = ChainSwapClaimed

	// Emit typed event
	if s.onEvent != nil {
		s.onEvent(ClaimEvent{
			SwapID: s.Id,
			TxID:   txid,
		})
	}
}

func (s *ChainSwap) Refund(txid string) {
	s.RefundTxid = txid
	s.Status = ChainSwapRefunded

	// Emit typed event
	if s.onEvent != nil {
		s.onEvent(RefundEvent{
			SwapID: s.Id,
			TxID:   txid,
		})
	}
}

func (s *ChainSwap) Fail(err string) {
	s.Status = ChainSwapFailed
	s.Error = err

	// Emit typed event
	if s.onEvent != nil {
		s.onEvent(FailEvent{
			SwapID: s.Id,
			Error:  err,
		})
	}
}

func (s *ChainSwap) RefundFailed(err string) {
	s.Status = ChainSwapRefundFailed
	s.Error = err

	// Emit typed event
	if s.onEvent != nil {
		s.onEvent(RefundFailedEvent{
			SwapID: s.Id,
			Error:  err,
		})
	}
}

func (s *ChainSwap) UserLockedFailed(err string) {
	s.Status = ChainSwapUserLockedFailed
	s.Error = err

	// Emit typed event
	if s.onEvent != nil {
		s.onEvent(UserLockFailedEvent{
			SwapID: s.Id,
			Error:  err,
		})
	}
}

// ChainSwapEvent is a marker interface for typed domain events
// Each event type represents a specific state transition with its own data
type ChainSwapEvent interface {
	isChainSwapEvent()
}

// UserLockEvent is emitted when user locks funds (Ark VTXO or BTC UTXO)
type UserLockEvent struct {
	SwapID string
	TxID   string
}

func (UserLockEvent) isChainSwapEvent() {}

// ServerLockEvent is emitted when server (Boltz) locks funds
type ServerLockEvent struct {
	SwapID string
	TxID   string
}

func (ServerLockEvent) isChainSwapEvent() {}

// ClaimEvent is emitted when swap is successfully claimed
type ClaimEvent struct {
	SwapID string
	TxID   string
}

func (ClaimEvent) isChainSwapEvent() {}

// RefundEvent is emitted when swap is refunded
type RefundEvent struct {
	SwapID string
	TxID   string
}

func (RefundEvent) isChainSwapEvent() {}

// FailEvent is emitted when swap fails
type FailEvent struct {
	SwapID string
	Error  string
}

func (FailEvent) isChainSwapEvent() {}

// RefundFailedEvent is emitted when refund attempt fails
type RefundFailedEvent struct {
	SwapID string
	Error  string
}

func (RefundFailedEvent) isChainSwapEvent() {}

// UserLockFailedEvent is emitted when user lock fails
type UserLockFailedEvent struct {
	SwapID string
	Error  string
}

func (UserLockFailedEvent) isChainSwapEvent() {}

// ChainSwapEventCallback is called whenever a chain swap event occurs
type ChainSwapEventCallback func(event ChainSwapEvent)

// ChainSwapArkToBtc performs an Ark → Bitcoin on-chain atomic swap
// This is the main entry point for swapping Ark VTXOs to Bitcoin on-chain
// Send ARK VTXO -> Receive BTC UTXO
// Boltz locks BTC UTXO and it sends details on how user can claim it in claimDetails and where to send ARK VTXO in lockupDetails
// claimLeaf(claimDetials) is used by user to cooperative claim BTC tx
// LockupDetails should container VHTLC address where Boltz's Fulmine is receiver
// 1. generate preimage
// 2. POST /swap/chain: preimageHash, claimPubKey, refundPubKey
//
//	{
//		"id": "KEBsfLtqhsmA",
//		"claimDetails": {
//			"serverPublicKey": "02a9750704fdf536a573472938b4457be73e75513ff5ba3d017b2d73e070055026",
//			"amount": 2797,
//			"lockupAddress": "bcrt1pyz4djuc8eqn9na9s5l5lqg24uawv5ycaw6a3r9vaz0w3ewen7maq7ldt8q",
//			"timeoutBlockHeight": 542,
//			"swapTree": {
//				"claimLeaf": {
//					"version": 192,
//					"output": "82012088a914608bc8a727928e8aa18c7a2489c003deb47ff08388207599756afc49ebf5a6f3ac5848ef0afe934edd7b669bca02029acf10cc7f83acac"
//				},
//				"refundLeaf": {
//					"version": 192,
//					"output": "20a9750704fdf536a573472938b4457be73e75513ff5ba3d017b2d73e070055026ad021e02b1"
//				}
//			}
//		},
//		"lockupDetails": {
//			"serverPublicKey": "025067f8c4f61cf3bcbf131edbe0256d890332d2cdba64355a4153db1101e84cd0",
//			"amount": 3000,
//			"lockupAddress": "tark1qz4a0tydelxxun8w62zz3zjk36sr6aqrs58gmne23r9ea37jwx9awtw542kccdpm6nsuwfdw808r56humw46hqrrrg8dsem5v6hqu5d97zgl6c",
//			"timeoutBlockHeight": 1769647586,
//			"timeouts": {
//				"refund": 1769647586,
//				"unilateralClaim": 6144,
//				"unilateralRefund": 6144,
//				"unilateralRefundWithoutReceiver": 12288
//			}
//		}
//	}
//
// what user needs to validate?
// - from claimDetails validate claimLeaf ? maybe we dont needs since for us it is important that we can refund vhtlc , validate HTLC
// - from lockupDetails validate(recreate) vhtlc address
// claimPubKey and preimage are used to claim BTC tx
// refundPubKey is used to refund ARK VTXO tx after timeout if something goes wrong
//  3. SendOffchain -> send VTXO to address claimable by Boltz Fulmine(receiverPubKey)
//  4. Once Boltz notices VTXO, it will send(lock) coins on BTC mainnet - BTC lockup TX
//  5. Boltz send server.mempool and server.confimed events via WebSocket and we than decide when to claim
//     5.1 Cooperative claim so Boltz doesnt need to scan mainchain for preimage
//     5.2 Unilateral claim if Boltz not responsive
//  6. User Refunds in case something goes wrong, we should schedule unilateral refund?
//     6.1 Try Cooperative Refund
//     6.2 Try Unilateral Refund
//  7. Quote mechanism: What if user sends(locks) less amount than what he announced in swap request?
//     in transaction.lockupfailed get quote and accept quote
func (h *SwapHandler) ChainSwapArkToBtc(
	ctx context.Context,
	amount uint64,
	btcDestinationAddress string,
	network *chaincfg.Params,
	eventCallback ChainSwapEventCallback,
	unilateralRefundCallback func(swapId string, opts vhtlc.Opts) error,
) (*ChainSwap, error) {
	log.Infof("Initiating Ark → BTC chain swap for %d sats to %s", amount, btcDestinationAddress)

	var (
		arkToBtc           = true
		btcClaimPrivKey    = h.privateKey
		btcClaimPubKey     = btcClaimPrivKey.PubKey()
		vhtlcRefundPrivKey = h.privateKey
		vhtlcRefundPubKey  = vhtlcRefundPrivKey.PubKey()
	)

	preimage, preimageHashSHA256, preimageHashHASH160, err := genPreimageInfo()
	if err != nil {
		return nil, fmt.Errorf("failed to generate preimage: %w", err)
	}

	createReq := boltz.CreateChainSwapRequest{
		From:            boltz.CurrencyArk,
		To:              boltz.CurrencyBtc,
		PreimageHash:    hex.EncodeToString(preimageHashSHA256[:]),
		ClaimPublicKey:  hex.EncodeToString(btcClaimPubKey.SerializeCompressed()),
		RefundPublicKey: hex.EncodeToString(vhtlcRefundPubKey.SerializeCompressed()),
		UserLockAmount:  amount,
	}

	swapResp, err := h.boltzSvc.CreateChainSwap(createReq)
	if err != nil {
		return nil, fmt.Errorf("failed to create chain swap with Boltz: %w", err)
	}

	// validate proposed BTC script so that we are sure that we can claim BTC UTXO before we send VTXO
	if err := validateBtcClaimOrRefundPossible(
		swapResp.GetSwapTree(arkToBtc),
		arkToBtc,
		swapResp.ClaimDetails.ServerPublicKey,
		btcClaimPubKey,
		preimageHashHASH160,
		nil,
		0,
	); err != nil {
		return nil, fmt.Errorf("invalid HTLC: %w", err)
	}

	log.Infof("Created chain swap %s with Boltz", swapResp.Id)

	vhtlcOpts, err := validateVHTLC(ctx, h, arkToBtc, swapResp, preimageHashHASH160)
	if err != nil {
		return nil, fmt.Errorf("invalid VHTLC: %w", err)
	}

	if err := validateBtcLockupAddress(
		network,
		swapResp.ClaimDetails.LockupAddress,
		swapResp.ClaimDetails.ServerPublicKey,
		btcClaimPrivKey.PubKey(),
		swapResp.GetSwapTree(arkToBtc),
	); err != nil {
		return nil, fmt.Errorf("BTC lockup address validation failed: %w", err)
	}

	chainSwap, err := NewChainSwap(swapResp.Id, amount, preimage, vhtlcOpts, eventCallback)
	if err != nil {
		return nil, fmt.Errorf("failed to create chain swap: %w", err)
	}

	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Errorf("panic in monitorAndClaimArkToBtcSwap: %v", r)
			}
		}()

		h.monitorAndClaimArkToBtcSwap(
			ctx,
			network,
			eventCallback,
			unilateralRefundCallback,
			btcClaimPrivKey,
			preimage,
			btcDestinationAddress,
			swapResp,
			chainSwap,
		)
	}()

	return chainSwap, nil
}

//	ChainSwapBtcToArk performs a Bitcoin on-chain → Ark atomic swap
//
// This is the reverse direction: user locks BTC on-chain, receives Ark VTXOs
// Send BTC -> Receive VTXO
//
//	{
//		"id": "rZfDV8vtQ5Jk",
//		"claimDetails": {
//			"serverPublicKey": "025067f8c4f61cf3bcbf131edbe0256d890332d2cdba64355a4153db1101e84cd0",
//			"amount": 2801,
//			"lockupAddress": "tark1qz4a0tydelxxun8w62zz3zjk36sr6aqrs58gmne23r9ea37jwx9a0g5xczda6llpnevn3gnw3muwwnw9cze8988g0j2dvhssdfqkkg8n4jt5ln",
//			"timeoutBlockHeight": 1769678334,
//			"timeouts": {
//				"refund": 1769678334,
//				"unilateralClaim": 6144,
//				"unilateralRefund": 6144,
//				"unilateralRefundWithoutReceiver": 12288
//			}
//		},
//		"lockupDetails": {
//			"serverPublicKey": "028923258347dd79d51195e2054d9f92a6c4cfcbce86a92e3b9e2f7b51a0750d2b",
//			"amount": 3000,
//			"lockupAddress": "bcrt1pugmgfs2zx4w48w2cgnsvvrhpdy0zlntdz8gch2rz6tafnm8v8ewqm5mpjg",
//			"timeoutBlockHeight": 760,
//			"swapTree": {
//				"claimLeaf": {
//					"version": 192,
//					"output": "82012088a9140f49a45d0bea33b5be812590dc8d284a0ebe195c88208923258347dd79d51195e2054d9f92a6c4cfcbce86a92e3b9e2f7b51a0750d2bac"
//				},
//				"refundLeaf": {
//					"version": 192,
//					"output": "207599756afc49ebf5a6f3ac5848ef0afe934edd7b669bca02029acf10cc7f83acad02f802b1"
//				}
//			},
//		}
//	}
func (h *SwapHandler) ChainSwapBtcToArk(
	_ context.Context,
	amount uint64,
	network *chaincfg.Params,
	eventCallback ChainSwapEventCallback,
) (*ChainSwap, error) {
	log.Infof("Initiating BTC → Ark chain swap for %d sats", amount)

	var (
		arkToBtc    = false
		claimPubKey = h.privateKey.PubKey()
		refundKey   = h.privateKey
	)

	preimage, preimageHashSHA256, preimageHashHASH160, err := genPreimageInfo()
	if err != nil {
		return nil, fmt.Errorf("failed to generate preimage: %w", err)
	}

	createReq := boltz.CreateChainSwapRequest{
		From:            boltz.CurrencyBtc,
		To:              boltz.CurrencyArk,
		PreimageHash:    hex.EncodeToString(preimageHashSHA256[:]),
		ClaimPublicKey:  hex.EncodeToString(claimPubKey.SerializeCompressed()),
		RefundPublicKey: hex.EncodeToString(refundKey.PubKey().SerializeCompressed()),
		UserLockAmount:  amount,
	}

	swapResp, err := h.boltzSvc.CreateChainSwap(createReq)
	if err != nil {
		return nil, fmt.Errorf("failed to create chain swap with Boltz: %w", err)
	}

	if err := validateBtcClaimOrRefundPossible(
		swapResp.GetSwapTree(arkToBtc),
		arkToBtc,
		"",
		nil,
		nil,
		refundKey.PubKey(),
		uint32(swapResp.LockupDetails.TimeoutBlockHeight),
	); err != nil {
		return nil, fmt.Errorf("invalid BTC HTLC refund path: %w", err)
	}

	vhtlcOpts, err := validateVHTLC(context.Background(), h, arkToBtc, swapResp, preimageHashHASH160)
	if err != nil {
		return nil, fmt.Errorf("invalid VHTLC: %w", err)
	}

	log.Infof("Created BTC→ARK chain swap %s with Boltz", swapResp.Id)
	log.Infof("Please send %d sats to: %s", swapResp.LockupDetails.Amount, swapResp.LockupDetails.LockupAddress)

	if err := validateBtcLockupAddress(
		network,
		swapResp.LockupDetails.LockupAddress,
		swapResp.LockupDetails.ServerPublicKey,
		refundKey.PubKey(),
		swapResp.GetSwapTree(arkToBtc),
	); err != nil {
		return nil, fmt.Errorf("BTC lockup address validation failed: %w", err)
	}

	chainSwap, err := NewChainSwap(swapResp.Id, amount, preimage, vhtlcOpts, eventCallback)
	if err != nil {
		return nil, fmt.Errorf("failed to create chain swap: %w", err)
	}

	chainSwap.UserBtcLockupAddress = swapResp.LockupDetails.LockupAddress

	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Errorf("panic in monitorBtcToArkChainSwap: %v", r)
			}
		}()

		h.monitorBtcToArkChainSwap(
			context.Background(),
			eventCallback,
			preimage,
			refundKey,
			swapResp,
			chainSwap,
		)
	}()

	return chainSwap, nil
}

func genPreimageInfo() (preimage []byte, preimageHashSHA256, preimageHashHASH160 []byte, err error) {
	preimage = make([]byte, 32)

	if _, err = rand.Read(preimage); err != nil {
		err = fmt.Errorf("failed to generate preimage: %w", err)
		return
	}

	sha := sha256.Sum256(preimage)
	preimageHashSHA256 = sha[:]
	preimageHashHASH160 = input.Ripemd160H(preimageHashSHA256)
	return
}
