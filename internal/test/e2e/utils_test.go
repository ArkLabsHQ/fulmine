package e2e_test

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	pb "github.com/ArkLabsHQ/fulmine/api-spec/protobuf/gen/go/fulmine/v1"
	"github.com/ArkLabsHQ/fulmine/pkg/vhtlc"
	arklib "github.com/arkade-os/arkd/pkg/ark-lib"
	"github.com/arkade-os/arkd/pkg/ark-lib/offchain"
	"github.com/arkade-os/arkd/pkg/ark-lib/script"
	"github.com/arkade-os/arkd/pkg/ark-lib/txutils"
	"github.com/arkade-os/arkd/pkg/client-lib/client"
	grpcclient "github.com/arkade-os/arkd/pkg/client-lib/client/grpc"
	clientTypes "github.com/arkade-os/arkd/pkg/client-lib/types"
	arksdk "github.com/arkade-os/go-sdk"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/btcsuite/btcd/btcutil/psbt"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	"github.com/ccoveille/go-safecast"
	"github.com/creack/pty"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

const (
	lnd = "docker exec lnd lncli --network=regtest"
	cln = "docker exec cln lightning-cli --network=regtest"
)

func newFulmineClient(url string) (pb.ServiceClient, error) {
	opts := grpc.WithTransportCredentials(insecure.NewCredentials())
	conn, err := grpc.NewClient(url, opts)
	if err != nil {
		return nil, err
	}
	return pb.NewServiceClient(conn), nil
}

func newFulmineWalletClient(url string) (pb.WalletServiceClient, error) {
	opts := grpc.WithTransportCredentials(insecure.NewCredentials())
	conn, err := grpc.NewClient(url, opts)
	if err != nil {
		return nil, err
	}
	return pb.NewWalletServiceClient(conn), nil
}

func newFulmineOffchainAddress(t *testing.T, client pb.ServiceClient) string {
	t.Helper()

	resp, err := client.GetAddress(t.Context(), &pb.GetAddressRequest{})
	require.NoError(t, err)

	addr, err := url.Parse(resp.GetAddress())
	require.NoError(t, err)

	offchainAddr := addr.Query().Get("ark")
	require.NotEmpty(t, offchainAddr)

	return offchainAddr
}

func lndAddInvoice(ctx context.Context, sats int) (string, string, error) {
	command := fmt.Sprintf("%s addinvoice --amt %d", lnd, sats)
	out, err := runCommand(ctx, command)
	if err != nil {
		return "", "", err
	}

	var resp struct {
		PaymentRequest string `json:"payment_request"`
		RHash          string `json:"r_hash"`
	}
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		return "", "", err
	}
	return resp.PaymentRequest, resp.RHash, nil
}

func lndPayInvoice(ctx context.Context, invoice string) error {
	command := fmt.Sprintf("%s payinvoice --force %s", lnd, invoice)
	_, err := runCommand(ctx, command)
	return err
}

func lndCancelInvoice(ctx context.Context, rHash string) error {
	command := fmt.Sprintf("%s cancelinvoice %s", lnd, rHash)
	_, err := runCommand(ctx, command)
	return err
}

