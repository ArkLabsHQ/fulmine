package main

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"time"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	ark_node_pb "github.com/ArkLabsHQ/ark-node/api-spec/protobuf/gen/go/ark_node/v1"
	boltz_mockv1 "github.com/ArkLabsHQ/ark-node/api-spec/protobuf/gen/go/boltz_mock/v1"
	"github.com/ArkLabsHQ/ark-node/utils"
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
			Flags: append(urlFlags, &cli.Uint64Flag{
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
		{
			Name:    "reverse-swap", // ln -> ark
			Aliases: []string{"rs"},
			Usage:   "reverse-swap",
			Action:  WithLoader(reverseSwap),
			Flags: append(urlFlags, &cli.Uint64Flag{
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
		logrus.Fatal(err)
	}

}

func swap(c *cli.Context) error {
	amount := c.Uint64("amount")
	nodeURL := c.String("node-url")
	boltzURL := c.String("boltz-url")
	preimage := c.String("preimage")

	if len(preimage) == 0 {
		// generate 32 bytes preimage
		preimageBytes := make([]byte, 32)
		if _, err := rand.Read(preimageBytes); err != nil {
			return err
		}

		preimage = hex.EncodeToString(preimageBytes)
	}

	nodeClient, err := arkNodeClient(nodeURL)
	if err != nil {
		return err
	}

	boltzClient, err := boltzClient(boltzURL)
	if err != nil {
		return err
	}

	preimageBytes, err := hex.DecodeString(preimage)
	if err != nil {
		return err
	}

	preimageHash := hex.EncodeToString(btcutil.Hash160(preimageBytes))

	fmt.Printf("preimage hash = %s", preimageHash)

	response, err := boltzClient.SubmarineSwap(c.Context, &boltz_mockv1.SubmarineSwapRequest{
		From:            "ARK",
		To:              "LN",
		Invoice:         "todoinvoice", // receiving invoice
		PreimageHash:    preimage,      // only boltz-mock: we reveal the preimage to allow the server to claim without LN
		RefundPublicKey: "todopubkey",
		InvoiceAmount:   amount,
	})
	if err != nil {
		return err
	}

	addr := response.GetAddress()
	addr = utils.GetArkAddress(addr)

	_, err = nodeClient.BotlzFundVHTLC(c.Context, &ark_node_pb.BotlzFundVHTLCRequest{
		PreimageHash: preimageHash,
		Address:      addr,
		Amount:       response.ExpectedAmount,
	})
	if err != nil {
		return err
	}

	// wait for the invoice to be paid
	// reveal preimage

	// wait for the vHTLC to be claimed
	claimed := false
	for !claimed {
		time.Sleep(3 * time.Second)

		listResponse, err := nodeClient.ListVHTLC(c.Context, &ark_node_pb.ListVHTLCRequest{
			PreimageHashFilter: &preimageHash,
		})
		if err != nil {
			return err
		}

		if len(listResponse.Vhtlcs) == 0 {
			claimed = true
		}
	}

	fmt.Println("vHTLC claimed")

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

	addrResponse, err := nodeClient.GetAddress(c.Context, &ark_node_pb.GetAddressRequest{})
	if err != nil {
		return err
	}

	// address receiving vHTLC
	addr := addrResponse.GetAddress()
	addr = utils.GetArkAddress(addr)

	response, err := boltzClient.ReverseSubmarineSwap(c.Context, &boltz_mockv1.ReverseSubmarineSwapRequest{
		From:          "LN",
		To:            "ARK",
		InvoiceAmount: amount,
		OnchainAmount: amount,
		Address:       addr,
	})
	if err != nil {
		return err
	}

	invoice := response.GetInvoice()
	preimage := response.GetPreimageHash() // for mock only, boltz provides the preimage
	preimageBytes, err := hex.DecodeString(preimage)
	if err != nil {
		return err
	}

	preimageHash := hex.EncodeToString(btcutil.Hash160(preimageBytes))

	// pay the invoice
	fmt.Printf("invoice: %s", invoice)
	// search for vHTLC

	var vhtlc *ark_node_pb.Vtxo

	for vhtlc == nil {
		time.Sleep(3 * time.Second)

		listResponse, err := nodeClient.ListVHTLC(c.Context, &ark_node_pb.ListVHTLCRequest{
			PreimageHashFilter: &preimageHash,
		})
		if err != nil {
			return err
		}

		if len(listResponse.Vhtlcs) == 0 {
			continue
		}

		vhtlc = listResponse.Vhtlcs[0]
	}

	_, err = nodeClient.BoltzClaimVHTLC(c.Context, &ark_node_pb.BoltzClaimVHTLCRequest{
		Preimage: preimage,
	})
	if err != nil {
		return err
	}

	fmt.Println("vHTLC claimed !")
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
		Value:   "localhost:7001",
		Usage:   "ark node server URL (e.g. localhost:7001)",
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
