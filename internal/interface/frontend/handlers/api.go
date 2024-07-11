package handlers

import (
	"errors"
	"net/http"

	"github.com/ArkLabsHQ/ark-wallet/templates/components"
	"github.com/a-h/templ"
	"github.com/angelofallars/htmx-go"

	"github.com/gin-gonic/gin"
)

func toastHandler(t templ.Component, c *gin.Context) {
	if !htmx.IsHTMX(c.Request) {
		c.AbortWithError(http.StatusBadRequest, errors.New("non-htmx request"))
		return
	}
	htmx.NewResponse().
		Retarget("#toast").
		AddTrigger(htmx.Trigger("toast")).
		RenderTempl(c, c.Writer, t)
}

func SettingsApiPost(c *gin.Context) {
	// TODO: manage new settings posted
	info := components.Toast("Saved")
	toastHandler(info, c)
}
