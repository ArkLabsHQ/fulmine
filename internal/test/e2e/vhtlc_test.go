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
	clientTypes "github.com/arkade-os/arkd/pkg/client-lib/types"
	arksdk "github.com/arkade-os/go-sdk"
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

// TestClaimVHTLCWithOutpoint funds the same VHTLC address twice with different
// amounts, then claims only the second VTXO by specifying its outpoint. It
// verifies that the targeted VTXO is claimed and the first remains untouched.
func TestClaimVHTLCWithOutpoint(t *testing.T) {
	f, err := newFulmineClient("localhost:7000")
	require.NoError(t, err)
	require.NotNil(t, f)

	ctx := t.Context()

	info, err := f.GetInfo(ctx, &pb.GetInfoRequest{})
	require.NoError(t, err)
	require.NotEmpty(t, info)

	// Create a VHTLC (sender = receiver for simplicity)
	preimage := make([]byte, 32)
	_, err = rand.Read(preimage)
	require.NoError(t, err)
	sha256Hash := sha256.Sum256(preimage)
	preimageHash := hex.EncodeToString(input.Ripemd160H(sha256Hash[:]))

	vhtlcResp, err := f.CreateVHTLC(ctx, &pb.CreateVHTLCRequest{
		PreimageHash:   preimageHash,
		ReceiverPubkey: info.GetPubkey(),
		// Keep the collaborative refund-without-receiver path immediately spendable.
		// The test is about pending finalization, not waiting for refund expiry.
		RefundLocktime: uint32(1577836800),
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
	})
	require.NoError(t, err)
	require.NotEmpty(t, vhtlcResp.Address)

	// Fund the VHTLC address twice with different amounts
	_, err = f.SendOffChain(ctx, &pb.SendOffChainRequest{
		Address: vhtlcResp.Address,
		Amount:  1000,
	})
	require.NoError(t, err)

	_, err = f.SendOffChain(ctx, &pb.SendOffChainRequest{
		Address: vhtlcResp.Address,
		Amount:  2000,
	})
	require.NoError(t, err)

	// List VHTLCs and verify there are two VTXOs
	vhtlcs, err := f.ListVHTLC(ctx, &pb.ListVHTLCRequest{VhtlcId: vhtlcResp.GetId()})
	require.NoError(t, err)
	require.Len(t, vhtlcs.GetVhtlcs(), 2, "expected exactly 2 VTXOs at the VHTLC address")

	// Identify the 2000-sat VTXO and the 1000-sat VTXO
	var targetVtxo, otherVtxo *pb.Vtxo
	for _, v := range vhtlcs.GetVhtlcs() {
		if v.Amount == 2000 {
			targetVtxo = v
		} else if v.Amount == 1000 {
			otherVtxo = v
		}
	}
	require.NotNil(t, targetVtxo, "expected a 2000-sat VTXO")
	require.NotNil(t, otherVtxo, "expected a 1000-sat VTXO")

	// Claim only the 2000-sat VTXO by specifying its outpoint
	redeemTxid, err := f.ClaimVHTLC(ctx, &pb.ClaimVHTLCRequest{
		VhtlcId:  vhtlcResp.Id,
		Preimage: hex.EncodeToString(preimage),
		Outpoint: &pb.Input{
			Txid: targetVtxo.Outpoint.GetTxid(),
			Vout: targetVtxo.Outpoint.GetVout(),
		},
	})
	require.NoError(t, err)
	require.NotNil(t, redeemTxid)
	require.NotEmpty(t, redeemTxid.GetRedeemTxid())

	// Verify the 1000-sat VTXO still exists (unclaimed)
	remainingVhtlcs, err := f.ListVHTLC(ctx, &pb.ListVHTLCRequest{VhtlcId: vhtlcResp.GetId()})
	require.NoError(t, err)

	var foundOther bool
	for _, v := range remainingVhtlcs.GetVhtlcs() {
		if v.Outpoint.GetTxid() == otherVtxo.Outpoint.GetTxid() &&
			v.Outpoint.GetVout() == otherVtxo.Outpoint.GetVout() &&
			!v.IsSpent {
			foundOther = true
		}
	}
	require.True(t, foundOther, "the 1000-sat VTXO should still exist and be unspent")
}

