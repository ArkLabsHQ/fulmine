package handlers

import (
	"net/http"

	"github.com/a-h/templ"
	"github.com/angelofallars/htmx-go"

	"github.com/ArkLabsHQ/ark-wallet/templates"
	"github.com/ArkLabsHQ/ark-wallet/templates/pages"

	"github.com/gin-gonic/gin"
)

func viewHandler(bodyContent templ.Component, c *gin.Context) {
	indexTemplate := templates.Layout(bodyContent)
	if err := htmx.NewResponse().RenderTempl(c.Request.Context(), c.Writer, indexTemplate); err != nil {
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}
}

func Index(c *gin.Context) {
	bodyContent := pages.HistoryBodyContent()
	viewHandler(bodyContent, c)
}

func Receive(c *gin.Context) {
	bodyContent := pages.ReceiveBodyContent()
	viewHandler(bodyContent, c)
}

func Send(c *gin.Context) {
	bodyContent := pages.SendBodyContent()
	viewHandler(bodyContent, c)
}

func Swap(c *gin.Context) {
	bodyContent := pages.SwapBodyContent()
	viewHandler(bodyContent, c)
}
