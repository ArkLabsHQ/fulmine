package web

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/ArkLabsHQ/ark-node/utils"
	"github.com/a-h/templ"
	"github.com/angelofallars/htmx-go"
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

func getNewPrivateKey() string {
	words := getNewMnemonic()
	mnemonic := strings.Join(words, " ")
	privateKey, err := utils.PrivateKeyFromMnemonic(mnemonic)
	if err != nil {
		return ""
	}
	return privateKey
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
	if unixTime == 0 {
		return "0"
	}
	return time.Unix(unixTime, 0).Format("02/01/2006")
}

func prettyHour(unixTime int64) string {
	if unixTime == 0 {
		return "0"
	}
	return time.Unix(unixTime, 0).Format("15:04")
}
