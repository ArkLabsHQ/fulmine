package e2e_test

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"testing"
	"time"

	pb "github.com/ArkLabsHQ/fulmine/api-spec/protobuf/gen/go/fulmine/v1"
	"github.com/lightningnetwork/lnd/input"
	"github.com/stretchr/testify/require"
)

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

// TestRefundVhtlcSettlement tests the VHTLC refund path integration:
// 1. Create a VHTLC (submarine swap scenario)
// 2. Send offchain funds to the VHTLC address
// 3. Invoke SettleVHTLC with RefundPath (2-of-2 multisig: Sender+Server)
// 4. Verify refund succeeds and funds are returned
//
// Note: This test uses a very short refund locktime to avoid waiting
func TestRefundVhtlcSettlement(t *testing.T) {
	t.Skip("Skipping refund test - requires specific locktime constraints and environment setup")

	f, err := newFulmineClient("localhost:7000")
	require.NoError(t, err)
	require.NotNil(t, f)

	ctx := t.Context()

	// Get initial balance
	balanceBefore, err := f.GetBalance(ctx, &pb.GetBalanceRequest{})
	require.NoError(t, err)
	require.NotNil(t, balanceBefore)
	t.Logf("Initial balance: %d sats", balanceBefore.GetAmount())

	// For this test, sender = receiver to simulate failed swap scenario
	info, err := f.GetInfo(ctx, &pb.GetInfoRequest{})
	require.NoError(t, err)
	require.NotEmpty(t, info)

	// Create the VHTLC (simulating a submarine swap that will fail)
	// Use past refund locktime to allow immediate refund
	preimage := make([]byte, 32)
	_, err = rand.Read(preimage)
	require.NoError(t, err)
	sha256Hash := sha256.Sum256(preimage)
	preimageHash := hex.EncodeToString(input.Ripemd160H(sha256Hash[:]))

	// Use a refund locktime in the past to allow immediate refund
	pastRefundLocktime := uint32(time.Now().Unix() - 3600) // 1 hour ago

	req := &pb.CreateVHTLCRequest{
		PreimageHash:   preimageHash,
		ReceiverPubkey: info.GetPubkey(),
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
			Value: 1024,
		},
	}
	vhtlc, err := f.CreateVHTLC(ctx, req)
	require.NoError(t, err)
	require.NotEmpty(t, vhtlc.Address)
	require.NotEmpty(t, vhtlc.Id)
	t.Logf("Created VHTLC: id=%s, address=%s, refund_locktime=%d",
		vhtlc.Id, vhtlc.Address, pastRefundLocktime)

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

	// Refund the VHTLC using the SettleVHTLC RPC with RefundPath (without receiver)
	t.Log("Refunding VHTLC via SettleVHTLC RPC (2-of-2 without receiver)...")
	settleResp, err := f.SettleVHTLC(ctx, &pb.SettleVHTLCRequest{
		VhtlcId: vhtlc.Id,
		SettlementType: &pb.SettleVHTLCRequest_Refund{
			Refund: &pb.RefundPath{
				WithReceiver: false, // 2-of-2 multisig without receiver (after CLTV)
			},
		},
	})
	require.NoError(t, err, "SettleVHTLC with refund path should succeed")
	require.NotNil(t, settleResp)
	require.NotEmpty(t, settleResp.GetTxid())
	t.Logf("Refund successful: txid=%s", settleResp.GetTxid())

	// Wait for settlement to complete
	t.Log("Waiting for batch settlement to finalize...")
	time.Sleep(5 * time.Second)

	// Verify balance returned to approximately initial value (minus small fees)
	balanceAfter, err := f.GetBalance(ctx, &pb.GetBalanceRequest{})
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
