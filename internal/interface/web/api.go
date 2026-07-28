package web

import (
	"net/http"

	"github.com/ArkLabsHQ/fulmine/internal/interface/web/templates/components"
	"github.com/ArkLabsHQ/fulmine/utils"
	"github.com/gin-gonic/gin"
)

func (s *service) getBalanceApi(c *gin.Context) {
	balance, err := s.svc.Balance(c)
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

func (s *service) forgotApi(c *gin.Context) {
	if err := s.svc.ResetWallet(c); err != nil {
		toast := components.Toast("Unable to delete previous wallet", true)
		toastHandler(toast, c)
		return
	}
	redirect("/welcome", c)
}

func (s *service) validateBip21Api(c *gin.Context) {
	var data gin.H
	bip21 := c.PostForm("bip21")
	sats := utils.SatsFromBip21(bip21)
	if sats > 0 {
		data = gin.H{
			"sats":  sats,
			"valid": true,
		}
	} else {
		data = gin.H{
			"valid": false,
			"error": "invalid invoice",
		}
	}
	c.JSON(http.StatusOK, data)
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
