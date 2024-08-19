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
	return "50640"
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

func getSettings() types.Settings {
	settings, err := ReadSettings()
	if err != nil {
		log.WithError(err).Warn("Error getting settings")
	}
	return settings
}

func getTransactions() [][]string {
	var transactions [][]string
	transactions = append(transactions, []string{"cd21", "send", "pending", "10/08/2024", "21:42", "+56632"})
	transactions = append(transactions, []string{"abcd", "send", "waiting", "09/08/2024", "21:42", "+212110"})
	transactions = append(transactions, []string{"1234", "send", "success", "08/08/2024", "21:42", "-645543"})
	transactions = append(transactions, []string{"ab12", "send", "success", "07/08/2024", "21:42", "-645543"})
	transactions = append(transactions, []string{"f3f3", "recv", "success", "06/08/2024", "21:42", "+56632"})
	transactions = append(transactions, []string{"ffee", "recv", "failure", "05/08/2024", "21:42", "+655255"})
	transactions = append(transactions, []string{"445d", "swap", "success", "04/08/2024", "21:42", "+42334"})
	return transactions
}

func redirect(path string, c *gin.Context) {
	c.Header("HX-Redirect", path)
	c.Status(303)
}
