package handlers

import (
	"context"
	"encoding/hex"
	"fmt"
	"time"

	arknodepb "github.com/ArkLabsHQ/ark-node/api-spec/protobuf/gen/go/ark_node/v1"
	pb "github.com/ArkLabsHQ/ark-node/api-spec/protobuf/gen/go/boltz_mock/v1"
	"github.com/ark-network/ark/common"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

const (
	fakeInvoice  = "lightning=LNBC10U1P3PJ257PP5YZTKWJCZ5FTL5LAXKAV23ZMZEKAW37ZK6KMV80PK4XAEV5QHTZ7QDPDWD3XGER9WD5KWM36YPRX7U3QD36KUCMGYP282ETNV3SHJCQZPGXQYZ5VQSP5USYC4LK9CHSFP53KVCNVQ456GANH60D89REYKDNGSMTJ6YW3NHVQ9QYYSSQJCEWM5CJWZ4A6RFJX77C490YCED6PEMK0UPKXHY89CMM7SCT66K8GNEANWYKZGDRWRFJE69H9U5U0W57RRCSYSAS7GADWMZXC8C6T0SPJAZUP6"
	fakePreimage = "0001020304050607080910111213141516171819202122232425262728293031"
)

type boltzMockHandler struct {
	arknode arknodepb.ServiceClient
}

func NewBoltzMockHandler(arknodeURL string) pb.ServiceServer {
	conn, err := grpc.NewClient(arknodeURL, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		logrus.Fatalf("failed to connect to ark-node: %v", err)
	}

	return &boltzMockHandler{
		arknode: arknodepb.NewServiceClient(conn),
	}
}

// ln --> ark
func (b *boltzMockHandler) ReverseSubmarineSwap(ctx context.Context, req *pb.ReverseSubmarineSwapRequest) (*pb.ReverseSubmarineSwapResponse, error) {
	// create invoice from req (preimageHash, boltz address, invoice amount)
	//// MOCK ONLY /////
	invoice := fakeInvoice
	//// MOCK ONLY /////

	go func() {
		time.Sleep(30 * time.Second) // wait for the invoice to be paid

		logrus.Debugf("invoice paid, funding vHTLC")
		// claim the LN funds

		fundVHTLCresponse, err := b.arknode.BotlzFundVHTLC(ctx, &arknodepb.BotlzFundVHTLCRequest{
			PreimageHash: req.PreimageHash,
			Address:      req.Address,
			Amount:       req.OnchainAmount,
		})
		if err != nil {
			logrus.Errorf("failed to fund vHTLC: %v", err)
			return
		}
		logrus.Debugf("new vHTLC funded: %s", fundVHTLCresponse.RedeemTx)
	}()

	addrResponse, err := b.arknode.GetAddress(ctx, &arknodepb.GetAddressRequest{})
	if err != nil {
		return nil, err
	}

	_, boltzPubkey, _, err := common.DecodeAddress(addrResponse.Address)
	if err != nil {
		return nil, err
	}

	return &pb.ReverseSubmarineSwapResponse{
		Invoice:         invoice,
		LockupAddress:   req.Address,
		RefundPublicKey: hex.EncodeToString(boltzPubkey.SerializeCompressed()),
	}, nil
}

// ark --> ln
func (b *boltzMockHandler) SubmarineSwap(ctx context.Context, req *pb.SubmarineSwapRequest) (*pb.SubmarineSwapResponse, error) {
	///// MOCK ONLY /////
	// verify that the preimageHash in the request is the expected one in order to simulate the user revealing the preimage
	preimage, err := hex.DecodeString(fakePreimage)
	if err != nil {
		return nil, err
	}

	if hex.EncodeToString(btcutil.Hash160(preimage)) != req.PreimageHash {
		return nil, fmt.Errorf("not the expected preimage")
	}
	///// MOCK ONLY /////

	go func() {
		ctx := context.Background()

		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()

		var vhtlc *arknodepb.Vtxo

		for range ticker.C {
			// check if vHTLC has been funded by the user
			resp, err := b.arknode.ListVHTLC(ctx, &arknodepb.ListVHTLCRequest{PreimageHashFilter: &req.PreimageHash})
			if err != nil {
				logrus.Errorf("failed to list vHTLCs: %v", err)
				continue
			}

			if len(resp.Vhtlcs) == 0 {
				continue
			}

			vhtlc = resp.Vhtlcs[0]
			break
		}

		logrus.Debugf("vHTLC found: %s", vhtlc.Outpoint.Txid)
		// pay the invoice
		time.Sleep(5 * time.Second) // simulate the payment

		// once user reveals the preimage, claim the vHTLC
		_, err := b.arknode.BoltzClaimVHTLC(ctx, &arknodepb.BoltzClaimVHTLCRequest{
			Preimage: hex.EncodeToString(preimage),
		})
		if err != nil {
			logrus.Errorf("failed to claim vHTLC: %v", err)
			return
		}

		logrus.Debugf("vHTLC claimed successfully (amount = %d)", vhtlc.Receiver.Amount)
	}()

	addrResponse, err := b.arknode.GetAddress(ctx, &arknodepb.GetAddressRequest{})
	if err != nil {
		return nil, err
	}

	return &pb.SubmarineSwapResponse{
		Address:        addrResponse.Address,
		ExpectedAmount: req.GetInvoiceAmount(),
		AcceptZeroConf: true,
	}, nil
}
