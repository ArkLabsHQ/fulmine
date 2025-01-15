package main

import (
	"fmt"
	"os"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	arknodepb "github.com/ArkLabsHQ/ark-node/api-spec/protobuf/gen/go/ark_node/v1"
	boltz_mockv1 "github.com/ArkLabsHQ/ark-node/api-spec/protobuf/gen/go/boltz_mock/v1"
)

func main() {
	log.SetLevel(log.DebugLevel)
	app := cli.NewApp()
	app.Name = "ark swapper"
	app.Usage = "swap ark to ln and ln to ark"

	app.Flags = []cli.Flag{}

	app.Commands = []*cli.Command{
		{
			Name:    "swap", // ark -> ln
			Aliases: []string{"s"},
			Usage:   "swap",
			Action:  swap,
			Flags: append(urlFlags,
				&cli.Uint64Flag{
					Name:     "amount",
					Aliases:  []string{"a"},
					Required: true,
					Usage:    "amount to swap",
				},
			),
		},
		{
			Name:    "reverse-swap", // ln -> ark
			Aliases: []string{"rs"},
			Usage:   "reverse-swap",
			Action:  reverseSwap,
			Flags: append(urlFlags,
				&cli.Uint64Flag{
					Name:     "amount",
					Aliases:  []string{"a"},
					Required: true,
					Usage:    "amount to swap",
				},
				&cli.StringFlag{
					Name:    "preimage",
					Aliases: []string{"p"},
					Usage:   "secret preimage",
				},
			),
		},
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}

}

func swap(c *cli.Context) error {
	log.Info("processing submarine swap")

	amount := c.Uint64("amount")
	nodeURL := c.String("node-url")
	boltzURL := c.String("boltz-url")

	nodeClient, err := arkNodeClient(nodeURL)
	if err != nil {
		return err
	}

	boltzClient, err := boltzClient(boltzURL)
	if err != nil {
		return err
	}

	addrResp, err := nodeClient.GetAddress(c.Context, &arknodepb.GetAddressRequest{})
	if err != nil {
		return err
	}

	log.Debug("creating invoice...")

	invoiceResp, err := nodeClient.CreateInvoice(c.Context, &arknodepb.CreateInvoiceRequest{
		Amount: amount,
	})
	if err != nil {
		return fmt.Errorf("failed to create invoice: %w", err)
	}
	invoice := invoiceResp.GetInvoice()
	preimageHash := invoiceResp.GetPreimageHash()

	log.Debugf("invoice: %s", invoice)
	log.Debugf("preimage hash: %s", preimageHash)
	log.Debug("calling Boltz submarine swap API...")

	swapResponse, err := boltzClient.SubmarineSwap(c.Context, &boltz_mockv1.SubmarineSwapRequest{
		From:            "ARK",
		To:              "LN",
		Invoice:         invoice,
		PreimageHash:    preimageHash,
		RefundPublicKey: addrResp.GetPubkey(),
		InvoiceAmount:   amount,
	})
	if err != nil {
		return err
	}

	log.Debugf("vHTLC returned by Boltz: %s", swapResponse.GetAddress())
	log.Debug("verifying vHTLC...")

	vhtlcResponse, err := nodeClient.CreateVHTLC(c.Context, &arknodepb.CreateVHTLCRequest{
		PreimageHash:   preimageHash,
		ReceiverPubkey: swapResponse.GetClaimPublicKey(),
	})
	if err != nil {
		return fmt.Errorf("failed to verify vHTLC: %v", err)
	}
	if swapResponse.GetAddress() != vhtlcResponse.GetAddress() {
		return fmt.Errorf("boltz is trying to scam us, vHTLCs do not match")
	}

	log.Debug("vHTLC verified, funding...")

	sendResp, err := nodeClient.SendOffChain(c.Context, &arknodepb.SendOffChainRequest{
		Address: swapResponse.GetAddress(),
		Amount:  amount,
	})
	if err != nil {
		return fmt.Errorf("failed to send to vHTLC address: %v", err)
	}

	log.Debugf("vHTLC funded: %s", sendResp.GetTxid())
	log.Debug("waiting for invoice to be paid...")

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		resp, err := nodeClient.IsInvoiceSettled(c.Context, &arknodepb.IsInvoiceSettledRequest{
			Invoice: invoice,
		})
		if err != nil {
			return fmt.Errorf("failed to check invoice status: %s", err)
		}
		if resp.GetSettled() {
			break
		}
	}

	log.Debug("invoice was paid")
	log.Info("submarine swap completed successfully 🎉")
	return nil
}

