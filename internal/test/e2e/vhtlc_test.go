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

func TestVHTLC(t *testing.T) {
	f, err := newFulmineClient("localhost:7000")
	require.NoError(t, err)
	require.NotNil(t, f)

	ctx := t.Context()
	// For sake of simplicity, in this test sender = receiver to test both
	// funding and claiming the VHTLC via API
	info, err := f.GetInfo(ctx, &pb.GetInfoRequest{})
	require.NoError(t, err)
	require.NotEmpty(t, info)

	// Create the VHTLC
	preimage := make([]byte, 32)
	_, err = rand.Read(preimage)
	require.NoError(t, err)
	sha256Hash := sha256.Sum256(preimage)
	preimageHash := hex.EncodeToString(input.Ripemd160H(sha256Hash[:]))

	req := &pb.CreateVHTLCRequest{
		PreimageHash:   preimageHash,
		ReceiverPubkey: info.GetPubkey(),
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
	vhtlc, err := f.CreateVHTLC(ctx, req)
	require.NoError(t, err)
	require.NotEmpty(t, vhtlc.Address)
	require.NotEmpty(t, vhtlc.ClaimPubkey)
	require.NotEmpty(t, vhtlc.RefundPubkey)
	require.NotEmpty(t, vhtlc.ServerPubkey)

	// Ensure duplication are not allowed
	{
		vhtlc, err := f.CreateVHTLC(ctx, req)
		require.Error(t, err)
		require.Nil(t, vhtlc)
	}

	// Fund the VHTLC
	_, err = f.SendOffChain(ctx, &pb.SendOffChainRequest{
		Address: vhtlc.Address,
		Amount:  1000,
	})
	require.NoError(t, err)

	// Get the VHTLC
	vhtlcs, err := f.ListVHTLC(ctx, &pb.ListVHTLCRequest{VhtlcId: vhtlc.GetId()})
	require.NoError(t, err)
	require.NotNil(t, vhtlcs)
	require.NotEmpty(t, vhtlcs.GetVhtlcs())

	// Claim the VHTLC
	redeemTxid, err := f.ClaimVHTLC(ctx, &pb.ClaimVHTLCRequest{
		VhtlcId:  vhtlc.Id,
		Preimage: hex.EncodeToString(preimage),
	})
	require.NoError(t, err)
	require.NotNil(t, redeemTxid)
	require.NotEmpty(t, redeemTxid.GetRedeemTxid())
}

// TestClaimVhtlcSettlement tests the VHTLC claim path integration:
// 1. Create a VHTLC (reverse submarine swap scenario)
// 2. Send offchain funds to the VHTLC address
// 3. Invoke SettleVHTLC with ClaimPath and the preimage
// 4. Verify settlement succeeds and funds are received
func TestClaimVhtlcSettlement(t *testing.T) {
	f, err := newFulmineClient("localhost:7000")
	require.NoError(t, err)
	require.NotNil(t, f)

	ctx := t.Context()

	// Get initial balance
	balanceBefore, err := f.GetBalance(ctx, &pb.GetBalanceRequest{})
	require.NoError(t, err)
	require.NotNil(t, balanceBefore)
	t.Logf("Initial balance: %d sats", balanceBefore.GetAmount())

	// For sake of simplicity, in this test sender = receiver
	info, err := f.GetInfo(ctx, &pb.GetInfoRequest{})
	require.NoError(t, err)
	require.NotEmpty(t, info)

	// Create the VHTLC
	preimage := make([]byte, 32)
	_, err = rand.Read(preimage)
	require.NoError(t, err)
	sha256Hash := sha256.Sum256(preimage)
	preimageHash := hex.EncodeToString(input.Ripemd160H(sha256Hash[:]))

	req := &pb.CreateVHTLCRequest{
		PreimageHash:   preimageHash,
		ReceiverPubkey: info.GetPubkey(),
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
	vhtlc, err := f.CreateVHTLC(ctx, req)
	require.NoError(t, err)
	require.NotEmpty(t, vhtlc.Address)
	require.NotEmpty(t, vhtlc.Id)
	t.Logf("Created VHTLC: id=%s, address=%s", vhtlc.Id, vhtlc.Address)

	// Fund the VHTLC with 1000 sats
	fundAmount := uint64(1000)
	sendResp, err := f.SendOffChain(ctx, &pb.SendOffChainRequest{
		Address: vhtlc.Address,
		Amount:  fundAmount,
	})
	require.NoError(t, err)
	t.Logf("Funded VHTLC with %d sats, txid: %s", fundAmount, sendResp.GetTxid())

	// Verify VHTLC has funds
	vhtlcs, err := f.ListVHTLC(ctx, &pb.ListVHTLCRequest{VhtlcId: vhtlc.GetId()})
	require.NoError(t, err)
	require.NotNil(t, vhtlcs)
	require.NotEmpty(t, vhtlcs.GetVhtlcs())
	require.Greater(t, int(vhtlcs.GetVhtlcs()[0].GetAmount()), 0, "VHTLC should have funds")
	t.Logf("VHTLC has %d sats", vhtlcs.GetVhtlcs()[0].GetAmount())

	// Claim the VHTLC using the SettleVHTLC RPC with ClaimPath
	t.Log("Claiming VHTLC with preimage via SettleVHTLC RPC...")
	settleResp, err := f.SettleVHTLC(ctx, &pb.SettleVHTLCRequest{
		VhtlcId: vhtlc.Id,
		SettlementType: &pb.SettleVHTLCRequest_Claim{
			Claim: &pb.ClaimPath{
				Preimage: hex.EncodeToString(preimage),
			},
		},
	})
	require.NoError(t, err, "SettleVHTLC with claim path should succeed")
	require.NotNil(t, settleResp)
	require.NotEmpty(t, settleResp.GetTxid())
	t.Logf("Claim successful: txid=%s", settleResp.GetTxid())

	// Wait for settlement to complete - may need longer wait for batch to finalize
	t.Log("Waiting for batch settlement to finalize...")
	time.Sleep(5 * time.Second)

	// Verify balance changed appropriately
	balanceAfter, err := f.GetBalance(ctx, &pb.GetBalanceRequest{})
	require.NoError(t, err)
	require.NotNil(t, balanceAfter)
	t.Logf("Final balance: %d sats", balanceAfter.GetAmount())

	// Calculate balance difference
	balanceDiff := int64(balanceAfter.GetAmount()) - int64(balanceBefore.GetAmount())
	t.Logf("Balance change: %d sats", balanceDiff)

	// The claim should result in a positive balance change or at worst a small negative (fees)
	// In a real claim scenario, funds move from VHTLC back to the user
	require.GreaterOrEqual(t, balanceDiff, int64(-200),
		"Balance should not decrease by more than 200 sats (accounting for fees)")

	t.Logf("VHTLC claim settlement test completed successfully")
}

// TestRefundVhtlcSettlement tests the VHTLC refund path integration, this can be used by Boltz Fulmine when
// they want to refund in reverse SWAP without receiver if swap fails
// 1. Create a VHTLC (submarine swap scenario)
// 2. Send offchain funds to the VHTLC address
// 3. Invoke SettleVHTLC with RefundPath (2-of-2 multisig: Sender+Server)
// 4. Verify refund succeeds and funds are returned
//
// Note: This test uses a very short refund locktime to avoid waiting
func TestRefundVhtlcSettlement(t *testing.T) {
	fulmineClient, err := newFulmineClient("localhost:7000")
	require.NoError(t, err)
	require.NotNil(t, fulmineClient)

	ctx := t.Context()

	// Get initial balance
	balanceBefore, err := fulmineClient.GetBalance(ctx, &pb.GetBalanceRequest{})
	require.NoError(t, err)
	require.NotNil(t, balanceBefore)
	t.Logf("Initial balance: %d sats", balanceBefore.GetAmount())

	// For this test, sender = receiver to simulate failed swap scenario
	info, err := fulmineClient.GetInfo(ctx, &pb.GetInfoRequest{})
	require.NoError(t, err)
	require.NotEmpty(t, info)

	// Create the VHTLC (simulating a submarine swap that will fail)
	// Use past refund locktime to allow immediate refund
	preimage := make([]byte, 32)
	_, err = rand.Read(preimage)
	require.NoError(t, err)
	sha256Hash := sha256.Sum256(preimage)
	preimageHash := hex.EncodeToString(input.Ripemd160H(sha256Hash[:]))

	receiverPrivKey, err := btcec.NewPrivateKey()
	require.NoError(t, err)

	// Use a timestamp far in the past (Jan 1, 2020) so CLTV is already expired in regtest
	// This ensures the refund locktime is before the blockchain's current block time
	pastRefundLocktime := uint32(1577836800) // Jan 1, 2020 00:00:00 UTC
	req := &pb.CreateVHTLCRequest{
		PreimageHash:   preimageHash,
		ReceiverPubkey: hex.EncodeToString(receiverPrivKey.PubKey().SerializeCompressed()),
		RefundLocktime: pastRefundLocktime,
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
			Value: 512,
		},
	}
	vhtlc, err := fulmineClient.CreateVHTLC(ctx, req)
	require.NoError(t, err)

	fundAmount := uint64(1000)
	sendResp, err := fulmineClient.SendOffChain(ctx, &pb.SendOffChainRequest{
		Address: vhtlc.Address,
		Amount:  fundAmount,
	})
	require.NoError(t, err)
	t.Logf("Funded VHTLC with %d sats, txid: %s", fundAmount, sendResp.GetTxid())

	// Verify VHTLC has funds
	vhtlcs, err := fulmineClient.ListVHTLC(ctx, &pb.ListVHTLCRequest{VhtlcId: vhtlc.GetId()})
	require.NoError(t, err)
	require.NotNil(t, vhtlcs)
	require.NotEmpty(t, vhtlcs.GetVhtlcs())
	require.Greater(t, int(vhtlcs.GetVhtlcs()[0].GetAmount()), 0, "VHTLC should have funds")
	t.Logf("VHTLC has %d sats", vhtlcs.GetVhtlcs()[0].GetAmount())

	settleResp, err := fulmineClient.SettleVHTLC(ctx, &pb.SettleVHTLCRequest{
		VhtlcId: vhtlc.Id,
		SettlementType: &pb.SettleVHTLCRequest_Refund{
			Refund: &pb.RefundPath{},
		},
	})
	require.NoError(t, err, "SettleVHTLC with refund path should succeed")
	require.NotNil(t, settleResp)
	require.NotEmpty(t, settleResp.GetTxid())
	t.Logf("Refund successful: txid=%s", settleResp.GetTxid())

	time.Sleep(2 * time.Second)

	// Verify balance returned to approximately initial value (minus small fees)
	balanceAfter, err := fulmineClient.GetBalance(ctx, &pb.GetBalanceRequest{})
	require.NoError(t, err)
	require.NotNil(t, balanceAfter)
	t.Logf("Final balance: %d sats", balanceAfter.GetAmount())

	// Balance should be close to initial (allowing for small fees)
	balanceDiff := int64(balanceAfter.GetAmount()) - int64(balanceBefore.GetAmount())
	t.Logf("Balance change: %d sats", balanceDiff)

	require.GreaterOrEqual(t, balanceDiff, int64(-200),
		"Balance should be close to initial after refund (small fees allowed)")

	t.Logf("VHTLC refund settlement test completed successfully")
}

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
