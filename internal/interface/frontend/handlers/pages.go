package handlers

import (
	"net/http"

	"github.com/a-h/templ"
	"github.com/angelofallars/htmx-go"

	"github.com/ArkLabsHQ/ark-wallet/templates"
	"github.com/ArkLabsHQ/ark-wallet/templates/pages"

	"github.com/gin-gonic/gin"
)

func getBalance() string {
	return "1930547"
}

func viewHandler(bodyContent templ.Component, c *gin.Context) {
	indexTemplate := templates.Layout(bodyContent)
	if err := htmx.NewResponse().RenderTempl(c.Request.Context(), c.Writer, indexTemplate); err != nil {
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}
}

func Index(c *gin.Context) {
	arkAddress := "ark18746676365652bcdabdbacdabcd63546354634"
	bodyContent := pages.HistoryBodyContent(arkAddress, getBalance())
	viewHandler(bodyContent, c)
}

func Receive(c *gin.Context) {
	bodyContent := pages.ReceiveBodyContent(getBalance())
	viewHandler(bodyContent, c)
}

func Send(c *gin.Context) {
	bodyContent := pages.SendBodyContent(getBalance())
	viewHandler(bodyContent, c)
}

func Swap(c *gin.Context) {
	bodyContent := pages.SwapBodyContent(getBalance())
	viewHandler(bodyContent, c)
}
