package main

import (
	"context"
	"fmt"
	"time"

	arknodepb "github.com/ArkLabsHQ/ark-node/api-spec/protobuf/gen/go/ark_node/v1"
	pb "github.com/ArkLabsHQ/ark-node/api-spec/protobuf/gen/go/boltz_mock/v1"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

type boltzMockHandler struct {
	arknode arknodepb.ServiceClient
}

func newBoltzMockHandler(arknodeURL string) pb.ServiceServer {
	conn, err := grpc.NewClient(arknodeURL, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("failed to connect to ark-node: %v", err)
	}

	return &boltzMockHandler{
		arknode: arknodepb.NewServiceClient(conn),
	}
}

// ln --> ark
func (b *boltzMockHandler) ReverseSubmarineSwap(ctx context.Context, req *pb.ReverseSubmarineSwapRequest) (*pb.ReverseSubmarineSwapResponse, error) {
	invoiceResponse, err := b.arknode.CreateInvoice(ctx, &arknodepb.CreateInvoiceRequest{
		Amount: req.GetInvoiceAmount(),
	})
	if err != nil {
		log.Errorf("failed to get invoice: %v", err)
		return nil, err
	}

	log.Debugf("invoice: %s", invoiceResponse.GetInvoice())
	log.Debugf("preimage hash: %s", invoiceResponse.GetPreimageHash())

	// TODO: wait for the invoice to be paid

	response, err := b.arknode.CreateVHTLC(ctx, &arknodepb.CreateVHTLCRequest{
		PreimageHash:   invoiceResponse.GetPreimageHash(),
		ReceiverPubkey: req.GetPubkey(),
	})
	if err != nil {
		log.Errorf("failed to fund vHTLC: %v", err)
		return nil, err
	}

	vhtlcAddress := response.GetAddress()

	log.Debugf("vhtlc created: %s", vhtlcAddress)

	sendResp, err := b.arknode.SendOffChain(ctx, &arknodepb.SendOffChainRequest{
		Address: vhtlcAddress,
		Amount:  req.GetInvoiceAmount(),
	})
	if err != nil {
		log.Errorf("failed to send to vHTLC address: %v", err)
		return nil, err
	}
	log.Debugf("vhtlc funded: %s", sendResp.GetRoundId())

	return &pb.ReverseSubmarineSwapResponse{
		Invoice:         invoiceResponse.GetInvoice(),
		LockupAddress:   vhtlcAddress,
		RefundPublicKey: response.GetRefundPubkey(),
		PreimageHash:    invoiceResponse.GetPreimageHash(),
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
	log.Infof("created VHTLC: %s", vhtlcAddress)
	go func() {
		ctx := context.Background()

		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()

		var vhtlc *arknodepb.Vtxo

		log.Info("waiting for vHTLC to be funded...")
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

		log.Infof("vHTLC funded %s", vhtlc.Outpoint.Txid)
		log.Info("paying invoice...")

		resp, err := b.arknode.PayInvoice(ctx, &arknodepb.PayInvoiceRequest{
			Invoice: req.GetInvoice(),
		})
		if err != nil {
			log.Errorf("failed to pay invoice: %v", err)
			return
		}

		// claim the VHTLC with the preimage revealed when paying the invoice
		claimResp, err := b.arknode.ClaimVHTLC(ctx, &arknodepb.ClaimVHTLCRequest{
			Preimage: resp.GetPreimage(),
		})
		if err != nil {
			log.Errorf("failed to claim vHTLC: %v", err)
			return
		}

		log.Debugf("vHTLC claimed successfully %s", claimResp.GetRedeemTxid())
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
