package web

import (
	"net/http"
	"time"

	"github.com/ArkLabsHQ/ark-node/internal/core/domain"
	"github.com/ArkLabsHQ/ark-node/internal/interface/web/templates/components"
	"github.com/ArkLabsHQ/ark-node/utils"

	"github.com/gin-gonic/gin"
)

func (s *service) getBalanceApi(c *gin.Context) {
	balance, err := s.svc.Balance(c, false)
	if err != nil {
		// nolint:all
		c.AbortWithError(http.StatusInternalServerError, err)
		return
	}

	onchainBalance := balance.OnchainBalance.SpendableAmount
	for _, amount := range balance.OnchainBalance.LockedAmount {
		onchainBalance += amount.Amount
	}
	data := gin.H{
		"offchain": balance.OffchainBalance.Total,
		"onchain":  onchainBalance,
		"total":    balance.OffchainBalance.Total + onchainBalance,
	}
	c.JSON(http.StatusOK, data)
}

func (s *service) updateSettingsApi(c *gin.Context) {
	settings := domain.Settings{}
	if apiroot := c.PostForm("apiroot"); len(apiroot) > 0 {
		settings.ApiRoot = apiroot
	}

	if currency := c.PostForm("currency"); len(currency) > 0 {
		settings.Currency = currency
	}

	if eventServer := c.PostForm("eventserver"); len(eventServer) > 0 {
		settings.EventServer = eventServer
	}

	if fullNode := c.PostForm("fullnode"); len(fullNode) > 0 {
		settings.FullNode = fullNode
	}

	// TODO lnconnect

	if lnURL := c.PostForm("lnurl"); len(lnURL) > 0 {
		settings.LnUrl = lnURL
	}

	if unit := c.PostForm("unit"); len(unit) > 0 {
		settings.Unit = unit
	}

	if err := s.svc.UpdateSettings(c, settings); err != nil {
		toast := components.Toast(err.Error(), true)
		toastHandler(toast, c)
		return
	}

	toast := components.Toast("Saved")
	toastHandler(toast, c)
}

func (s *service) connectNodeApi(c *gin.Context) {
	// TODO: manage node connection
	toast := components.Toast("Connected")
	toastHandler(toast, c)
}

func (s *service) disconnectNodeApi(c *gin.Context) {
	// TODO: manage node connection
	toast := components.Toast("Disconnected")
	toastHandler(toast, c)
}

func (s *service) validateMnemonicApi(c *gin.Context) {
	var data gin.H
	mnemonic := c.PostForm("mnemonic")
	err := utils.IsValidMnemonic(mnemonic)
	if err == nil {
		data = gin.H{
			"valid": true,
		}
	} else {
		data = gin.H{
			"valid": false,
			"error": err.Error(),
		}
	}
	c.JSON(http.StatusOK, data)
}

func (s *service) validateUrlApi(c *gin.Context) {
	url := c.PostForm("url")
	valid := utils.IsValidURL(url)
	data := gin.H{
		"valid": valid,
	}
	c.JSON(http.StatusOK, data)
}

func (s *service) claimApi(c *gin.Context) {
	if _, err := s.svc.ClaimPending(c); err != nil {
		toast := components.Toast(err.Error(), true)
		toastHandler(toast, c)
		return
	}
	time.Sleep(3 * time.Second) // wait or the reload will not change the UI
	reload(c)
}

func (s *service) lockApi(c *gin.Context) {
	password := c.PostForm("password")
	if password == "" {
		toast := components.Toast("Password can't be empty", true)
		toastHandler(toast, c)
		return
	}

	if err := s.svc.Lock(c, password); err != nil {
		toast := components.Toast(err.Error(), true)
		toastHandler(toast, c)
		return
	}

	redirect("/", c)
}

func (s *service) unlockApi(c *gin.Context) {
	password := c.PostForm("password")
	if password == "" {
		toast := components.Toast("Password can't be empty", true)
		toastHandler(toast, c)
		return
	}

	if err := s.svc.UnlockNode(c, password); err != nil {
		toast := components.Toast(err.Error(), true)
		toastHandler(toast, c)
		return
	}

	redirect("/", c)
}
