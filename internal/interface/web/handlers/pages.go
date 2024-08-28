package handlers

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/a-h/templ"
	"github.com/angelofallars/htmx-go"
	arksdk "github.com/ark-network/ark/pkg/client-sdk"

	"github.com/ArkLabsHQ/ark-node/internal/interface/web/templates"
	"github.com/ArkLabsHQ/ark-node/internal/interface/web/templates/components"
	"github.com/ArkLabsHQ/ark-node/internal/interface/web/templates/pages"

	"github.com/gin-gonic/gin"
	log "github.com/sirupsen/logrus"
)

func redirectedBecauseWalletIsLocked(c *gin.Context) bool {
	arkClient := getArkClient(c)
	redirect := arkClient == nil || arkClient.IsLocked(c)
	if redirect {
		c.Redirect(http.StatusFound, "/")
	}
	return redirect
}

func onboardSome(c *gin.Context, arkClient arksdk.ArkClient) {
	if arkClient.IsLocked(c) {
		log.Info("can't onboard, wallet still locked")
	} else {
		balance, err := arkClient.Balance(c, true)
		if err != nil {
			log.WithError(err).Info("error getting balance")
			return
		}
		if balance.OnchainBalance.SpendableAmount >= 100_000 { // TODO
			txid, err := arkClient.Onboard(c, 50_000)
			if err != nil {
				log.WithError(err).Info("error onboarding")
			}
			log.Infof("TxId %s", txid)
		}
	}
}

