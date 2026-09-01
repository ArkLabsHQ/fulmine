package e2e_test

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"sync"
	"testing"
	"time"

	pb "github.com/ArkLabsHQ/fulmine/api-spec/protobuf/gen/go/fulmine/v1"
	arklib "github.com/arkade-os/arkd/pkg/ark-lib"
	"github.com/arkade-os/arkd/pkg/ark-lib/intent"
	"github.com/arkade-os/arkd/pkg/ark-lib/tree"
	"github.com/arkade-os/arkd/pkg/ark-lib/txutils"
	clientTypes "github.com/arkade-os/arkd/pkg/client-lib/types"
	arksdk "github.com/arkade-os/go-sdk"
	"github.com/arkade-os/go-sdk/vhtlc"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcutil/psbt"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	"github.com/lightningnetwork/lnd/input"
	"github.com/stretchr/testify/require"
)

func TestVHTLC(t *testing.T) {
	for _, target := range clientTargets {
		t.Run(target.name, func(t *testing.T) {
			f, err := newFulmineClient(target.url)
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
				SenderPubkey:   info.GetPubkey(),
				RefundLocktime: uint32(time.Now().Add(100 * time.Second).Unix()),
				UnilateralClaimDelay: &pb.RelativeLocktime{
					Type:  pb.RelativeLocktime_LOCKTIME_TYPE_BLOCK,
					Value: 105,
				},
				UnilateralRefundDelay: &pb.RelativeLocktime{
					Type:  pb.RelativeLocktime_LOCKTIME_TYPE_BLOCK,
					Value: 110,
				},
				UnilateralRefundWithoutReceiverDelay: &pb.RelativeLocktime{
					Type:  pb.RelativeLocktime_LOCKTIME_TYPE_BLOCK,
					Value: 115,
				},
			}
			vhtlc, err := f.CreateVHTLC(ctx, req)
			require.NoError(t, err)
			require.NotEmpty(t, vhtlc.Address)
			require.NotEmpty(t, vhtlc.ClaimPubkey)
			require.NotEmpty(t, vhtlc.RefundPubkey)
			require.NotEmpty(t, vhtlc.ServerPubkey)

			// Ensure duplication is not allowed for single key only. Impossible to duplicate one
			// calling this api with hd identity.
			if target.name == "singlekey" {
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

			// Get the VHTLC. The vtxo is indexed asynchronously after
			// SendOffChain, so poll instead of racing the indexer.
			var vhtlcs *pb.ListVHTLCResponse
			require.Eventually(t, func() bool {
				var err error
				vhtlcs, err = f.ListVHTLC(ctx, &pb.ListVHTLCRequest{VhtlcId: vhtlc.GetId()})
				return err == nil && len(vhtlcs.GetVhtlcs()) > 0
			}, 30*time.Second, time.Second, "no VTXO indexed at the VHTLC address within 30s")

			// Claim the VHTLC
			redeemTxid, err := f.ClaimVHTLC(ctx, &pb.ClaimVHTLCRequest{
				VhtlcId:  vhtlc.Id,
				Preimage: hex.EncodeToString(preimage),
			})
			require.NoError(t, err)
			require.NotNil(t, redeemTxid)
			require.NotEmpty(t, redeemTxid.GetRedeemTxid())
		})
	}
}

