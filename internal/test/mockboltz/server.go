package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/ArkLabsHQ/fulmine/pkg/boltz"
	"github.com/ArkLabsHQ/fulmine/pkg/vhtlc"
	arklib "github.com/arkade-os/arkd/pkg/ark-lib"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/btcsuite/btcd/btcec/v2/schnorr/musig2"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/btcutil/psbt"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	"github.com/gorilla/websocket"
	"github.com/lightningnetwork/lnd/input"
	log "github.com/sirupsen/logrus"
)

const (
	claimModeSuccess  = "success"
	claimModeFail     = "fail"
	refundModeSuccess = "success"
	refundModeFail    = "fail"
)

type Config struct {
	ListenAddr                  string
	ArkdURL                     string
	ArkHRP                      string
	Network                     *chaincfg.Params
	AutoSwapCreatedDelay        time.Duration
	ArkRefundLocktimeSeconds    int64
	BtcLockupTimeoutBlocks      uint32
	UnilateralClaimDelay        uint32
	UnilateralRefundDelay       uint32
	UnilateralRefundNoRecvDelay uint32
	ServiceFeePPM               uint64
	MinerFeeSat                 uint64
}

type runtimeConfig struct {
	ClaimMode                    string `json:"claimMode"`
	RefundMode                   string `json:"refundMode"`
	ArkRefundLocktimeSeconds     int64  `json:"arkRefundLocktimeSeconds"`
	ArkRefundAtUnix              int64  `json:"arkRefundAtUnix"`
	ArkRefundSafetyMarginSeconds int64  `json:"arkRefundSafetyMarginSeconds"`
	BtcLockupTimeoutBlocks       uint32 `json:"btcLockupTimeoutBlocks"`
	UnilateralClaimDelay         uint32 `json:"unilateralClaimDelay"`
	UnilateralRefundDelay        uint32 `json:"unilateralRefundDelay"`
	UnilateralRefundNoRecvDelay  uint32 `json:"unilateralRefundNoRecvDelay"`
	QuoteAmount                  uint64 `json:"quoteAmount"`
	QuoteOnchainAmount           uint64 `json:"quoteOnchainAmount"`
}

type updateRuntimeConfigRequest struct {
	ClaimMode                    *string `json:"claimMode"`
	RefundMode                   *string `json:"refundMode"`
	ArkRefundLocktimeSeconds     *int64  `json:"arkRefundLocktimeSeconds"`
	ArkRefundAtUnix              *int64  `json:"arkRefundAtUnix"`
	ArkRefundSafetyMarginSeconds *int64  `json:"arkRefundSafetyMarginSeconds"`
	BtcLockupTimeoutBlocks       *uint32 `json:"btcLockupTimeoutBlocks"`
	UnilateralClaimDelay         *uint32 `json:"unilateralClaimDelay"`
	UnilateralRefundDelay        *uint32 `json:"unilateralRefundDelay"`
	UnilateralRefundNoRecvDelay  *uint32 `json:"unilateralRefundNoRecvDelay"`
	QuoteAmount                  *uint64 `json:"quoteAmount"`
	QuoteOnchainAmount           *uint64 `json:"quoteOnchainAmount"`
}

type swapState struct {
	ID                 string
	From               boltz.Currency
	To                 boltz.Currency
	CreatedAt          time.Time
	PreimageHashSHA256 string
	PreimageHash160    []byte

	ClaimPubKey  *btcec.PublicKey
	RefundPubKey *btcec.PublicKey

	ClaimPubKeyHex  string
	RefundPubKeyHex string
	ServerPubKeyHex string

	UserLockAmount   uint64
	ServerLockAmount uint64

	BTCSwapTree      boltz.SwapTree
	BTCLockupScript  []byte
	BTCLockupAddress string
	BTCTimeoutHeight uint32

	ARKLockupAddress string
	ARKTimeouts      boltz.ArkTimeouts

	UserLockTxID    string
	UserLockTxHex   string
	ServerLockTxID  string
	ServerLockTxHex string

	LastStatus string

	QuoteGets      int
	QuoteAccepts   int
	LastQuote      boltz.QuoteResponse
	ClaimRequests  int
	RefundRequests int
}

type wsClient struct {
	subs map[string]struct{}
	mu   sync.Mutex
}

type Server struct {
	cfg Config

	runtimeMu sync.RWMutex
	runtime   runtimeConfig

	mu    sync.RWMutex
	swaps map[string]*swapState

	wsMu      sync.RWMutex
	wsClients map[*websocket.Conn]*wsClient
	upgrader  websocket.Upgrader

	privateKey *btcec.PrivateKey
	publicKey  *btcec.PublicKey
	arkSigner  *btcec.PublicKey

	httpServer *http.Server
	listener   net.Listener
}

func New(cfg Config) (*Server, error) {
	if cfg.ListenAddr == "" {
		cfg.ListenAddr = ":9001"
	}
	if cfg.Network == nil {
		cfg.Network = &chaincfg.RegressionNetParams
	}
	if cfg.ArkHRP == "" {
		cfg.ArkHRP = "tark"
	}
	if cfg.AutoSwapCreatedDelay <= 0 {
		cfg.AutoSwapCreatedDelay = 50 * time.Millisecond
	}
	if cfg.ArkRefundLocktimeSeconds <= 0 {
		cfg.ArkRefundLocktimeSeconds = 60
	}
	if cfg.BtcLockupTimeoutBlocks == 0 {
		cfg.BtcLockupTimeoutBlocks = 720
	}
	if cfg.UnilateralClaimDelay == 0 {
		cfg.UnilateralClaimDelay = 512
	}
	if cfg.UnilateralRefundDelay == 0 {
		cfg.UnilateralRefundDelay = 512
	}
	if cfg.UnilateralRefundNoRecvDelay == 0 {
		cfg.UnilateralRefundNoRecvDelay = 1024
	}
	if cfg.ServiceFeePPM == 0 {
		cfg.ServiceFeePPM = 20000 // 2%
	}
	if cfg.MinerFeeSat == 0 {
		cfg.MinerFeeSat = 200
	}

	priv, err := btcec.NewPrivateKey()
	if err != nil {
		return nil, fmt.Errorf("new server private key: %w", err)
	}

	arkSigner, err := fetchArkSignerPubKey(cfg.ArkdURL)
	if err != nil {
		return nil, err
	}

	s := &Server{
		cfg: cfg,
		runtime: runtimeConfig{
			ClaimMode:                    claimModeSuccess,
			RefundMode:                   refundModeSuccess,
			ArkRefundLocktimeSeconds:     cfg.ArkRefundLocktimeSeconds,
			ArkRefundAtUnix:              0,
			ArkRefundSafetyMarginSeconds: 0,
			BtcLockupTimeoutBlocks:       cfg.BtcLockupTimeoutBlocks,
			UnilateralClaimDelay:         cfg.UnilateralClaimDelay,
			UnilateralRefundDelay:        cfg.UnilateralRefundDelay,
			UnilateralRefundNoRecvDelay:  cfg.UnilateralRefundNoRecvDelay,
		},
		swaps:      make(map[string]*swapState),
		wsClients:  make(map[*websocket.Conn]*wsClient),
		upgrader:   websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }},
		privateKey: priv,
		publicKey:  priv.PubKey(),
		arkSigner:  arkSigner,
	}

	return s, nil
}