func reverseSwap(c *cli.Context) error {
	log.Info("processing reverse submarine swap")

	amount := c.Uint64("amount")
	nodeURL := c.String("node-url")
	boltzURL := c.String("boltz-url")

	nodeClient, err := arkNodeClient(nodeURL)
	if err != nil {
		return err
	}

	boltzClient, err := boltzClient(boltzURL)
	if err != nil {
		return err
	}

	addrResp, err := nodeClient.GetAddress(c.Context, &arknodepb.GetAddressRequest{})
	if err != nil {
		return err
	}

	log.Debug("calling Boltz reverse submarine swap API...")

	swapResponse, err := boltzClient.ReverseSubmarineSwap(c.Context, &boltz_mockv1.ReverseSubmarineSwapRequest{
		From:          "LN",
		To:            "ARK",
		InvoiceAmount: amount,
		OnchainAmount: amount,
		Pubkey:        addrResp.GetPubkey(),
	})
	if err != nil {
		return err
	}

	log.Debugf("vHTLC returned by Boltz: %s", swapResponse.GetLockupAddress())
	log.Debug("verifying vHTLC...")

	vhtlcResponse, err := nodeClient.CreateVHTLC(c.Context, &arknodepb.CreateVHTLCRequest{
		PreimageHash: swapResponse.GetPreimageHash(),
		SenderPubkey: swapResponse.GetRefundPublicKey(),
	})
	if err != nil {
		return fmt.Errorf("failed to verify vHTLC: %s", err)
	}
	if swapResponse.GetLockupAddress() != vhtlcResponse.GetAddress() {
		return fmt.Errorf("boltz is trying to scam us, vHTLCs do not match")
	}

	log.Debug("vHTLC verified, paying invoice...")

	invoiceResp, err := nodeClient.PayInvoice(c.Context, &arknodepb.PayInvoiceRequest{
		Invoice: swapResponse.GetInvoice(),
	})
	if err != nil {
		return err
	}

	log.Debugf("invoice was paid, waiting for vHTLC to be funded...")

	ticker := time.NewTicker(3 * time.Second)
	for range ticker.C {
		listResponse, err := nodeClient.ListVHTLC(c.Context, &arknodepb.ListVHTLCRequest{
			PreimageHashFilter: swapResponse.GetPreimageHash(),
		})
		if err != nil {
			continue
		}
		if len(listResponse.GetVhtlcs()) == 0 {
			continue
		}

		break
	}
	ticker.Stop()

	log.Debugf("vHTLC funded, claiming...")

	claimResponse, err := nodeClient.ClaimVHTLC(c.Context, &arknodepb.ClaimVHTLCRequest{
		Preimage: invoiceResp.GetPreimage(),
	})
	if err != nil {
		return err
	}

	log.Debugf("claimed funds from vHTLC: %s", claimResponse.GetRedeemTxid())
	log.Info("submarine swap completed successfully 🎉")
	return nil
}

func arkNodeClient(url string) (arknodepb.ServiceClient, error) {
	conn, err := grpc.NewClient(url, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, err
	}

	return arknodepb.NewServiceClient(conn), nil
}

func boltzClient(url string) (boltz_mockv1.ServiceClient, error) {
	conn, err := grpc.NewClient(url, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, err
	}

	return boltz_mockv1.NewServiceClient(conn), nil
}

var urlFlags = []cli.Flag{
	&cli.StringFlag{
		Name:    "node-url",
		Aliases: []string{"n"},
		Value:   "localhost:7000",
		Usage:   "ark node server URL (e.g. localhost:7000)",
	},
	&cli.StringFlag{
		Name:    "boltz-url",
		Aliases: []string{"b"},
		Value:   "localhost:9000",
		Usage:   "boltz-mock server URL (e.g. localhost:9000)",
	},
}
