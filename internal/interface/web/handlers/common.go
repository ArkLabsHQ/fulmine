package handlers

import (
	"strconv"
	"strings"
	"time"

	"github.com/ArkLabsHQ/ark-node/internal/interface/web/types"
	"github.com/ark-network/ark/pkg/client-sdk/client"
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

// Function to format Unix timestamp to a pretty date string
func prettyUnixTimestamp(unixTime int64) string {
	return time.Unix(unixTime, 0).Format(time.RFC3339) // Adjust format as needed
}

// from vtxos list and asp info, reproduce list of transactions
// 1. every vtxo represents a creation event (aggregated by pool tx)
// 2. every vtxo with spentBy represents a spend event (aggregated by pool tx)
// 3. for each pool tx on created event list, create a transaction event
// 4. for each pool tx on spent events list, create a transaction event if no creation
// 5. for each swept spendable utxo, mark as swept transaction
func getTxHistory(c *gin.Context) (transactions []types.Transaction) {
	arkClient := getArkClient(c)
	if arkClient == nil {
		return
	}

	spendableVtxos, spentVtxos, err := arkClient.ListVtxos(c)
	if err != nil {
		return
	}

	roundLifetime := int64(604672)
	createdVtxos := make(map[string]*types.PoolTxs)
	absentVtxos := make(map[string]*types.PoolTxs)

	// 1. Aggregate creation events by pool tx
	for _, v := range append(spendableVtxos, spentVtxos...) {
		if _, exists := createdVtxos[v.RoundTxid]; !exists {
			blockTime := v.ExpiresAt.Unix() - roundLifetime
			createdVtxos[v.RoundTxid] = &types.PoolTxs{
				DateCreated: blockTime,
				Vtxos:       []client.Vtxo{v},
			}
		} else {
			createdVtxos[v.RoundTxid].Vtxos = append(createdVtxos[v.RoundTxid].Vtxos, v)
		}
	}

	// 2. Aggregate spend events by pool tx
	for _, v := range spentVtxos {
		if v.RoundTxid == "" {
			continue
		}
		if _, exists := absentVtxos[v.RoundTxid]; !exists {
			dateCreated := v.ExpiresAt.Unix() - roundLifetime
			if _, exists := createdVtxos[v.RoundTxid]; exists {
				dateCreated = createdVtxos[v.RoundTxid].DateCreated
			}
			absentVtxos[v.RoundTxid] = &types.PoolTxs{
				DateCreated: dateCreated,
				Vtxos:       []client.Vtxo{v},
			}
		} else {
			absentVtxos[v.RoundTxid].Vtxos = append(absentVtxos[v.RoundTxid].Vtxos, v)
		}
	}

	// 3. Create transaction events for created events
	for roundTxid, created := range createdVtxos {
		createdAmount := int64(0)
		for _, v := range created.Vtxos {
			createdAmount += int64(v.Amount)
		}
		spentAmount := int64(0)
		if spent, exists := absentVtxos[roundTxid]; exists {
			for _, v := range spent.Vtxos {
				spentAmount += int64(v.Amount)
			}
		}
		amount := createdAmount - spentAmount
		transactions = append(transactions, types.Transaction{
			Amount:   amount,
			Date:     prettyUnixTimestamp(created.DateCreated),
			Txid:     roundTxid,
			UnixDate: created.DateCreated,
		})
	}

	// 4. Create transaction events for spent events if no creation
	for roundTxid, spent := range absentVtxos {
		if _, exists := createdVtxos[roundTxid]; exists {
			continue
		}
		spentAmount := int64(0)
		for _, v := range spent.Vtxos {
			spentAmount += int64(v.Amount)
		}
		transactions = append(transactions, types.Transaction{
			Amount:   -spentAmount,
			Date:     prettyUnixTimestamp(spent.DateCreated),
			Txid:     roundTxid,
			UnixDate: spent.DateCreated,
		})
	}

	return
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