func (s *Server) Start() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/v2/ws", s.handleWS)
	mux.HandleFunc("/v2/swap/chain", s.handleChainRoot)
	mux.HandleFunc("/v2/swap/chain/", s.handleChainSubroutes)
	mux.HandleFunc("/admin/reset", s.handleAdminReset)
	mux.HandleFunc("/admin/config", s.handleAdminConfig)
	mux.HandleFunc("/admin/swaps/", s.handleAdminSwap)

	s.httpServer = &http.Server{Handler: mux}

	ln, err := net.Listen("tcp", s.cfg.ListenAddr)
	if err != nil {
		return err
	}
	s.listener = ln

	go func() {
		if err := s.httpServer.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.WithError(err).Error("mock boltz server stopped unexpectedly")
		}
	}()

	return nil
}

func (s *Server) Stop() error {
	s.wsMu.Lock()
	for conn := range s.wsClients {
		_ = conn.Close()
	}
	s.wsClients = make(map[*websocket.Conn]*wsClient)
	s.wsMu.Unlock()

	if s.httpServer == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return s.httpServer.Shutdown(ctx)
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleChainRoot(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req boltz.CreateChainSwapRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}

	resp, state, err := s.createSwap(req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	s.mu.Lock()
	s.swaps[state.ID] = state
	s.mu.Unlock()

	go func(id string, delay time.Duration) {
		time.Sleep(delay)
		s.pushSwapUpdate(id, "swap.created", "", "")
	}(state.ID, s.cfg.AutoSwapCreatedDelay)

	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleChainSubroutes(w http.ResponseWriter, r *http.Request) {
	parts := splitPath(r.URL.Path)
	if len(parts) < 5 {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	id := parts[3]
	suffix := parts[4:]

	switch {
	case len(suffix) == 1 && suffix[0] == "claim" && r.Method == http.MethodGet:
		s.handleGetClaimDetails(w, id)
	case len(suffix) == 1 && suffix[0] == "claim" && r.Method == http.MethodPost:
		s.handleSubmitClaim(w, r, id)
	case len(suffix) == 1 && suffix[0] == "quote" && r.Method == http.MethodGet:
		s.handleGetQuote(w, id)
	case len(suffix) == 1 && suffix[0] == "quote" && r.Method == http.MethodPost:
		s.handleAcceptQuote(w, r, id)
	case len(suffix) == 2 && suffix[0] == "refund" && suffix[1] == "ark" && r.Method == http.MethodPost:
		s.handleRefundARK(w, r, id)
	default:
		writeError(w, http.StatusNotFound, "not found")
	}
}

func (s *Server) handleGetClaimDetails(w http.ResponseWriter, id string) {
	st, ok := s.getSwap(id)
	if !ok {
		writeError(w, http.StatusNotFound, "swap not found")
		return
	}

	serverNonce, err := musig2.GenNonces(musig2.WithPublicKey(s.publicKey))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	txHash := strings.Repeat("00", 32)
	if st.ServerLockTxID != "" {
		// The downstream parser expects a 32-byte hex string.
		txHash = st.ServerLockTxID
		if len(txHash) != 64 {
			txHash = strings.Repeat("00", 32)
		}
	}

	resp := boltz.ChainSwapClaimDetailsResponse{
		PubNonce:        hex.EncodeToString(serverNonce.PubNonce[:]),
		PublicKey:       st.ServerPubKeyHex,
		TheirPublicKey:  st.ClaimPubKeyHex,
		TransactionHash: txHash,
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleSubmitClaim(w http.ResponseWriter, r *http.Request, id string) {
	s.mu.Lock()
	st, ok := s.swaps[id]
	if !ok {
		s.mu.Unlock()
		writeError(w, http.StatusNotFound, "swap not found")
		return
	}
	st.ClaimRequests++
	stCopy := *st
	stCopy.PreimageHash160 = append([]byte(nil), st.PreimageHash160...)
	stCopy.BTCLockupScript = append([]byte(nil), st.BTCLockupScript...)
	s.mu.Unlock()

	var req boltz.ChainSwapClaimRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}

	cfg := s.getRuntime()
	if cfg.ClaimMode == claimModeFail {
		writeError(w, http.StatusInternalServerError, "cooperative claim disabled by mock config")
		return
	}

	// Cross-signature from BTC->ARK flow (non-critical path in Fulmine).
	if req.Signature.PartialSignature != "" {
		writeJSON(w, http.StatusOK, boltz.PartialSignatureResponse{})
		return
	}

	nonceHex, partialHex, err := s.makeServerPartialSignature(&stCopy, req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, boltz.PartialSignatureResponse{
		PubNonce:         nonceHex,
		PartialSignature: partialHex,
	})
}

func (s *Server) handleGetQuote(w http.ResponseWriter, id string) {
	s.mu.Lock()
	st, ok := s.swaps[id]
	if !ok {
		s.mu.Unlock()
		writeError(w, http.StatusNotFound, "swap not found")
		return
	}
	st.QuoteGets++
	stCopy := *st
	s.mu.Unlock()

	cfg := s.getRuntime()
	quoteAmount := stCopy.ServerLockAmount
	quoteOnchainAmount := stCopy.UserLockAmount
	if cfg.QuoteAmount > 0 {
		quoteAmount = cfg.QuoteAmount
	}
	if cfg.QuoteOnchainAmount > 0 {
		quoteOnchainAmount = cfg.QuoteOnchainAmount
	}

	writeJSON(w, http.StatusOK, boltz.QuoteResponse{
		Amount:             quoteAmount,
		OnchainAmount:      quoteOnchainAmount,
		TimeoutBlockHeight: stCopy.BTCTimeoutHeight,
	})
}

func (s *Server) handleAcceptQuote(w http.ResponseWriter, r *http.Request, id string) {
	var req boltz.QuoteResponse
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}

	s.mu.Lock()
	st, ok := s.swaps[id]
	if !ok {
		s.mu.Unlock()
		writeError(w, http.StatusNotFound, "swap not found")
		return
	}
	st.QuoteAccepts++
	st.LastQuote = req
	st.ServerLockAmount = req.Amount
	st.UserLockAmount = req.OnchainAmount
	st.ServerLockTxID = ""
	st.ServerLockTxHex = ""
	s.mu.Unlock()

	writeJSON(w, http.StatusOK, req)
}

func (s *Server) handleRefundARK(w http.ResponseWriter, r *http.Request, id string) {
	s.mu.Lock()
	st, ok := s.swaps[id]
	if !ok {
		s.mu.Unlock()
		writeError(w, http.StatusNotFound, "swap not found")
		return
	}
	st.RefundRequests++
	s.mu.Unlock()

	var req boltz.RefundSwapRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}

	cfg := s.getRuntime()
	if cfg.RefundMode == refundModeFail {
		writeError(w, http.StatusServiceUnavailable, "refund endpoint disabled by mock config")
		return
	}

	signedRefund, err := s.signCollaborativeRefundPSBT(req.Transaction)
	if err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("sign refund tx: %v", err))
		return
	}
	signedCheckpoint, err := s.signCollaborativeRefundPSBT(req.Checkpoint)
	if err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("sign checkpoint tx: %v", err))
		return
	}

	writeJSON(w, http.StatusOK, boltz.RefundSwapResponse{
		Transaction: signedRefund,
		Checkpoint:  signedCheckpoint,
	})
}

