package e2e_test

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
	"time"

	pb "github.com/ArkLabsHQ/fulmine/api-spec/protobuf/gen/go/fulmine/v1"
	"github.com/ArkLabsHQ/fulmine/pkg/vhtlc"
	arklib "github.com/arkade-os/arkd/pkg/ark-lib"
	"github.com/arkade-os/arkd/pkg/ark-lib/intent"
	"github.com/arkade-os/arkd/pkg/ark-lib/tree"
	"github.com/arkade-os/arkd/pkg/ark-lib/txutils"
	arksdk "github.com/arkade-os/go-sdk"
	"github.com/arkade-os/go-sdk/client"
	grpcclient "github.com/arkade-os/go-sdk/client/grpc"
	"github.com/arkade-os/go-sdk/store"
	"github.com/arkade-os/go-sdk/types"
	"github.com/arkade-os/go-sdk/wallet"
	singlekeywallet "github.com/arkade-os/go-sdk/wallet/singlekey"
	inmemorystore "github.com/arkade-os/go-sdk/wallet/singlekey/store/inmemory"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/btcutil/psbt"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	"github.com/lightningnetwork/lnd/input"
	"github.com/stretchr/testify/require"
)

// TestSettleVHTLCByDelegateRefund tests the VHTLC delegate refund flow which is applicable in SWAP
// flow when user sends to VHTLC and if swap fails he can refund
// 1. Create a VHTLC between sender and receiver boltz's fulmine
// 2. Fund the VHTLC with offchain funds
// 3. Counterparty (VHTLC receiver) builds intent proof and partial forfeit
// 4. Fulmine acts as delegate to complete the refund settlement
// 5. Verify settlement completes and funds return to sender
//
// This tests the delegate pattern adapted from arkd's TestDelegateRefresh,
// where a third party (Fulmine) completes a batch session on behalf of
// the VTXO owner (counterparty) using a pre-signed intent and partial forfeit.
func TestSettleVHTLCByDelegateRefund(t *testing.T) {
	fulmineClient, err := newFulmineClient("localhost:7000")
	require.NoError(t, err)
	require.NotNil(t, fulmineClient)

	ctx := t.Context()

	info, err := fulmineClient.GetInfo(ctx, &pb.GetInfoRequest{})
	require.NoError(t, err)
	receiverPubKey := info.Pubkey
	require.NotEmpty(t, info.Pubkey)

	senderArkClient, _, senderPubKey, _ := setupArkSDK(t)
	_, offchain, boarding, _, err := senderArkClient.GetAddresses(ctx)
	require.NoError(t, err)
	err = faucet(ctx, strings.TrimSpace(boarding[0]), 0.001)
	require.NoError(t, err)
	time.Sleep(5 * time.Second)
	_, err = senderArkClient.Settle(ctx)
	require.NoError(t, err)

	preimage := make([]byte, 32)
	_, err = rand.Read(preimage)
	require.NoError(t, err)
	sha256Hash := sha256.Sum256(preimage)
	preimageHash := hex.EncodeToString(input.Ripemd160H(sha256Hash[:]))

	vhtlcReq := &pb.CreateVHTLCRequest{
		PreimageHash:   preimageHash,
		SenderPubkey:   hex.EncodeToString(senderPubKey.SerializeCompressed()),
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
	}
	vhtlcAddrInfo, err := fulmineClient.CreateVHTLC(ctx, vhtlcReq)
	require.NoError(t, err)

	senderBalance, err := senderArkClient.Balance(ctx)
	require.NoError(t, err)
	t.Logf("Sender Balance: %d", senderBalance.OffchainBalance.Total)
	senderOffchainBalanceInit := senderBalance.OffchainBalance.Total

	_, err = senderArkClient.SendOffChain(ctx, []types.Receiver{
		{
			To:     vhtlcAddrInfo.Address,
			Amount: 1000,
		},
	})
	require.NoError(t, err)

	vhtlcs, err := fulmineClient.ListVHTLC(ctx, &pb.ListVHTLCRequest{VhtlcId: vhtlcAddrInfo.GetId()})
	require.NoError(t, err)

	vhtlcVtxo := vhtlcs.GetVhtlcs()[0]
	t.Logf("funded VHTLC address with: %d", vhtlcVtxo.Amount)

	senderBalance, err = senderArkClient.Balance(ctx)
	require.NoError(t, err)
	t.Logf("Sender Balance after sending to VHTLC address: %d", senderBalance.OffchainBalance.Total)

	validAt := time.Now()
	intentMessage, err := intent.RegisterMessage{
		BaseMessage: intent.BaseMessage{
			Type: intent.IntentMessageTypeRegister,
		},
		ExpireAt:            validAt.Add(5 * time.Minute).Unix(),
		ValidAt:             validAt.Unix(),
		CosignersPublicKeys: []string{receiverPubKey},
	}.Encode()
	require.NoError(t, err)

	senderOffchainAddrStr := offchain[0]
	senderOffchainAddr, err := arklib.DecodeAddressV0(senderOffchainAddrStr)
	require.NoError(t, err)
	senderPkScript, err := senderOffchainAddr.GetPkScript()
	require.NoError(t, err)

	taprootTree := vhtlcAddrInfo.GetSwapTree()
	vhtlcScript, err := vhtlc.NewVhtlcScript(
		preimageHash,
		taprootTree.GetClaimLeaf().GetOutput(),
		taprootTree.GetRefundLeaf().GetOutput(),
		taprootTree.GetRefundWithoutBoltzLeaf().GetOutput(),
		taprootTree.GetUnilateralClaimLeaf().GetOutput(),
		taprootTree.GetUnilateralRefundLeaf().GetOutput(),
		taprootTree.GetUnilateralRefundWithoutBoltzLeaf().GetOutput(),
	)
	require.NoError(t, err)

	intentProof, err := buildDelegateIntentProof(
		t,
		ctx,
		senderArkClient,
		intentMessage,
		vhtlcVtxo,
		vhtlcAddrInfo.GetAddress(),
		vhtlcScript,
		senderPkScript,
	)
	require.NoError(t, err)

	cfg, err := senderArkClient.GetConfigData(ctx)
	require.NoError(t, err)
	forfeitOutputAddr, err := btcutil.DecodeAddress(cfg.ForfeitAddress, nil)
	require.NoError(t, err)

	forfeitOutputScript, err := txscript.PayToAddrScript(forfeitOutputAddr)
	require.NoError(t, err)

	partialForfeitTx, err := buildDelegatePartialForfeit(
		t,
		ctx,
		senderArkClient,
		vhtlcVtxo,
		vhtlcAddrInfo.GetAddress(),
		vhtlcScript,
		forfeitOutputScript,
		int64(cfg.Dust),
	)
	require.NoError(t, err)

	t.Log("Calling SettleVHTLC with delegate parameters...")
	settleResp, err := fulmineClient.SettleVHTLC(ctx, &pb.SettleVHTLCRequest{
		VhtlcId: vhtlcAddrInfo.GetId(),
		SettlementType: &pb.SettleVHTLCRequest_Refund{
			Refund: &pb.RefundPath{
				DelegateParams: &pb.DelegateRefundParams{
					SignedIntentProof: intentProof,
					IntentMessage:     intentMessage,
					PartialForfeitTx:  partialForfeitTx,
				},
			},
		},
	})
	require.NoError(t, err)
	t.Logf("Delegate refund successful: txid=%s", settleResp.GetTxid())

	time.Sleep(2 * time.Second)

	senderBalance, err = senderArkClient.Balance(ctx)
	require.NoError(t, err)
	require.Equal(t, senderOffchainBalanceInit, senderBalance.OffchainBalance.Total)
	t.Logf("Sender Balance after delegate refund: %d", senderBalance.OffchainBalance.Total)
}

