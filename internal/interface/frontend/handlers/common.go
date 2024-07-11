package handlers

import (
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/ArkLabsHQ/ark-wallet/types"
)

func getAddress() string {
	return "ark18746676365652bcdabdbacdabcd63546354634"
}

func getBalance() string {
	return "1930547"
}

func getNewMnemonic() []string {
	mnemonic := "ski this panic exit erode peasant nose swim spell sleep unique bag"
	return strings.Fields(mnemonic)
}

func getSettings() types.Settings {
	return types.Settings{
		ApiRoot:     "https://fulmine.io/api/D9D90N192031",
		Currency:    "usd",
		FullNode:    "http://arklabs.to/node/213908123",
		EventServer: "http://arklabs.to/node/jupiter29",
		Unit:        "sat",
	}
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