func (s *Server) signCollaborativeRefundPSBT(raw string) (string, error) {
	ptx, err := psbt.NewFromRawBytes(strings.NewReader(raw), true)
	if err != nil {
		return "", fmt.Errorf("decode psbt: %w", err)
	}

	prevouts := make(map[wire.OutPoint]*wire.TxOut, len(ptx.UnsignedTx.TxIn))
	for i, in := range ptx.Inputs {
		if in.WitnessUtxo == nil {
			return "", fmt.Errorf("input %d missing witness utxo", i)
		}
		prevouts[ptx.UnsignedTx.TxIn[i].PreviousOutPoint] = in.WitnessUtxo
	}
	prevFetcher := txscript.NewMultiPrevOutFetcher(prevouts)
	sigHashes := txscript.NewTxSigHashes(ptx.UnsignedTx, prevFetcher)
	xOnly := schnorr.SerializePubKey(s.publicKey)

	for i, in := range ptx.Inputs {
		if len(in.TaprootLeafScript) == 0 {
			return "", fmt.Errorf("input %d missing taproot leaf script", i)
		}
		leaf := txscript.NewBaseTapLeaf(in.TaprootLeafScript[0].Script)
		leafHash := leaf.TapHash()
		sigHashType := txscript.SigHashDefault

		msgHash, err := txscript.CalcTapscriptSignaturehash(
			sigHashes, sigHashType, ptx.UnsignedTx, i, prevFetcher, leaf,
		)
		if err != nil {
			return "", fmt.Errorf("input %d calc sighash: %w", i, err)
		}
		sig, err := schnorr.Sign(s.privateKey, msgHash)
		if err != nil {
			return "", fmt.Errorf("input %d sign: %w", i, err)
		}

		// Keep only one signature for our key+leaf pair.
		filtered := make([]*psbt.TaprootScriptSpendSig, 0, len(in.TaprootScriptSpendSig)+1)
		for _, existing := range in.TaprootScriptSpendSig {
			if bytes.Equal(existing.XOnlyPubKey, xOnly) && bytes.Equal(existing.LeafHash, leafHash[:]) {
				continue
			}
			filtered = append(filtered, existing)
		}
		filtered = append(filtered, &psbt.TaprootScriptSpendSig{
			XOnlyPubKey: xOnly,
			LeafHash:    leafHash[:],
			Signature:   sig.Serialize(),
			SigHash:     sigHashType,
		})
		ptx.Inputs[i].TaprootScriptSpendSig = filtered
	}

	encoded, err := ptx.B64Encode()
	if err != nil {
		return "", fmt.Errorf("encode psbt: %w", err)
	}
	return encoded, nil
}

func (s *Server) handleAdminReset(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	s.mu.Lock()
	s.swaps = make(map[string]*swapState)
	s.mu.Unlock()

	s.runtimeMu.Lock()
	s.runtime = runtimeConfig{
		ClaimMode:                    claimModeSuccess,
		RefundMode:                   refundModeSuccess,
		ArkRefundLocktimeSeconds:     s.cfg.ArkRefundLocktimeSeconds,
		ArkRefundAtUnix:              0,
		ArkRefundSafetyMarginSeconds: 0,
		BtcLockupTimeoutBlocks:       s.cfg.BtcLockupTimeoutBlocks,
		UnilateralClaimDelay:         s.cfg.UnilateralClaimDelay,
		UnilateralRefundDelay:        s.cfg.UnilateralRefundDelay,
		UnilateralRefundNoRecvDelay:  s.cfg.UnilateralRefundNoRecvDelay,
	}
	s.runtimeMu.Unlock()

	writeJSON(w, http.StatusOK, map[string]string{"result": "ok"})
}

