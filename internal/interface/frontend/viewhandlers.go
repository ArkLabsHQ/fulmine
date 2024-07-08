package main

import (
	"net/http"

	"github.com/a-h/templ"
	"github.com/angelofallars/htmx-go"

	"github.com/ArkLabsHQ/ark-wallet/templates"
	"github.com/ArkLabsHQ/ark-wallet/templates/components"
	"github.com/ArkLabsHQ/ark-wallet/templates/pages"

	"github.com/gin-gonic/gin"
)

func viewHandler(heroContent, bodyContent templ.Component, c *gin.Context) {
	indexTemplate := templates.Layout(heroContent, bodyContent)
	if err := htmx.NewResponse().RenderTempl(c.Request.Context(), c.Writer, indexTemplate); err != nil {
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}
}

func IndexViewHandler(c *gin.Context) {
	heroContent := components.Hero("1930124")
	bodyContent := pages.HistoryBodyContent()
	viewHandler(heroContent, bodyContent, c)
}

func ReceiveViewHandler(c *gin.Context) {
	heroContent := components.Header("Receive")
	bodyContent := pages.ReceiveBodyContent()
	viewHandler(heroContent, bodyContent, c)
}

func SendViewHandler(c *gin.Context) {
	heroContent := components.Header("Send")
	bodyContent := pages.SendBodyContent()
	viewHandler(heroContent, bodyContent, c)
}

func SwapViewHandler(c *gin.Context) {
	heroContent := components.Header("Swap")
	bodyContent := pages.SwapBodyContent()
	viewHandler(heroContent, bodyContent, c)
}
