package swap

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"testing"

	"github.com/ArkLabsHQ/fulmine/pkg/boltz"
	arklib "github.com/arkade-os/arkd/pkg/ark-lib"
	clientTypes "github.com/arkade-os/arkd/pkg/client-lib/types"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/btcutil/psbt"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	"github.com/stretchr/testify/require"
)

// fakeBoltz is a fake BoltzClient for failure-path tests. It embeds the
// interface (so any unexpected method call panics) and only overrides
// SubmitChainSwapClaim — the single Boltz call on the cooperative claim path.
type fakeBoltz struct {
	BoltzClient
	submitErr error
}

func (f *fakeBoltz) SubmitChainSwapClaim(string, boltz.ChainSwapClaimRequest) (*boltz.PartialSignatureResponse, error) {
	return nil, f.submitErr
}

// claimMockExplorer is a minimal ExplorerClient: it lets the claim tx build and
// "broadcast", returning a known txid so the test can assert which path landed.
type claimMockExplorer struct {
	broadcastTxid string
	broadcastErr  error
	broadcastTx   *wire.MsgTx // captured so tests can assert the claim tx's witness
}

func (m *claimMockExplorer) GetFeeRate() (float64, error)           { return 1, nil }
func (m *claimMockExplorer) GetCurrentBlockHeight() (uint32, error) { return 100, nil }
func (m *claimMockExplorer) GetTransaction(string) (string, error)  { return "", nil }
func (m *claimMockExplorer) GetTransactionStatus(string) (*TransactionStatus, error) {
	return nil, nil
}
func (m *claimMockExplorer) BroadcastTransaction(tx *wire.MsgTx) (string, error) {
	m.broadcastTx = tx
	return m.broadcastTxid, m.broadcastErr
}

// buildSwapTreeFixture synthesizes a Boltz swap tree plus a server lockup tx
// paying to its taproot lockup script. It derives the lockup script via the
// production computeSwapTreeMerkleRoot/computeExpectedLockupScript, so the
// output is exactly what prepareClaimTransaction expects — no live regtest swap
// or hand-rolled taproot crypto needed. The leaf scripts are valid-but-simple
// (validateSwapTree only checks hex + version 0xc0; the claim/refund flows build
// and broadcast through a mocked explorer, so consensus validity isn't required).
func buildSwapTreeFixture(t *testing.T) (serverKey, claimKey *btcec.PrivateKey, tree boltz.SwapTree, serverLockupHex, btcAddress string) {
	t.Helper()

	var err error
	serverKey, err = btcec.NewPrivateKey()
	require.NoError(t, err)
	claimKey, err = btcec.NewPrivateKey()
	require.NoError(t, err)

	leaf := func(k *btcec.PublicKey) string {
		s, err := txscript.NewScriptBuilder().
			AddData(schnorr.SerializePubKey(k)).
			AddOp(txscript.OP_CHECKSIG).
			Script()
		require.NoError(t, err)
		return hex.EncodeToString(s)
	}
	tree = boltz.SwapTree{
		ClaimLeaf:  boltz.SwapTreeLeaf{Version: 0xc0, Output: leaf(claimKey.PubKey())},
		RefundLeaf: boltz.SwapTreeLeaf{Version: 0xc0, Output: leaf(serverKey.PubKey())},
	}

	merkleRoot, err := computeSwapTreeMerkleRoot(tree)
	require.NoError(t, err)
	lockupScript, err := computeExpectedLockupScript(serverKey.PubKey(), claimKey.PubKey(), merkleRoot)
	require.NoError(t, err)

	lockupTx := wire.NewMsgTx(2)
	lockupTx.AddTxIn(wire.NewTxIn(wire.NewOutPoint(&chainhash.Hash{}, 0), nil, nil))
	lockupTx.AddTxOut(wire.NewTxOut(100_000, lockupScript))
	var buf bytes.Buffer
	require.NoError(t, lockupTx.Serialize(&buf))
	serverLockupHex = hex.EncodeToString(buf.Bytes())

	destKey, err := btcec.NewPrivateKey()
	require.NoError(t, err)
	addr, err := btcutil.NewAddressTaproot(schnorr.SerializePubKey(destKey.PubKey()), &chaincfg.RegressionNetParams)
	require.NoError(t, err)
	btcAddress = addr.String()

	return serverKey, claimKey, tree, serverLockupHex, btcAddress
}

