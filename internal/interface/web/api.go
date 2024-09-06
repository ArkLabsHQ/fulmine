package web

import (
	"fmt"
	"net/http"

	"github.com/ArkLabsHQ/ark-node/internal/core/domain"
	"github.com/ArkLabsHQ/ark-node/internal/interface/web/templates/components"

	"github.com/gin-gonic/gin"
	"github.com/tyler-smith/go-bip39"
)

func (s *service) getBalanceApi(c *gin.Context) {
	balance, err := s.svc.Balance(c, true)
	if err != nil {
		// nolint:all
		c.AbortWithError(http.StatusInternalServerError, err)
		return
	}

	data := gin.H{
		"offchain": balance.OffchainBalance.Total,
		"onchain":  balance.OnchainBalance.SpendableAmount,
		"total":    balance.OffchainBalance.Total + balance.OnchainBalance.SpendableAmount,
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
	mnemonic := c.PostForm("mnemonic")
	isValid := bip39.IsMnemonicValid(mnemonic)
	data := gin.H{
		"valid": isValid,
	}
	c.JSON(http.StatusOK, data)
}

func (s *service) claimApi(c *gin.Context) {
	if _, err := s.svc.Claim(c); err != nil {
		toast := components.Toast(err.Error(), true)
		toastHandler(toast, c)
		return
	}
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
	fmt.Println("AAAAA", password, "BBBBBBB")
	if err := s.svc.Unlock(c, password); err != nil {
		toast := components.Toast(err.Error(), true)
		toastHandler(toast, c)
		return
	}

	redirect("/", c)
}
