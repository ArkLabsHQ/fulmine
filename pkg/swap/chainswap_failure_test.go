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
}

func (m *claimMockExplorer) GetFeeRate() (float64, error)           { return 1, nil }
func (m *claimMockExplorer) GetCurrentBlockHeight() (uint32, error) { return 100, nil }
func (m *claimMockExplorer) GetTransaction(string) (string, error)  { return "", nil }
func (m *claimMockExplorer) GetTransactionStatus(string) (*TransactionStatus, error) {
	return nil, nil
}
func (m *claimMockExplorer) BroadcastTransaction(*wire.MsgTx) (string, error) {
	return m.broadcastTxid, nil
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

	h := &arkToBtcHandler{swapHandler: &SwapHandler{
		boltzSvc:       &fakeBoltz{submitErr: errors.New("boltz refused the cooperative claim")},
		explorerClient: &claimMockExplorer{broadcastTxid: "script-path-claim-txid"},
		config:         clientTypes.Config{Network: arklib.BitcoinRegTest, Dust: 546},
	}}

	txid, err := h.claimBtcLockup(
		context.Background(), "swap-1", preimage, claimKey, btcAddress,
		&chaincfg.RegressionNetParams, tree, serverKey.PubKey(), serverLockupHex,
	)

	require.NoError(t, err)
	// the script-path broadcast txid — proves the fallback ran, not the cooperative path.
	require.Equal(t, "script-path-claim-txid", txid)
}