func clnAddOffer(ctx context.Context, sats int) (string, string, error) {
	label := fmt.Sprintf("funding-%s", time.Now().Format(time.RFC3339))
	command := fmt.Sprintf(`%s offer %d "%s"`, cln, sats, label)
	out, err := runCommand(ctx, command)
	if err != nil {
		return "", "", err
	}

	var resp struct {
		PaymentHash string `json:"offer_id"`
		Bolt11      string `json:"bolt12"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &resp); err != nil {
		return "", "", err
	}
	return resp.Bolt11, resp.PaymentHash, nil
}

func faucet(ctx context.Context, address string, amount float64) error {
	command := fmt.Sprintf("nigiri faucet %s %.8f", address, amount)
	_, err := runCommand(ctx, command)
	return err
}

func runCommand(ctx context.Context, command string) (string, error) {
	cmd := exec.CommandContext(ctx, "sh", "-c", command)

	ptmx, err := pty.Start(cmd)
	if err != nil {
		return "", err
	}
	defer func() { _ = ptmx.Close() }()

	var out bytes.Buffer
	done := make(chan error, 1)
	go func() {
		_, err := io.Copy(&out, ptmx)
		done <- err
	}()

	select {
	case <-ctx.Done():
		_ = cmd.Process.Kill()
		return "", ctx.Err()
	case copyErr := <-done:
		if copyErr != nil && !errors.Is(copyErr, syscall.EIO) {
			return "", fmt.Errorf("read command output: %w", copyErr)
		}
		if err := cmd.Wait(); err != nil {
			return "", fmt.Errorf("%s", strings.TrimSpace(out.String()))
		}
		return out.String(), nil
	}
}

func restartDockerComposeServices(t *testing.T, ctx context.Context, services ...string) {
	t.Helper()
	composePath := findComposeFile(t)
	requireServices := strings.Join(services, " ")
	command := fmt.Sprintf("docker compose -f %s restart %s", composePath, requireServices)
	_, err := runCommand(ctx, command)
	if err != nil {
		t.Fatalf("restart docker services (%s): %v", requireServices, err)
	}
}

func findComposeFile(t *testing.T) string {
	t.Helper()
	path, err := findComposeFilePath()
	if err != nil {
		t.Fatalf("%v", err)
	}
	return path
}

func findComposeFilePath() (string, error) {
	if env := os.Getenv("FULMINE_COMPOSE_FILE"); env != "" {
		return env, nil
	}

	wd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("getwd failed: %w", err)
	}

	dir := wd
	for {
		candidate := filepath.Join(dir, "test.docker-compose.yml")
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	return "", fmt.Errorf("test.docker-compose.yml not found from %s; set FULMINE_COMPOSE_FILE", wd)
}

func unlockAndSettle(addr string, pass string) error {
	var walletClient pb.WalletServiceClient
	var serviceClient pb.ServiceClient

	// Retry loop: after a docker restart the gRPC server may need a few
	// seconds before it starts accepting connections.
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		wc, err := newFulmineWalletClient(addr)
		if err != nil {
			time.Sleep(500 * time.Millisecond)
			continue
		}
		_, err = wc.Unlock(context.Background(), &pb.UnlockRequest{Password: pass})
		if err != nil {
			// "connection reset" / "connection refused" means the server
			// isn't ready yet – keep retrying.
			errMsg := err.Error()
			if strings.Contains(errMsg, "connection reset") ||
				strings.Contains(errMsg, "connection refused") ||
				strings.Contains(errMsg, "unavailable") {
				time.Sleep(500 * time.Millisecond)
				continue
			}
			return fmt.Errorf("unlock %s: %w", addr, err)
		}
		walletClient = wc
		break
	}
	if walletClient == nil {
		return fmt.Errorf("wallet client %s not ready within 30s", addr)
	}

	sc, err := newFulmineClient(addr)
	if err != nil {
		return err
	}
	serviceClient = sc

	// After unlock the service may still be syncing with the chain.
	// Retry Settle until it succeeds or the deadline is reached.
	settleDeadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(settleDeadline) {
		_, err = serviceClient.Settle(context.Background(), &pb.SettleRequest{})
		if err == nil {
			return nil
		}
		errMsg := err.Error()
		if strings.Contains(errMsg, "syncing") ||
			strings.Contains(errMsg, "connection reset") ||
			strings.Contains(errMsg, "connection refused") ||
			strings.Contains(errMsg, "unavailable") {
			time.Sleep(1 * time.Second)
			continue
		}
		return fmt.Errorf("settle %s: %w", addr, err)
	}

	return fmt.Errorf("settle %s: timed out (last error: %w)", addr, err)
}

func generateNote(t *testing.T, amount uint64) string {
	adminHttpClient := &http.Client{
		Timeout: 15 * time.Second,
	}

	reqBody := bytes.NewReader([]byte(fmt.Sprintf(`{"amount": "%d"}`, amount)))
	req, err := http.NewRequest("POST", "http://localhost:7071/v1/admin/note", reqBody)
	if err != nil {
		t.Fatalf("failed to prepare note request: %s", err)
	}
	req.Header.Set("Authorization", "Basic YWRtaW46YWRtaW4=")
	req.Header.Set("Content-Type", "application/json")

	resp, err := adminHttpClient.Do(req)
	if err != nil {
		t.Fatalf("failed to create note: %s", err)
	}
	defer resp.Body.Close()

	var noteResp struct {
		Notes []string `json:"notes"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&noteResp); err != nil {
		t.Fatalf("failed to parse response: %s", err)
	}
	if len(noteResp.Notes) == 0 {
		t.Fatalf("no notes returned from admin API")
	}

	return noteResp.Notes[0]
}

