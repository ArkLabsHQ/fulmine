package e2e_test

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"testing"
	"time"

	pb "github.com/ArkLabsHQ/fulmine/api-spec/protobuf/gen/go/fulmine/v1"
	arklib "github.com/arkade-os/arkd/pkg/ark-lib"
	"github.com/arkade-os/arkd/pkg/client-lib/indexer"
	indexergrpc "github.com/arkade-os/arkd/pkg/client-lib/indexer/grpc"
	bancov1 "github.com/arkade-os/bancod/api-spec/protobuf/gen/go/bancod/v1"
	"github.com/arkade-os/bancod/pkg/preimage"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/txscript"
	"github.com/decred/dcrd/dcrec/secp256k1/v4"
	"github.com/lightningnetwork/lnd/input"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

const (
	bancodGRPCAddr = "localhost:7270"
	arkdGRPCAddr   = "localhost:7070"
)

// TestNonInteractiveClaim creates a VHTLC with the non interative option
// and let bancod solver claims the VHTLC instead of the recipient himself
func TestNonInteractiveClaim(t *testing.T) {
	f, err := newFulmineClient("localhost:7000")
	require.NoError(t, err)
	require.NotNil(t, f)

	ctx := t.Context()

	info, err := f.GetInfo(ctx, &pb.GetInfoRequest{})
	require.NoError(t, err)
	require.NotEmpty(t, info)

	// create receiver "wallet"
	receiverPriv, err := btcec.NewPrivateKey()
	require.NoError(t, err)
	receiverPkScript, err := txscript.PayToTaprootScript(receiverPriv.PubKey())
	require.NoError(t, err)

	// fetch the solver's encryption pubkey and the introspector pubkey
	solverPub, introPub := fetchBancodPubKeys(t)

	// generate a preimage
	preimg := make([]byte, 32)
	_, err = rand.Read(preimg)
	require.NoError(t, err)
	// build the non interactive packet (encrypted preimage)
	pkt, err := preimage.BuildPacket(preimg, solverPub, receiverPkScript)
	require.NoError(t, err)
	body, err := pkt.Serialize()
	require.NoError(t, err)
	extraPacketHex := hex.EncodeToString(
		append([]byte{pkt.Type()}, body...),
	)
	sha := sha256.Sum256(preimg)
	preimageHashHex := hex.EncodeToString(input.Ripemd160H(sha[:]))
	introPubHex := hex.EncodeToString(introPub.SerializeCompressed())

	// create the VHTLC with the non-interactive claim option.
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
			ClaimReceiverAddress: addressFromPrivKey(t, info, receiverPriv),
			IntrospectorPubkey:   introPubHex,
			ExtraPacket:          &extraPacketHex,
		},
	})
	require.NoError(t, err)
	require.NotEmpty(t, createResp.Address)

	// fund the VHTLC
	const amount uint64 = 10_000
	sendResp, err := f.SendOffChain(ctx, &pb.SendOffChainRequest{
		Address: createResp.Address,
		Amount:  amount,
	})
	require.NoError(t, err)
	require.NotEmpty(t, sendResp.GetTxid())

	// wait for solver to auto claim. after 30s, receiverPkScript should have received the funds
	v := pollForVtxoAtScript(t, ctx, receiverPkScript, 30*time.Second)
	require.Equal(t, amount, v.Amount, "solver should pay the full input value to the receiver")
}

// fetchBancodPubKeys dials the bancod gRPC and returns its (solver, introspector) pubkeys.
func fetchBancodPubKeys(t *testing.T) (*btcec.PublicKey, *btcec.PublicKey) {
	t.Helper()
	conn, err := grpc.NewClient(bancodGRPCAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	client := bancov1.NewPreimageServiceClient(conn)
	resp, err := client.GetSolverPubKey(t.Context(), &bancov1.GetSolverPubKeyRequest{})
	require.NoError(t, err)

	solverRaw, err := hex.DecodeString(resp.GetSolverPubKey())
	require.NoError(t, err)
	solver, err := btcec.ParsePubKey(solverRaw)
	require.NoError(t, err)

	introRaw, err := hex.DecodeString(resp.GetIntrospectorPubKey())
	require.NoError(t, err)
	intro, err := btcec.ParsePubKey(introRaw)
	require.NoError(t, err)
	return solver, intro
}

func pollForVtxoAtScript(t *testing.T, ctx context.Context, pkScript []byte, timeout time.Duration) struct {
	Txid   string
	VOut   uint32
	Amount uint64
} {
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
			v := resp.Vtxos[0]
			return struct {
				Txid   string
				VOut   uint32
				Amount uint64
			}{Txid: v.Txid, VOut: v.VOut, Amount: v.Amount}
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("no VTXO appeared at pkScript %s within %v",
		hex.EncodeToString(pkScript), timeout)
	return struct {
		Txid   string
		VOut   uint32
		Amount uint64
	}{}
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

func addressFromPrivKey(t *testing.T, info *pb.GetInfoResponse, receiverPrivKey *secp256k1.PrivateKey) string {
	// Build the receiver's bech32 Ark address using arkd's signer pubkey.
	serverPubBytes, err := hex.DecodeString(info.GetSignerPubkey())
	require.NoError(t, err)
	serverPub, err := btcec.ParsePubKey(serverPubBytes)
	require.NoError(t, err)
	receiverAddr, err := (&arklib.Address{
		HRP:        addrHRPFromNetwork(info.GetNetwork()),
		Signer:     serverPub,
		VtxoTapKey: receiverPrivKey.PubKey(),
	}).EncodeV0()
	require.NoError(t, err)
	return receiverAddr
}