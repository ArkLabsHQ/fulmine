package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	arknodepb "github.com/ArkLabsHQ/ark-node/api-spec/protobuf/gen/go/ark_node/v1"
	pb "github.com/ArkLabsHQ/ark-node/api-spec/protobuf/gen/go/boltz_mock/v1"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

const (
	fakeInvoice = "lightning=LNBC10U1P3PJ257PP5YZTKWJCZ5FTL5LAXKAV23ZMZEKAW37ZK6KMV80PK4XAEV5QHTZ7QDPDWD3XGER9WD5KWM36YPRX7U3QD36KUCMGYP282ETNV3SHJCQZPGXQYZ5VQSP5USYC4LK9CHSFP53KVCNVQ456GANH60D89REYKDNGSMTJ6YW3NHVQ9QYYSSQJCEWM5CJWZ4A6RFJX77C490YCED6PEMK0UPKXHY89CMM7SCT66K8GNEANWYKZGDRWRFJE69H9U5U0W57RRCSYSAS7GADWMZXC8C6T0SPJAZUP6"
)

type boltzMockHandler struct {
	arknode arknodepb.ServiceClient
}

func newBoltzMockHandler(arknodeURL string) pb.ServiceServer {
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
	// MOCK ONLY //
	preimage := make([]byte, 32)
	if _, err := rand.Read(preimage); err != nil {
		return nil, err
	}

	preimageHash := hex.EncodeToString(btcutil.Hash160(preimage))

	// For the mock, we'll use the fakeInvoice constant
	invoice := fakeInvoice

	response, err := b.arknode.CreateVHTLC(ctx, &arknodepb.CreateVHTLCRequest{
		PreimageHash:   preimageHash,
		ReceiverPubkey: req.GetPubkey(),
	})
	if err != nil {
		logrus.Errorf("failed to fund vHTLC: %v", err)
		return nil, err
	}

	vhtlcAddress := response.Address

	_, err = b.arknode.SendOffChain(ctx, &arknodepb.SendOffChainRequest{
		Address: vhtlcAddress,
		Amount:  req.InvoiceAmount,
	})
	if err != nil {
		logrus.Errorf("failed to send to vHTLC address: %v", err)
		return nil, err
	}

	addrResponse, err := b.arknode.GetAddress(ctx, &arknodepb.GetAddressRequest{})
	if err != nil {
		return nil, err
	}

	return &pb.ReverseSubmarineSwapResponse{
		Invoice:         invoice,
		LockupAddress:   addrResponse.Address,
		RefundPublicKey: response.GetRefundPubkey(),
		PreimageHash:    hex.EncodeToString(preimage), // MOCK ONLY
	}, nil
}

// ark --> ln
func (b *boltzMockHandler) SubmarineSwap(ctx context.Context, req *pb.SubmarineSwapRequest) (*pb.SubmarineSwapResponse, error) {
	preimageHash, err := parsePreimageHash(req.GetPreimageHash())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	refundPubkey, err := parsePubkey(req.GetRefundPublicKey())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	resp, err := b.arknode.CreateVHTLC(ctx, &arknodepb.CreateVHTLCRequest{
		PreimageHash: preimageHash,
		SenderPubkey: refundPubkey,
	})
	if err != nil {
		return nil, err
	}

	vhtlcAddress := resp.GetAddress()
	swapTree := swapTree{resp.GetSwapTree()}.toProto()
	logrus.Infof("created VHTLC: %s", vhtlcAddress)
	go func() {
		ctx := context.Background()

		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()

		var vhtlc *arknodepb.Vtxo

		logrus.Info("waiting for vHTLC to be funded...")
		for range ticker.C {
			// check if vHTLC has been funded by the user
			resp, err := b.arknode.ListVHTLC(ctx, &arknodepb.ListVHTLCRequest{
				PreimageHashFilter: &preimageHash,
			})
			if err != nil {
				continue
			}

			if len(resp.GetVhtlcs()) == 0 {
				continue
			}

			vhtlc = resp.GetVhtlcs()[0]
			break
		}

		logrus.Infof("vHTLC funded %s", vhtlc.Outpoint.Txid)
		logrus.Info("paying invoice...")

		resp, err := b.arknode.PayInvoice(ctx, &arknodepb.PayInvoiceRequest{
			Invoice: req.GetInvoice(),
		})
		if err != nil {
			logrus.Errorf("failed to pay invoice: %v", err)
			return
		}

		// once user reveals the preimage, claim the vHTLC
		claimResp, err := b.arknode.ClaimVHTLC(ctx, &arknodepb.ClaimVHTLCRequest{
			Preimage: resp.GetPreimage(),
		})
		if err != nil {
			logrus.Errorf("failed to claim vHTLC: %v", err)
			return
		}

		logrus.Debugf("vHTLC claimed successfully %s", claimResp.GetRedeemTxid())
	}()

	return &pb.SubmarineSwapResponse{
		Address:        vhtlcAddress,
		ClaimPublicKey: "todopubkey", // TODO add a way to return a rawpubkey from arknode's wallet
		ExpectedAmount: req.GetInvoiceAmount(),
		AcceptZeroConf: true,
		SwapTree:       swapTree,
	}, nil
}

func parsePreimageHash(preimageHash string) (string, error) {
	if len(preimageHash) != 40 {
		return "", fmt.Errorf("invalid preimage hash")
	}

	return preimageHash, nil
}

func parsePubkey(pubkey string) (string, error) {
	if len(pubkey) != 66 {
		return "", fmt.Errorf("invalid pubkey")
	}

	return pubkey, nil
}

type swapTree struct {
	*arknodepb.TaprootTree
}

func (t swapTree) toProto() *pb.TaprootTree {
	claimLeaf := t.GetClaimLeaf()
	refundLeaf := t.GetRefundLeaf()
	refundWithoutBoltzLeaf := t.GetRefundWithoutBoltzLeaf()
	unilateralClaimLeaf := t.GetUnilateralClaimLeaf()
	unilateralRefundLeaf := t.GetUnilateralRefundLeaf()
	unilateralRefundWithoutBoltzLeaf := t.GetUnilateralRefundWithoutBoltzLeaf()
	return &pb.TaprootTree{
		ClaimLeaf: &pb.TaprootLeaf{
			Version: claimLeaf.GetVersion(),
			Output:  claimLeaf.GetOutput(),
		},
		RefundLeaf: &pb.TaprootLeaf{
			Version: refundLeaf.GetVersion(),
			Output:  refundLeaf.GetOutput(),
		},
		RefundWithoutBoltzLeaf: &pb.TaprootLeaf{
			Version: refundWithoutBoltzLeaf.GetVersion(),
			Output:  refundWithoutBoltzLeaf.GetOutput(),
		},
		UnilateralClaimLeaf: &pb.TaprootLeaf{
			Version: unilateralClaimLeaf.GetVersion(),
			Output:  unilateralClaimLeaf.GetOutput(),
		},
		UnilateralRefundLeaf: &pb.TaprootLeaf{
			Version: unilateralRefundLeaf.GetVersion(),
			Output:  unilateralRefundLeaf.GetOutput(),
		},
		UnilateralRefundWithoutBoltzLeaf: &pb.TaprootLeaf{
			Version: unilateralRefundWithoutBoltzLeaf.GetVersion(),
			Output:  unilateralRefundWithoutBoltzLeaf.GetOutput(),
		},
	}
}