func faucetOffchain(t *testing.T, client arksdk.ArkClient, amount float64) clientTypes.Vtxo {
	offchainAddr, err := client.NewOffchainAddress(t.Context())
	require.NoError(t, err)

	note := generateNote(t, uint64(amount*1e8))

	wg := &sync.WaitGroup{}
	wg.Add(1)
	var incomingFunds []clientTypes.Vtxo
	var incomingErr error
	go func() {
		incomingFunds, incomingErr = client.NotifyIncomingFunds(t.Context(), offchainAddr)
		wg.Done()
	}()

	txid, err := client.RedeemNotes(t.Context(), []string{note})
	require.NoError(t, err)
	require.NotEmpty(t, txid)

	wg.Wait()

	require.NoError(t, incomingErr)
	require.NotEmpty(t, incomingFunds)

	time.Sleep(time.Second)
	return incomingFunds[0]
}

func newDelegatorClient(url string) (pb.DelegatorServiceClient, error) {
	opts := grpc.WithTransportCredentials(insecure.NewCredentials())
	conn, err := grpc.NewClient(url, opts)
	if err != nil {
		return nil, err
	}
	return pb.NewDelegatorServiceClient(conn), nil
}

func setupArkSDKwithPublicKey(
	t *testing.T,
) (arksdk.ArkClient, *btcec.PublicKey, client.TransportClient) {
	t.Helper()

	serverUrl := "localhost:7070"
	password := "pass"

	arkClient, err := arksdk.NewArkClient("", false)
	require.NoError(t, err)

	privkey, err := btcec.NewPrivateKey()
	require.NoError(t, err)

	privkeyHex := hex.EncodeToString(privkey.Serialize())

	err = arkClient.Init(t.Context(), serverUrl, privkeyHex, password)
	require.NoError(t, err)

	err = arkClient.Unlock(t.Context(), password)
	require.NoError(t, err)

	syncCtx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()

	select {
	case syncEvent, ok := <-arkClient.IsSynced(syncCtx):
		require.True(t, ok, "sync channel closed before client reported sync")
		require.NoError(t, syncEvent.Err)
		require.True(t, syncEvent.Synced, "ark client did not report synced")
	case <-syncCtx.Done():
		t.Fatalf("timed out waiting for ark client sync: %v", syncCtx.Err())
	}

	grpcClient, err := grpcclient.NewClient(serverUrl)
	require.NoError(t, err)

	return arkClient, privkey.PubKey(), grpcClient
}

// issueAsset issues a new asset with the given supply and returns the asset ID string.
func issueAsset(t *testing.T, client arksdk.ArkClient, supply uint64) string {
	t.Helper()
	_, assetIds, err := client.IssueAsset(t.Context(), supply, nil, nil)
	require.NoError(t, err)
	require.Len(t, assetIds, 1)
	return assetIds[0].String()
}

// listVtxosWithAsset returns all spendable VTXOs that contain the given asset ID.
func listVtxosWithAsset(t *testing.T, client arksdk.ArkClient, assetID string) []clientTypes.Vtxo {
	t.Helper()
	vtxos, _, err := client.ListVtxos(t.Context())
	require.NoError(t, err)

	assetVtxos := make([]clientTypes.Vtxo, 0, len(vtxos))
	for _, vtxo := range vtxos {
		for _, asset := range vtxo.Assets {
			if asset.AssetId == assetID {
				assetVtxos = append(assetVtxos, vtxo)
				break
			}
		}
	}
	return assetVtxos
}

// requireVtxoHasAsset asserts that the given VTXO contains an asset with the given ID and amount.
func requireVtxoHasAsset(t *testing.T, vtxo clientTypes.Vtxo, assetID string, expectedAmount uint64) {
	t.Helper()
	for _, asset := range vtxo.Assets {
		if asset.AssetId == assetID {
			require.Equal(t, expectedAmount, asset.Amount, "asset %s amount mismatch", assetID)
			return
		}
	}
	t.Fatalf("vtxo %s:%d does not contain asset %s", vtxo.Txid, vtxo.VOut, assetID)
}