// TestClaimBtcLockupFallsBackToScriptPath pins the disaster-recovery transition
// @sekulicd called out: when the cooperative MuSig2 claim fails (Boltz refuses
// to co-sign), claimBtcLockup must fall back to the non-cooperative script-path
// claim and still land the funds. Without that fallback a user can't recover BTC
// from an uncooperative Boltz. The cooperative live e2e never exercises this; a
// fake counterparty whose SubmitChainSwapClaim errors does.
func TestClaimBtcLockupFallsBackToScriptPath(t *testing.T) {
	serverKey, claimKey, tree, serverLockupHex, btcAddress := buildSwapTreeFixture(t)
	preimage := bytes.Repeat([]byte{0x01}, 32)

	exp := &claimMockExplorer{broadcastTxid: "script-path-claim-txid"}
	h := &arkToBtcHandler{swapHandler: &SwapHandler{
		boltzSvc:       &fakeBoltz{submitErr: errors.New("boltz refused the cooperative claim")},
		explorerClient: exp,
		config:         clientTypes.Config{Network: arklib.BitcoinRegTest, Dust: 546},
	}}

	txid, err := h.claimBtcLockup(
		context.Background(), "swap-1", preimage, claimKey, btcAddress,
		&chaincfg.RegressionNetParams, tree, serverKey.PubKey(), serverLockupHex,
	)

	require.NoError(t, err)
	// the script-path broadcast txid — proves the fallback ran, not the cooperative path.
	require.Equal(t, "script-path-claim-txid", txid)

	// Pin the script-path witness shape: [signature, preimage, claimScript,
	// controlBlock]. Without this the test stays green even if the preimage were
	// dropped — a claim with no preimage is unspendable, but invisible to a mock
	// that discards the tx.
	require.NotNil(t, exp.broadcastTx)
	require.Len(t, exp.broadcastTx.TxIn[0].Witness, 4)
	require.Equal(t, preimage, exp.broadcastTx.TxIn[0].Witness[1], "preimage must be the 2nd witness element")
}

// TestClaimBtcLockupReturnsErrorWhenBothPathsFail pins the actual disaster case:
// when the cooperative claim fails AND the script-path broadcast also fails, the
// user cannot recover the BTC at all, so claimBtcLockup must surface that error
// rather than swallow it. The script-path broadcast failure is what's returned.
func TestClaimBtcLockupReturnsErrorWhenBothPathsFail(t *testing.T) {
	serverKey, claimKey, tree, serverLockupHex, btcAddress := buildSwapTreeFixture(t)
	preimage := bytes.Repeat([]byte{0x01}, 32)
	broadcastErr := errors.New("mempool rejected the script-path claim")

	h := &arkToBtcHandler{swapHandler: &SwapHandler{
		boltzSvc:       &fakeBoltz{submitErr: errors.New("boltz refused the cooperative claim")},
		explorerClient: &claimMockExplorer{broadcastErr: broadcastErr},
		config:         clientTypes.Config{Network: arklib.BitcoinRegTest, Dust: 546},
	}}

	_, err := h.claimBtcLockup(
		context.Background(), "swap-1", preimage, claimKey, btcAddress,
		&chaincfg.RegressionNetParams, tree, serverKey.PubKey(), serverLockupHex,
	)

	require.ErrorIs(t, err, broadcastErr)
}

