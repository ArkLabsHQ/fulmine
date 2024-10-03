package web

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/ArkLabsHQ/ark-node/internal/interface/web/templates"
	"github.com/ArkLabsHQ/ark-node/internal/interface/web/templates/components"
	"github.com/ArkLabsHQ/ark-node/internal/interface/web/templates/modals"
	"github.com/ArkLabsHQ/ark-node/internal/interface/web/templates/pages"
	"github.com/ArkLabsHQ/ark-node/internal/interface/web/types"
	"github.com/ArkLabsHQ/ark-node/utils"
	"github.com/a-h/templ"
	"github.com/angelofallars/htmx-go"
	arksdk "github.com/ark-network/ark/pkg/client-sdk"
	"github.com/gin-gonic/gin"
	log "github.com/sirupsen/logrus"

	qrcode "github.com/skip2/go-qrcode"
)

func (s *service) backupInitial(c *gin.Context) {
	if s.redirectedBecauseWalletIsLocked(c) {
		return
	}
	bodyContent := pages.BackupInitialBodyContent()
	s.pageViewHandler(bodyContent, c)
}

func (s *service) backupSecret(c *gin.Context) {
	if s.redirectedBecauseWalletIsLocked(c) {
		return
	}
	secret, err := s.svc.Dump(c)
	if err != nil {
		toast := components.Toast("Unable to get secret", true)
		toastHandler(toast, c)
		return
	}
	bodyContent := pages.BackupSecretBodyContent(secret)
	partialViewHandler(bodyContent, c)
}

func (s *service) backupAck(c *gin.Context) {
	if s.redirectedBecauseWalletIsLocked(c) {
		return
	}
	bodyContent := pages.BackupAckBodyContent()
	partialViewHandler(bodyContent, c)
}

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
	c.Redirect(http.StatusFound, "/app/welcome")
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
	if !utils.IsValidURL(aspurl) {
		toast := components.Toast("Invalid ASP URL", true)
		toastHandler(toast, c)
		return
	}

	privateKey := c.PostForm("privateKey")
	if privateKey == "" {
		toast := components.Toast("Private key can't be empty", true)
		toastHandler(toast, c)
		return
	}
	if err := utils.IsValidPrivateKey(privateKey); err != nil {
		toast := components.Toast(err.Error(), true)
		toastHandler(toast, c)
		return
	}

	password := c.PostForm("password")
	if password == "" {
		toast := components.Toast("Password can't be empty", true)
		toastHandler(toast, c)
		return
	}
	if err := utils.IsValidPassword(password); err != nil {
		toast := components.Toast(err.Error(), true)
		toastHandler(toast, c)
		return
	}

	if err := s.svc.Setup(c, aspurl, password, privateKey); err != nil {
		log.WithError(err).Warn("failed to initialize")
		toast := components.Toast(err.Error(), true)
		toastHandler(toast, c)
		return
	}
	redirect("/done", c)
}

// nolint:all
func (s *service) importWalletMnemonic(c *gin.Context) {
	var empty []string
	empty = append(empty, "")
	bodyContent := pages.ManageMnemonicContent(empty)
	s.pageViewHandler(bodyContent, c)
}