func buildDelegateIntentProof(
	t *testing.T,
	ctx context.Context,
	senderArkClient arksdk.ArkClient,
	intentMessage string,
	vtxoToDelegate *pb.Vtxo,
	vhtlcAddress string,
	vhtlcScript *vhtlc.VHTLCScript,
	senderPkScript []byte,
) (string, error) {
	// Parse VHTLC outpoint from funded VHTLC
	vhtlcTxHash, err := chainhash.NewHashFromStr(vtxoToDelegate.Outpoint.GetTxid())
	require.NoError(t, err)

	vtxoToDelegateOutpoint := &wire.OutPoint{
		Hash:  *vhtlcTxHash,
		Index: vtxoToDelegate.Outpoint.GetVout(),
	}

	vhtlcAddr, err := arklib.DecodeAddressV0(vhtlcAddress)
	require.NoError(t, err)
	vhtlcPkScript, err := vhtlcAddr.GetPkScript()
	require.NoError(t, err)

	// Get CSV sequence from UnilateralClaimDelay
	opts := vhtlcScript.Opts()
	csvSequence, err := arklib.BIP68Sequence(opts.UnilateralClaimDelay)
	require.NoError(t, err)

	intentProof, err := intent.New(
		intentMessage,
		[]intent.Input{
			{
				OutPoint: vtxoToDelegateOutpoint,
				Sequence: csvSequence,
				WitnessUtxo: &wire.TxOut{
					Value:    int64(vtxoToDelegate.Amount),
					PkScript: vhtlcPkScript,
				},
			},
		},
		[]*wire.TxOut{
			{
				Value:    int64(vtxoToDelegate.Amount),
				PkScript: senderPkScript,
			},
		},
		0, // locktime - intent never expires
	)
	require.NoError(t, err)

	refundClaimTapscript, err := vhtlcScript.RefundTapscript(true)
	require.NoError(t, err)
	cb, err := refundClaimTapscript.ControlBlock.ToBytes()
	require.NoError(t, err)
	exitLeaf := &psbt.TaprootTapLeafScript{
		ControlBlock: cb,
		Script:       refundClaimTapscript.RevealedScript,
		LeafVersion:  txscript.BaseLeafVersion,
	}
	intentProof.Inputs[0].TaprootLeafScript = []*psbt.TaprootTapLeafScript{exitLeaf}
	intentProof.Inputs[1].TaprootLeafScript = []*psbt.TaprootTapLeafScript{exitLeaf}

	err = txutils.SetArkPsbtField(&intentProof.Packet, 1, txutils.VtxoTaprootTreeField, vhtlcScript.GetRevealedTapscripts())
	require.NoError(t, err)

	encodedIntentProof, err := intentProof.B64Encode()
	require.NoError(t, err)

	partialySignedProof, err := senderArkClient.SignTransaction(ctx, encodedIntentProof)
	require.NoError(t, err)

	return partialySignedProof, nil
}

