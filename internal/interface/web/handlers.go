package web

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/ArkLabsHQ/ark-node/internal/interface/web/templates"
	"github.com/ArkLabsHQ/ark-node/internal/interface/web/templates/components"
	"github.com/ArkLabsHQ/ark-node/internal/interface/web/templates/modals"
	"github.com/ArkLabsHQ/ark-node/internal/interface/web/templates/pages"
	"github.com/ArkLabsHQ/ark-node/internal/interface/web/types"
	"github.com/a-h/templ"
	"github.com/angelofallars/htmx-go"
	arksdk "github.com/ark-network/ark/pkg/client-sdk"
	"github.com/gin-gonic/gin"
	log "github.com/sirupsen/logrus"
)

func (s *service) done(c *gin.Context) {
	bodyContent := pages.DoneBodyContent()
	s.pageViewHandler(bodyContent, c)
}

func (s *service) forgot(c *gin.Context) {
	if err := s.svc.Reset(c); err != nil {
		toast := components.Toast("Unable to delete previous wallet", true)
		toastHandler(toast, c)
		return
	}
	c.Redirect(http.StatusFound, "/welcome")
}

func (s *service) index(c *gin.Context) {
	bodyContent := pages.Welcome()
	if s.svc.IsReady() {
		if s.svc.IsLocked(c) {
			bodyContent = pages.Unlock()
		} else {
			var offchainAddr string
			var isOnline bool
			if addr, _, err := s.svc.Receive(c); err == nil {
				offchainAddr = addr
				isOnline = true
			} else {
				log.WithError(err).Warn("failed to get receiving address")
			}
			spendableBalance, err := s.getSpendableBalance(c)
			if err != nil {
				log.WithError(err).Warn("failed to get spendable balance")
			}
			txHistory, err := s.getTxHistory(c)
			if err != nil {
				log.WithError(err).Warn("failed to get tx history")
			}
			s.logVtxos(c) // TODO: remove
			bodyContent = pages.HistoryBodyContent(
				spendableBalance, offchainAddr, txHistory, isOnline,
			)
		}
	}

	s.pageViewHandler(bodyContent, c)
}

func (s *service) initialize(c *gin.Context) {
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

	if err := s.svc.Setup(c, aspurl, password, mnemonic); err != nil {
		log.WithError(err).Warn("failed to initialize")
		redirect("/", c)
		return
	}
	redirect("/done", c)
}

func (s *service) importWallet(c *gin.Context) {
	var empty []string
	empty = append(empty, "")
	bodyContent := pages.ManageMnemonicContent(empty)
	s.pageViewHandler(bodyContent, c)
}

func (s *service) lock(c *gin.Context) {
	bodyContent := pages.Lock()
	s.pageViewHandler(bodyContent, c)
}

func (s *service) unlock(c *gin.Context) {
	log.Infof("referer %s", c.Request.Referer())
	bodyContent := pages.Unlock()
	s.pageViewHandler(bodyContent, c)
}

func (s *service) newWallet(c *gin.Context) {
	bodyContent := pages.ManageMnemonicContent(getNewMnemonic())
	s.pageViewHandler(bodyContent, c)
}

func (s *service) receiveEdit(c *gin.Context) {
	if s.redirectedBecauseWalletIsLocked(c) {
		return
	}
	bodyContent := pages.ReceiveEditContent()
	s.pageViewHandler(bodyContent, c)
}

func (s *service) receiveQrCode(c *gin.Context) {
	if s.redirectedBecauseWalletIsLocked(c) {
		return
	}

	offchainAddr, onchainAddr, err := s.svc.Receive(c)
	if err != nil {
		// nolint:all
		c.AbortWithError(http.StatusInternalServerError, err)
		return
	}

	sats := c.PostForm("sats")
	bip21 := genBip21(offchainAddr, onchainAddr, sats)
	bodyContent := pages.ReceiveQrCodeContent(bip21, offchainAddr, onchainAddr, sats)
	s.pageViewHandler(bodyContent, c)
}

func (s *service) receiveSuccess(c *gin.Context) {
	offchainAddr := c.PostForm("offchainAddr")
	onchainAddr := c.PostForm("onchainAddr")
	sats := c.PostForm("sats")
	partial := pages.ReceiveSuccessContent(offchainAddr, onchainAddr, sats)
	partialViewHandler(partial, c)
}