type testVHTLC struct {
	script *vhtlc.VHTLCScript
	vtxo   *clientTypes.Vtxo
}

func buildTestVHTLC(
	t *testing.T,
	fulmineClient pb.ServiceClient,
	vhtlcResp *pb.CreateVHTLCResponse,
	preimageHash string,
) testVHTLC {
	t.Helper()

	vhtlcScript, err := vhtlc.NewVHTLCScriptFromOpts(vhtlc.Opts{
		Sender:         mustParseSchnorrPubKey(t, vhtlcResp.GetRefundPubkey()),
		Receiver:       mustParseSchnorrPubKey(t, vhtlcResp.GetClaimPubkey()),
		Server:         mustParseSchnorrPubKey(t, vhtlcResp.GetServerPubkey()),
		PreimageHash:   mustDecodeHex(t, preimageHash),
		RefundLocktime: arklib.AbsoluteLocktime(vhtlcResp.GetRefundLocktime()),
		UnilateralClaimDelay: arklib.RelativeLocktime{
			Type:  arklib.LocktimeTypeSecond,
			Value: uint32(vhtlcResp.GetUnilateralClaimDelay()),
		},
		UnilateralRefundDelay: arklib.RelativeLocktime{
			Type:  arklib.LocktimeTypeSecond,
			Value: uint32(vhtlcResp.GetUnilateralRefundDelay()),
		},
		UnilateralRefundWithoutReceiverDelay: arklib.RelativeLocktime{
			Type:  arklib.LocktimeTypeSecond,
			Value: uint32(vhtlcResp.GetUnilateralRefundWithoutReceiverDelay()),
		},
	})
	require.NoError(t, err)

	return testVHTLC{
		script: vhtlcScript,
		vtxo:   findUnspentVHTLCVtxo(t, fulmineClient, vhtlcResp.GetId()),
	}
}

func findUnspentVHTLCVtxo(
	t *testing.T,
	fulmineClient pb.ServiceClient,
	vhtlcID string,
) *clientTypes.Vtxo {
	t.Helper()

	resp, err := fulmineClient.ListVHTLC(t.Context(), &pb.ListVHTLCRequest{VhtlcId: vhtlcID})
	require.NoError(t, err)
	require.NotEmpty(t, resp.GetVhtlcs())

	var unspent *pb.Vtxo
	for _, vtxo := range resp.GetVhtlcs() {
		if !vtxo.IsSpent {
			unspent = vtxo
			break
		}
	}
	require.NotNil(t, unspent, "expected an unspent VTXO at the VHTLC address")

	return &clientTypes.Vtxo{
		Outpoint: clientTypes.Outpoint{
			Txid: unspent.Outpoint.GetTxid(),
			VOut: unspent.Outpoint.GetVout(),
		},
		Amount:    unspent.Amount,
		CreatedAt: time.Unix(unspent.CreatedAt, 0),
	}
}

func mustParseSchnorrPubKey(t *testing.T, hexPubKey string) *btcec.PublicKey {
	t.Helper()

	pubKeyBytes, err := hex.DecodeString(hexPubKey)
	require.NoError(t, err)

	pubKey, err := schnorr.ParsePubKey(pubKeyBytes)
	require.NoError(t, err)

	return pubKey
}

func mustDecodeHex(t *testing.T, value string) []byte {
	t.Helper()

	decoded, err := hex.DecodeString(value)
	require.NoError(t, err)

	return decoded
}