// TestClaimVHTLCOldestVtxo funds the same VHTLC address 3 times with different
// amounts, oldest vtxo should be claimed
func TestClaimVHTLCOldestVtxo(t *testing.T) {
	f, err := newFulmineClient("localhost:7000")
	require.NoError(t, err)
	require.NotNil(t, f)

	ctx := t.Context()

	info, err := f.GetInfo(ctx, &pb.GetInfoRequest{})
	require.NoError(t, err)
	require.NotEmpty(t, info)

	// Create a VHTLC (sender = receiver for simplicity)
	preimage := make([]byte, 32)
	_, err = rand.Read(preimage)
	require.NoError(t, err)
	sha256Hash := sha256.Sum256(preimage)
	preimageHash := hex.EncodeToString(input.Ripemd160H(sha256Hash[:]))

	vhtlcResp, err := f.CreateVHTLC(ctx, &pb.CreateVHTLCRequest{
		PreimageHash:   preimageHash,
		ReceiverPubkey: info.GetPubkey(),
		RefundLocktime: uint32(1577836800),
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
	})
	require.NoError(t, err)
	require.NotEmpty(t, vhtlcResp.Address)

	// Fund the VHTLC address twice with different amounts
	_, err = f.SendOffChain(ctx, &pb.SendOffChainRequest{
		Address: vhtlcResp.Address,
		Amount:  1000,
	})
	require.NoError(t, err)

	time.Sleep(1 * time.Second)

	_, err = f.SendOffChain(ctx, &pb.SendOffChainRequest{
		Address: vhtlcResp.Address,
		Amount:  2000,
	})
	require.NoError(t, err)

	time.Sleep(1 * time.Second)

	_, err = f.SendOffChain(ctx, &pb.SendOffChainRequest{
		Address: vhtlcResp.Address,
		Amount:  3000,
	})
	require.NoError(t, err)

	// Claim only the 2000-sat VTXO by specifying its outpoint
	redeemTxid, err := f.ClaimVHTLC(ctx, &pb.ClaimVHTLCRequest{
		VhtlcId:  vhtlcResp.Id,
		Preimage: hex.EncodeToString(preimage),
	})
	require.NoError(t, err)
	require.NotNil(t, redeemTxid)
	require.NotEmpty(t, redeemTxid.GetRedeemTxid())

	vtxos, err := f.ListVHTLC(ctx, &pb.ListVHTLCRequest{VhtlcId: vhtlcResp.GetId()})
	require.NoError(t, err)

	for _, v := range vtxos.GetVhtlcs() {
		if v.Amount == 1000 {
			require.True(t, v.IsSpent)
		} else {
			require.False(t, v.IsSpent)
		}
	}
}

