package e2e_test

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"testing"
	"time"

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

func TestVHTLCEventsStream(t *testing.T) {
	client, err := newFulmineClient("localhost:7000")
	require.NoError(t, err)
	require.NotNil(t, client)

	notificationClient, err := newNotificationClient("localhost:7000")
	require.NoError(t, err)
	require.NotNil(t, notificationClient)

	ctx := t.Context()
	eventStream, err := notificationClient.GetVhtlcEvents(ctx, &pb.GetVhtlcEventsRequest{})
	require.NoError(t, err)
	require.NotNil(t, eventStream)

	info, err := client.GetInfo(ctx, &pb.GetInfoRequest{})
	require.NoError(t, err)
	require.NotEmpty(t, info)

	preimage := make([]byte, 32)
	_, err = rand.Read(preimage)
	require.NoError(t, err)
	sha256Hash := sha256.Sum256(preimage)
	preimageHash := hex.EncodeToString(input.Ripemd160H(sha256Hash[:]))

	createReq := &pb.CreateVHTLCRequest{
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

	vhtlc, err := client.CreateVHTLC(ctx, createReq)
	require.NoError(t, err)
	require.NotEmpty(t, vhtlc.GetId())

	createdEvent := waitForVhtlcEvent(t, eventStream, 30*time.Second, func(event *pb.VhtlcEvent) bool {
		return event.GetId() == vhtlc.GetId() &&
			event.GetType() == pb.EventType_EVENT_TYPE_VHTLC_CREATED
	})
	require.Equal(t, vhtlc.GetId(), createdEvent.GetId())

	_, err = client.SendOffChain(ctx, &pb.SendOffChainRequest{
		Address: vhtlc.GetAddress(),
		Amount:  1000,
	})
	require.NoError(t, err)

	fundedEvent := waitForVhtlcEvent(t, eventStream, 30*time.Second, func(event *pb.VhtlcEvent) bool {
		return event.GetId() == vhtlc.GetId() &&
			event.GetType() == pb.EventType_EVENT_TYPE_VHTLC_FUNDED
	})
	require.Equal(t, vhtlc.GetId(), fundedEvent.GetId())
	require.NotEmpty(t, fundedEvent.GetTxid())
	require.Empty(t, fundedEvent.GetPreimage())

	claimResp, err := client.ClaimVHTLC(ctx, &pb.ClaimVHTLCRequest{
		VhtlcId:  vhtlc.GetId(),
		Preimage: hex.EncodeToString(preimage),
	})
	require.NoError(t, err)
	require.NotEmpty(t, claimResp.GetRedeemTxid())

	claimedEvent := waitForVhtlcEvent(t, eventStream, 30*time.Second, func(event *pb.VhtlcEvent) bool {
		return event.GetId() == vhtlc.GetId() &&
			event.GetType() == pb.EventType_EVENT_TYPE_VHTLC_CLAIMED
	})
	require.Equal(t, vhtlc.GetId(), claimedEvent.GetId())
	require.Equal(t, claimResp.GetRedeemTxid(), claimedEvent.GetTxid())
	require.Equal(t, hex.EncodeToString(preimage), claimedEvent.GetPreimage())
}

func waitForVhtlcEvent(
	t *testing.T,
	stream pb.NotificationService_GetVhtlcEventsClient,
	timeout time.Duration,
	match func(event *pb.VhtlcEvent) bool,
) *pb.VhtlcEvent {
	t.Helper()

	ctx, cancel := context.WithTimeout(t.Context(), timeout)
	defer cancel()

	for {
		eventCh := make(chan *pb.GetVhtlcEventsResponse, 1)
		errCh := make(chan error, 1)
		go func() {
			resp, err := stream.Recv()
			if err != nil {
				errCh <- err
				return
			}
			eventCh <- resp
		}()

		select {
		case <-ctx.Done():
			t.Fatalf("timed out waiting for matching vhtlc event")
		case err := <-errCh:
			if err == io.EOF {
				t.Fatalf("event stream closed before matching vhtlc event")
			}
			t.Fatalf("failed to receive vhtlc event: %v", err)
		case resp := <-eventCh:
			if resp != nil && resp.GetEvent() != nil && match(resp.GetEvent()) {
				return resp.GetEvent()
			}
		}
	}
}
