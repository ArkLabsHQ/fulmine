package swap

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"testing"

	"github.com/ArkLabsHQ/fulmine/pkg/boltz"
	arklib "github.com/arkade-os/arkd/pkg/ark-lib"
	clientTypes "github.com/arkade-os/arkd/pkg/client-lib/types"
	arksdk "github.com/arkade-os/go-sdk"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
	"github.com/stretchr/testify/require"
)

// refundMockExplorer is a minimal ExplorerClient for the BTC→ARK refund guards.
// Only GetTransaction and GetCurrentBlockHeight are reached before the CLTV
// gate; the rest are unreached stubs.
type refundMockExplorer struct {
	txHex         string
	height        uint32
	txErr         error
	broadcastTxid string
	broadcastTx   *wire.MsgTx // captured so tests can assert the refund tx's fields
}

func (m *refundMockExplorer) GetTransaction(string) (string, error)  { return m.txHex, m.txErr }
func (m *refundMockExplorer) GetCurrentBlockHeight() (uint32, error) { return m.height, nil }
func (m *refundMockExplorer) BroadcastTransaction(tx *wire.MsgTx) (string, error) {
	m.broadcastTx = tx
	return m.broadcastTxid, nil
}
func (m *refundMockExplorer) GetFeeRate() (float64, error) { return 1, nil }
func (m *refundMockExplorer) GetTransactionStatus(string) (*TransactionStatus, error) {
	// confirmed, so RefundBtcToArkSwap's post-broadcast wait loop exits at once.
	return &TransactionStatus{Confirmed: true}, nil
}

// buildLockupFixture synthesizes a regtest taproot lockup output and the Boltz
// chain-swap response that references it, with a refund leaf carrying the given
// absolute CLTV block height. The lockup tx pays the swap's lockup address, so
// findOutputForAddress resolves it — enough to drive RefundBtcToArkSwap through
// deserialization, output lookup and refund-leaf parsing up to the CLTV gate,
// with no live Boltz or regtest state. payToAddrScript is the package's own, so
// the synthesized output matches what findOutputForAddress derives.
func buildLockupFixture(t *testing.T, timeout int64) (lockupTxHex, swapRespJSON string) {
	t.Helper()

	k, err := btcec.NewPrivateKey()
	require.NoError(t, err)
	addr, err := btcutil.NewAddressTaproot(schnorr.SerializePubKey(k.PubKey()), &chaincfg.RegressionNetParams)
	require.NoError(t, err)
	pkScript, err := payToAddrScript(addr)
	require.NoError(t, err)

	lockupTx := wire.NewMsgTx(2)
	// a real lockup tx is funded; give it one input so the zero-input SegWit
	// serialization ambiguity (a 0x00 input count read as the witness marker)
	// doesn't bite on deserialize round-trip.
	lockupTx.AddTxIn(wire.NewTxIn(wire.NewOutPoint(&chainhash.Hash{}, 0), nil, nil))
	lockupTx.AddTxOut(wire.NewTxOut(100_000, pkScript))
	var buf bytes.Buffer
	require.NoError(t, lockupTx.Serialize(&buf))

	serverKey, err := btcec.NewPrivateKey()
	require.NoError(t, err)

	resp := boltz.CreateChainSwapResponse{
		LockupDetails: boltz.SwapLeg{
			LockupAddress:   addr.String(),
			ServerPublicKey: hex.EncodeToString(serverKey.PubKey().SerializeCompressed()),
			SwapTree: &boltz.SwapTree{
				// A distinct claim leaf so the swap tree is a real 2-leaf merkle
				// tree — createControlBlockFromSwapTree uses it as the refund
				// path's sibling hash. Its content is irrelevant (only hashed);
				// timeout+1 just keeps it distinct from the refund leaf.
				ClaimLeaf:  boltz.SwapTreeLeaf{Version: 0xc0, Output: buildRefundLeafScript(t, testRefundXOnlyPubKey(t), timeout+1)},
				RefundLeaf: boltz.SwapTreeLeaf{Version: 0xc0, Output: buildRefundLeafScript(t, testRefundXOnlyPubKey(t), timeout)},
			},
		},
	}
	respJSON, err := json.Marshal(resp)
	require.NoError(t, err)

	return hex.EncodeToString(buf.Bytes()), string(respJSON)
}

func newRefundTestHandler(exp ExplorerClient) *SwapHandler {
	return &SwapHandler{
		explorerClient: exp,
		config:         clientTypes.Config{Network: arklib.BitcoinRegTest},
	}
}

