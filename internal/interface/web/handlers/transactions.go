package handlers

import (
	"math"
	"sort"
	"strconv"
	"time"

	"github.com/ArkLabsHQ/ark-node/internal/interface/web/types"
	"github.com/ark-network/ark/pkg/client-sdk/client"
	"github.com/gin-gonic/gin"
	log "github.com/sirupsen/logrus"
)

// Function to format Unix timestamp to a pretty date string
func prettyUnixTimestamp(unixTime int64) string {
	// return time.Unix(unixTime, 0).Format(time.RFC3339) // Adjust format as needed
	return time.Unix(unixTime, 0).Format("02/01/2006 15:04")
}

func prettyDay(unixTime int64) string {
	return time.Unix(unixTime, 0).Format("02/01/2006")
}

func prettyHour(unixTime int64) string {
	return time.Unix(unixTime, 0).Format("15:04")
}

func findVtxosBySpentBy(allVtxos []client.Vtxo, txid string) (vtxos []client.Vtxo) {
	for _, v := range allVtxos {
		if v.SpentBy == txid {
			vtxos = append(vtxos, v)
		}
	}
	return
}

// from vtxos list and asp info, reproduce list of transactions
func getTxHistory(c *gin.Context) (transactions []types.Transaction) {
	arkClient := getArkClient(c)
	if arkClient == nil {
		return
	}

	spendableVtxos, spentVtxos, err := arkClient.ListVtxos(c)
	if err != nil {
		return
	}

	log.Info("spendableVtxos")
	for _, v := range spendableVtxos {
		log.Info("---------")
		log.Infof("Amount %d", v.Amount)
		log.Infof("ExpiresAt %v", v.ExpiresAt)
		log.Infof("Pending %v", v.Pending)
		log.Infof("RoundTxid %v", v.RoundTxid)
		log.Infof("Txid %v", v.Txid)
		log.Infof("SpentBy %v", v.SpentBy)
		log.Info("---------")
	}

	log.Info("spentVtxos")
	for _, v := range spentVtxos {
		log.Info("---------")
		log.Infof("Amount %d", v.Amount)
		log.Infof("ExpiresAt %v", v.ExpiresAt)
		log.Infof("Pending %v", v.Pending)
		log.Infof("RoundTxid %v", v.RoundTxid)
		log.Infof("Txid %v", v.Txid)
		log.Infof("SpentBy %v", v.SpentBy)
		log.Info("---------")
	}

	roundLifetime := int64(604672) // TODO

	for _, v := range append(spendableVtxos, spentVtxos...) {
		// ignore not pending tx
		if !v.Pending {
			continue
		}
		// initialize some vars
		amount := int64(0)
		if v.Amount < math.MaxInt64 {
			amount = int64(v.Amount)
		}

		dateCreated := v.ExpiresAt.Unix() - roundLifetime
		// find other spent vtxos that spent this one
		relatedVtxos := findVtxosBySpentBy(spentVtxos, v.Txid)
		for _, r := range relatedVtxos {
			if r.Amount < math.MaxInt64 {
				amount -= int64(r.Amount)
			}
		}
		// what kind of tx was this? send or receive?
		kind := "recv"
		if amount < 0 {
			kind = "send"
		}
		// check if is a pending tx
		status := "success"
		if len(v.RoundTxid) == 0 && len(v.SpentBy) == 0 {
			status = "pending"
		}
		// add transaction
		transactions = append(transactions, types.Transaction{
			Amount:   strconv.FormatInt(amount, 10),
			Date:     prettyUnixTimestamp(dateCreated),
			Day:      prettyDay(dateCreated),
			Hour:     prettyHour(dateCreated),
			Kind:     kind,
			Txid:     v.Txid,
			Status:   status,
			UnixDate: dateCreated,
		})
	}

	// Sort the slice by age
	sort.Slice(transactions, func(i, j int) bool {
		return transactions[i].UnixDate > transactions[j].UnixDate
	})

	return
}
