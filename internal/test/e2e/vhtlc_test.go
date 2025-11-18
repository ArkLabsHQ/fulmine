package e2e_test

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"testing"

	pb "github.com/ArkLabsHQ/fulmine/api-spec/protobuf/gen/go/fulmine/v1"
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

	vhtlc, err := f.CreateVHTLC(ctx, &pb.CreateVHTLCRequest{
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
	require.NotEmpty(t, vhtlc.Address)
	require.NotEmpty(t, vhtlc.ClaimPubkey)
	require.NotEmpty(t, vhtlc.RefundPubkey)
	require.NotEmpty(t, vhtlc.ServerPubkey)

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
