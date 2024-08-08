package handlers

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/ArkLabsHQ/ark-node/internal/interface/web/templates/components"
	"github.com/a-h/templ"
	"github.com/angelofallars/htmx-go"

	"github.com/gin-gonic/gin"
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

func ValidateMnemonicApiPost(c *gin.Context) {
	// TODO: validate mnemonic with bip39
	var toast templ.Component
	for i := 1; i <= 12; i++ {
		id := "word_" + strconv.Itoa(i)
		word := c.PostForm(id)
		if len(word) == 0 {
			toast = components.Toast("Invalid mnemonic", true)
		}
	}
	if toast == nil {
		toast = components.Toast("Valid mnemonic")
	}
	toastHandler(toast, c)
}
