package main

import (
	"net/http"

	"github.com/a-h/templ"
	"github.com/angelofallars/htmx-go"

	"github.com/ArkLabsHQ/ark-wallet/templates"
	"github.com/ArkLabsHQ/ark-wallet/templates/pages"

	"github.com/gin-gonic/gin"
)

func viewHandler(bodyContent templ.Component, activeTab string, c *gin.Context) {
	currentBalance := "197543"
	indexTemplate := templates.Layout(bodyContent, activeTab, currentBalance)
	if err := htmx.NewResponse().RenderTempl(c.Request.Context(), c.Writer, indexTemplate); err != nil {
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}
}

func aspViewHandler(c *gin.Context) {
	bodyContent := pages.AspBodyContent()
	viewHandler(bodyContent, "asp", c)
}

func historyViewHandler(c *gin.Context) {
	bodyContent := pages.HistoryBodyContent()
	viewHandler(bodyContent, "history", c)
}

func lightningViewHandler(c *gin.Context) {
	bodyContent := pages.LightningBodyContent()
	viewHandler(bodyContent, "lightning", c)
}

func settingsViewHandler(c *gin.Context) {
	bodyContent := pages.SettingsBodyContent()
	viewHandler(bodyContent, "settings", c)
}
