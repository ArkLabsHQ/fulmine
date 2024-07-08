package main

import (
	"errors"
	"net/http"

	"github.com/ArkLabsHQ/ark-wallet/templates/modals"
	"github.com/a-h/templ"
	"github.com/angelofallars/htmx-go"

	"github.com/gin-gonic/gin"
)

func apiHandler(c *gin.Context, t templ.Component) {
	if !htmx.IsHTMX(c.Request) {
		c.AbortWithError(http.StatusBadRequest, errors.New("non-htmx request"))
		return
	}
	htmx.NewResponse().RenderTempl(c, c.Writer, t)
}

func historyAPIHandler(c *gin.Context) {
	hash := c.Param("hash")
	info := modals.TransactionInfo(hash)
	apiHandler(c, info)
}

func receiveAPIHandler(c *gin.Context) {
	info := modals.Receive()
	apiHandler(c, info)
}

func sendAPIHandler(c *gin.Context) {
	info := modals.Send()
	apiHandler(c, info)
}

func swapAPIHandler(c *gin.Context) {
	info := modals.Swap()
	apiHandler(c, info)
}
