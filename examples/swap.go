package examples

import (
	"context"
	"encoding/hex"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/ArkLabsHQ/fulmine/pkg/boltz"
	"github.com/ArkLabsHQ/fulmine/pkg/swap"
	arklib "github.com/arkade-os/arkd/pkg/ark-lib"
	arksdk "github.com/arkade-os/go-sdk"
	grpcclient "github.com/arkade-os/go-sdk/client/grpc"
	indexerTransport "github.com/arkade-os/go-sdk/indexer/grpc"
	"github.com/arkade-os/go-sdk/store"
	"github.com/arkade-os/go-sdk/types"
	"github.com/btcsuite/btcd/btcec/v2"
)

type swapExampleClient struct {
	handler *swap.SwapHandler
}

func newSwapExampleClient(ctx context.Context, serverUrl, explorerUrl, boltzUrl, boltzWsUrl string, swapTimeout uint32) (*swapExampleClient, error) {
	tempDir := os.TempDir()

	defaultPassword := "secret"

	storeConfig := store.Config{
		ConfigStoreType:  types.InMemoryStore,
		AppDataStoreType: types.SQLStore,
		BaseDir:          tempDir,
	}

	storeSvc, err := store.NewStore(storeConfig)
	if err != nil {
		return nil, err
	}

	arkClient, err := arksdk.NewArkClient(storeSvc)
	if err != nil {
		return nil, err
	}

	privateKey, err := btcec.NewPrivateKey()
	if err != nil {
		return nil, err
	}

	seed := hex.EncodeToString(privateKey.Serialize())
	err = arkClient.Init(ctx, arksdk.InitArgs{
		ClientType:           arksdk.GrpcClient,
		WalletType:           arksdk.SingleKeyWallet,
		ServerUrl:            serverUrl,
		ExplorerURL:          explorerUrl,
		Password:             defaultPassword,
		Seed:                 seed,
		WithTransactionFeed:  true,
		ExplorerPollInterval: 2 * time.Second,
	})

	if err != nil {
		return nil, err
	}

	err = arkClient.Unlock(ctx, defaultPassword)
	if err != nil {
		return nil, err
	}

	transportClient, err := grpcclient.NewClient(serverUrl)
	if err != nil {
		return nil, err
	}

	indexerClient, err := indexerTransport.NewClient(serverUrl)
	if err != nil {
		return nil, err
	}

	boltzAPI := &boltz.Api{
		URL:   boltzUrl,
		WSURL: boltzWsUrl,
		Client: http.Client{
			Timeout: 30 * time.Second,
		},
	}

	clientInstance := &swapExampleClient{
		handler: swap.NewSwapHandler(arkClient, transportClient, indexerClient, boltzAPI, privateKey.PubKey(), swapTimeout),
	}

	return clientInstance, nil
}

func (c *swapExampleClient) PayInvoice(ctx context.Context, invoice string) (*swap.Swap, error) {
	return c.handler.PayInvoice(ctx, invoice, func(s swap.Swap) error {
		// schedule Unilateral Refund
		go func() {
			locktime := arklib.AbsoluteLocktime(s.TimeoutInfo.RefundLocktime)
			if locktime.IsSeconds() {
				at := time.Unix(int64(locktime), 0)
				refundTime := time.Until(at.Add(10 * time.Second))
				fmt.Printf("Scheduling unilateral refund in %s\n", refundTime.String())
				time.Sleep(refundTime)
				c.handler.RefundVHTLC(context.Background(), s.Id, false, *s.Opts)
			} else {
				fmt.Println("Explorer needed for block based Refunding")
			}
		}()

		return nil
	})
}

func (c *swapExampleClient) GetSwapInvoice(ctx context.Context, amountSats uint64) (string, error) {
	swapDetails, err := c.handler.GetInvoice(ctx, amountSats, func(s swap.Swap) error {
		if s.Status == swap.SwapSuccess {
			fmt.Printf("Swap %s succeeded!\n", s.Id)
		} else {
			fmt.Printf("Swap %s failed!\n", s.Id)
		}

		return nil
	})
	if err != nil {
		return "", err
	}

	return swapDetails.Invoice, nil
}
