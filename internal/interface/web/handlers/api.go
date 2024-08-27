package handlers

import (
	"errors"
	"net/http"

	"github.com/ArkLabsHQ/ark-node/internal/interface/web/templates/components"
	"github.com/a-h/templ"
	"github.com/angelofallars/htmx-go"

	"github.com/gin-gonic/gin"
	"github.com/tyler-smith/go-bip39"
)

func toastHandler(t templ.Component, c *gin.Context) {
	if !htmx.IsHTMX(c.Request) {
		//nolint:all
		c.AbortWithError(http.StatusBadRequest, errors.New("non-htmx request"))
		return
	}
	htmx.NewResponse().
		Retarget("#toast").
		AddTrigger(htmx.Trigger("toast")).
		//nolint:all
		RenderTempl(c, c.Writer, t)
}

func BalanceApiGet(c *gin.Context) {
	if arkClient := getArkClient(c); arkClient != nil {
		if balance, err := arkClient.Balance(c, true); err == nil {
			data := gin.H{
				"offchain": balance.OffchainBalance.Total,
				"onchain":  balance.OnchainBalance.SpendableAmount,
				"total":    balance.OffchainBalance.Total + balance.OnchainBalance.SpendableAmount,
			}
			c.JSON(http.StatusOK, data)
			return
		}
	}
	// something went wrong
	c.AbortWithStatus(http.StatusInternalServerError)
}

func SettingsApiPost(c *gin.Context) {
	// TODO: manage new settings posted
	settings := getSettings()

	apiroot := c.PostForm("apiroot")
	if len(apiroot) > 0 {
		settings.ApiRoot = apiroot
	}

	currency := c.PostForm("currency")
	if len(currency) > 0 {
		settings.Currency = currency
	}

	eventserver := c.PostForm("eventserver")
	if len(eventserver) > 0 {
		settings.EventServer = eventserver
	}

	fullnode := c.PostForm("fullnode")
	if len(fullnode) > 0 {
		settings.FullNode = fullnode
	}

	// TODO lnconnect

	lnurl := c.PostForm("lnurl")
	if len(lnurl) > 0 {
		settings.LnUrl = lnurl
	}

	unit := c.PostForm("unit")
	if len(unit) > 0 {
		settings.Unit = unit
	}

	err := WriteSettings(settings)
	if err != nil {
		toast := components.Toast(err.Error(), true)
		toastHandler(toast, c)
		return
	}

	toast := components.Toast("Saved")
	toastHandler(toast, c)
}

func NodeConnectApiPost(c *gin.Context) {
	// TODO: manage node connection
	toast := components.Toast("Connected")
	toastHandler(toast, c)
}

func NodeDisconnectApiPost(c *gin.Context) {
	// TODO: manage node connection
	toast := components.Toast("Disconnected")
	toastHandler(toast, c)
}

func ValidateMnemonic(c *gin.Context) {
	mnemonic := c.PostForm("mnemonic")
	isValid := bip39.IsMnemonicValid(mnemonic)
	data := gin.H{
		"valid": isValid,
	}
	c.JSON(http.StatusOK, data)
}

func Lock(c *gin.Context) {
	password := c.PostForm("password")
	if password == "" {
		toast := components.Toast("Password can't be empty", true)
		toastHandler(toast, c)
		return
	}

	arkClient := getArkClient(c)
	if arkClient == nil {
		toast := components.Toast("Ark client not found", true)
		toastHandler(toast, c)
		return
	}

	err := arkClient.Lock(c, password)
	if err != nil {
		toast := components.Toast(err.Error(), true)
		toastHandler(toast, c)
		return
	}

	redirect("/", c)
}

func Unlock(c *gin.Context) {
	password := c.PostForm("password")
	if password == "" {
		toast := components.Toast("Password can't be empty", true)
		toastHandler(toast, c)
		return
	}

	arkClient := getArkClient(c)
	if arkClient == nil {
		toast := components.Toast("Ark client not found", true)
		toastHandler(toast, c)
		return
	}

	err := arkClient.Unlock(c, password)
	if err != nil {
		toast := components.Toast(err.Error(), true)
		toastHandler(toast, c)
		return
	}

	redirect("/", c)
}