func TestSettleVHTLCClaimWithOutpoint(t *testing.T) {
	f, err := newFulmineClient("localhost:7000")
	require.NoError(t, err)
	require.NotNil(t, f)

	ctx := t.Context()

	info, err := f.GetInfo(ctx, &pb.GetInfoRequest{})
	require.NoError(t, err)
	require.NotEmpty(t, info)

	preimage := make([]byte, 32)
	_, err = rand.Read(preimage)
	require.NoError(t, err)
	sha256Hash := sha256.Sum256(preimage)
	preimageHash := hex.EncodeToString(input.Ripemd160H(sha256Hash[:]))

	vhtlcResp, err := f.CreateVHTLC(ctx, &pb.CreateVHTLCRequest{
		PreimageHash:   preimageHash,
		ReceiverPubkey: info.GetPubkey(),
		RefundLocktime: uint32(1577836800),
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
	})
	require.NoError(t, err)

	_, err = f.SendOffChain(ctx, &pb.SendOffChainRequest{
		Address: vhtlcResp.Address,
		Amount:  1000,
	})
	require.NoError(t, err)

	_, err = f.SendOffChain(ctx, &pb.SendOffChainRequest{
		Address: vhtlcResp.Address,
		Amount:  2000,
	})
	require.NoError(t, err)

	vhtlcs, err := f.ListVHTLC(ctx, &pb.ListVHTLCRequest{VhtlcId: vhtlcResp.GetId()})
	require.NoError(t, err)

	targetVtxo, otherVtxo := findVHTLCsByAmount(t, vhtlcs.GetVhtlcs(), 2000, 1000)

	settleResp, err := f.SettleVHTLC(ctx, &pb.SettleVHTLCRequest{
		VhtlcId: vhtlcResp.Id,
		Outpoint: &pb.Input{
			Txid: targetVtxo.Outpoint.GetTxid(),
			Vout: targetVtxo.Outpoint.GetVout(),
		},
		SettlementType: &pb.SettleVHTLCRequest_Claim{
			Claim: &pb.ClaimPath{
				Preimage: hex.EncodeToString(preimage),
			},
		},
	})
	require.NoError(t, err)
	require.NotNil(t, settleResp)
	require.NotEmpty(t, settleResp.GetTxid())

	time.Sleep(2 * time.Second)

	updatedVHTLCs, err := f.ListVHTLC(ctx, &pb.ListVHTLCRequest{VhtlcId: vhtlcResp.GetId()})
	require.NoError(t, err)

	requireVHTLCSpentState(t, updatedVHTLCs.GetVhtlcs(), targetVtxo, true)
	requireVHTLCSpentState(t, updatedVHTLCs.GetVhtlcs(), otherVtxo, false)
}

// TestClaimVhtlcSettlement tests the VHTLC claim path integration
func TestClaimVhtlcSettlement(t *testing.T) {
	f, err := newFulmineClient("localhost:7000")
	require.NoError(t, err)

	ctx := t.Context()

	// Get initial balance
	balanceBefore, err := f.GetBalance(ctx, &pb.GetBalanceRequest{})
	require.NoError(t, err)
	require.NotNil(t, balanceBefore)

	info, err := f.GetInfo(ctx, &pb.GetInfoRequest{})
	require.NoError(t, err)

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

	fundAmount := uint64(1000)
	sendResp, err := f.SendOffChain(ctx, &pb.SendOffChainRequest{
		Address: vhtlc.Address,
		Amount:  fundAmount,
	})
	require.NoError(t, err)
	require.NotNil(t, sendResp)

	// Verify VHTLC has funds
	vhtlcs, err := f.ListVHTLC(ctx, &pb.ListVHTLCRequest{VhtlcId: vhtlc.GetId()})
	require.NoError(t, err)
	require.Len(t, vhtlcs.Vhtlcs, 1)

	// claim VHTLC
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

	time.Sleep(1 * time.Second)

	// Verify balance changed appropriately
	balanceAfter, err := f.GetBalance(ctx, &pb.GetBalanceRequest{})
	require.NoError(t, err)
	require.Equal(t, balanceBefore.Amount, balanceAfter.Amount)
}

