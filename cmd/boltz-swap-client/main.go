package main

import (
	"fmt"
	"os"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	ark_node_pb "github.com/ArkLabsHQ/ark-node/api-spec/protobuf/gen/go/ark_node/v1"
	boltz_mockv1 "github.com/ArkLabsHQ/ark-node/api-spec/protobuf/gen/go/boltz_mock/v1"
)

func main() {
	app := cli.NewApp()
	app.Name = "ark swapper"
	app.Usage = "swap ark to ln and ln to ark"

	app.Flags = []cli.Flag{}

	app.Commands = []*cli.Command{
		{
			Name:    "swap", // ark -> ln
			Aliases: []string{"s"},
			Usage:   "swap",
			Action:  WithLoader(swap),
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
			Action:  WithLoader(reverseSwap),
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

	log.Info("retrieving wallet pubkey...")
	addrResp, err := nodeClient.GetAddress(c.Context, &ark_node_pb.GetAddressRequest{})
	if err != nil {
		return err
	}

	log.Info("creating invoice...")
	invoiceResp, err := nodeClient.CreateInvoice(c.Context, &ark_node_pb.CreateInvoiceRequest{
		Amount: amount,
	})
	if err != nil {
		return fmt.Errorf("failed to get invoice: %w", err)
	}
	invoice := invoiceResp.GetInvoice()
	preimageHash := invoiceResp.GetPreimageHash()

	log.Info("calling Boltz submarine swap API...")
	log.Infof("params:\nrefund pubkey: %s\ninvoice and preimage hash: %s %s\namount %d", addrResp.GetPubkey(), invoice, preimageHash, amount)
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
	log.Infof("vHTLC created: %s", swapResponse.GetAddress())

	// verify that the vHTLC is correct and Boltz is not try to scam us
	// - ask our node for a vhtlc with the same params and expect some leafs to match
	htlcResponse, err := nodeClient.CreateVHTLC(c.Context, &ark_node_pb.CreateVHTLCRequest{
		PreimageHash: preimageHash,
		SenderPubkey: addrResp.GetPubkey(),
	})
	if err != nil {
		return fmt.Errorf("failed to create verification vHTLC: %v", err)
	}

	// TODO: maybe there's a better way to compare the two vHTLCs
	treeB := swapResponse.GetSwapTree()
	treeA := htlcResponse.GetSwapTree()
	if treeB.GetRefundWithoutBoltzLeaf().String() != treeA.GetRefundWithoutBoltzLeaf().String() || treeB.GetUnilateralRefundWithoutBoltzLeaf().String() != treeA.GetUnilateralRefundWithoutBoltzLeaf().String() {
		return fmt.Errorf("boltz is trying to scam us, vHTLCs do not match")
	}

	log.Info("funding vHTLC...")
	if _, err = nodeClient.SendOffChain(c.Context, &ark_node_pb.SendOffChainRequest{
		Address: swapResponse.GetAddress(),
		Amount:  amount,
	}); err != nil {
		return fmt.Errorf("failed to send to vHTLC address: %v", err)
	}

	log.Info("done!")

	return nil
}

func reverseSwap(c *cli.Context) error {
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

	addrResp, err := nodeClient.GetAddress(c.Context, &ark_node_pb.GetAddressRequest{})
	if err != nil {
		return err
	}

	response, err := boltzClient.ReverseSubmarineSwap(c.Context, &boltz_mockv1.ReverseSubmarineSwapRequest{
		From:          "LN",
		To:            "ARK",
		InvoiceAmount: amount,
		OnchainAmount: amount,
		Pubkey:        addrResp.GetPubkey(),
	})
	if err != nil {
		return err
	}

	invoice := response.GetInvoice()
	preimageHash := response.GetPreimageHash()

	log.Infof("paying invoice...")
	invoiceResp, err := nodeClient.PayInvoice(c.Context, &ark_node_pb.PayInvoiceRequest{
		Invoice: invoice,
	})
	if err != nil {
		return err
	}

	log.Infof("invoice paid %s", invoice)
	log.Info("waiting for vHTLC to be funded...")

	ticker := time.NewTicker(3 * time.Second)
	for range ticker.C {
		listResponse, err := nodeClient.ListVHTLC(c.Context, &ark_node_pb.ListVHTLCRequest{
			PreimageHashFilter: &preimageHash,
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

	log.Infof("vHTLC funded, claiming...")
	claimResponse, err := nodeClient.ClaimVHTLC(c.Context, &ark_node_pb.ClaimVHTLCRequest{
		Preimage: invoiceResp.GetPreimage(),
	})
	if err != nil {
		return err
	}

	log.Infof("vHTLC claimed %s", claimResponse.GetRedeemTxid())
	return nil
}

func arkNodeClient(url string) (ark_node_pb.ServiceClient, error) {
	conn, err := grpc.NewClient(url, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, err
	}

	return ark_node_pb.NewServiceClient(conn), nil
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

// Loader function with animation
func loader(done chan bool) {
	chars := []rune{'|', '/', '-', '\\'}
	for {
		select {
		case <-done:
			fmt.Printf("\rDone!          \n")
			return
		default:
			for _, r := range chars {
				fmt.Printf("\r%c Loading...", r)
				time.Sleep(100 * time.Millisecond)
			}
		}
	}
}

// Wrapper for cli.ActionFunc that adds a loader animation
func WithLoader(action cli.ActionFunc) cli.ActionFunc {
	return func(c *cli.Context) error {
		done := make(chan bool)

		// Start the loader animation in a separate goroutine
		go loader(done)

		// Execute the original action
		err := action(c)

		// Stop the loader animation
		done <- true

		// Optional: Add a newline for proper formatting after loader stops
		fmt.Println()

		return err
	}
}