func (s *Server) handleAdminConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.getRuntime())
	case http.MethodPost:
		var req updateRuntimeConfigRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid body")
			return
		}
		if err := s.updateRuntime(req); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, s.getRuntime())
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Server) handleAdminSwap(w http.ResponseWriter, r *http.Request) {
	parts := splitPath(r.URL.Path)
	if len(parts) < 3 {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	id := parts[2]

	if len(parts) == 3 && r.Method == http.MethodGet {
		st, ok := s.getSwap(id)
		if !ok {
			writeError(w, http.StatusNotFound, "swap not found")
			return
		}
		writeJSON(w, http.StatusOK, st)
		return
	}

	if len(parts) == 4 && parts[3] == "event" && r.Method == http.MethodPost {
		var req struct {
			Status string `json:"status"`
			TxID   string `json:"txid"`
			TxHex  string `json:"txhex"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid body")
			return
		}
		if req.Status == "" {
			writeError(w, http.StatusBadRequest, "status is required")
			return
		}
		if err := s.pushSwapUpdate(id, req.Status, req.TxID, req.TxHex); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		st, _ := s.getSwap(id)
		writeJSON(w, http.StatusOK, st)
		return
	}

	writeError(w, http.StatusNotFound, "not found")
}

func (s *Server) handleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.WithError(err).Warn("websocket upgrade failed")
		return
	}

	s.wsMu.Lock()
	s.wsClients[conn] = &wsClient{subs: make(map[string]struct{})}
	s.wsMu.Unlock()

	defer func() {
		s.wsMu.Lock()
		delete(s.wsClients, conn)
		s.wsMu.Unlock()
		_ = conn.Close()
	}()

	for {
		var msg struct {
			Op      string   `json:"op"`
			Channel string   `json:"channel"`
			Args    []string `json:"args"`
		}
		if err := conn.ReadJSON(&msg); err != nil {
			return
		}

		if msg.Op == "subscribe" && msg.Channel == "swap.update" {
			s.wsMu.RLock()
			client := s.wsClients[conn]
			s.wsMu.RUnlock()
			if client == nil {
				continue
			}
			for _, id := range msg.Args {
				client.subs[id] = struct{}{}
			}

			ack := map[string]any{
				"event":   "subscribe",
				"channel": "swap.update",
				"args":    msg.Args,
			}
			client.mu.Lock()
			_ = conn.WriteJSON(ack)
			client.mu.Unlock()
		}
	}
}

func (s *Server) createSwap(req boltz.CreateChainSwapRequest) (*boltz.CreateChainSwapResponse, *swapState, error) {
	if req.From == "" || req.To == "" {
		return nil, nil, fmt.Errorf("from and to are required")
	}
	if req.From == req.To {
		return nil, nil, fmt.Errorf("from and to must differ")
	}
	if req.PreimageHash == "" {
		return nil, nil, fmt.Errorf("preimage hash is required")
	}
	if req.ClaimPublicKey == "" || req.RefundPublicKey == "" {
		return nil, nil, fmt.Errorf("claim and refund keys are required")
	}

	claimPubKeyBytes, err := hex.DecodeString(req.ClaimPublicKey)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid claim public key: %w", err)
	}
	claimPubKey, err := btcec.ParsePubKey(claimPubKeyBytes)
	if err != nil {
		return nil, nil, fmt.Errorf("parse claim public key: %w", err)
	}

	refundPubKeyBytes, err := hex.DecodeString(req.RefundPublicKey)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid refund public key: %w", err)
	}
	refundPubKey, err := btcec.ParsePubKey(refundPubKeyBytes)
	if err != nil {
		return nil, nil, fmt.Errorf("parse refund public key: %w", err)
	}

	preimageHashBytes, err := hex.DecodeString(req.PreimageHash)
	if err != nil {
		return nil, nil, fmt.Errorf("decode preimage hash: %w", err)
	}
	preimageHash160 := normalizePreimageHash(preimageHashBytes)
	if len(preimageHash160) != 20 {
		return nil, nil, fmt.Errorf("expected 20-byte preimage hash, got %d", len(preimageHash160))
	}

	lockAmount := req.UserLockAmount
	if lockAmount == 0 {
		lockAmount = req.ServerLockAmount
	}
	if lockAmount == 0 {
		return nil, nil, fmt.Errorf("lock amount is required")
	}

	fee := uint64(math.Ceil(float64(lockAmount) * float64(s.cfg.ServiceFeePPM) / 1_000_000))
	if fee > lockAmount {
		fee = 0
	}
	serverAmount := lockAmount - fee
	if serverAmount > s.cfg.MinerFeeSat {
		serverAmount -= s.cfg.MinerFeeSat
	}

	id := fmt.Sprintf("mock_%d", time.Now().UnixNano())
	rt := s.getRuntime()

	isArkToBTC := req.From == boltz.CurrencyArk && req.To == boltz.CurrencyBtc
	isBTCToArk := req.From == boltz.CurrencyBtc && req.To == boltz.CurrencyArk
	if !isArkToBTC && !isBTCToArk {
		return nil, nil, fmt.Errorf("unsupported direction %s -> %s", req.From, req.To)
	}

	claimScriptKey := xOnly(claimPubKey)
	refundScriptKey := xOnly(s.publicKey)
	clientKeyForLockup := claimPubKey
	if isBTCToArk {
		claimScriptKey = xOnly(s.publicKey)
		refundScriptKey = xOnly(refundPubKey)
		clientKeyForLockup = refundPubKey
	}

	swapTree, err := buildSwapTree(preimageHash160, claimScriptKey, refundScriptKey, rt.BtcLockupTimeoutBlocks)
	if err != nil {
		return nil, nil, err
	}

	btcAddress, btcScript, err := s.buildBTCLockup(clientKeyForLockup, swapTree)
	if err != nil {
		return nil, nil, err
	}

	// Keep advertised timeout and script locktime identical, otherwise Fulmine
	// validation (address re-derivation from timeouts) fails.
	// Optional safety margin moves both values backwards for regtest determinism.
	refundAt := time.Now().Unix() + rt.ArkRefundLocktimeSeconds - rt.ArkRefundSafetyMarginSeconds
	if rt.ArkRefundAtUnix > 0 {
		refundAt = rt.ArkRefundAtUnix
	}
	arkTimeouts := boltz.ArkTimeouts{
		Refund:                          int(refundAt),
		UnilateralClaim:                 int(rt.UnilateralClaimDelay),
		UnilateralRefund:                int(rt.UnilateralRefundDelay),
		UnilateralRefundWithoutReceiver: int(rt.UnilateralRefundNoRecvDelay),
	}

	arkAddress, err := s.buildARKLockup(isArkToBTC, claimPubKey, refundPubKey, preimageHash160, refundAt, rt)
	if err != nil {
		return nil, nil, err
	}

	state := &swapState{
		ID:                 id,
		From:               req.From,
		To:                 req.To,
		CreatedAt:          time.Now(),
		PreimageHashSHA256: req.PreimageHash,
		PreimageHash160:    append([]byte(nil), preimageHash160...),
		ClaimPubKey:        claimPubKey,
		RefundPubKey:       refundPubKey,
		ClaimPubKeyHex:     req.ClaimPublicKey,
		RefundPubKeyHex:    req.RefundPublicKey,
		ServerPubKeyHex:    hex.EncodeToString(s.publicKey.SerializeCompressed()),
		UserLockAmount:     lockAmount,
		ServerLockAmount:   serverAmount,
		BTCSwapTree:        swapTree,
		BTCLockupScript:    btcScript,
		BTCLockupAddress:   btcAddress,
		BTCTimeoutHeight:   rt.BtcLockupTimeoutBlocks,
		ARKLockupAddress:   arkAddress,
		ARKTimeouts:        arkTimeouts,
		LastStatus:         "swap.created",
	}

	resp := &boltz.CreateChainSwapResponse{Id: id}
	if isArkToBTC {
		resp.ClaimDetails = boltz.SwapLeg{
			ServerPublicKey:    state.ServerPubKeyHex,
			Amount:             int(serverAmount),
			LockupAddress:      btcAddress,
			TimeoutBlockHeight: int(rt.BtcLockupTimeoutBlocks),
			SwapTree:           &state.BTCSwapTree,
		}
		resp.LockupDetails = boltz.SwapLeg{
			ServerPublicKey:    state.ServerPubKeyHex,
			Amount:             int(lockAmount),
			LockupAddress:      arkAddress,
			TimeoutBlockHeight: int(refundAt),
			Timeouts:           &arkTimeouts,
		}
	} else {
		resp.LockupDetails = boltz.SwapLeg{
			ServerPublicKey:    state.ServerPubKeyHex,
			Amount:             int(lockAmount),
			LockupAddress:      btcAddress,
			TimeoutBlockHeight: int(rt.BtcLockupTimeoutBlocks),
			SwapTree:           &state.BTCSwapTree,
		}
		resp.ClaimDetails = boltz.SwapLeg{
			ServerPublicKey:    state.ServerPubKeyHex,
			Amount:             int(serverAmount),
			LockupAddress:      arkAddress,
			TimeoutBlockHeight: int(refundAt),
			Timeouts:           &arkTimeouts,
		}
	}

	return resp, state, nil
}

func (s *Server) buildARKLockup(
	isArkToBTC bool,
	claimPubKey, refundPubKey *btcec.PublicKey,
	preimageHash160 []byte,
	refundAt int64,
	rt runtimeConfig,
) (string, error) {
	var receiver, sender *btcec.PublicKey
	if isArkToBTC {
		receiver = s.publicKey
		sender = refundPubKey
	} else {
		receiver = claimPubKey
		sender = s.publicKey
	}

	opts := vhtlc.Opts{
		Sender:                               sender,
		Receiver:                             receiver,
		Server:                               s.arkSigner,
		PreimageHash:                         preimageHash160,
		RefundLocktime:                       arklib.AbsoluteLocktime(refundAt),
		UnilateralClaimDelay:                 parseRelativeLocktime(rt.UnilateralClaimDelay),
		UnilateralRefundDelay:                parseRelativeLocktime(rt.UnilateralRefundDelay),
		UnilateralRefundWithoutReceiverDelay: parseRelativeLocktime(rt.UnilateralRefundNoRecvDelay),
	}

	script, err := vhtlc.NewVHTLCScriptFromOpts(opts)
	if err != nil {
		return "", fmt.Errorf("create vhtlc script: %w", err)
	}

	addr, err := script.Address(s.cfg.ArkHRP)
	if err != nil {
		return "", fmt.Errorf("encode vhtlc address: %w", err)
	}
	return addr, nil
}

func (s *Server) buildBTCLockup(clientPubKey *btcec.PublicKey, swapTree boltz.SwapTree) (string, []byte, error) {
	merkleRoot, err := swapTreeMerkleRoot(swapTree)
	if err != nil {
		return "", nil, err
	}

	agg, _, _, err := musig2.AggregateKeys([]*btcec.PublicKey{s.publicKey, clientPubKey}, false)
	if err != nil {
		return "", nil, fmt.Errorf("aggregate keys: %w", err)
	}

	tweaked := txscript.ComputeTaprootOutputKey(agg.FinalKey, merkleRoot)
	pkScript, err := txscript.PayToTaprootScript(tweaked)
	if err != nil {
		return "", nil, fmt.Errorf("build p2tr script: %w", err)
	}

	addr, err := btcutil.NewAddressTaproot(schnorr.SerializePubKey(tweaked), s.cfg.Network)
	if err != nil {
		return "", nil, fmt.Errorf("encode p2tr address: %w", err)
	}

	return addr.EncodeAddress(), pkScript, nil
}

func (s *Server) makeServerPartialSignature(st *swapState, req boltz.ChainSwapClaimRequest) (string, string, error) {
	if req.ToSign.ClaimTx == "" {
		return "", "", fmt.Errorf("missing toSign.transaction")
	}
	if req.ToSign.Nonce == "" {
		return "", "", fmt.Errorf("missing toSign.nonce")
	}

	claimTx, err := deserializeTx(req.ToSign.ClaimTx)
	if err != nil {
		return "", "", fmt.Errorf("invalid claim tx: %w", err)
	}
	if req.ToSign.Index < 0 || req.ToSign.Index >= len(claimTx.TxIn) {
		return "", "", fmt.Errorf("invalid toSign.index")
	}

	clientNonce, err := parsePubNonce(req.ToSign.Nonce)
	if err != nil {
		return "", "", fmt.Errorf("invalid client nonce: %w", err)
	}

	serverNonces, err := musig2.GenNonces(musig2.WithPublicKey(s.publicKey))
	if err != nil {
		return "", "", fmt.Errorf("generate server nonce: %w", err)
	}

	combinedNonce, err := musig2.AggregateNonces([][66]byte{clientNonce, serverNonces.PubNonce})
	if err != nil {
		return "", "", fmt.Errorf("aggregate nonces: %w", err)
	}

	prevOut := &wire.TxOut{Value: int64(st.ServerLockAmount), PkScript: st.BTCLockupScript}
	prevOutPoint := claimTx.TxIn[req.ToSign.Index].PreviousOutPoint
	prevFetcher := txscript.NewMultiPrevOutFetcher(map[wire.OutPoint]*wire.TxOut{prevOutPoint: prevOut})
	sigHashes := txscript.NewTxSigHashes(claimTx, prevFetcher)
	msg, err := txscript.CalcTaprootSignatureHash(sigHashes, txscript.SigHashDefault, claimTx, req.ToSign.Index, prevFetcher)
	if err != nil {
		return "", "", fmt.Errorf("taproot message: %w", err)
	}

	merkleRoot, err := swapTreeMerkleRoot(st.BTCSwapTree)
	if err != nil {
		return "", "", err
	}

	var msg32 [32]byte
	copy(msg32[:], msg)

	partial, err := musig2.Sign(
		serverNonces.SecNonce,
		s.privateKey,
		combinedNonce,
		[]*btcec.PublicKey{s.publicKey, st.ClaimPubKey},
		msg32,
		musig2.WithTaprootSignTweak(merkleRoot),
		musig2.WithFastSign(),
	)
	if err != nil {
		return "", "", fmt.Errorf("musig sign: %w", err)
	}

	var scalar [32]byte
	partial.S.PutBytesUnchecked(scalar[:])

	return hex.EncodeToString(serverNonces.PubNonce[:]), hex.EncodeToString(scalar[:]), nil
}

func (s *Server) pushSwapUpdate(id, status, txID, txHex string) error {
	s.mu.Lock()
	st, ok := s.swaps[id]
	if !ok {
		s.mu.Unlock()
		return fmt.Errorf("swap %s not found", id)
	}

	if strings.HasPrefix(status, "transaction.mempool.") {
		if st.ServerLockTxHex == "" || st.ServerLockTxID == "" {
			if err := s.ensureServerLockTx(st); err != nil {
				s.mu.Unlock()
				return err
			}
		}
		if txID == "" {
			txID = st.ServerLockTxID
		}
		if txHex == "" {
			txHex = st.ServerLockTxHex
		}
	}

	if strings.HasPrefix(status, "transaction.") && !strings.HasPrefix(status, "transaction.mempool.") {
		if txID == "" {
			txID = randomTxID()
		}
		if txHex == "" {
			txHex = randomTxHex()
		}
		if status == "transaction.mempool" || status == "transaction.confirmed" {
			st.UserLockTxID = txID
			st.UserLockTxHex = txHex
		}
	}

	st.LastStatus = status
	s.mu.Unlock()

	payload := map[string]any{
		"event":   "update",
		"channel": "swap.update",
		"args": []map[string]any{
			{
				"id":     id,
				"status": status,
			},
		},
	}
	if txID != "" {
		payload["args"].([]map[string]any)[0]["transaction"] = map[string]string{
			"id":  txID,
			"hex": txHex,
		}
	}

	s.wsMu.RLock()
	defer s.wsMu.RUnlock()

	for conn, client := range s.wsClients {
		if _, ok := client.subs[id]; !ok {
			continue
		}
		client.mu.Lock()
		err := conn.WriteJSON(payload)
		client.mu.Unlock()
		if err != nil {
			log.WithError(err).Warn("failed to push ws event")
		}
	}

	return nil
}

func (s *Server) ensureServerLockTx(st *swapState) error {
	if st.ServerLockTxHex != "" && st.ServerLockTxID != "" {
		return nil
	}

	tx := wire.NewMsgTx(2)
	dummyHash := chainhash.DoubleHashH([]byte("mock-server-lock-" + st.ID))
	tx.AddTxIn(&wire.TxIn{
		PreviousOutPoint: wire.OutPoint{Hash: dummyHash, Index: 0},
		Sequence:         wire.MaxTxInSequenceNum,
	})
	tx.AddTxOut(&wire.TxOut{Value: int64(st.ServerLockAmount), PkScript: st.BTCLockupScript})

	var buf bytes.Buffer
	if err := tx.Serialize(&buf); err != nil {
		return fmt.Errorf("serialize server lock tx: %w", err)
	}

	st.ServerLockTxHex = hex.EncodeToString(buf.Bytes())
	st.ServerLockTxID = tx.TxHash().String()
	return nil
}

func (s *Server) getSwap(id string) (*swapState, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	st, ok := s.swaps[id]
	if !ok {
		return nil, false
	}
	cp := *st
	cp.PreimageHash160 = append([]byte(nil), st.PreimageHash160...)
	cp.BTCLockupScript = append([]byte(nil), st.BTCLockupScript...)
	return &cp, true
}

func (s *Server) getRuntime() runtimeConfig {
	s.runtimeMu.RLock()
	defer s.runtimeMu.RUnlock()
	return s.runtime
}

func (s *Server) updateRuntime(req updateRuntimeConfigRequest) error {
	s.runtimeMu.Lock()
	defer s.runtimeMu.Unlock()

	if req.ClaimMode != nil {
		mode := strings.ToLower(strings.TrimSpace(*req.ClaimMode))
		if mode != claimModeSuccess && mode != claimModeFail {
			return fmt.Errorf("unsupported claimMode: %s", mode)
		}
		s.runtime.ClaimMode = mode
	}
	if req.RefundMode != nil {
		mode := strings.ToLower(strings.TrimSpace(*req.RefundMode))
		if mode != refundModeSuccess && mode != refundModeFail {
			return fmt.Errorf("unsupported refundMode: %s", mode)
		}
		s.runtime.RefundMode = mode
	}
	if req.ArkRefundLocktimeSeconds != nil {
		if *req.ArkRefundLocktimeSeconds <= 0 {
			return fmt.Errorf("arkRefundLocktimeSeconds must be > 0")
		}
		s.runtime.ArkRefundLocktimeSeconds = *req.ArkRefundLocktimeSeconds
	}
	if req.ArkRefundAtUnix != nil {
		if *req.ArkRefundAtUnix < 0 {
			return fmt.Errorf("arkRefundAtUnix must be >= 0")
		}
		s.runtime.ArkRefundAtUnix = *req.ArkRefundAtUnix
	}
	if req.ArkRefundSafetyMarginSeconds != nil {
		if *req.ArkRefundSafetyMarginSeconds < 0 {
			return fmt.Errorf("arkRefundSafetyMarginSeconds must be >= 0")
		}
		s.runtime.ArkRefundSafetyMarginSeconds = *req.ArkRefundSafetyMarginSeconds
	}
	if req.BtcLockupTimeoutBlocks != nil {
		if *req.BtcLockupTimeoutBlocks < 144 {
			return fmt.Errorf("btcLockupTimeoutBlocks must be >= 144")
		}
		s.runtime.BtcLockupTimeoutBlocks = *req.BtcLockupTimeoutBlocks
	}
	if req.UnilateralClaimDelay != nil {
		s.runtime.UnilateralClaimDelay = *req.UnilateralClaimDelay
	}
	if req.UnilateralRefundDelay != nil {
		s.runtime.UnilateralRefundDelay = *req.UnilateralRefundDelay
	}
	if req.UnilateralRefundNoRecvDelay != nil {
		s.runtime.UnilateralRefundNoRecvDelay = *req.UnilateralRefundNoRecvDelay
	}
	if req.QuoteAmount != nil {
		s.runtime.QuoteAmount = *req.QuoteAmount
	}
	if req.QuoteOnchainAmount != nil {
		s.runtime.QuoteOnchainAmount = *req.QuoteOnchainAmount
	}

	return nil
}

func splitPath(path string) []string {
	trimmed := strings.Trim(strings.TrimSpace(path), "/")
	if trimmed == "" {
		return nil
	}
	return strings.Split(trimmed, "/")
}

func buildSwapTree(preimageHash160, claimKeyXOnly, refundKeyXOnly []byte, timeout uint32) (boltz.SwapTree, error) {
	claimScript, err := txscript.NewScriptBuilder().
		AddOp(txscript.OP_SIZE).
		AddData([]byte{0x20}).
		AddOp(txscript.OP_EQUALVERIFY).
		AddOp(txscript.OP_HASH160).
		AddData(preimageHash160).
		AddOp(txscript.OP_EQUALVERIFY).
		AddData(claimKeyXOnly).
		AddOp(txscript.OP_CHECKSIG).
		Script()
	if err != nil {
		return boltz.SwapTree{}, fmt.Errorf("build claim script: %w", err)
	}

	refundScript, err := txscript.NewScriptBuilder().
		AddData(refundKeyXOnly).
		AddOp(txscript.OP_CHECKSIGVERIFY).
		AddInt64(int64(timeout)).
		AddOp(txscript.OP_CHECKLOCKTIMEVERIFY).
		Script()
	if err != nil {
		return boltz.SwapTree{}, fmt.Errorf("build refund script: %w", err)
	}

	return boltz.SwapTree{
		ClaimLeaf:  boltz.SwapTreeLeaf{Version: uint8(txscript.BaseLeafVersion), Output: hex.EncodeToString(claimScript)},
		RefundLeaf: boltz.SwapTreeLeaf{Version: uint8(txscript.BaseLeafVersion), Output: hex.EncodeToString(refundScript)},
	}, nil
}

func swapTreeMerkleRoot(tree boltz.SwapTree) ([]byte, error) {
	claimScript, err := hex.DecodeString(tree.ClaimLeaf.Output)
	if err != nil {
		return nil, fmt.Errorf("decode claim leaf: %w", err)
	}
	refundScript, err := hex.DecodeString(tree.RefundLeaf.Output)
	if err != nil {
		return nil, fmt.Errorf("decode refund leaf: %w", err)
	}

	claimLeaf := txscript.NewBaseTapLeaf(claimScript)
	refundLeaf := txscript.NewBaseTapLeaf(refundScript)
	treeBuilder := txscript.AssembleTaprootScriptTree(claimLeaf, refundLeaf)
	if treeBuilder == nil || treeBuilder.RootNode == nil {
		return nil, fmt.Errorf("assemble taproot tree")
	}
	h := treeBuilder.RootNode.TapHash()
	return h[:], nil
}

func normalizePreimageHash(b []byte) []byte {
	switch len(b) {
	case 20:
		return b
	case 32:
		return input.Ripemd160H(b)
	default:
		return nil
	}
}

func xOnly(pub *btcec.PublicKey) []byte {
	return schnorr.SerializePubKey(pub)
}

func parseRelativeLocktime(locktime uint32) arklib.RelativeLocktime {
	if locktime >= 512 {
		return arklib.RelativeLocktime{Type: arklib.LocktimeTypeSecond, Value: locktime}
	}
	return arklib.RelativeLocktime{Type: arklib.LocktimeTypeBlock, Value: locktime}
}

func parsePubNonce(nonceHex string) ([66]byte, error) {
	var out [66]byte
	b, err := hex.DecodeString(nonceHex)
	if err != nil {
		return out, err
	}
	if len(b) != 66 {
		return out, fmt.Errorf("expected 66 bytes, got %d", len(b))
	}
	copy(out[:], b)
	return out, nil
}

func deserializeTx(txHex string) (*wire.MsgTx, error) {
	raw, err := hex.DecodeString(txHex)
	if err != nil {
		return nil, err
	}
	tx := wire.NewMsgTx(2)
	if err := tx.Deserialize(bytes.NewReader(raw)); err != nil {
		return nil, err
	}
	return tx, nil
}

func randomTxID() string {
	r := make([]byte, 32)
	_, _ = rand.Read(r)
	sum := sha256.Sum256(r)
	return hex.EncodeToString(sum[:])
}

func randomTxHex() string {
	r := make([]byte, 100)
	_, _ = rand.Read(r)
	return hex.EncodeToString(r)
}

func fetchArkSignerPubKey(arkdURL string) (*btcec.PublicKey, error) {
	if strings.TrimSpace(arkdURL) == "" {
		return nil, fmt.Errorf("MOCK_BOLTZ_ARKD_URL is required")
	}
	url := strings.TrimRight(strings.TrimSpace(arkdURL), "/") + "/v1/info"
	var lastErr error
	for i := 0; i < 30; i++ {
		resp, err := http.Get(url)
		if err != nil {
			lastErr = err
			time.Sleep(500 * time.Millisecond)
			continue
		}
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			lastErr = fmt.Errorf("http %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
			time.Sleep(500 * time.Millisecond)
			continue
		}
		var payload struct {
			SignerPubkey string `json:"signerPubkey"`
		}
		if err := json.Unmarshal(body, &payload); err != nil {
			lastErr = err
			time.Sleep(500 * time.Millisecond)
			continue
		}
		if payload.SignerPubkey == "" {
			lastErr = fmt.Errorf("signerPubkey missing in arkd /v1/info")
			time.Sleep(500 * time.Millisecond)
			continue
		}
		pkb, err := hex.DecodeString(payload.SignerPubkey)
		if err != nil {
			return nil, err
		}
		pk, err := btcec.ParsePubKey(pkb)
		if err != nil {
			return nil, err
		}
		return pk, nil
	}
	return nil, fmt.Errorf("failed to fetch ark signer public key from %s: %w", url, lastErr)
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func (s *swapState) MarshalJSON() ([]byte, error) {
	type alias struct {
		ID               string              `json:"id"`
		From             string              `json:"from"`
		To               string              `json:"to"`
		CreatedAt        string              `json:"createdAt"`
		LastStatus       string              `json:"lastStatus"`
		UserLockAmount   uint64              `json:"userLockAmount"`
		ServerLockAmount uint64              `json:"serverLockAmount"`
		BTCLockupAddress string              `json:"btcLockupAddress"`
		ARKLockupAddress string              `json:"arkLockupAddress"`
		QuoteGets        int                 `json:"quoteGets"`
		QuoteAccepts     int                 `json:"quoteAccepts"`
		ClaimRequests    int                 `json:"claimRequests"`
		RefundRequests   int                 `json:"refundRequests"`
		LastQuote        boltz.QuoteResponse `json:"lastQuote"`
		UserLockTxID     string              `json:"userLockTxid"`
		ServerLockTxID   string              `json:"serverLockTxid"`
	}
	return json.Marshal(alias{
		ID:               s.ID,
		From:             string(s.From),
		To:               string(s.To),
		CreatedAt:        s.CreatedAt.Format(time.RFC3339),
		LastStatus:       s.LastStatus,
		UserLockAmount:   s.UserLockAmount,
		ServerLockAmount: s.ServerLockAmount,
		BTCLockupAddress: s.BTCLockupAddress,
		ARKLockupAddress: s.ARKLockupAddress,
		QuoteGets:        s.QuoteGets,
		QuoteAccepts:     s.QuoteAccepts,
		ClaimRequests:    s.ClaimRequests,
		RefundRequests:   s.RefundRequests,
		LastQuote:        s.LastQuote,
		UserLockTxID:     s.UserLockTxID,
		ServerLockTxID:   s.ServerLockTxID,
	})
}

func (s *Server) String() string {
	if s.listener == nil {
		return ""
	}
	addr := s.listener.Addr().String()
	if strings.HasPrefix(addr, "[") {
		return "http://" + addr
	}
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return "http://" + addr
	}
	if host == "" || host == "::" || host == "0.0.0.0" {
		host = "127.0.0.1"
	}
	return "http://" + net.JoinHostPort(host, port)
}

func main() {
	log.SetFormatter(&log.TextFormatter{FullTimestamp: true})
	level, err := log.ParseLevel(envOrDefault("MOCK_BOLTZ_LOG_LEVEL", "info"))
	if err != nil {
		level = log.InfoLevel
	}
	log.SetLevel(level)

	cfg := Config{
		ListenAddr:                  envOrDefault("MOCK_BOLTZ_LISTEN_ADDR", ":9001"),
		ArkdURL:                     envOrDefault("MOCK_BOLTZ_ARKD_URL", "http://arkd:7070"),
		ArkHRP:                      envOrDefault("MOCK_BOLTZ_ARK_HRP", "tark"),
		Network:                     parseNetwork(envOrDefault("MOCK_BOLTZ_NETWORK", "regtest")),
		AutoSwapCreatedDelay:        parseDuration("MOCK_BOLTZ_AUTO_SWAP_CREATED_DELAY", 50*time.Millisecond),
		ArkRefundLocktimeSeconds:    parseInt64("MOCK_BOLTZ_ARK_REFUND_LOCKTIME_SECONDS", 60),
		BtcLockupTimeoutBlocks:      parseUint32("MOCK_BOLTZ_BTC_LOCKUP_TIMEOUT_BLOCKS", 720),
		UnilateralClaimDelay:        parseUint32("MOCK_BOLTZ_UNILATERAL_CLAIM_DELAY", 512),
		UnilateralRefundDelay:       parseUint32("MOCK_BOLTZ_UNILATERAL_REFUND_DELAY", 512),
		UnilateralRefundNoRecvDelay: parseUint32("MOCK_BOLTZ_UNILATERAL_REFUND_NO_RECV_DELAY", 1024),
		ServiceFeePPM:               parseUint64("MOCK_BOLTZ_SERVICE_FEE_PPM", 20000),
		MinerFeeSat:                 parseUint64("MOCK_BOLTZ_MINER_FEE_SAT", 200),
	}

	srv, err := New(cfg)
	if err != nil {
		log.WithError(err).Fatal("failed to create mock boltz server")
	}

	if err := srv.Start(); err != nil {
		log.WithError(err).Fatal("failed to start mock boltz server")
	}
	log.Infof("mock boltz started on %s", cfg.ListenAddr)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	if err := srv.Stop(); err != nil {
		log.WithError(err).Error("failed to stop mock boltz server")
	}
}

func envOrDefault(key, fallback string) string {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	return v
}

func parseDuration(key string, fallback time.Duration) time.Duration {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return fallback
	}
	return d
}

func parseUint64(key string, fallback uint64) uint64 {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	n, err := strconv.ParseUint(v, 10, 64)
	if err != nil {
		return fallback
	}
	return n
}

func parseUint32(key string, fallback uint32) uint32 {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	n, err := strconv.ParseUint(v, 10, 32)
	if err != nil {
		return fallback
	}
	return uint32(n)
}

func parseInt64(key string, fallback int64) int64 {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return fallback
	}
	return n
}

func parseNetwork(network string) *chaincfg.Params {
	switch strings.ToLower(strings.TrimSpace(network)) {
	case "mainnet", "bitcoin":
		return &chaincfg.MainNetParams
	case "testnet":
		return &chaincfg.TestNet3Params
	case "signet":
		return &chaincfg.SigNetParams
	default:
		return &chaincfg.RegressionNetParams
	}
}
