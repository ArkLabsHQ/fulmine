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

func SettingsApiPost(c *gin.Context) {
	// TODO: manage new settings posted
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
