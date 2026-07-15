package e2e_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	pb "github.com/ArkLabsHQ/fulmine/api-spec/protobuf/gen/go/fulmine/v1"
	arklib "github.com/arkade-os/arkd/pkg/ark-lib"
	"github.com/arkade-os/arkd/pkg/client-lib/indexer"
	indexergrpc "github.com/arkade-os/arkd/pkg/client-lib/indexer/grpc"
	clientTypes "github.com/arkade-os/arkd/pkg/client-lib/types"
	"github.com/arkade-os/covclaimd/pkg/preimage"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/txscript"
	"github.com/lightningnetwork/lnd/input"
	"github.com/stretchr/testify/require"
)

const (
	covclaimdHTTPAddr = "http://localhost:7271"
	arkdGRPCAddr      = "localhost:7070"
)

// TestNonInteractiveClaim creates a VHTLC with the non-interactive claim
// option, funds it, and reveals the preimage to covclaimd out of band (direct
// reveal — no OP_RETURN packet in the funding tx). covclaimd then claims the
// VHTLC through the emulator on the receiver's behalf.
func TestNonInteractiveClaim(t *testing.T) {
	f, err := newFulmineClient(clientFulmineURL)
	require.NoError(t, err)
	require.NotNil(t, f)

	ctx := t.Context()

	info, err := f.GetInfo(ctx, &pb.GetInfoRequest{})
	require.NoError(t, err)
	require.NotEmpty(t, info)

	// create receiver "wallet" (just a keypair)
	receiverPriv, err := btcec.NewPrivateKey()
	require.NoError(t, err)
	receiverPkScript, err := txscript.PayToTaprootScript(receiverPriv.PubKey())
	require.NoError(t, err)

	// fetch covclaimd's encryption pubkey and its emulator (emulator) pubkey
	covclaimdPub, _ := fetchCovclaimdPubKeys(t)

	// generate a preimage
	preimg := make([]byte, 32)
	_, err = rand.Read(preimg)
	require.NoError(t, err)
	sha := sha256.Sum256(preimg)
	preimageHashHex := hex.EncodeToString(input.Ripemd160H(sha[:]))

	// create the VHTLC with the non-interactive claim option
	createResp, err := f.CreateVHTLC(ctx, &pb.CreateVHTLCRequest{
		PreimageHash:   preimageHashHex,
		ReceiverPubkey: hex.EncodeToString(receiverPriv.PubKey().SerializeCompressed()),
		UnilateralClaimDelay: &pb.RelativeLocktime{
			Type:  pb.RelativeLocktime_LOCKTIME_TYPE_SECOND,
			Value: 512,
		},
		UnilateralRefundDelay: &pb.RelativeLocktime{
			Type:  pb.RelativeLocktime_LOCKTIME_TYPE_SECOND,
			Value: 512,
		},
		UnilateralRefundWithoutReceiverDelay: &pb.RelativeLocktime{
			Type:  pb.RelativeLocktime_LOCKTIME_TYPE_SECOND,
			Value: 1024,
		},
		NonInteractiveClaim: &pb.NonInteractiveClaim{
			ClaimAddress: receiverArkAddress(t, info, receiverPriv.PubKey()),
		},
	})
	require.NoError(t, err)
	require.NotEmpty(t, createResp.Address)

	// direct reveal to claimer
	revealToCovclaimd(t, createResp.Address, preimg, covclaimdPub, receiverPkScript)

	// fund the VHTLC
	const amount uint64 = 10_000
	sendResp, err := f.SendOffChain(ctx, &pb.SendOffChainRequest{
		Address: createResp.Address,
		Amount:  amount,
	})
	require.NoError(t, err)
	require.NotEmpty(t, sendResp.GetTxid())

	// wait for covclaimd to auto claim to the receiver's script
	v := pollForVtxoAtScript(t, ctx, receiverPkScript, 30*time.Second)
	require.Equal(t, amount, v.Amount, "covclaimd should pay the full input value to the receiver")
}