func TestRefundVHTLCWithoutReceiverWithOutpoint(t *testing.T) {
	fulmineClient, err := newFulmineClient("localhost:7000")
	require.NoError(t, err)
	require.NotNil(t, fulmineClient)

	ctx := t.Context()

	preimage := make([]byte, 32)
	_, err = rand.Read(preimage)
	require.NoError(t, err)
	sha256Hash := sha256.Sum256(preimage)
	preimageHash := hex.EncodeToString(input.Ripemd160H(sha256Hash[:]))

	receiverPrivKey, err := btcec.NewPrivateKey()
	require.NoError(t, err)

	vhtlc, err := fulmineClient.CreateVHTLC(ctx, &pb.CreateVHTLCRequest{
		PreimageHash:   preimageHash,
		ReceiverPubkey: hex.EncodeToString(receiverPrivKey.PubKey().SerializeCompressed()),
		RefundLocktime: uint32(1577836800),
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
	})
	require.NoError(t, err)

	_, err = fulmineClient.SendOffChain(ctx, &pb.SendOffChainRequest{
		Address: vhtlc.Address,
		Amount:  1000,
	})
	require.NoError(t, err)

	_, err = fulmineClient.SendOffChain(ctx, &pb.SendOffChainRequest{
		Address: vhtlc.Address,
		Amount:  2000,
	})
	require.NoError(t, err)

	vhtlcs, err := fulmineClient.ListVHTLC(ctx, &pb.ListVHTLCRequest{VhtlcId: vhtlc.GetId()})
	require.NoError(t, err)

	targetVtxo, otherVtxo := findVHTLCsByAmount(t, vhtlcs.GetVhtlcs(), 2000, 1000)

	refundResp, err := fulmineClient.RefundVHTLCWithoutReceiver(
		ctx,
		&pb.RefundVHTLCWithoutReceiverRequest{
			VhtlcId: vhtlc.Id,
			Outpoint: &pb.Input{
				Txid: targetVtxo.Outpoint.GetTxid(),
				Vout: targetVtxo.Outpoint.GetVout(),
			},
		},
	)
	require.NoError(t, err)
	require.NotNil(t, refundResp)
	require.NotEmpty(t, refundResp.GetRedeemTxid())

	time.Sleep(2 * time.Second)

	updatedVHTLCs, err := fulmineClient.ListVHTLC(ctx, &pb.ListVHTLCRequest{VhtlcId: vhtlc.GetId()})
	require.NoError(t, err)

	requireVHTLCSpentState(t, updatedVHTLCs.GetVhtlcs(), targetVtxo, true)
	requireVHTLCSpentState(t, updatedVHTLCs.GetVhtlcs(), otherVtxo, false)
}

// TestRefundVhtlcSettlement tests the VHTLC refund path integration, this can be used by Boltz Fulmine when
// they want to refund in reverse SWAP without receiver if swap fails
func TestRefundVhtlcSettlement(t *testing.T) {
	fulmineClient, err := newFulmineClient("localhost:7000")
	require.NoError(t, err)

	ctx := t.Context()

	balanceBefore, err := fulmineClient.GetBalance(ctx, &pb.GetBalanceRequest{})
	require.NoError(t, err)

	info, err := fulmineClient.GetInfo(ctx, &pb.GetInfoRequest{})
	require.NoError(t, err)
	require.NotEmpty(t, info)

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
	require.NotNil(t, sendResp)

	// Verify VHTLC has funds
	vhtlcs, err := fulmineClient.ListVHTLC(ctx, &pb.ListVHTLCRequest{VhtlcId: vhtlc.GetId()})
	require.NoError(t, err)
	require.Len(t, vhtlcs.Vhtlcs, 1)

	settleResp, err := fulmineClient.SettleVHTLC(ctx, &pb.SettleVHTLCRequest{
		VhtlcId: vhtlc.Id,
		SettlementType: &pb.SettleVHTLCRequest_Refund{
			Refund: &pb.RefundPath{},
		},
	})
	require.NoError(t, err, "SettleVHTLC with refund path should succeed")
	require.NotNil(t, settleResp)

	time.Sleep(2 * time.Second)

	// Verify balance returned to approximately initial value (minus small fees)
	balanceAfter, err := fulmineClient.GetBalance(ctx, &pb.GetBalanceRequest{})
	require.NoError(t, err)
	require.NotNil(t, balanceAfter)
	require.Equal(t, balanceBefore.Amount, balanceAfter.Amount)
}