// TestRefundBtcToArkSwapGuards pins the input-validation and CLTV-gate guards of
// the BTC→ARK unilateral refund — the path a user takes to reclaim BTC when Boltz
// goes down mid-swap. The CLTV gate is the fund-safety crux: broadcasting a
// refund before its absolute timelock is consensus-invalid (fees burned on a
// doomed tx), while a gate that never opened would strand the funds. All these
// guards sit before any ArkClient call, so they run with only a mocked explorer.
func TestRefundBtcToArkSwapGuards(t *testing.T) {
	ctx := context.Background()

	t.Run("rejects an empty lockup txid", func(t *testing.T) {
		_, err := newRefundTestHandler(&refundMockExplorer{}).
			RefundBtcToArkSwap(ctx, "swap", 1000, "", "{}")
		require.ErrorContains(t, err, "userLockupTxid empty")
	})

	t.Run("rejects an empty swap response", func(t *testing.T) {
		_, err := newRefundTestHandler(&refundMockExplorer{}).
			RefundBtcToArkSwap(ctx, "swap", 1000, "lockuptxid", "")
		require.ErrorContains(t, err, "boltzSwapRespJson empty")
	})

	t.Run("rejects a malformed swap response", func(t *testing.T) {
		lockupHex, _ := buildLockupFixture(t, 800_000)
		_, err := newRefundTestHandler(&refundMockExplorer{txHex: lockupHex}).
			RefundBtcToArkSwap(ctx, "swap", 1000, "lockuptxid", "{not-json")
		require.Error(t, err)
	})

	t.Run("rejects when the CLTV timeout has not been reached", func(t *testing.T) {
		const timeout = 800_000
		lockupHex, respJSON := buildLockupFixture(t, timeout)
		exp := &refundMockExplorer{txHex: lockupHex, height: timeout - 1}

		_, err := newRefundTestHandler(exp).
			RefundBtcToArkSwap(ctx, "swap", 1000, "lockuptxid", respJSON)

		require.ErrorContains(t, err, "CLTV timeout not yet reached")
	})
}

// refundMockArkClient is a fake arksdk.ArkClient for the post-gate refund flow.
// It embeds the interface (so any unexpected call panics) and overrides the two
// ArkClient calls RefundBtcToArkSwap makes: NewBoardingAddress (the destination
// for the reclaimed BTC) and Settle (boarding that BTC as a VTXO).
type refundMockArkClient struct {
	arksdk.ArkClient
	boardingAddr string
}

func (m *refundMockArkClient) NewBoardingAddress(context.Context) (string, error) {
	return m.boardingAddr, nil
}

func (m *refundMockArkClient) Settle(context.Context, ...arksdk.BatchSessionOption) (string, error) {
	return "settle-txid", nil
}

// TestRefundBtcToArkSwapBroadcastsPastGate covers the fund-recovery payoff once
// the CLTV gate opens: RefundBtcToArkSwap must construct the refund tx, sign the
// taproot refund leaf, and broadcast it to move the locked BTC to the user's
// boarding address. The cooperative e2e never reaches a unilateral refund, so
// this drives it with synthesized swap crypto + a mocked explorer/ArkClient. It
// asserts the broadcast txid is returned (the funds left the lockup), proving
// the whole build->sign->broadcast path ran, not just that the gate opened.
func TestRefundBtcToArkSwapBroadcastsPastGate(t *testing.T) {
	const timeout = 800_000
	lockupHex, respJSON := buildLockupFixture(t, timeout)

	// constructClaimTransaction hex-decodes the lockup txid into the claim tx's
	// input outpoint, and the taproot prevout fetcher matches on it, so derive the
	// real txid from the fixture rather than passing a placeholder.
	raw, err := hex.DecodeString(lockupHex)
	require.NoError(t, err)
	var lockupTx wire.MsgTx
	require.NoError(t, lockupTx.Deserialize(bytes.NewReader(raw)))
	lockupTxid := lockupTx.TxHash().String()

	userKey, err := btcec.NewPrivateKey()
	require.NoError(t, err)

	// a valid regtest taproot address for the refund destination
	destKey, err := btcec.NewPrivateKey()
	require.NoError(t, err)
	boardingAddr, err := btcutil.NewAddressTaproot(schnorr.SerializePubKey(destKey.PubKey()), &chaincfg.RegressionNetParams)
	require.NoError(t, err)

	exp := &refundMockExplorer{
		txHex:         lockupHex,
		height:        timeout, // gate opens: currentHeight >= timeout
		broadcastTxid: "refund-broadcast-txid",
	}
	h := &SwapHandler{
		explorerClient: exp,
		arkClient:      &refundMockArkClient{boardingAddr: boardingAddr.String()},
		privateKey:     userKey,
		config:         clientTypes.Config{Network: arklib.BitcoinRegTest},
	}

	txid, err := h.RefundBtcToArkSwap(context.Background(), "swap", 1000, lockupTxid, respJSON)
	require.NoError(t, err)
	require.Equal(t, "refund-broadcast-txid", txid)

	// Pin the consensus-critical fields of the broadcast refund tx. Without these
	// the test would stay green even if the CLTV locktime, the non-final input
	// sequence, or the taproot witness were dropped — each unspendable on a real
	// node, but invisible to a mock that only hands back a canned txid.
	require.NotNil(t, exp.broadcastTx)
	require.Equal(t, uint32(timeout), exp.broadcastTx.LockTime, "refund tx must carry the CLTV locktime")
	require.Equal(t, wire.MaxTxInSequenceNum-1, exp.broadcastTx.TxIn[0].Sequence, "input must be non-final for CLTV")
	require.Len(t, exp.broadcastTx.TxIn[0].Witness, 3, "tapscript refund witness: [signature, refundScript, controlBlock]")
}
