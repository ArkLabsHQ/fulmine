package web

import (
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ArkLabsHQ/ark-node/internal/interface/web/types"
	"github.com/ArkLabsHQ/ark-node/utils"
	sdktypes "github.com/ark-network/ark/pkg/client-sdk/types"
	"github.com/gin-gonic/gin"
	log "github.com/sirupsen/logrus"
	"github.com/tyler-smith/go-bip39"
)

func getExplorerUrl(network string) string {
	switch network {
	case "liquid":
		return "https://liquid.network"
	case "bitcoin":
		return "https://mempool.space"
	case "signet":
		return "https://mutinynet.com"
	case "liquidtestnet":
		return "https://liquid.network/testnet"
	case "liquidregtest":
		return "http://localhost:5001"
	default:
		return "http://localhost:5000"
	}
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

func getNewPrivateKey() string {
	words := getNewMnemonic()
	mnemonic := strings.Join(words, " ")
	privateKey, err := utils.PrivateKeyFromMnemonic(mnemonic)
	if err != nil {
		return ""
	}
	return privateKey
}

func (s *service) getNodeBalance() string {
	return "50640" // TODO
}

func (s *service) getNodeStatus() bool {
	return true // TODO
}

func (s *service) getSpendableBalance(c *gin.Context) (string, error) {
	balance, err := s.svc.Balance(c, false)
	if err != nil {
		return "", err
	}
	onchainBalance := balance.OnchainBalance.SpendableAmount
	for _, amount := range balance.OnchainBalance.LockedAmount {
		onchainBalance += amount.Amount
	}
	return strconv.FormatUint(
		balance.OffchainBalance.Total+onchainBalance, 10,
	), nil
}

func (s *service) getTxHistory(c *gin.Context) (transactions []types.Transaction, err error) {
	// get tx history from ASP
	history, err := s.svc.GetTransactionHistory(c)
	if err != nil {
		return nil, err
	}
	data, err := s.svc.GetConfigData(c)
	if err != nil {
		return nil, err
	}
	// sort history by time but with pending first
	sort.Slice(history, func(i, j int) bool {
		if history[i].IsPending && !history[j].IsPending {
			return true
		}
		if !history[i].IsPending && history[j].IsPending {
			return false
		}
		return history[i].CreatedAt.Unix() > history[j].CreatedAt.Unix()
	})
	// transform each sdktypes.Transaction to types.Transaction
	for _, tx := range history {
		// amount
		amount := strconv.FormatUint(tx.Amount, 10)
		if tx.Type == sdktypes.TxSent {
			amount = "-" + amount
		}
		// date of creation
		dateCreated := tx.CreatedAt.Unix()
		// TODO: use tx.ExpiresAt when it will be available
		expiresAt := tx.CreatedAt.Unix() + data.RoundLifetime
		// status of tx
		status := "success"
		if tx.IsPending {
			status = "pending"
		}
		emptyTime := time.Time{}
		if tx.CreatedAt == emptyTime {
			status = "unconfirmed"
			dateCreated = 0
		}
		// get one txid to identify tx
		txid := tx.RoundTxid
		explorable := true
		if len(txid) == 0 {
			txid = tx.RedeemTxid
			explorable = false
		}
		if len(txid) == 0 {
			txid = tx.BoardingTxid
			explorable = true
		}
		// add to slice of transactions
		transactions = append(transactions, types.Transaction{
			Amount:     amount,
			CreatedAt:  prettyUnixTimestamp(dateCreated),
			Day:        prettyDay(dateCreated),
			ExpiresAt:  prettyUnixTimestamp(expiresAt),
			Explorable: explorable,
			Hour:       prettyHour(dateCreated),
			Kind:       string(tx.Type),
			Txid:       txid,
			Status:     status,
			UnixDate:   dateCreated,
		})
	}
	log.Infof("history %+v", history)
	log.Infof("transactions %+v", transactions)
	return
}

func (s *service) redirectedBecauseWalletIsLocked(c *gin.Context) bool {
	redirect := s.svc.IsLocked(c)
	if redirect {
		c.Redirect(http.StatusFound, "/")
	}
	return redirect
}
