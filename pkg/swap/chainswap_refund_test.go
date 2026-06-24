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
	txHex  string
	height uint32
	txErr  error
}

func (m *refundMockExplorer) GetTransaction(string) (string, error)  { return m.txHex, m.txErr }
func (m *refundMockExplorer) GetCurrentBlockHeight() (uint32, error) { return m.height, nil }
func (m *refundMockExplorer) BroadcastTransaction(*wire.MsgTx) (string, error) {
	return "", nil
}
func (m *refundMockExplorer) GetFeeRate() (float64, error) { return 1, nil }
func (m *refundMockExplorer) GetTransactionStatus(string) (*TransactionStatus, error) {
	return nil, nil
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

	resp := boltz.CreateChainSwapResponse{
		LockupDetails: boltz.SwapLeg{
			LockupAddress: addr.String(),
			SwapTree: &boltz.SwapTree{
				RefundLeaf: boltz.SwapTreeLeaf{
					Output: buildRefundLeafScript(t, testRefundXOnlyPubKey(t), timeout),
				},
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
