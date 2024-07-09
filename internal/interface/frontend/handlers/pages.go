package handlers

import (
	"fmt"
	"net/http"

	"github.com/a-h/templ"
	"github.com/angelofallars/htmx-go"

	"github.com/ArkLabsHQ/ark-wallet/templates"
	"github.com/ArkLabsHQ/ark-wallet/templates/pages"

	"github.com/gin-gonic/gin"
)

func pageViewHandler(bodyContent templ.Component, c *gin.Context) {
	indexTemplate := templates.Layout(bodyContent)
	if err := htmx.NewResponse().RenderTempl(c.Request.Context(), c.Writer, indexTemplate); err != nil {
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}
}

func partialViewHandler(bodyContent templ.Component, c *gin.Context) {
	if err := htmx.NewResponse().RenderTempl(c.Request.Context(), c.Writer, bodyContent); err != nil {
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}
}

func Index(c *gin.Context) {
	bodyContent := pages.HistoryBodyContent(getBalance(), getAddress(), getTransactions())
	pageViewHandler(bodyContent, c)
}

func Receive(c *gin.Context) {
	bodyContent := pages.ReceiveBodyContent(getBalance())
	pageViewHandler(bodyContent, c)
}

func ReceivePreview(c *gin.Context) {
	addr := getAddress()
	sats := c.PostForm("sats")
	bip21 := fmt.Sprintf("ark:%s?amount:%s", addr, sats)
	info := pages.ReceivePreview(bip21)
	partialViewHandler(info, c)
}

func Send(c *gin.Context) {
	bodyContent := pages.SendBodyContent(getBalance())
	pageViewHandler(bodyContent, c)
}

func SendPreview(c *gin.Context) {
	address := c.PostForm("address")
	amount := c.PostForm("amount")
	bodyContent := pages.SendPreviewContent(address, amount)
	partialViewHandler(bodyContent, c)
}

func SendConfirm(c *gin.Context) {
	address := c.PostForm("address")
	amount := c.PostForm("amount")
	bodyContent := pages.SendSuccessContent(address, amount)
	partialViewHandler(bodyContent, c)
}

func Swap(c *gin.Context) {
	bodyContent := pages.SwapBodyContent(getBalance())
	pageViewHandler(bodyContent, c)
}

func Tx(c *gin.Context) {
	txid := c.Param("txid")
	var transaction []string
	for _, tx := range getTransactions() {
		if tx[0] == txid {
			transaction = tx
			break
		}
	}
	bodyContent := pages.TxBodyContent(transaction)
	pageViewHandler(bodyContent, c)
}

func Welcome(c *gin.Context) {
	bodyContent := pages.Welcome()
	pageViewHandler(bodyContent, c)
}
