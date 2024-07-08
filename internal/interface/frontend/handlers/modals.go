package handlers

import (
	"errors"
	"fmt"
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

func HistoryAPIHandler(c *gin.Context) {
	hash := c.Param("hash")
	info := modals.TransactionInfo(hash)
	apiHandler(c, info)
}

func ReceiveAPIHandler(c *gin.Context) {
	addr := getAddress()
	sats := c.PostForm("sats")
	bip21 := fmt.Sprintf("ark:%s?amount:%s", addr, sats)
	info := modals.Receive(bip21)
	apiHandler(c, info)
}

func SendAPIHandler(c *gin.Context) {
	info := modals.Send()
	apiHandler(c, info)
}

func SwapAPIHandler(c *gin.Context) {
	info := modals.Swap()
	apiHandler(c, info)
}