func pageViewHandler(bodyContent templ.Component, c *gin.Context) {
	indexTemplate := templates.Layout(bodyContent, getSettings())
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

func Forgot(c *gin.Context) {
	deleteOldState()
	c.Redirect(http.StatusFound, "/welcome")
}

func Index(c *gin.Context) {
	bodyContent := pages.Welcome()
	if arkClient := getArkClient(c); arkClient != nil {
		if arkClient.IsLocked(c) {
			bodyContent = pages.Unlock()
		} else {
			onboardSome(c, arkClient) // TODO
			bodyContent = pages.HistoryBodyContent(
				getSpendableBalance(c),
				getAddress(c),
				getTransactions(),
				isOnline(c),
			)
		}
	}
	pageViewHandler(bodyContent, c)
}

func Initialize(c *gin.Context) {
	aspurl := c.PostForm("aspurl")
	if aspurl == "" {
		toast := components.Toast("ASP URL can't be empty", true)
		toastHandler(toast, c)
		return
	}

	mnemonic := c.PostForm("mnemonic")
	if mnemonic == "" {
		toast := components.Toast("Mnemonic can't be empty", true)
		toastHandler(toast, c)
		return
	}

	password := c.PostForm("password")
	if password == "" {
		toast := components.Toast("Password can't be empty", true)
		toastHandler(toast, c)
		return
	}

	log.Info(aspurl, mnemonic, password)

	if _, err := setupFileBasedArkClient(aspurl, mnemonic, password); err == nil {
		c.Set("arkClient", nil)
		redirect("/done", c)
	} else {
		log.WithError(err).Info("error initializing")
		redirect("/", c)
	}
}

func ImportWallet(c *gin.Context) {
	var empty []string
	empty = append(empty, "")
	bodyContent := pages.ManageMnemonicContent(empty)
	pageViewHandler(bodyContent, c)
}

func LockWallet(c *gin.Context) {
	bodyContent := pages.Lock()
	pageViewHandler(bodyContent, c)
}

func UnlockWallet(c *gin.Context) {
	bodyContent := pages.Unlock()
	pageViewHandler(bodyContent, c)
}

func NewWallet(c *gin.Context) {
	bodyContent := pages.ManageMnemonicContent(getNewMnemonic())
	pageViewHandler(bodyContent, c)
}

func ReceiveEdit(c *gin.Context) {
	if redirectedBecauseWalletIsLocked(c) {
		return
	}
	bodyContent := pages.ReceiveEditContent()
	pageViewHandler(bodyContent, c)
}

func ReceiveQrCode(c *gin.Context) {
	if redirectedBecauseWalletIsLocked(c) {
		return
	}
	arkClient := getArkClient(c)
	offchainAddr, onchainAddr, err := arkClient.Receive(c)
	if err != nil {
		log.Fatal(err)
	}
	sats := c.PostForm("sats")
	bip21 := genBip21(offchainAddr, onchainAddr, sats)
	bodyContent := pages.ReceiveQrCodeContent(bip21, offchainAddr, onchainAddr, sats)
	pageViewHandler(bodyContent, c)
}

func ReceiveSuccess(c *gin.Context) {
	offchainAddr := c.PostForm("offchainAddr")
	onchainAddr := c.PostForm("onchainAddr")
	sats := c.PostForm("sats")
	partial := pages.ReceiveSuccessContent(offchainAddr, onchainAddr, sats)
	partialViewHandler(partial, c)
}

func Send(c *gin.Context) {
	if redirectedBecauseWalletIsLocked(c) {
		return
	}
	bodyContent := pages.SendBodyContent(getSpendableBalance(c))
	pageViewHandler(bodyContent, c)
}

func SendPreview(c *gin.Context) {
	if redirectedBecauseWalletIsLocked(c) {
		return
	}
	addr := ""
	dest := c.PostForm("address")
	sats := c.PostForm("sats")

	if isBip21(dest) {
		offchainAddress := getArkAddress(dest)
		if len(offchainAddress) > 0 {
			addr = offchainAddress
		} else {
			onchainAddress := getBtcAddress(dest)
			if len(onchainAddress) > 0 {
				addr = onchainAddress
			}
		}
	} else {
		if isValidBtcAddress(dest) || isValidArkAddress(dest) {
			addr = dest
		}
	}

	if len(addr) == 0 {
		toast := components.Toast("Invalid address", true)
		toastHandler(toast, c)
	} else {
		bodyContent := pages.SendPreviewContent(addr, sats)
		partialViewHandler(bodyContent, c)
	}
}

func SendConfirm(c *gin.Context) {
	if redirectedBecauseWalletIsLocked(c) {
		return
	}

	arkClient := getArkClient(c)

	address := c.PostForm("address")
	sats := c.PostForm("sats")
	txId := ""

	value, err := strconv.ParseUint(sats, 10, 64)
	if err != nil {
		toast := components.Toast("Invalid amount", true)
		toastHandler(toast, c)
		return
	}

	receivers := []arksdk.Receiver{
		arksdk.NewBitcoinReceiver(address, value),
	}

	if isValidArkAddress(address) {
		txId, err = arkClient.SendOffChain(c, true, receivers)
		if err != nil {
			toast := components.Toast(err.Error(), true)
			toastHandler(toast, c)
			return
		}
	}

	if isValidBtcAddress(address) {
		txId, err = arkClient.SendOnChain(c, receivers)
		if err != nil {
			toast := components.Toast(err.Error(), true)
			toastHandler(toast, c)
			return
		}
	}

	if len(txId) == 0 {
		toast := components.Toast("Something went wrong", true)
		toastHandler(toast, c)
		return
	}

	bodyContent := pages.SendSuccessContent(address, sats, txId)
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
	arkClient := getArkClient(c)
	locked := arkClient != nil && arkClient.IsLocked(c)
	active := c.Param("active")
	bodyContent := pages.SettingsBodyContent(active, getSettings(), getNodeStatus(), locked)
	pageViewHandler(bodyContent, c)
}

func Swap(c *gin.Context) {
	if redirectedBecauseWalletIsLocked(c) {
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
		balance = getSpendableBalance(c)
	}
	bodyContent := pages.SwapPartialContent(active, balance)
	partialViewHandler(bodyContent, c)
}

func SwapConfirm(c *gin.Context) {
	if redirectedBecauseWalletIsLocked(c) {
		return
	}
	kind := c.PostForm("kind")
	sats := c.PostForm("sats")
	bodyContent := pages.SwapSuccessContent(kind, sats)
	partialViewHandler(bodyContent, c)
}

func SwapPreview(c *gin.Context) {
	if redirectedBecauseWalletIsLocked(c) {
		return
	}
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
	c.Set("arkClient", nil)
	bodyContent := pages.Welcome()
	pageViewHandler(bodyContent, c)
}