// TestClaimVHTLCWithOutpoint funds the same VHTLC address twice with different
// amounts, then claims only the second VTXO by specifying its outpoint. It
// verifies that the targeted VTXO is claimed and the first remains untouched.
func TestClaimVHTLCWithOutpoint(t *testing.T) {
	for _, target := range clientTargets {
		t.Run(target.name, func(t *testing.T) {
			f, err := newFulmineClient(target.url)
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
				SenderPubkey:   info.GetPubkey(),
				RefundLocktime: uint32(time.Now().Add(100 * time.Second).Unix()),
				UnilateralClaimDelay: &pb.RelativeLocktime{
					Type:  pb.RelativeLocktime_LOCKTIME_TYPE_BLOCK,
					Value: 105,
				},
				UnilateralRefundDelay: &pb.RelativeLocktime{
					Type:  pb.RelativeLocktime_LOCKTIME_TYPE_BLOCK,
					Value: 110,
				},
				UnilateralRefundWithoutReceiverDelay: &pb.RelativeLocktime{
					Type:  pb.RelativeLocktime_LOCKTIME_TYPE_BLOCK,
					Value: 115,
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

			// List VHTLCs and verify there are two VTXOs. Both deposits are
			// indexed asynchronously, so wait for the second to land before
			// asserting the exact count — otherwise this races the indexer and
			// fails whenever only the first deposit is visible yet.
			var vhtlcs *pb.ListVHTLCResponse
			require.Eventually(t, func() bool {
				var err error
				vhtlcs, err = f.ListVHTLC(ctx, &pb.ListVHTLCRequest{VhtlcId: vhtlcResp.GetId()})
				return err == nil && len(vhtlcs.GetVhtlcs()) >= 2
			}, 30*time.Second, time.Second, "expected 2 VTXOs at the VHTLC address within 30s")
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
		})
	}
}

// TestClaimVHTLCOldestVtxo funds the same VHTLC address 3 times with different
// amounts, oldest vtxo should be claimed
func TestClaimVHTLCOldestVtxo(t *testing.T) {
	for _, target := range clientTargets {
		t.Run(target.name, func(t *testing.T) {
			f, err := newFulmineClient(target.url)
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
				SenderPubkey:   info.GetPubkey(),
				RefundLocktime: uint32(time.Now().Add(100 * time.Second).Unix()),
				UnilateralClaimDelay: &pb.RelativeLocktime{
					Type:  pb.RelativeLocktime_LOCKTIME_TYPE_BLOCK,
					Value: 105,
				},
				UnilateralRefundDelay: &pb.RelativeLocktime{
					Type:  pb.RelativeLocktime_LOCKTIME_TYPE_BLOCK,
					Value: 110,
				},
				UnilateralRefundWithoutReceiverDelay: &pb.RelativeLocktime{
					Type:  pb.RelativeLocktime_LOCKTIME_TYPE_BLOCK,
					Value: 115,
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

			// The claim is reflected in the listing asynchronously too, so wait
			// for the spend rather than asserting on whatever is visible now.
			var vtxos *pb.ListVHTLCResponse
			require.Eventually(t, func() bool {
				var err error
				vtxos, err = f.ListVHTLC(ctx, &pb.ListVHTLCRequest{VhtlcId: vhtlcResp.GetId()})
				if err != nil {
					return false
				}
				for _, v := range vtxos.GetVhtlcs() {
					if v.Amount == 1000 && v.IsSpent {
						return true
					}
				}
				return false
			}, 30*time.Second, time.Second, "the claimed VTXO was not marked spent within 30s")

			for _, v := range vtxos.GetVhtlcs() {
				if v.Amount == 1000 {
					require.True(t, v.IsSpent)
				} else {
					require.False(t, v.IsSpent)
				}
			}
		})
	}
}

// TestClaimVHTLCPendingFinalization verifies that calling ClaimVHTLC on a VHTLC
// whose VTXO was already submitted (SubmitTx) but not finalized (FinalizeTx)
// correctly detects the pending state and completes the finalization.
func TestClaimVHTLCPendingFinalization(t *testing.T) {
	for _, target := range clientTargets {
		t.Run(target.name, func(t *testing.T) {
			ctx := t.Context()

			f, err := newFulmineClient(target.url)
			require.NoError(t, err)

			arkadeWallet, _, _ := setupArkSDKwithPublicKey(t)
			_, _, boarding, _, err := arkadeWallet.GetAddresses(ctx)
			require.NoError(t, err)

			faucetAndSettle(t, ctx, arkadeWallet, boarding[0], 0.001)

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
				SenderPubkey:   info.GetPubkey(),
				RefundLocktime: uint32(time.Now().Add(100 * time.Second).Unix()),
				UnilateralClaimDelay: &pb.RelativeLocktime{
					Type:  pb.RelativeLocktime_LOCKTIME_TYPE_BLOCK,
					Value: 105,
				},
				UnilateralRefundDelay: &pb.RelativeLocktime{
					Type:  pb.RelativeLocktime_LOCKTIME_TYPE_BLOCK,
					Value: 110,
				},
				UnilateralRefundWithoutReceiverDelay: &pb.RelativeLocktime{
					Type:  pb.RelativeLocktime_LOCKTIME_TYPE_BLOCK,
					Value: 115,
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
		})
	}
}

func TestRefundVHTLCWithoutReceiverWithOutpoint(t *testing.T) {
	for _, target := range clientTargets {
		t.Run(target.name, func(t *testing.T) {
			fulmineClient, err := newFulmineClient(target.url)
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
				// For sake of testing,the refund locktime is sey to block height 1 to be sure it can be
				// refunded alone immediately as it's already expired.
				RefundLocktime: 1,
				UnilateralClaimDelay: &pb.RelativeLocktime{
					Type:  pb.RelativeLocktime_LOCKTIME_TYPE_BLOCK,
					Value: 105,
				},
				UnilateralRefundDelay: &pb.RelativeLocktime{
					Type:  pb.RelativeLocktime_LOCKTIME_TYPE_BLOCK,
					Value: 110,
				},
				UnilateralRefundWithoutReceiverDelay: &pb.RelativeLocktime{
					Type:  pb.RelativeLocktime_LOCKTIME_TYPE_BLOCK,
					Value: 115,
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

			// Both deposits are indexed asynchronously; findVHTLCsByAmount
			// requires both to be present, so wait for them.
			var vhtlcs *pb.ListVHTLCResponse
			require.Eventually(t, func() bool {
				var err error
				vhtlcs, err = fulmineClient.ListVHTLC(ctx, &pb.ListVHTLCRequest{VhtlcId: vhtlc.GetId()})
				return err == nil && len(vhtlcs.GetVhtlcs()) >= 2
			}, 30*time.Second, time.Second, "expected 2 VTXOs at the VHTLC address within 30s")

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

			// Wait for the refund to be reflected instead of sleeping a fixed
			// interval and hoping the indexer kept up.
			var updatedVHTLCs *pb.ListVHTLCResponse
			require.Eventually(t, func() bool {
				var err error
				updatedVHTLCs, err = fulmineClient.ListVHTLC(
					ctx, &pb.ListVHTLCRequest{VhtlcId: vhtlc.GetId()},
				)
				if err != nil {
					return false
				}
				for _, v := range updatedVHTLCs.GetVhtlcs() {
					if v.Outpoint.GetTxid() == targetVtxo.Outpoint.GetTxid() &&
						v.Outpoint.GetVout() == targetVtxo.Outpoint.GetVout() {
						return v.IsSpent
					}
				}
				return false
			}, 30*time.Second, time.Second, "the refunded VTXO was not marked spent within 30s")

			requireVHTLCSpentState(t, updatedVHTLCs.GetVhtlcs(), targetVtxo, true)
			requireVHTLCSpentState(t, updatedVHTLCs.GetVhtlcs(), otherVtxo, false)
		})
	}
}

// TestRefundVHTLCPendingFinalization verifies that calling RefundVHTLCWithoutReceiver
// on a VHTLC whose VTXO was already submitted (SubmitTx) but not finalized (FinalizeTx)
// correctly detects the pending state and completes the finalization.
func TestRefundVHTLCPendingFinalization(t *testing.T) {
	for _, target := range clientTargets {
		t.Run(target.name, func(t *testing.T) {
			ctx := t.Context()

			f, err := newFulmineClient(target.url)
			require.NoError(t, err)

			arkClient, _, _ := setupArkSDKwithPublicKey(t)
			_, _, boarding, _, err := arkClient.GetAddresses(ctx)
			require.NoError(t, err)

			faucetAndSettle(t, ctx, arkClient, boarding[0], 0.001)

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
				// For sake of testing,the refund locktime is sey to block height 1 to be sure it can be
				// refunded alone immediately as it's already expired.
				RefundLocktime: 1,
				UnilateralClaimDelay: &pb.RelativeLocktime{
					Type:  pb.RelativeLocktime_LOCKTIME_TYPE_BLOCK,
					Value: 105,
				},
				UnilateralRefundDelay: &pb.RelativeLocktime{
					Type:  pb.RelativeLocktime_LOCKTIME_TYPE_BLOCK,
					Value: 110,
				},
				UnilateralRefundWithoutReceiverDelay: &pb.RelativeLocktime{
					Type:  pb.RelativeLocktime_LOCKTIME_TYPE_BLOCK,
					Value: 115,
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
		})
	}
}

// TestGetVHTLCSpendingTxFinalized verifies that GetVHTLCSpendingTx returns the fully signed ark
// transaction for a VHTLC that was claimed as expected.
func TestGetVHTLCSpendingTxFinalized(t *testing.T) {
	for _, target := range clientTargets {
		t.Run(target.name, func(t *testing.T) {
			f, err := newFulmineClient(target.url)
			require.NoError(t, err)

			ctx := t.Context()

			info, err := f.GetInfo(ctx, &pb.GetInfoRequest{})
			require.NoError(t, err)

			preimage := make([]byte, 32)
			_, err = rand.Read(preimage)
			require.NoError(t, err)
			sha256Hash := sha256.Sum256(preimage)
			preimageHash := hex.EncodeToString(input.Ripemd160H(sha256Hash[:]))

			vhtlcResp, err := f.CreateVHTLC(ctx, &pb.CreateVHTLCRequest{
				PreimageHash:   preimageHash,
				SenderPubkey:   info.GetPubkey(),
				RefundLocktime: uint32(time.Now().Add(100 * time.Second).Unix()),
				UnilateralClaimDelay: &pb.RelativeLocktime{
					Type:  pb.RelativeLocktime_LOCKTIME_TYPE_BLOCK,
					Value: 105,
				},
				UnilateralRefundDelay: &pb.RelativeLocktime{
					Type:  pb.RelativeLocktime_LOCKTIME_TYPE_BLOCK,
					Value: 110,
				},
				UnilateralRefundWithoutReceiverDelay: &pb.RelativeLocktime{
					Type:  pb.RelativeLocktime_LOCKTIME_TYPE_BLOCK,
					Value: 115,
				},
			})
			require.NoError(t, err)
			require.NotEmpty(t, vhtlcResp.Address)

			// Fund the VHTLC (creates a finalized VTXO at the VHTLC address)
			_, err = f.SendOffChain(ctx, &pb.SendOffChainRequest{
				Address: vhtlcResp.Address,
				Amount:  1000,
			})
			require.NoError(t, err)

			claimResp, err := f.ClaimVHTLC(ctx, &pb.ClaimVHTLCRequest{
				VhtlcId:  vhtlcResp.GetId(),
				Preimage: hex.EncodeToString(preimage),
			})
			require.NoError(t, err)
			require.NotNil(t, claimResp)
			require.NotEmpty(t, claimResp.GetRedeemTxid())

			// The spending tx is registered asynchronously after ClaimVHTLC; poll for it.
			var resp *pb.GetVHTLCSpendingTxResponse
			require.Eventually(t, func() bool {
				var err error
				resp, err = f.GetVHTLCSpendingTx(
					ctx, &pb.GetVHTLCSpendingTxRequest{VhtlcId: vhtlcResp.GetId()},
				)
				return err == nil && resp.GetTx() != ""
			}, 30*time.Second, time.Second, "no spending tx registered for the VHTLC within 30s")

			// Verify the returned tx is a valid PSBT
			ptx, err := psbt.NewFromRawBytes(strings.NewReader(resp.GetTx()), true)
			require.NoError(t, err)
			require.NotNil(t, ptx)
			require.Equal(t, claimResp.GetRedeemTxid(), ptx.UnsignedTx.TxID())

			// Assert the preimage is there
			witnesses, err := txutils.GetArkPsbtFields(ptx, 0, txutils.ConditionWitnessField)
			require.NoError(t, err)
			require.NotEmpty(t, witnesses)
			require.NotEmpty(t, witnesses[0])
			require.Equal(t, preimage, []byte(witnesses[0][0]))
		})
	}
}

// TestGetVHTLCSpendingTxPending verifies that GetVHTLCSpendingTx returns the fully signed ark
// transaction for a VHTLC spent by a pending tx (only SubmitTx was called)
func TestGetVHTLCSpendingTxPending(t *testing.T) {
	for _, target := range clientTargets {
		t.Run(target.name, func(t *testing.T) {
			ctx := t.Context()

			f, err := newFulmineClient(target.url)
			require.NoError(t, err)

			arkadeWallet, _, _ := setupArkSDKwithPublicKey(t)
			_, _, boarding, _, err := arkadeWallet.GetAddresses(ctx)
			require.NoError(t, err)

			faucetAndSettle(t, ctx, arkadeWallet, boarding[0], 0.001)

			info, err := f.GetInfo(ctx, &pb.GetInfoRequest{})
			require.NoError(t, err)

			preimage := make([]byte, 32)
			_, err = rand.Read(preimage)
			require.NoError(t, err)
			sha256Hash := sha256.Sum256(preimage)
			preimageHash := hex.EncodeToString(input.Ripemd160H(sha256Hash[:]))

			vhtlcResp, err := f.CreateVHTLC(ctx, &pb.CreateVHTLCRequest{
				PreimageHash:   preimageHash,
				SenderPubkey:   info.GetPubkey(),
				RefundLocktime: uint32(time.Now().Add(100 * time.Second).Unix()),
				UnilateralClaimDelay: &pb.RelativeLocktime{
					Type:  pb.RelativeLocktime_LOCKTIME_TYPE_BLOCK,
					Value: 105,
				},
				UnilateralRefundDelay: &pb.RelativeLocktime{
					Type:  pb.RelativeLocktime_LOCKTIME_TYPE_BLOCK,
					Value: 110,
				},
				UnilateralRefundWithoutReceiverDelay: &pb.RelativeLocktime{
					Type:  pb.RelativeLocktime_LOCKTIME_TYPE_BLOCK,
					Value: 115,
				},
			})
			require.NoError(t, err)
			require.NotEmpty(t, vhtlcResp.Address)

			// Fund from external wallet and wait for incoming VTXO
			offchainAddr := newFulmineOffchainAddress(t, f)
			wg := &sync.WaitGroup{}
			wg.Add(1)
			var incomingFunds []clientTypes.Vtxo
			go func() {
				incomingFunds, _ = arkadeWallet.NotifyIncomingFunds(ctx, vhtlcResp.Address)
				wg.Done()
			}()

			_, err = arkadeWallet.SendOffChain(ctx, []clientTypes.Receiver{{
				To:     vhtlcResp.Address,
				Amount: 1000,
			}})
			require.NoError(t, err)
			_ = offchainAddr

			wg.Wait()
			require.NotEmpty(t, incomingFunds)

			// Build the VHTLC script from the create response
			vhtlcScript, err := vhtlc.NewVHTLCScriptFromOpts(vhtlc.Opts{
				Sender:         mustParseSchnorrPubKey(t, vhtlcResp.GetRefundPubkey()),
				Receiver:       mustParseSchnorrPubKey(t, vhtlcResp.GetClaimPubkey()),
				Server:         mustParseSchnorrPubKey(t, vhtlcResp.GetServerPubkey()),
				PreimageHash:   mustDecodeHex(t, preimageHash),
				RefundLocktime: arklib.AbsoluteLocktime(vhtlcResp.GetRefundLocktime()),
				UnilateralClaimDelay: arklib.RelativeLocktime{
					Type:  arklib.LocktimeTypeBlock,
					Value: uint32(vhtlcResp.GetUnilateralClaimDelay()),
				},
				UnilateralRefundDelay: arklib.RelativeLocktime{
					Type:  arklib.LocktimeTypeBlock,
					Value: uint32(vhtlcResp.GetUnilateralRefundDelay()),
				},
				UnilateralRefundWithoutReceiverDelay: arklib.RelativeLocktime{
					Type:  arklib.LocktimeTypeBlock,
					Value: uint32(vhtlcResp.GetUnilateralRefundWithoutReceiverDelay()),
				},
			})
			require.NoError(t, err)

			// Use the VTXO we got from the incoming funds notification
			fundedVtxo := incomingFunds[0]
			testVhtlc := testVHTLC{
				script: vhtlcScript,
				vtxo:   &fundedVtxo,
			}

			// Submit a claim tx but don't finalize → creates pending state
			pendingTxid := submitPendingClaimVHTLC(t, arkadeWallet, f, testVhtlc, preimage)
			require.NotEmpty(t, pendingTxid)

			// GetVHTLCTransaction should return the pending tx. Registration is
			// asynchronous here too, so poll rather than asserting immediately.
			var resp *pb.GetVHTLCSpendingTxResponse
			require.Eventually(t, func() bool {
				var err error
				resp, err = f.GetVHTLCSpendingTx(ctx, &pb.GetVHTLCSpendingTxRequest{
					VhtlcId: vhtlcResp.GetId(),
				})
				return err == nil && resp.GetTx() != ""
			}, 30*time.Second, time.Second, "no pending spending tx registered within 30s")

			// Parse the pending tx and extract the condition witness (preimage)
			ptx, err := psbt.NewFromRawBytes(strings.NewReader(resp.GetTx()), true)
			require.NoError(t, err)
			require.NotNil(t, ptx)
			require.Equal(t, pendingTxid, ptx.UnsignedTx.TxID())

			// Assert the preimage is there
			witnesses, err := txutils.GetArkPsbtFields(ptx, 0, txutils.ConditionWitnessField)
			require.NoError(t, err)
			require.NotEmpty(t, witnesses)
			require.NotEmpty(t, witnesses[0])
			require.Equal(t, preimage, []byte(witnesses[0][0]))
		})
	}
}

func buildDelegateIntentProof(
	t *testing.T,
	ctx context.Context,
	senderArkClient arksdk.Wallet,
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
	senderArkClient arksdk.Wallet,
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
