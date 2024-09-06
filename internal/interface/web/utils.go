package web

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/ArkLabsHQ/ark-node/internal/interface/web/types"
	"github.com/a-h/templ"
	"github.com/angelofallars/htmx-go"
	"github.com/ark-network/ark/pkg/client-sdk/client"
	"github.com/gin-gonic/gin"
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

func (s *service) getNodeStatus() bool {
	return true // TODO
}

func redirect(path string, c *gin.Context) {
	c.Header("HX-Redirect", path)
	c.Status(303)
}

func reload(c *gin.Context) {
	c.Header("HX-Refresh", "true")
}

func toastHandler(t templ.Component, c *gin.Context) {
	if !htmx.IsHTMX(c.Request) {
		// nolint:all
		c.AbortWithError(http.StatusBadRequest, errors.New("non-htmx request"))
		return
	}
	htmx.NewResponse().
		Retarget("#toast").
		AddTrigger(htmx.Trigger("toast")).
		// nolint:all
		RenderTempl(c, c.Writer, t)
}

func partialViewHandler(bodyContent templ.Component, c *gin.Context) {
	if err := htmx.NewResponse().RenderTempl(c.Request.Context(), c.Writer, bodyContent); err != nil {
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}
}

func modalHandler(t templ.Component, c *gin.Context) {
	if !htmx.IsHTMX(c.Request) {
		// nolint:all
		c.AbortWithError(http.StatusBadRequest, fmt.Errorf("non-htmx request"))
		return
	}
	// nolint:all
	htmx.NewResponse().RenderTempl(c, c.Writer, t)
}

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

func getOnchainTxs(network, addr string) ([]types.Transaction, error) {
	url := getExplorerUrl(network)

	switch network {
	case "regtest":
		url = "http://localhost:3000"
	case "liquidregtest":
		url = "http://localhost:3001"
	default:
		url += "/api"
	}

	req, err := http.NewRequest("GET", fmt.Sprintf("%s/address/%s/utxo", url, addr), nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	type utxo struct {
		Txid   string `json:"txid"`
		Vout   int    `json:"vout"`
		Amount int    `json:"value"`
		Status struct {
			Blocktime int64 `json:"blocktime"`
		} `json:"status,omitempty"`
	}

	var utxos []utxo
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(body, &utxos); err != nil {
		return nil, err
	}

	txs := make([]types.Transaction, 0, len(utxos))
	for _, utxo := range utxos {
		date := time.Now().Unix()
		if utxo.Status.Blocktime > 0 {
			date = utxo.Status.Blocktime
		}
		txs = append(txs, types.Transaction{
			Txid:     utxo.Txid,
			Date:     prettyUnixTimestamp(date),
			Day:      prettyDay(date),
			Hour:     prettyHour(date),
			Amount:   strconv.Itoa(utxo.Amount),
			Kind:     "recv",
			UnixDate: date,
			Status:   "pending",
		})
	}
	return txs, nil
}