// fetchCovclaimdPubKeys returns covclaimd's (encryption, emulator) pubkeys.
func fetchCovclaimdPubKeys(t *testing.T) (*btcec.PublicKey, *btcec.PublicKey) {
	t.Helper()
	resp, err := http.Get(covclaimdHTTPAddr + "/v1/preimage/covclaimd-pubkey")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var body struct {
		CovclaimdPubKey string `json:"covclaimd_pub_key"`
		EmulatorPubKey  string `json:"emulator_pub_key"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))

	covclaimdPub := mustParseCompressedPubKey(t, body.CovclaimdPubKey)
	emulatorPub := mustParseCompressedPubKey(t, body.EmulatorPubKey)
	return covclaimdPub, emulatorPub
}

// revealToCovclaimd registers the {swap address, claim packet} pair on
// covclaimd's reveal endpoint (direct reveal mode).
func revealToCovclaimd(
	t *testing.T, swapAddress string, preimg []byte,
	covclaimdPub *btcec.PublicKey, receiverPkScript []byte,
) {
	t.Helper()
	ciphertext, err := preimage.Encrypt(covclaimdPub, preimg)
	require.NoError(t, err)
	arkadeScript, err := preimage.EnforcePayTo(receiverPkScript)
	require.NoError(t, err)

	payload, err := json.Marshal(map[string]any{
		"swap_address": swapAddress,
		"packet": map[string]string{
			"ciphertext":    base64.StdEncoding.EncodeToString(ciphertext),
			"arkade_script": base64.StdEncoding.EncodeToString(arkadeScript),
		},
	})
	require.NoError(t, err)

	resp, err := http.Post(
		covclaimdHTTPAddr+"/v1/reveal", "application/json", bytes.NewReader(payload),
	)
	require.NoError(t, err)
	defer resp.Body.Close()
	var respBody bytes.Buffer
	_, _ = respBody.ReadFrom(resp.Body)
	require.Equal(t, http.StatusOK, resp.StatusCode, "reveal failed: %s", respBody.String())
}

func pollForVtxoAtScript(
	t *testing.T, ctx context.Context, pkScript []byte, timeout time.Duration,
) clientTypes.Vtxo {
	t.Helper()
	idx, err := indexergrpc.NewClient(arkdGRPCAddr)
	require.NoError(t, err)

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp, err := idx.GetVtxos(ctx,
			indexer.WithScripts([]string{hex.EncodeToString(pkScript)}),
			indexer.WithSpendableOnly(),
		)
		if err == nil && len(resp.Vtxos) > 0 {
			return resp.Vtxos[0]
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("no VTXO appeared at pkScript %s within %v", hex.EncodeToString(pkScript), timeout)
	return clientTypes.Vtxo{}
}

func mustParseCompressedPubKey(t *testing.T, pubHex string) *btcec.PublicKey {
	t.Helper()
	raw, err := hex.DecodeString(pubHex)
	require.NoError(t, err)
	pub, err := btcec.ParsePubKey(raw)
	require.NoError(t, err)
	return pub
}

// receiverArkAddress builds the receiver's bech32m Ark address from arkd's
// signer pubkey and the receiver key, matching the P2TR pkScript the covenant
// enforces as claim output.
func receiverArkAddress(t *testing.T, info *pb.GetInfoResponse, receiverPub *btcec.PublicKey) string {
	t.Helper()
	serverPubBytes, err := hex.DecodeString(info.GetSignerPubkey())
	require.NoError(t, err)
	serverPub, err := btcec.ParsePubKey(serverPubBytes)
	require.NoError(t, err)
	addr, err := (&arklib.Address{
		HRP:        addrHRPFromNetwork(info.GetNetwork()),
		Signer:     serverPub,
		VtxoTapKey: receiverPub,
	}).EncodeV0()
	require.NoError(t, err)
	return addr
}

func addrHRPFromNetwork(network pb.GetInfoResponse_Network) string {
	switch network {
	case pb.GetInfoResponse_NETWORK_MAINNET:
		return arklib.Bitcoin.Addr
	case pb.GetInfoResponse_NETWORK_TESTNET:
		return arklib.BitcoinTestNet.Addr
	default:
		return arklib.BitcoinRegTest.Addr
	}
}