// makeTestPSBT builds a minimal valid base64 PSBT (one input, one output, no
// signatures) — enough for collaborativeRefund to decode a "counterparty-signed"
// response without any real signing.
func makeTestPSBT(t *testing.T) string {
	t.Helper()
	tx := wire.NewMsgTx(2)
	tx.AddTxIn(wire.NewTxIn(wire.NewOutPoint(&chainhash.Hash{}, 0), nil, nil))
	tx.AddTxOut(wire.NewTxOut(1000, []byte{txscript.OP_TRUE}))
	p, err := psbt.NewFromUnsignedTx(tx)
	require.NoError(t, err)
	b64, err := p.B64Encode()
	require.NoError(t, err)
	return b64
}

// TestCollaborativeRefundPropagatesCounterpartyFailure pins @sekulicd's
// submarine/reverse failure path: when the counterparty's collaborative-refund
// call (RefundSubmarine / RefundChainSwap) errors, collaborativeRefund must
// surface that error rather than swallow it — otherwise the refund silently
// stalls. A cooperative live Boltz never returns this, so only a fake
// counterparty exercises it.
func TestCollaborativeRefundPropagatesCounterpartyFailure(t *testing.T) {
	h := &SwapHandler{}
	boltzErr := errors.New("boltz refused the collaborative refund")

	refundFunc := func(string, boltz.RefundSwapRequest) (*boltz.RefundSwapResponse, error) {
		return nil, boltzErr
	}

	_, _, err := h.collaborativeRefund(refundFunc, "swap-1", "refundtx", "checkpointtx")
	require.ErrorIs(t, err, boltzErr)
}

// TestCollaborativeRefundParsesCounterpartyPSBTs verifies the cooperative path:
// the refund request carries the unsigned txs, and the counterparty-signed
// refund + checkpoint PSBTs are decoded and returned.
func TestCollaborativeRefundParsesCounterpartyPSBTs(t *testing.T) {
	h := &SwapHandler{}
	refundPSBT := makeTestPSBT(t)
	checkpointPSBT := makeTestPSBT(t)

	refundFunc := func(_ string, req boltz.RefundSwapRequest) (*boltz.RefundSwapResponse, error) {
		require.Equal(t, "refundtx", req.Transaction)
		require.Equal(t, "checkpointtx", req.Checkpoint)
		return &boltz.RefundSwapResponse{Transaction: refundPSBT, Checkpoint: checkpointPSBT}, nil
	}

	refundPtx, checkpointPtx, err := h.collaborativeRefund(refundFunc, "swap-1", "refundtx", "checkpointtx")
	require.NoError(t, err)
	require.NotNil(t, refundPtx)
	require.NotNil(t, checkpointPtx)
}

// TestCollaborativeRefundRejectsMalformedResponse guards against a misbehaving
// counterparty: the refund and checkpoint PSBTs are decoded separately, so both
// decode branches are pinned by the distinct error each must surface (a bare
// require.Error couldn't tell which tx failed to decode, and left the checkpoint
// branch — a real fund-safety concern if a checkpoint silently fails to parse —
// uncovered).
func TestCollaborativeRefundRejectsMalformedResponse(t *testing.T) {
	h := &SwapHandler{}

	t.Run("malformed refund tx", func(t *testing.T) {
		refundFunc := func(string, boltz.RefundSwapRequest) (*boltz.RefundSwapResponse, error) {
			return &boltz.RefundSwapResponse{Transaction: "not-a-psbt", Checkpoint: makeTestPSBT(t)}, nil
		}
		_, _, err := h.collaborativeRefund(refundFunc, "swap-1", "refundtx", "checkpointtx")
		require.ErrorContains(t, err, "refund tx")
	})

	t.Run("malformed checkpoint tx", func(t *testing.T) {
		refundFunc := func(string, boltz.RefundSwapRequest) (*boltz.RefundSwapResponse, error) {
			return &boltz.RefundSwapResponse{Transaction: makeTestPSBT(t), Checkpoint: "not-a-psbt"}, nil
		}
		_, _, err := h.collaborativeRefund(refundFunc, "swap-1", "refundtx", "checkpointtx")
		require.ErrorContains(t, err, "checkpoint tx")
	})
}
