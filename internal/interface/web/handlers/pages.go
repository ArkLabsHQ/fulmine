package handlers

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/a-h/templ"
	"github.com/angelofallars/htmx-go"

	"github.com/ArkLabsHQ/ark-node/internal/interface/web/templates"
	"github.com/ArkLabsHQ/ark-node/internal/interface/web/templates/components"
	"github.com/ArkLabsHQ/ark-node/internal/interface/web/templates/pages"

	"github.com/gin-gonic/gin"
	log "github.com/sirupsen/logrus"
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

func Done(c *gin.Context) {
	bodyContent := pages.DoneBodyContent()
	pageViewHandler(bodyContent, c)
}

func Index(c *gin.Context) {
	bodyContent := pages.Welcome()
	if arkClient := getArkClient(c); arkClient != nil {
		if arkClient.IsLocked(c) {
			bodyContent = pages.Locked()
		} else {
			bodyContent = pages.HistoryBodyContent(getSpendableBalance(c), getAddress(), getTransactions())
		}
	}
	pageViewHandler(bodyContent, c)
}

func Initialize(c *gin.Context) {
	aspurl := c.PostForm("aspurl")
	if aspurl == "" {
		toast := components.Toast("ASP URL can't be empty")
		toastHandler(toast, c)
		return
	}

	mnemonic := c.PostForm("mnemonic")
	if mnemonic == "" {
		toast := components.Toast("Mnemonic can't be empty")
		toastHandler(toast, c)
		return
	}

	password := c.PostForm("password")
	if password == "" {
		toast := components.Toast("Password can't be empty")
		toastHandler(toast, c)
		return
	}

	log.Info(aspurl, mnemonic, password)

	if _, err := setupFileBasedArkClient(aspurl, mnemonic, password); err == nil {
		redirect("/done", c)
	} else {
		redirect("/", c)
	}
}

func ImportWallet(c *gin.Context) {
	var empty []string
	empty = append(empty, "")
	bodyContent := pages.ManageMnemonicContent(empty)
	pageViewHandler(bodyContent, c)
}

func Locked(c *gin.Context) {
	bodyContent := pages.Locked()
	pageViewHandler(bodyContent, c)
}

func NewWallet(c *gin.Context) {
	bodyContent := pages.ManageMnemonicContent(getNewMnemonic())
	pageViewHandler(bodyContent, c)
}

func Receive(c *gin.Context) {
	bodyContent := pages.ReceiveBodyContent(getSpendableBalance(c))
	pageViewHandler(bodyContent, c)
}

func ReceivePreview(c *gin.Context) {
	// get addresses
	arkClient := getArkClient(c)
	offchainAddr, onchainAddr, err := arkClient.Receive(c)
	if err != nil {
		log.Fatal(err)
	}
	// generate bip21
	bip21 := fmt.Sprintf("bitcoin:%s?ark:%s", onchainAddr, offchainAddr)
	// add amount if passed
	sats := c.PostForm("sats")
	if sats != "" {
		amount := fmt.Sprintf("&amount=%s", sats)
		bip21 = bip21 + amount
	}
	// show invoice in plain and qrcode
	info := pages.ReceivePreview(bip21)
	partialViewHandler(info, c)
}

func Send(c *gin.Context) {
	arkClient := getArkClient(c)
	if arkClient == nil || arkClient.IsLocked(c) {
		c.Redirect(http.StatusFound, "/")
		return
	}
	bodyContent := pages.SendBodyContent(getSpendableBalance(c))
	pageViewHandler(bodyContent, c)
}

func SendConfirm(c *gin.Context) {
	address := c.PostForm("address")
	amount := c.PostForm("amount")
	bodyContent := pages.SendSuccessContent(address, amount)
	partialViewHandler(bodyContent, c)
}

func SendPreview(c *gin.Context) {
	address := c.PostForm("address")
	sats := c.PostForm("sats")
	bodyContent := pages.SendPreviewContent(address, sats)
	partialViewHandler(bodyContent, c)
}

func SetMnemonic(c *gin.Context) {
	var words []string
	for i := 1; i <= 12; i++ {
		id := "word_" + strconv.Itoa(i)
		word := c.PostForm(id)
		if len(word) == 0 {
			toast := components.Toast("Invalid mnemonic", true)
			toastHandler(toast, c)
			return
		}
		words = append(words, word)
	}
	mnemonic := strings.Join(words, " ")
	bodyContent := pages.SetPasswordContent(mnemonic)
	partialViewHandler(bodyContent, c)
}

func SetPassword(c *gin.Context) {
	password := c.PostForm("password")
	pconfirm := c.PostForm("pconfirm")
	if password != pconfirm {
		toast := components.Toast("Passwords doesn't match", true)
		toastHandler(toast, c)
		return
	}
	mnemonic := c.PostForm("mnemonic")
	bodyContent := pages.AspUrlBodyContent(c.Query("aspurl"), mnemonic, password)
	partialViewHandler(bodyContent, c)
}

func Settings(c *gin.Context) {
	active := c.Param("active")
	settings := getSettings()
	nodeStatus := true
	bodyContent := pages.SettingsBodyContent(active, settings, nodeStatus)
	pageViewHandler(bodyContent, c)
}

func Swap(c *gin.Context) {
	arkClient := getArkClient(c)
	if arkClient == nil || arkClient.IsLocked(c) {
		c.Redirect(http.StatusFound, "/")
		return
	}
	bodyContent := pages.SwapBodyContent(getSpendableBalance(c), getNodeBalance())
	pageViewHandler(bodyContent, c)
}

func SwapActive(c *gin.Context) {
	active := c.Param("active")
	var balance string
	if active == "inbound" {
		balance = getNodeBalance()
	} else {
		balance = ""
	}
	bodyContent := pages.SwapPartialContent(active, balance)
	partialViewHandler(bodyContent, c)
}

func SwapConfirm(c *gin.Context) {
	kind := c.PostForm("kind")
	sats := c.PostForm("sats")
	bodyContent := pages.SwapSuccessContent(kind, sats)
	partialViewHandler(bodyContent, c)
}

func SwapPreview(c *gin.Context) {
	kind := c.PostForm("kind")
	sats := c.PostForm("sats")
	bodyContent := pages.SwapPreviewContent(kind, sats)
	partialViewHandler(bodyContent, c)
}

func Tx(c *gin.Context) {
	txid := c.Param("txid")
	var tx []string
	for _, transaction := range getTransactions() {
		if transaction[0] == txid {
			tx = transaction
			break
		}
	}
	bodyContent := pages.TxBodyContent(tx[0], tx[1], tx[2], tx[3], tx[4], tx[5])
	pageViewHandler(bodyContent, c)
}

func Welcome(c *gin.Context) {
	bodyContent := pages.Welcome()
	pageViewHandler(bodyContent, c)
}