func (s *service) send(c *gin.Context) {
	if s.redirectedBecauseWalletIsLocked(c) {
		return
	}
	spendableBalance, err := s.getSpendableBalance(c)
	if err != nil {
		// nolint:all
		c.AbortWithError(http.StatusInternalServerError, err)
		return
	}

	bodyContent := pages.SendBodyContent(spendableBalance)
	s.pageViewHandler(bodyContent, c)
}

func (s *service) sendPreview(c *gin.Context) {
	if s.redirectedBecauseWalletIsLocked(c) {
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

func (s *service) sendConfirm(c *gin.Context) {
	if s.redirectedBecauseWalletIsLocked(c) {
		return
	}

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
		txId, err = s.svc.SendAsync(c, true, receivers)
		if err != nil {
			toast := components.Toast(err.Error(), true)
			toastHandler(toast, c)
			return
		}
	}

	if isValidBtcAddress(address) {
		txId, err = s.svc.SendOnChain(c, receivers)
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

	data, err := s.svc.GetConfigData(c)
	if err != nil {
		toast := components.Toast(err.Error(), true)
		toastHandler(toast, c)
		return
	}
	explorerUrl := getExplorerUrl(data.Network.Name)

	bodyContent := pages.SendSuccessContent(address, sats, txId, explorerUrl)
	partialViewHandler(bodyContent, c)
}

func (s *service) setMnemonic(c *gin.Context) {
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

func (s *service) setPassword(c *gin.Context) {
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

func (s *service) settings(c *gin.Context) {
	settings, err := s.svc.GetSettings(c)
	if err != nil {
		// nolint:all
		c.AbortWithError(http.StatusInternalServerError, err)
		return
	}
	isLocked := s.svc.IsLocked(c)

	active := c.Param("active")
	bodyContent := pages.SettingsBodyContent(
		active, *settings, s.getNodeStatus(), isLocked,
	)
	s.pageViewHandler(bodyContent, c)
}

func (s *service) swap(c *gin.Context) {
	if s.redirectedBecauseWalletIsLocked(c) {
		return
	}
	spendableBalance, err := s.getSpendableBalance(c)
	if err != nil {
		// nolint:all
		c.AbortWithError(http.StatusInternalServerError, err)
		return
	}

	bodyContent := pages.SwapBodyContent(spendableBalance, s.getNodeBalance())
	s.pageViewHandler(bodyContent, c)
}

func (s *service) swapActive(c *gin.Context) {
	active := c.Param("active")
	var balance string
	if active == "inbound" {
		balance = s.getNodeBalance()
	} else {
		spendableBalance, err := s.getSpendableBalance(c)
		if err != nil {
			// nolint:all
			c.AbortWithError(http.StatusInternalServerError, err)
			return
		}
		balance = spendableBalance
	}
	bodyContent := pages.SwapPartialContent(active, balance)
	partialViewHandler(bodyContent, c)
}

func (s *service) swapConfirm(c *gin.Context) {
	if s.redirectedBecauseWalletIsLocked(c) {
		return
	}
	data, err := s.svc.GetConfigData(c)
	if err != nil {
		// nolint:all
		c.AbortWithError(http.StatusInternalServerError, err)
		return
	}
	kind := c.PostForm("kind")
	sats := c.PostForm("sats")
	explorerUrl := getExplorerUrl(data.Network.Name)

	bodyContent := pages.SwapSuccessContent(kind, sats, "TODO", explorerUrl)
	partialViewHandler(bodyContent, c)
}

func (s *service) swapPreview(c *gin.Context) {
	if s.redirectedBecauseWalletIsLocked(c) {
		return
	}

	kind := c.PostForm("kind")
	sats := c.PostForm("sats")
	bodyContent := pages.SwapPreviewContent(kind, sats)
	partialViewHandler(bodyContent, c)
}

func (s *service) getTx(c *gin.Context) {
	txHistory, err := s.getTxHistory(c)
	if err != nil {
		// nolint:all
		c.AbortWithError(http.StatusInternalServerError, err)
		return
	}

	txid := c.Param("txid")
	var tx types.Transaction
	for _, transaction := range txHistory {
		if transaction.Txid == txid {
			tx = transaction
			break
		}
	}
	bodyContent := pages.TxBodyContent(tx)
	s.pageViewHandler(bodyContent, c)
}

func (s *service) welcome(c *gin.Context) {
	bodyContent := pages.Welcome()
	s.pageViewHandler(bodyContent, c)
}

func (s *service) feeInfoModal(c *gin.Context) {
	info := modals.FeeInfo()
	modalHandler(info, c)
}

func (s *service) getSpendableBalance(c *gin.Context) (string, error) {
	balance, err := s.svc.Balance(c, true)
	if err != nil {
		return "", err
	}
	onchainBalance := balance.OnchainBalance.SpendableAmount
	for _, amount := range balance.OnchainBalance.LockedAmount {
		onchainBalance += amount.Amount
	}
	return strconv.FormatUint(
		balance.OffchainBalance.Total+onchainBalance, 10,
	), nil
}

func (s *service) getNodeBalance() string {
	return "50640" // TODO
}

func (s *service) logVtxos(c *gin.Context) {
	spendableVtxos, spentVtxos, err := s.svc.ListVtxos(c)
	if err != nil {
		return
	}

	log.Info("spendableVtxos")
	for _, v := range spendableVtxos {
		log.Info("---------")
		log.Infof("Amount %d", v.Amount)
		log.Infof("ExpiresAt %v", v.ExpiresAt)
		log.Infof("Pending %v", v.Pending)
		log.Infof("RoundTxid %v", v.RoundTxid)
		log.Infof("Txid %v", v.Txid)
		log.Infof("SpentBy %v", v.SpentBy)
		log.Info("---------")
	}

	log.Info("spentVtxos")
	for _, v := range spentVtxos {
		log.Info("---------")
		log.Infof("Amount %d", v.Amount)
		log.Infof("ExpiresAt %v", v.ExpiresAt)
		log.Infof("Pending %v", v.Pending)
		log.Infof("RoundTxid %v", v.RoundTxid)
		log.Infof("Txid %v", v.Txid)
		log.Infof("SpentBy %v", v.SpentBy)
		log.Info("---------")
	}
}

func (s *service) getTxHistory(
	c *gin.Context,
) (transactions []types.Transaction, err error) {
	// get tx history from ASP
	history, err := s.svc.GetTransactionHistory(c)
	if err != nil {
		return nil, err
	}
	// transform each arksdk.Transaction to types.Transaction
	for _, tx := range history {
		// amount
		amount := strconv.FormatUint(tx.Amount, 10)
		if tx.Type == arksdk.TxSent {
			amount = "-" + amount
		}
		// date of creation
		dateCreated := tx.CreatedAt.Unix()
		// status of tx
		status := "success"
		if tx.Pending {
			status = "pending"
		}
		emptyTime := time.Time{}
		if tx.CreatedAt == emptyTime {
			status = "unconfirmed"
			dateCreated = 0
		}
		// get one txid
		txid := tx.RoundTxid
		if len(txid) == 0 {
			txid = tx.RedeemTxid
		}
		if len(txid) == 0 {
			txid = tx.BoardingTxid
		}
		// add to slice of transactions
		transactions = append(transactions, types.Transaction{
			Amount:   amount,
			Date:     prettyUnixTimestamp(dateCreated),
			Day:      prettyDay(dateCreated),
			Hour:     prettyHour(dateCreated),
			Kind:     string(tx.Type),
			Txid:     txid,
			Status:   status,
			UnixDate: dateCreated,
		})
	}
	return
}

func (s *service) redirectedBecauseWalletIsLocked(c *gin.Context) bool {
	redirect := s.svc.IsLocked(c)
	if redirect {
		c.Redirect(http.StatusFound, "/")
	}
	return redirect
}

func (s *service) pageViewHandler(bodyContent templ.Component, c *gin.Context) {
	settings, err := s.svc.GetSettings(c)
	if err != nil {
		// nolint:all
		c.AbortWithError(http.StatusInternalServerError, err)
		return
	}
	indexTemplate := templates.Layout(bodyContent, *settings)
	if err := htmx.NewResponse().RenderTempl(c.Request.Context(), c.Writer, indexTemplate); err != nil {
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}
}