// submitPendingClaimVHTLC submits the claim transaction but intentionally skips
// FinalizeTx so the VTXO remains in the partially-executed pending state.
func submitPendingClaimVHTLC(
	t *testing.T,
	arkClient arksdk.ArkClient,
	fulmineClient pb.ServiceClient,
	vhtlc testVHTLC,
	preimage []byte,
) string {
	t.Helper()
	ctx := t.Context()

	cfg, err := arkClient.GetConfigData(ctx)
	require.NoError(t, err)

	vtxoTxHash, err := chainhash.NewHashFromStr(vhtlc.vtxo.Txid)
	require.NoError(t, err)

	vtxoOutpoint := &wire.OutPoint{
		Hash:  *vtxoTxHash,
		Index: vhtlc.vtxo.VOut,
	}

	destinationAddr := newFulmineOffchainAddress(t, fulmineClient)
	decodedAddr, err := arklib.DecodeAddressV0(destinationAddr)
	require.NoError(t, err)

	pkScript, err := script.P2TRScript(decodedAddr.VtxoTapKey)
	require.NoError(t, err)

	amount, err := safecast.ToInt64(vhtlc.vtxo.Amount)
	require.NoError(t, err)

	claimTapscript, err := vhtlc.script.ClaimTapscript()
	require.NoError(t, err)

	tapScript, _ := hex.DecodeString(cfg.CheckpointTapscript)
	arkTx, checkpoints, err := offchain.BuildTxs(
		[]offchain.VtxoInput{
			{
				RevealedTapscripts: vhtlc.script.GetRevealedTapscripts(),
				Outpoint:           vtxoOutpoint,
				Amount:             amount,
				Tapscript:          claimTapscript,
			},
		},
		[]*wire.TxOut{
			{
				Value:    amount,
				PkScript: pkScript,
			},
		},
		tapScript,
	)
	require.NoError(t, err)

	signTransaction := func(tx *psbt.Packet) (string, error) {
		err := txutils.SetArkPsbtField(
			tx, 0, txutils.ConditionWitnessField, wire.TxWitness{preimage},
		)
		require.NoError(t, err)

		encoded, err := tx.B64Encode()
		require.NoError(t, err)

		resp, err := fulmineClient.SignTransaction(ctx, &pb.SignTransactionRequest{
			Tx: encoded,
		})
		require.NoError(t, err)

		return resp.SignedTx, nil
	}

	signedArkTx, err := signTransaction(arkTx)
	require.NoError(t, err)

	checkpointTxs := make([]string, 0, len(checkpoints))
	for _, ptx := range checkpoints {
		tx, err := ptx.B64Encode()
		require.NoError(t, err)
		checkpointTxs = append(checkpointTxs, tx)
	}

	arkTxid, finalArkTx, _, err := arkClient.Client().SubmitTx(
		ctx, signedArkTx, checkpointTxs,
	)
	require.NoError(t, err)

	err = verifyFinalArkTx(
		finalArkTx, cfg.SignerPubKey, getInputTapLeaves(arkTx),
	)
	require.NoError(t, err)

	return arkTxid
}

// submitPendingRefundVHTLCWithoutReceiver submits the refund-without-receiver
// transaction but intentionally skips FinalizeTx so the VTXO remains pending.
func submitPendingRefundVHTLCWithoutReceiver(
	t *testing.T,
	arkClient arksdk.ArkClient,
	fulmineClient pb.ServiceClient,
	vhtlc testVHTLC,
) string {
	t.Helper()
	ctx := t.Context()

	cfg, err := arkClient.GetConfigData(ctx)
	require.NoError(t, err)

	vtxoTxHash, err := chainhash.NewHashFromStr(vhtlc.vtxo.Txid)
	require.NoError(t, err)

	vtxoOutpoint := &wire.OutPoint{
		Hash:  *vtxoTxHash,
		Index: vhtlc.vtxo.VOut,
	}

	destinationAddr := newFulmineOffchainAddress(t, fulmineClient)
	decodedAddr, err := arklib.DecodeAddressV0(destinationAddr)
	require.NoError(t, err)

	pkScript, err := script.P2TRScript(decodedAddr.VtxoTapKey)
	require.NoError(t, err)

	amount, err := safecast.ToInt64(vhtlc.vtxo.Amount)
	require.NoError(t, err)

	refundTapscript, err := vhtlc.script.RefundTapscript(false)
	require.NoError(t, err)

	tapScript, _ := hex.DecodeString(cfg.CheckpointTapscript)
	arkTx, checkpoints, err := offchain.BuildTxs(
		[]offchain.VtxoInput{
			{
				RevealedTapscripts: vhtlc.script.GetRevealedTapscripts(),
				Outpoint:           vtxoOutpoint,
				Amount:             amount,
				Tapscript:          refundTapscript,
			},
		},
		[]*wire.TxOut{
			{
				Value:    amount,
				PkScript: pkScript,
			},
		},
		tapScript,
	)
	require.NoError(t, err)

	encodedArkTx, err := arkTx.B64Encode()
	require.NoError(t, err)

	resp, err := fulmineClient.SignTransaction(ctx, &pb.SignTransactionRequest{
		Tx: encodedArkTx,
	})
	require.NoError(t, err)

	checkpointTxs := make([]string, 0, len(checkpoints))
	for _, ptx := range checkpoints {
		tx, err := ptx.B64Encode()
		require.NoError(t, err)
		checkpointTxs = append(checkpointTxs, tx)
	}

	arkTxid, finalArkTx, _, err := arkClient.Client().SubmitTx(
		ctx, resp.SignedTx, checkpointTxs,
	)
	require.NoError(t, err)

	err = verifyFinalArkTx(
		finalArkTx, cfg.SignerPubKey, getInputTapLeaves(arkTx),
	)
	require.NoError(t, err)

	return arkTxid
}