func TestSettleVHTLCRefundWithOutpoint(t *testing.T) {
	fulmineClient, err := newFulmineClient("localhost:7000")
	require.NoError(t, err)
	require.NotNil(t, fulmineClient)

	ctx := t.Context()

	preimage := make([]byte, 32)
	_, err = rand.Read(preimage)
	require.NoError(t, err)
	sha256Hash := sha256.Sum256(preimage)
	preimageHash := hex.EncodeToString(input.Ripemd160H(sha256Hash[:]))

	receiverPrivKey, err := btcec.NewPrivateKey()
	require.NoError(t, err)

	vhtlc, err := fulmineClient.CreateVHTLC(ctx, &pb.CreateVHTLCRequest{
		PreimageHash:   preimageHash,
		ReceiverPubkey: hex.EncodeToString(receiverPrivKey.PubKey().SerializeCompressed()),
		RefundLocktime: uint32(1577836800),
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
	})
	require.NoError(t, err)

	_, err = fulmineClient.SendOffChain(ctx, &pb.SendOffChainRequest{
		Address: vhtlc.Address,
		Amount:  1000,
	})
	require.NoError(t, err)

	_, err = fulmineClient.SendOffChain(ctx, &pb.SendOffChainRequest{
		Address: vhtlc.Address,
		Amount:  2000,
	})
	require.NoError(t, err)

	vhtlcs, err := fulmineClient.ListVHTLC(ctx, &pb.ListVHTLCRequest{VhtlcId: vhtlc.GetId()})
	require.NoError(t, err)

	targetVtxo, otherVtxo := findVHTLCsByAmount(t, vhtlcs.GetVhtlcs(), 2000, 1000)

	settleResp, err := fulmineClient.SettleVHTLC(ctx, &pb.SettleVHTLCRequest{
		VhtlcId: vhtlc.Id,
		Outpoint: &pb.Input{
			Txid: targetVtxo.Outpoint.GetTxid(),
			Vout: targetVtxo.Outpoint.GetVout(),
		},
		SettlementType: &pb.SettleVHTLCRequest_Refund{
			Refund: &pb.RefundPath{},
		},
	})
	require.NoError(t, err)
	require.NotNil(t, settleResp)
	require.NotEmpty(t, settleResp.GetTxid())

	time.Sleep(2 * time.Second)

	updatedVHTLCs, err := fulmineClient.ListVHTLC(ctx, &pb.ListVHTLCRequest{VhtlcId: vhtlc.GetId()})
	require.NoError(t, err)

	requireVHTLCSpentState(t, updatedVHTLCs.GetVhtlcs(), targetVtxo, true)
	requireVHTLCSpentState(t, updatedVHTLCs.GetVhtlcs(), otherVtxo, false)
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

	senderArkClient, senderPubKey, _ := setupArkSDKwithPublicKey(t)

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
		PreimageHash: preimageHash,
		SenderPubkey: hex.EncodeToString(senderPubKey.SerializeCompressed()),
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
	senderOffchainBalanceInit := senderBalance.OffchainBalance.Total

	_, err = senderArkClient.SendOffChain(ctx, []clientTypes.Receiver{
		{
			To:     vhtlcAddrInfo.Address,
			Amount: 1000,
		},
	})
	require.NoError(t, err)

	vhtlcs, err := fulmineClient.ListVHTLC(ctx, &pb.ListVHTLCRequest{VhtlcId: vhtlcAddrInfo.GetId()})
	require.NoError(t, err)

	vhtlcVtxo := vhtlcs.GetVhtlcs()[0]

	senderBalance, err = senderArkClient.Balance(ctx)
	require.NoError(t, err)

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
	require.NotNil(t, settleResp)

	time.Sleep(2 * time.Second)

	senderBalance, err = senderArkClient.Balance(ctx)
	require.NoError(t, err)
	require.Equal(t, senderOffchainBalanceInit, senderBalance.OffchainBalance.Total)
}

