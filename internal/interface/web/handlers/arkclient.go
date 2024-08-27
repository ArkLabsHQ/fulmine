package handlers

import (
	"context"
	"fmt"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/tyler-smith/go-bip39"

	arksdk "github.com/ark-network/ark/pkg/client-sdk"
	store "github.com/ark-network/ark/pkg/client-sdk/store"
	filestore "github.com/ark-network/ark/pkg/client-sdk/store/file"
)

func openStore() (store.ConfigStore, error) {
	storeSvc, err := filestore.NewConfigStore("./storage")
	if err != nil {
		return nil, fmt.Errorf("failed to open file store: %s", err)
	}
	return storeSvc, nil
}

func LoadArkClient() (arksdk.ArkClient, error) {
	if storeSvc, err := openStore(); err == nil {
		if arkClient, err := arksdk.LoadCovenantlessClient(storeSvc); err == nil {
			return arkClient, nil
		} else {
			return nil, fmt.Errorf("error loading client: %s", err)
		}
	} else {
		return nil, fmt.Errorf("error opening store: %s", err)
	}
}

func getArkClient(c *gin.Context) arksdk.ArkClient {
	if storedArkClient, exists := c.Get("arkClient"); exists {
		if arkClient, ok := storedArkClient.(arksdk.ArkClient); ok {
			return arkClient
		}
	}
	return nil
}

func deleteOldState() error {
	filePath := "./storage/state.json" // TODO
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return nil
	}
	err := os.Remove(filePath)
	if err != nil {
		return err
	}
	return nil
}

func setupFileBasedArkClient(aspurl, mnemonic, password string) (arksdk.ArkClient, error) {
	storeSvc, err := openStore()
	if err != nil {
		return nil, err
	}

	arkClient, err := arksdk.NewCovenantlessClient(storeSvc)
	if err != nil {
		return nil, err
	}

	seed := bip39.NewSeed(mnemonic, "")
	nostrKey, err := PrivateKeyFromSeed(seed)
	if err != nil {
		return nil, err
	}

	if err := arkClient.Init(context.Background(), arksdk.InitArgs{
		WalletType: arksdk.SingleKeyWallet,
		ClientType: arksdk.GrpcClient,
		AspUrl:     aspurl,
		Password:   password,
		Seed:       nostrKey,
	}); err != nil {
		return nil, err
	}

	return arkClient, nil
}