func verifyFinalArkTx(
	finalArkTx string, arkSigner *btcec.PublicKey, expectedTapLeaves map[int]txscript.TapLeaf,
) error {
	finalArkPtx, err := psbt.NewFromRawBytes(strings.NewReader(finalArkTx), true)
	if err != nil {
		return err
	}

	return verifyInputSignatures(finalArkPtx, arkSigner, expectedTapLeaves)
}

func getInputTapLeaves(tx *psbt.Packet) map[int]txscript.TapLeaf {
	tapLeaves := make(map[int]txscript.TapLeaf)
	for inputIndex, input := range tx.Inputs {
		if len(input.TaprootLeafScript) <= 0 {
			continue
		}
		tapLeaves[inputIndex] = txscript.NewBaseTapLeaf(input.TaprootLeafScript[0].Script)
	}
	return tapLeaves
}

func verifyInputSignatures(
	tx *psbt.Packet, pubkey *btcec.PublicKey, tapLeaves map[int]txscript.TapLeaf,
) error {
	xOnlyPubkey := schnorr.SerializePubKey(pubkey)

	prevouts := make(map[wire.OutPoint]*wire.TxOut)
	sigsToVerify := make(map[int]*psbt.TaprootScriptSpendSig)

	for inputIndex, input := range tx.Inputs {
		// collect previous outputs
		if input.WitnessUtxo == nil {
			return fmt.Errorf("input %d has no witness utxo, cannot verify signature", inputIndex)
		}

		outpoint := tx.UnsignedTx.TxIn[inputIndex].PreviousOutPoint
		prevouts[outpoint] = input.WitnessUtxo

		tapLeaf, ok := tapLeaves[inputIndex]
		if !ok {
			return fmt.Errorf(
				"input %d has no tapscript leaf, cannot verify signature", inputIndex,
			)
		}

		tapLeafHash := tapLeaf.TapHash()

		// check if pubkey has a tapscript sig
		hasSig := false
		for _, sig := range input.TaprootScriptSpendSig {
			if bytes.Equal(sig.XOnlyPubKey, xOnlyPubkey) &&
				bytes.Equal(sig.LeafHash, tapLeafHash[:]) {
				hasSig = true
				sigsToVerify[inputIndex] = sig
				break
			}
		}

		if !hasSig {
			return fmt.Errorf("input %d has no signature for pubkey %x", inputIndex, xOnlyPubkey)
		}
	}

	prevoutFetcher := txscript.NewMultiPrevOutFetcher(prevouts)
	txSigHashes := txscript.NewTxSigHashes(tx.UnsignedTx, prevoutFetcher)

	for inputIndex, sig := range sigsToVerify {
		msgHash, err := txscript.CalcTapscriptSignaturehash(
			txSigHashes,
			sig.SigHash,
			tx.UnsignedTx,
			inputIndex,
			prevoutFetcher,
			tapLeaves[inputIndex],
		)
		if err != nil {
			return fmt.Errorf("failed to calculate tapscript signature hash: %w", err)
		}

		signature, err := schnorr.ParseSignature(sig.Signature)
		if err != nil {
			return fmt.Errorf("failed to parse signature: %w", err)
		}

		if !signature.Verify(msgHash, pubkey) {
			return fmt.Errorf("input %d: invalid signature", inputIndex)
		}
	}

	return nil
}
