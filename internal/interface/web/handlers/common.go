package handlers

import (
	"strconv"
	"strings"

	"github.com/ArkLabsHQ/ark-node/internal/interface/web/types"
	"github.com/gin-gonic/gin"
	"github.com/tyler-smith/go-bip39"

	log "github.com/sirupsen/logrus"
)

func getAddress(c *gin.Context) string {
	arkClient := getArkClient(c)
	if offchainAddr, _, err := arkClient.Receive(c); err == nil {
		return offchainAddr
	}
	return ""
}

func getSpendableBalance(c *gin.Context) string {
	arkClient := getArkClient(c)
	if balance, err := arkClient.Balance(c, true); err == nil {
		return strconv.FormatUint(balance.OffchainBalance.Total+balance.OnchainBalance.SpendableAmount, 10)
	} else {
		log.Infof("error getting ark balance: %s", err)
	}
	return "0"
}

func getNodeBalance() string {
	return "50640" // TODO
}

func getNewMnemonic() []string {
	// 128 bits of entropy for a 12-word mnemonic
	entropy, err := bip39.NewEntropy(128)
	if err != nil {
		return strings.Fields("")
	}
	mnemonic, err := bip39.NewMnemonic(entropy)
	if err != nil {
		return strings.Fields("")
	}
	return strings.Fields(mnemonic)
}

func getNodeStatus() bool {
	return true // TODO
}

func getSettings() types.Settings {
	settings, err := ReadSettings()
	if err != nil {
		log.WithError(err).Warn("Error getting settings")
	}
	return settings
}

func isOnline(c *gin.Context) bool {
	if arkClient := getArkClient(c); arkClient != nil {
		_, err := arkClient.Balance(c, false)
		return err == nil
	}
	return false
}

func redirect(path string, c *gin.Context) {
	c.Header("HX-Redirect", path)
	c.Status(303)
}
