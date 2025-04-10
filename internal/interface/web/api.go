package web

import (
	"net/http"
	"strings"

	"github.com/ArkLabsHQ/fulmine/internal/core/domain"
	"github.com/ArkLabsHQ/fulmine/internal/interface/web/templates/components"
	"github.com/ArkLabsHQ/fulmine/utils"

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

	if lnURL := c.PostForm("lnurl"); utils.IsValidLnUrl(lnURL) {
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

func (s *service) connectLNDApi(c *gin.Context) {
	url := c.PostForm("lnurl")
	err := s.svc.ConnectLN(c.Request.Context(), url)
	if err != nil {
		toast := components.Toast(err.Error(), true)
		toastHandler(toast, c)
		return
	}
	reload(c)
}

func (s *service) disconnectLNDApi(c *gin.Context) {
	s.svc.DisconnectLN()
	reload(c)
}

func (s *service) forgotApi(c *gin.Context) {
	if err := s.svc.ResetWallet(c); err != nil {
		toast := components.Toast("Unable to delete previous wallet", true)
		toastHandler(toast, c)
		return
	}
	redirect("/welcome", c)
}

func (s *service) validateNoteApi(c *gin.Context) {
	var data gin.H
	note := c.PostForm("note")
	sats := utils.SatsFromNote(note)
	if sats > 0 {
		data = gin.H{
			"sats":  sats,
			"valid": true,
		}
	} else {
		data = gin.H{
			"valid": false,
			"error": "invalid note",
		}
	}
	c.JSON(http.StatusOK, data)
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

func (s *service) validatePrivateKeyApi(c *gin.Context) {
	var data gin.H
	privateKey := c.PostForm("privateKey")
	if strings.HasPrefix(privateKey, "nsec") {
		seed, err := utils.NsecToSeed(privateKey)
		if err != nil {
			data = gin.H{
				"valid": false,
				"error": err.Error(),
			}
			c.JSON(http.StatusOK, data)
			return
		}
		privateKey = seed
	}
	err := utils.IsValidPrivateKey(privateKey)
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

func (s *service) validateLnUrlApi(c *gin.Context) {
	url := c.PostForm("lnurl")
	valid := utils.IsValidLnUrl(url)
	data := gin.H{
		"valid": valid,
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