func TestSettleVHTLCByDelegateRefundWithOutpoint(t *testing.T) {
	fulmineClient, err := newFulmineClient("localhost:7000")
	require.NoError(t, err)
	require.NotNil(t, fulmineClient)

	ctx := t.Context()

	info, err := fulmineClient.GetInfo(ctx, &pb.GetInfoRequest{})
	require.NoError(t, err)
	receiverPubKey := info.Pubkey
	require.NotEmpty(t, info.Pubkey)

	senderArkClient, senderPubKey, _ := setupArkSDKwithPublicKey(t)

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
		PreimageHash: preimageHash,
		SenderPubkey: hex.EncodeToString(senderPubKey.SerializeCompressed()),
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

	_, err = senderArkClient.SendOffChain(ctx, []clientTypes.Receiver{{
		To:     vhtlcAddrInfo.Address,
		Amount: 1000,
	}})
	require.NoError(t, err)

	_, err = senderArkClient.SendOffChain(ctx, []clientTypes.Receiver{{
		To:     vhtlcAddrInfo.Address,
		Amount: 2000,
	}})
	require.NoError(t, err)

	vhtlcs, err := fulmineClient.ListVHTLC(ctx, &pb.ListVHTLCRequest{VhtlcId: vhtlcAddrInfo.GetId()})
	require.NoError(t, err)

	targetVtxo, otherVtxo := findVHTLCsByAmount(t, vhtlcs.GetVhtlcs(), 2000, 1000)

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
		targetVtxo,
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
		targetVtxo,
		vhtlcAddrInfo.GetAddress(),
		vhtlcScript,
		forfeitOutputScript,
		int64(cfg.Dust),
	)
	require.NoError(t, err)

	settleResp, err := fulmineClient.SettleVHTLC(ctx, &pb.SettleVHTLCRequest{
		VhtlcId: vhtlcAddrInfo.GetId(),
		Outpoint: &pb.Input{
			Txid: targetVtxo.Outpoint.GetTxid(),
			Vout: targetVtxo.Outpoint.GetVout(),
		},
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
	require.NotNil(t, settleResp)
	require.NotEmpty(t, settleResp.GetTxid())

	time.Sleep(2 * time.Second)

	updatedVHTLCs, err := fulmineClient.ListVHTLC(ctx, &pb.ListVHTLCRequest{VhtlcId: vhtlcAddrInfo.GetId()})
	require.NoError(t, err)

	requireVHTLCSpentState(t, updatedVHTLCs.GetVhtlcs(), targetVtxo, true)
	requireVHTLCSpentState(t, updatedVHTLCs.GetVhtlcs(), otherVtxo, false)
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

func findVHTLCsByAmount(
	t *testing.T, vhtlcs []*pb.Vtxo, targetAmount, otherAmount uint64,
) (*pb.Vtxo, *pb.Vtxo) {
	t.Helper()

	var targetVtxo, otherVtxo *pb.Vtxo
	for _, v := range vhtlcs {
		switch v.Amount {
		case targetAmount:
			targetVtxo = v
		case otherAmount:
			otherVtxo = v
		}
	}

	require.NotNil(t, targetVtxo, "expected target VTXO with amount %d", targetAmount)
	require.NotNil(t, otherVtxo, "expected other VTXO with amount %d", otherAmount)

	return targetVtxo, otherVtxo
}

func requireVHTLCSpentState(
	t *testing.T, vhtlcs []*pb.Vtxo, expected *pb.Vtxo, spent bool,
) {
	t.Helper()

	for _, v := range vhtlcs {
		if v.Outpoint.GetTxid() == expected.Outpoint.GetTxid() &&
			v.Outpoint.GetVout() == expected.Outpoint.GetVout() {
			require.Equal(t, spent, v.IsSpent)
			return
		}
	}

	t.Fatalf("vtxo %s:%d not found", expected.Outpoint.GetTxid(), expected.Outpoint.GetVout())
}

// TestClaimVHTLCPendingFinalization verifies that calling ClaimVHTLC on a VHTLC
// whose VTXO was already submitted (SubmitTx) but not finalized (FinalizeTx)
// correctly detects the pending state and completes the finalization.
func TestClaimVHTLCPendingFinalization(t *testing.T) {
	ctx := t.Context()

	f, err := newFulmineClient("localhost:7000")
	require.NoError(t, err)

	arkadeWallet, _, _ := setupArkSDKwithPublicKey(t)
	_, _, boarding, _, err := arkadeWallet.GetAddresses(ctx)
	require.NoError(t, err)

	err = faucet(ctx, strings.TrimSpace(boarding[0]), 0.001)
	require.NoError(t, err)
	time.Sleep(5 * time.Second)
	_, err = arkadeWallet.Settle(ctx)
	require.NoError(t, err)

	info, err := f.GetInfo(ctx, &pb.GetInfoRequest{})
	require.NoError(t, err)

	// Create a VHTLC (sender = receiver for simplicity)
	preimage := make([]byte, 32)
	_, err = rand.Read(preimage)
	require.NoError(t, err)
	sha256Hash := sha256.Sum256(preimage)
	preimageHash := hex.EncodeToString(input.Ripemd160H(sha256Hash[:]))

	vhtlcResp, err := f.CreateVHTLC(ctx, &pb.CreateVHTLCRequest{
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
	})
	require.NoError(t, err)
	require.NotEmpty(t, vhtlcResp.Address)

	_, err = arkadeWallet.SendOffChain(ctx, []clientTypes.Receiver{
		{
			To:     vhtlcResp.Address,
			Amount: 1000,
		},
	})
	require.NoError(t, err)

	vhtlc := buildTestVHTLC(t, f, vhtlcResp, preimageHash)
	pendingTxid := submitPendingClaimVHTLC(t, arkadeWallet, f, vhtlc, preimage)
	require.NotEmpty(t, pendingTxid)
	requirePendingVHTLC(t, arkadeWallet, vhtlc)
	// Now call ClaimVHTLC via the normal gRPC path.
	// The VTXO is spent (SubmitTx marked it) but not finalized.
	// The pending detection should find it and call FinalizePendingTxs.
	result, err := f.ClaimVHTLC(ctx, &pb.ClaimVHTLCRequest{
		VhtlcId:  vhtlcResp.Id,
		Preimage: hex.EncodeToString(preimage),
	})
	require.NoError(t, err, "ClaimVHTLC should succeed by finalizing the pending tx")
	require.NotNil(t, result)
	require.NotEmpty(t, result.GetRedeemTxid())
}

// TestRefundVHTLCPendingFinalization verifies that calling RefundVHTLCWithoutReceiver
// on a VHTLC whose VTXO was already submitted (SubmitTx) but not finalized (FinalizeTx)
// correctly detects the pending state and completes the finalization.
func TestRefundVHTLCPendingFinalization(t *testing.T) {
	ctx := t.Context()

	f, err := newFulmineClient("localhost:7000")
	require.NoError(t, err)

	arkClient, _, _ := setupArkSDKwithPublicKey(t)
	_, _, boarding, _, err := arkClient.GetAddresses(ctx)
	require.NoError(t, err)

	err = faucet(ctx, strings.TrimSpace(boarding[0]), 0.001)
	require.NoError(t, err)
	time.Sleep(5 * time.Second)
	_, err = arkClient.Settle(ctx)
	require.NoError(t, err)

	preimage := make([]byte, 32)
	_, err = rand.Read(preimage)
	require.NoError(t, err)
	sha256Hash := sha256.Sum256(preimage)
	preimageHash := hex.EncodeToString(input.Ripemd160H(sha256Hash[:]))

	receiverPrivKey, err := btcec.NewPrivateKey()
	require.NoError(t, err)

	vhtlcResp, err := f.CreateVHTLC(ctx, &pb.CreateVHTLCRequest{
		PreimageHash:   preimageHash,
		ReceiverPubkey: hex.EncodeToString(receiverPrivKey.PubKey().SerializeCompressed()),
		RefundLocktime: uint32(1577836800),
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
	})
	require.NoError(t, err)
	require.NotEmpty(t, vhtlcResp.Address)

	_, err = arkClient.SendOffChain(ctx, []clientTypes.Receiver{
		{
			To:     vhtlcResp.Address,
			Amount: 1000,
		},
	})
	require.NoError(t, err)

	vhtlc := buildTestVHTLC(t, f, vhtlcResp, preimageHash)
	pendingTxid := submitPendingRefundVHTLCWithoutReceiver(t, arkClient, f, vhtlc)
	require.NotEmpty(t, pendingTxid)
	requirePendingVHTLC(t, arkClient, vhtlc)

	// Now call RefundVHTLCWithoutReceiver via the normal gRPC path.
	// The VTXO is spent (SubmitTx marked it) but not finalized.
	// The pending detection should find it and call FinalizePendingTxs.
	result, err := f.RefundVHTLCWithoutReceiver(ctx, &pb.RefundVHTLCWithoutReceiverRequest{
		VhtlcId: vhtlcResp.Id,
	})
	require.NoError(t, err, "RefundVHTLC should succeed by finalizing the pending tx")
	require.NotNil(t, result)
	require.NotEmpty(t, result.GetRedeemTxid())
}
