package swap

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/ArkLabsHQ/fulmine/pkg/boltz"
	"github.com/ArkLabsHQ/fulmine/pkg/vhtlc"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/lightningnetwork/lnd/input"
	log "github.com/sirupsen/logrus"
)

type ResumeChainSwapParams struct {
	SwapID             string
	From               boltz.Currency
	To                 boltz.Currency
	Amount             uint64
	PreimageHex        string
	BoltzResponseJSON  string
	UserBtcAddress     string
	UserLockTxid       string
	ServerLockTxid     string
	ClaimTxid          string
	RefundTxid         string
	Status             ChainSwapStatus
	Error              string
	Timestamp          int64
	Network            *chaincfg.Params
	EventCallback      ChainSwapEventCallback
	UnilateralRefundCB func(swapId string, opts vhtlc.Opts) error
}

func (h *SwapHandler) ResumeChainSwap(
	ctx context.Context,
	params ResumeChainSwapParams,
) (*ChainSwap, error) {
	if params.SwapID == "" {
		return nil, fmt.Errorf("swap id is required")
	}
	if params.BoltzResponseJSON == "" {
		return nil, fmt.Errorf("boltz response json is required")
	}
	if params.PreimageHex == "" {
		return nil, fmt.Errorf("preimage is required")
	}
	if params.From == "" || params.To == "" {
		return nil, fmt.Errorf("swap direction is required")
	}

	preimage, err := hex.DecodeString(params.PreimageHex)
	if err != nil {
		return nil, fmt.Errorf("decode preimage: %w", err)
	}

	var swapResp boltz.CreateChainSwapResponse
	if err := json.Unmarshal([]byte(params.BoltzResponseJSON), &swapResp); err != nil {
		return nil, fmt.Errorf("unmarshal boltz response: %w", err)
	}

	arkToBtc := params.From == boltz.CurrencyArk && params.To == boltz.CurrencyBtc
	if !arkToBtc && !(params.From == boltz.CurrencyBtc && params.To == boltz.CurrencyArk) {
		return nil, fmt.Errorf("unsupported swap direction: %s -> %s", params.From, params.To)
	}

	preimageHashSHA := sha256.Sum256(preimage)
	preimageHash160 := input.Ripemd160H(preimageHashSHA[:])

	vhtlcOpts, err := validateVHTLC(ctx, h, arkToBtc, &swapResp, preimageHash160)
	if err != nil {
		return nil, fmt.Errorf("invalid VHTLC: %w", err)
	}

	if params.Network == nil {
		return nil, fmt.Errorf("network is required")
	}

	if err := validateBtcLockupAddress(
		params.Network,
		func() string {
			if arkToBtc {
				return swapResp.ClaimDetails.LockupAddress
			}
			return swapResp.LockupDetails.LockupAddress
		}(),
		func() string {
			if arkToBtc {
				return swapResp.ClaimDetails.ServerPublicKey
			}
			return swapResp.LockupDetails.ServerPublicKey
		}(),
		h.privateKey.PubKey(),
		swapResp.GetSwapTree(arkToBtc),
	); err != nil {
		return nil, fmt.Errorf("BTC lockup address validation failed: %w", err)
	}

	swap := &ChainSwap{
		Id:                   params.SwapID,
		Amount:               params.Amount,
		Preimage:             preimage,
		VhtlcOpts:            *vhtlcOpts,
		UserBtcLockupAddress: params.UserBtcAddress,
		UserLockTxid:         params.UserLockTxid,
		ServerLockTxid:       params.ServerLockTxid,
		ClaimTxid:            params.ClaimTxid,
		RefundTxid:           params.RefundTxid,
		Timestamp:            params.Timestamp,
		Status:               params.Status,
		Error:                params.Error,
		SwapRespJson:         params.BoltzResponseJSON,
		onEvent:              params.EventCallback,
	}

	if swap.Timestamp == 0 {
		swap.Timestamp = time.Now().Unix()
	}

	log.Infof("Resuming chain swap %s (%s→%s)", swap.Id, params.From, params.To)

	if arkToBtc {
		if params.UserBtcAddress == "" {
			return nil, fmt.Errorf("btc destination address missing for Ark→BTC swap %s", params.SwapID)
		}

		go func() {
			defer func() {
				if r := recover(); r != nil {
					log.Errorf("panic in monitorAndClaimArkToBtcSwap: %v", r)
				}
			}()

			h.monitorAndClaimArkToBtcSwap(
				ctx,
				params.Network,
				params.EventCallback,
				params.UnilateralRefundCB,
				h.privateKey,
				preimage,
				params.UserBtcAddress,
				&swapResp,
				swap,
			)
		}()

		return swap, nil
	}

	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Errorf("panic in monitorBtcToArkChainSwap: %v", r)
			}
		}()

		h.monitorBtcToArkChainSwap(
			ctx,
			params.EventCallback,
			preimage,
			h.privateKey,
			&swapResp,
			swap,
		)
	}()

	return swap, nil
}