func (s *service) importWalletPrivateKey(c *gin.Context) {
	bodyContent := pages.ManagePrivateKeyContent("")
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

// nolint:all
func (s *service) newWalletMnemonic(c *gin.Context) {
	bodyContent := pages.ManageMnemonicContent(getNewMnemonic())
	s.pageViewHandler(bodyContent, c)
}

func (s *service) newWalletPrivateKey(c *gin.Context) {
	bodyContent := pages.ManagePrivateKeyContent(getNewPrivateKey())
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

	var sats uint64
	var err error
	if c.PostForm("sats") != "" {
		sats, err = strconv.ParseUint(c.PostForm("sats"), 10, 0)
		if err != nil {
			// nolint:all
			c.AbortWithError(http.StatusInternalServerError, err)
			return
		}
	}
	bip21, _, _, err := s.svc.GetAddress(c, sats)
	if err != nil {
		// nolint:all
		c.AbortWithError(http.StatusInternalServerError, err)
		return
	}

	png, err := qrcode.Encode(bip21, qrcode.Medium, 256)
	if err != nil {
		return
	}
	encoded := base64.StdEncoding.EncodeToString(png)

	bodyContent := pages.ReceiveQrCodeContent(bip21, encoded, fmt.Sprintf("%d", sats))
	s.pageViewHandler(bodyContent, c)
}

func (s *service) receiveSuccess(c *gin.Context) {
	sats := c.PostForm("sats")
	bip21 := c.PostForm(("bip21"))
	offchainAddr := utils.GetArkAddress(bip21)
	partial := pages.ReceiveSuccessContent(offchainAddr, sats)
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

	sats, err := strconv.Atoi(c.PostForm("sats"))
	if err != nil {
		toast := components.Toast("Invalid amount", true)
		toastHandler(toast, c)
		return
	}

	feeAmount := 206 // TODO
	total := sats + feeAmount

	if utils.IsBip21(dest) {
		offchainAddress := utils.GetArkAddress(dest)
		if len(offchainAddress) > 0 {
			addr = offchainAddress
		} else {
			onchainAddress := utils.GetBtcAddress(dest)
			if len(onchainAddress) > 0 {
				addr = onchainAddress
			}
		}
	} else {
		if utils.IsValidBtcAddress(dest) || utils.IsValidArkAddress(dest) {
			addr = dest
		}
	}

	if len(addr) == 0 {
		toast := components.Toast("Invalid address", true)
		toastHandler(toast, c)
		return
	}

	bodyContent := pages.SendPreviewContent(addr, strconv.Itoa(sats), strconv.Itoa(feeAmount), strconv.Itoa(total))
	partialViewHandler(bodyContent, c)
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

	if utils.IsValidArkAddress(address) {
		txId, err = s.svc.SendAsync(c, false, receivers)
		if err != nil {
			toast := components.Toast(err.Error(), true)
			toastHandler(toast, c)
			return
		}
	}

	if utils.IsValidBtcAddress(address) {
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
	privateKey := c.PostForm("privateKey")
	bodyContent := pages.AspUrlBodyContent(c.Query("aspurl"), privateKey, password)
	partialViewHandler(bodyContent, c)
}

func (s *service) setPrivateKey(c *gin.Context) {
	privateKey := c.PostForm("privateKey")
	bodyContent := pages.SetPasswordContent(privateKey)
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

	sats, err := strconv.Atoi(c.PostForm("sats"))
	if err != nil {
		toast := components.Toast("Invalid amount", true)
		toastHandler(toast, c)
		return
	}

	feeAmount := 206 // TODO
	total := sats + feeAmount

	bodyContent := pages.SwapPreviewContent(kind, strconv.Itoa(sats), strconv.Itoa(feeAmount), strconv.Itoa(total))
	partialViewHandler(bodyContent, c)
}

func (s *service) getTx(c *gin.Context) {
	if s.redirectedBecauseWalletIsLocked(c) {
		return
	}

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

	data, err := s.svc.GetConfigData(c)
	if err != nil {
		toast := components.Toast(err.Error(), true)
		toastHandler(toast, c)
		return
	}

	nextClaim := prettyUnixTimestamp(s.svc.WhenNextClaim(c).Unix())
	explorerUrl := getExplorerUrl(data.Network.Name)

	var bodyContent templ.Component
	if tx.Status == "pending" {
		bodyContent = pages.TxPendingContent(tx, nextClaim)
	} else {
		bodyContent = pages.TxBodyContent(tx, explorerUrl)
	}
	s.pageViewHandler(bodyContent, c)
}

func (s *service) welcome(c *gin.Context) {
	if _, err := s.svc.GetSettings(c); err != nil {
		if err := s.svc.AddDefaultSettings(c); err != nil {
			return
		}
	}
	bodyContent := pages.Welcome()
	s.pageViewHandler(bodyContent, c)
}

func (s *service) feeInfoModal(c *gin.Context) {
	info := modals.FeeInfo()
	modalHandler(info, c)
}

func (s *service) getSpendableBalance(c *gin.Context) (string, error) {
	balance, err := s.svc.Balance(c, false)
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

func (s *service) getTxHistory(c *gin.Context) (transactions []types.Transaction, err error) {
	// get tx history from ASP
	history, err := s.svc.GetTransactionHistory(c)
	if err != nil {
		return nil, err
	}
	data, err := s.svc.GetConfigData(c)
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
		// TODO: use tx.ExpiresAt when it will be available
		expiresAt := tx.CreatedAt.Unix() + data.RoundLifetime
		// status of tx
		status := "success"
		if tx.IsPending {
			status = "pending"
		}
		emptyTime := time.Time{}
		if tx.CreatedAt == emptyTime {
			status = "unconfirmed"
			dateCreated = 0
		}
		// get one txid to identify tx
		txid := tx.RoundTxid
		explorable := true
		if len(txid) == 0 {
			txid = tx.RedeemTxid
			explorable = false
		}
		if len(txid) == 0 {
			txid = tx.BoardingTxid
			explorable = true
		}
		// add to slice of transactions
		transactions = append(transactions, types.Transaction{
			Amount:     amount,
			CreatedAt:  prettyUnixTimestamp(dateCreated),
			Day:        prettyDay(dateCreated),
			ExpiresAt:  prettyUnixTimestamp(expiresAt),
			Explorable: explorable,
			Hour:       prettyHour(dateCreated),
			Kind:       string(tx.Type),
			Txid:       txid,
			Status:     status,
			UnixDate:   dateCreated,
		})
	}
	log.Infof("history %+v", history)
	log.Infof("transactions %+v", transactions)
	return
}

func (s *service) redirectedBecauseWalletIsLocked(c *gin.Context) bool {
	redirect := s.svc.IsLocked(c)
	if redirect {
		c.Redirect(http.StatusFound, "/app")
	}
	return redirect
}

func (s *service) reversibleInfoModal(c *gin.Context) {
	info := modals.ReversibleInfo()
	modalHandler(info, c)
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

func (s *service) seedInfoModal(c *gin.Context) {
	seed, err := s.svc.ArkClient.Dump(c)
	if err != nil {
		toast := components.Toast("Unable to get seed", true)
		toastHandler(toast, c)
		return
	}
	info := modals.SeedInfo(seed)
	modalHandler(info, c)
}