func buildDelegatePartialForfeit(
	t *testing.T,
	ctx context.Context,
	senderArkClient arksdk.ArkClient,
	vhtlcVtxo *pb.Vtxo,
	vhtlcAddress string,
	vhtlcScript *vhtlc.VHTLCScript,
	forfeitOutputScript []byte,
	connectorAmount int64,
) (string, error) {
	vhtlcTxHash, err := chainhash.NewHashFromStr(vhtlcVtxo.Outpoint.GetTxid())
	require.NoError(t, err)

	vhtlcOutpoint := &wire.OutPoint{
		Hash:  *vhtlcTxHash,
		Index: vhtlcVtxo.Outpoint.GetVout(),
	}

	vhtlcAmount := int64(vhtlcVtxo.Amount)

	vhtlcAddr, err := arklib.DecodeAddressV0(vhtlcAddress)
	require.NoError(t, err)
	vhtlcPkScript, err := vhtlcAddr.GetPkScript()
	require.NoError(t, err)

	forfeitPtx, err := tree.BuildForfeitTxWithOutput(
		[]*wire.OutPoint{vhtlcOutpoint},
		[]uint32{wire.MaxTxInSequenceNum},
		[]*wire.TxOut{
			{
				Value:    vhtlcAmount,
				PkScript: vhtlcPkScript,
			},
		},
		&wire.TxOut{
			Value:    vhtlcAmount + connectorAmount,
			PkScript: forfeitOutputScript,
		},
		0,
	)
	require.NoError(t, err)

	updater, err := psbt.NewUpdater(forfeitPtx)
	require.NoError(t, err)

	err = updater.AddInSighashType(txscript.SigHashAnyOneCanPay|txscript.SigHashAll, 0)
	require.NoError(t, err)

	refundTapscript, err := vhtlcScript.RefundTapscript(true)
	require.NoError(t, err)

	controlBlockBytes, err := refundTapscript.ControlBlock.ToBytes()
	require.NoError(t, err)

	updater.Upsbt.Inputs[0].TaprootLeafScript = []*psbt.TaprootTapLeafScript{
		{
			ControlBlock: controlBlockBytes,
			Script:       refundTapscript.RevealedScript,
			LeafVersion:  txscript.BaseLeafVersion,
		},
	}

	b64partialForfeitTx, err := updater.Upsbt.B64Encode()
	require.NoError(t, err)

	signedPartialForfeitTx, err := senderArkClient.SignTransaction(ctx, b64partialForfeitTx)
	require.NoError(t, err)

	return signedPartialForfeitTx, nil
}

func setupArkSDK(
	t *testing.T,
) (arksdk.ArkClient, wallet.WalletService, *btcec.PublicKey, client.TransportClient) {
	serverUrl := "localhost:7070"
	password := "pass"

	appDataStore, err := store.NewStore(store.Config{
		ConfigStoreType:  types.InMemoryStore,
		AppDataStoreType: types.KVStore,
	})
	require.NoError(t, err)

	client, err := arksdk.NewArkClient(appDataStore)
	require.NoError(t, err)

	walletStore, err := inmemorystore.NewWalletStore()
	require.NoError(t, err)
	require.NotNil(t, walletStore)

	wallet, err := singlekeywallet.NewBitcoinWallet(appDataStore.ConfigStore(), walletStore)
	require.NoError(t, err)

	privkey, err := btcec.NewPrivateKey()
	require.NoError(t, err)

	privkeyHex := hex.EncodeToString(privkey.Serialize())

	err = client.InitWithWallet(context.Background(), arksdk.InitWithWalletArgs{
		Wallet:     wallet,
		ClientType: arksdk.GrpcClient,
		ServerUrl:  serverUrl,
		Password:   password,
		Seed:       privkeyHex,
	})
	require.NoError(t, err)

	err = client.Unlock(context.Background(), password)
	require.NoError(t, err)

	grpcClient, err := grpcclient.NewClient(serverUrl)
	require.NoError(t, err)

	return client, wallet, privkey.PubKey(), grpcClient
}
