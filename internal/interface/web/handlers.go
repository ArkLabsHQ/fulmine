package web

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ArkLabsHQ/fulmine/internal/core/domain"
	"github.com/ArkLabsHQ/fulmine/internal/interface/web/templates"
	"github.com/ArkLabsHQ/fulmine/internal/interface/web/templates/components"
	"github.com/ArkLabsHQ/fulmine/internal/interface/web/templates/modals"
	"github.com/ArkLabsHQ/fulmine/internal/interface/web/templates/pages"
	"github.com/ArkLabsHQ/fulmine/internal/interface/web/types"
	"github.com/ArkLabsHQ/fulmine/pkg/boltz"
	"github.com/ArkLabsHQ/fulmine/utils"
	"github.com/a-h/templ"
	"github.com/angelofallars/htmx-go"
	"github.com/ark-network/ark/common"
	arksdk "github.com/ark-network/ark/pkg/client-sdk"
	sdktypes "github.com/ark-network/ark/pkg/client-sdk/types"
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

func (s *service) backupAck(c *gin.Context) {
	if s.redirectedBecauseWalletIsLocked(c) {
		return
	}
	bodyContent := pages.BackupAckBodyContent()
	partialViewHandler(bodyContent, c)
}

func (s *service) backupSecret(c *gin.Context) {
	if s.redirectedBecauseWalletIsLocked(c) {
		return
	}
	seed, err := s.svc.Dump(c)
	if err != nil {
		toast := components.Toast("Unable to get seed", true)
		toastHandler(toast, c)
		return
	}
	nsec, err := utils.SeedToNsec(seed)
	if err != nil {
		toast := components.Toast("Unable to convert to nsec", true)
		toastHandler(toast, c)
		return
	}
	bodyContent := pages.BackupSecretBodyContent(seed, nsec)
	partialViewHandler(bodyContent, c)
}

func (s *service) backupTabActive(c *gin.Context) {
	active := c.Param("active")
	seed, err := s.svc.Dump(c)
	if err != nil {
		toast := components.Toast("Unable to get seed", true)
		toastHandler(toast, c)
		return
	}
	secret := seed
	if active == "nsec" {
		nsec, err := utils.SeedToNsec(seed)
		if err != nil {
			toast := components.Toast("Unable to convert to nsec", true)
			toastHandler(toast, c)
			return
		}
		secret = nsec
	}
	bodyContent := pages.BackupPartialContent(active, secret)
	partialViewHandler(bodyContent, c)
}

func (s *service) done(c *gin.Context) {
	bodyContent := pages.DoneBodyContent()
	s.pageViewHandler(bodyContent, c)
}

func (s *service) events(c *gin.Context) {
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")

	channel := s.svc.GetTransactionEventChannel(c.Request.Context())
	for {
		select {
		case <-s.stopCh:
			return
		case <-c.Request.Context().Done():
			return
		case event, ok := <-channel:
			if !ok {
				return
			}
			c.SSEvent(event.Type.String(), event)
			c.Writer.Flush()
		}
	}
}

func (s *service) index(c *gin.Context) {
	bodyContent := pages.Welcome()
	if s.svc.IsReady() {
		if s.svc.IsLocked(c) {
			bodyContent = pages.Unlock()
		} else {
			bodyContent = pages.IndexBodyContent()
		}
	}
	s.pageViewHandler(bodyContent, c)
}

func (s *service) initialize(c *gin.Context) {
	serverUrl := c.PostForm("serverUrl")
	if serverUrl == "" {
		toast := components.Toast("Server URL can't be empty", true)
		toastHandler(toast, c)
		return
	}
	if !utils.IsValidURL(serverUrl) {
		toast := components.Toast("Invalid server URL", true)
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

	if err := s.svc.Setup(c, serverUrl, password, privateKey); err != nil {
		log.WithError(err).Warn("failed to initialize")
		toast := components.Toast(err.Error(), true)
		toastHandler(toast, c)
		return
	}

	redirect("/done", c)
}

func (s *service) importWalletPrivateKey(c *gin.Context) {
	bodyContent := pages.ManagePrivateKeyContent("")
	s.pageViewHandler(bodyContent, c)
}

func (s *service) lock(c *gin.Context) {
	if err := s.svc.LockNode(c); err != nil {
		toast := components.Toast(err.Error(), true)
		toastHandler(toast, c)
		return
	}
	c.Redirect(http.StatusFound, "/")
}

func (s *service) unlock(c *gin.Context) {
	bodyContent := pages.Unlock()
	s.pageViewHandler(bodyContent, c)
}

func (s *service) newWalletPrivateKey(c *gin.Context) {
	nsec, err := utils.SeedToNsec(utils.GetNewPrivateKey())
	if err != nil {
		// nolint:all
		c.AbortWithError(http.StatusInternalServerError, err)
		return
	}
	bodyContent := pages.ManagePrivateKeyContent(nsec)
	s.pageViewHandler(bodyContent, c)
}

func (s *service) noteConfirm(c *gin.Context) {
	if s.redirectedBecauseWalletIsLocked(c) {
		return
	}

	note := c.PostForm("note")

	sats := utils.SatsFromNote(note)
	if sats == 0 {
		toast := components.Toast("invalid ark note", true)
		toastHandler(toast, c)
		return
	}

	txId, err := s.svc.RedeemNotes(c, []string{note})

	if err != nil {
		toast := components.Toast(err.Error(), true)
		toastHandler(toast, c)
		return
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

	bodyContent := pages.NoteSuccessContent(strconv.Itoa(sats), txId, explorerUrl)
	partialViewHandler(bodyContent, c)
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
	bip21, offchainAddr, boardingAddr, _, err := s.svc.GetAddress(c, sats)
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

	bodyContent := pages.ReceiveQrCodeContent(bip21, offchainAddr, boardingAddr, encoded, fmt.Sprintf("%d", sats))
	s.pageViewHandler(bodyContent, c)
}

func (s *service) receiveSuccess(c *gin.Context) {
	bip21 := c.PostForm(("bip21"))

	txHistory, err := s.svc.GetTransactionHistory(c)
	if err != nil {
		toast := components.Toast(err.Error(), true)
		toastHandler(toast, c)
		return
	}

	lastTx := txHistory[0]

	sats := strconv.Itoa(int(lastTx.Amount))

	var addr string
	if len(lastTx.BoardingTxid) > 0 {
		addr = utils.GetBtcAddress(bip21)
	} else {
		addr = utils.GetArkAddress(bip21)
	}

	partial := pages.ReceiveSuccessContent(addr, sats)
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

	config, err := s.svc.GetConfigData(c)
	if err != nil {
		toast := components.Toast(err.Error(), true)
		toastHandler(toast, c)
		return
	}

	dest := c.PostForm("address")
	var addr, onchainAddr, offchainAddr string

	sats, err := strconv.Atoi(c.PostForm("sats"))
	if err != nil {
		toast := components.Toast("Invalid amount", true)
		toastHandler(toast, c)
		return
	}

	feeAmount := 0 // TODO
	total := sats + feeAmount

	if utils.IsValidArkNote(dest) {
		sats := utils.SatsFromNote(dest)

		if config.VtxoMaxAmount != -1 && int64(sats) > config.VtxoMaxAmount {
			toast := components.Toast("Amount too high", true)
			toastHandler(toast, c)
			return
		}

		bodyContent := pages.NotePreviewContent(dest, strconv.Itoa(sats))
		partialViewHandler(bodyContent, c)
	}

	if utils.IsBip21(dest) {
		offchainAddr = utils.GetArkAddress(dest)
		onchainAddr = utils.GetBtcAddress(dest)
	}
	if utils.IsValidBtcAddress(dest) {
		onchainAddr = dest
	}
	if utils.IsValidArkAddress(dest) {
		offchainAddr = dest
	}

	if len(offchainAddr) > 0 {
		if config.VtxoMaxAmount != -1 && int64(total) > config.VtxoMaxAmount {
			if len(onchainAddr) > 0 && (config.UtxoMaxAmount == -1 || int64(total) <= config.UtxoMaxAmount) {
				addr = onchainAddr
			} else {
				toast := components.Toast("Amount too high", true)
				toastHandler(toast, c)
				return
			}
		} else {
			addr = offchainAddr
		}
	} else if len(onchainAddr) > 0 {
		if config.UtxoMaxAmount != -1 && int64(total) > config.UtxoMaxAmount {
			toast := components.Toast("Amount too high", true)
			toastHandler(toast, c)
			return
		} else {
			addr = onchainAddr
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
		txId, err = s.svc.SendOffChain(c, false, receivers, true)
		if err != nil {
			toast := components.Toast(err.Error(), true)
			toastHandler(toast, c)
			return
		}
	}

	if utils.IsValidBtcAddress(address) {
		txId, err = s.svc.CollaborativeExit(c, address, value, false)
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
	// validate passwords
	password := c.PostForm("password")
	pconfirm := c.PostForm("pconfirm")
	if password != pconfirm {
		toast := components.Toast("Passwords doesn't match", true)
		toastHandler(toast, c)
		return
	}

	privateKey := c.PostForm("privateKey")

	// priority rules to serverUrl:
	// 1. from query string (aka urlOnQuery)
	// 2. from env variable (aka cfg.ArkServer)
	// 3. user inserts on form
	serverUrl := c.PostForm("urlOnQuery")
	if serverUrl == "" {
		serverUrl = s.arkServer
	}

	bodyContent := pages.ServerUrlBodyContent(serverUrl, privateKey, password)
	partialViewHandler(bodyContent, c)
}

func (s *service) setPrivateKey(c *gin.Context) {
	privateKey := c.PostForm("privateKey")
	if strings.HasPrefix(privateKey, "nsec") {
		seed, err := utils.NsecToSeed(privateKey)
		if err != nil {
			toast := components.Toast("Invalid nsec", true)
			toastHandler(toast, c)
			return
		}
		privateKey = seed
	}
	bodyContent := pages.SetPasswordContent(privateKey)
	partialViewHandler(bodyContent, c)
}

func (s *service) settings(c *gin.Context) {
	if s.redirectedBecauseWalletIsLocked(c) {
		return
	}

	settings, err := s.svc.GetSettings(c)
	if err != nil {
		// nolint:all
		c.AbortWithError(http.StatusInternalServerError, err)
		return
	}

	active := c.Param("active")
	bodyContent := pages.SettingsBodyContent(
		active, *settings, s.svc.IsConnectedLN(), s.svc.IsLocked(c), s.svc.BuildInfo.Version,
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

	bodyContent := pages.SwapBodyContent(spendableBalance, s.getNodeBalance(c), s.svc.IsConnectedLN())
	s.pageViewHandler(bodyContent, c)
}

func (s *service) swapActive(c *gin.Context) {
	active := c.Param("active")
	var balance string
	if active == "inbound" {
		balance = s.getNodeBalance(c)
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

	satsUint64, err := strconv.ParseUint(sats, 10, 64)
	if err != nil {
		toast := components.Toast("Invalid amount", true)
		toastHandler(toast, c)
		return
	}

	txid := ""

	if kind == "inbound" {
		txid, err = s.svc.IncreaseInboundCapacity(c, satsUint64)
		if err != nil {
			toast := components.Toast(err.Error(), true)
			toastHandler(toast, c)
			return
		}
	} else {
		txid, err = s.svc.IncreaseOutboundCapacity(c, satsUint64)
		if err != nil {
			toast := components.Toast(err.Error(), true)
			toastHandler(toast, c)
			return
		}
	}

	bodyContent := pages.SwapSuccessContent(kind, sats, txid, explorerUrl)
	partialViewHandler(bodyContent, c)
}

func (s *service) swapPreview(c *gin.Context) {
	if s.redirectedBecauseWalletIsLocked(c) {
		return
	}

	config, err := s.svc.GetConfigData(c)
	if err != nil {
		toast := components.Toast(err.Error(), true)
		toastHandler(toast, c)
		return
	}

	kind := c.PostForm("kind")

	sats, err := strconv.Atoi(c.PostForm("sats"))
	if err != nil {
		toast := components.Toast("Invalid amount", true)
		toastHandler(toast, c)
		return
	}

	feeAmount := 0 // TODO
	total := sats + feeAmount

	if config.VtxoMaxAmount != -1 && int64(total) > config.VtxoMaxAmount {
		toast := components.Toast("Amount too high", true)
		toastHandler(toast, c)
		return
	}

	bodyContent := pages.SwapPreviewContent(kind, strconv.Itoa(sats), strconv.Itoa(feeAmount), strconv.Itoa(total))
	partialViewHandler(bodyContent, c)
}

func (s *service) getTransfer(c *gin.Context, transfer types.Transfer, explorerUrl string) templ.Component {
	if transfer.Status == "pending" {
		var nextSettlementStr string
		nextSettlement := s.svc.WhenNextSettlement(c)
		if nextSettlement.IsZero() {
			// if no next settlement, it means it is about to be scheduled for a boarding tx
			// fallback to now + boarding timelock to show a time closest to next settlement
			data, err := s.svc.GetConfigData(c)
			if err != nil {
				nextSettlementStr = "unknown"
			} else {
				// TODO: use boardingExitDelay https://github.com/ark-network/ark/pull/501
				boardingTimelock := common.RelativeLocktime{Type: data.UnilateralExitDelay.Type, Value: data.UnilateralExitDelay.Value * 2}
				closeToBoardingSettlement := time.Now().Add(time.Duration(boardingTimelock.Seconds()) * time.Second)
				nextSettlement = closeToBoardingSettlement
			}
		}

		if nextSettlementStr != "unknown" {
			nextSettlementStr = prettyUnixTimestamp(nextSettlement.Unix())
		}

		return pages.TransferTxPendingContent(transfer, explorerUrl, nextSettlementStr)
	} else {
		return pages.TransferTxBodyContent(transfer, explorerUrl)
	}
}

// TODO: Ensure the correct Content are being displayed
func (s *service) getSwap(swap types.Swap, vhtlc *types.Transfer, redeem *types.Transfer) templ.Component {
	return pages.SwapContent(swap, vhtlc, redeem)
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

	data, err := s.svc.GetConfigData(c)
	if err != nil {
		toast := components.Toast(err.Error(), true)
		toastHandler(toast, c)
		return
	}
	explorerUrl := getExplorerUrl(data.Network.Name)

	txid := c.Param("txid")
	var tx types.Transaction
	for _, transaction := range txHistory {
		if transaction.Id == txid {
			tx = transaction
			break
		}
		// Display the redeem transfer for swap transactions Providing option to redeem
		if transaction.Kind == "swap" && transaction.RedeemTransfer != nil && transaction.RedeemTransfer.Txid == txid {
			bodyContent := s.getTransfer(c, *transaction.RedeemTransfer, explorerUrl)
			s.pageViewHandler(bodyContent, c)
			return
		}
	}

	var bodyContent templ.Component
	if len(tx.Id) == 0 {
		bodyContent = pages.TxNotFoundContent()
	} else if tx.Kind == "transfer" {
		bodyContent = s.getTransfer(c, *tx.Transfer, explorerUrl)
	} else {
		bodyContent = s.getSwap(*tx.Swap, tx.VHTLCTransfer, tx.RedeemTransfer)
	}
	s.pageViewHandler(bodyContent, c)
}

func (s *service) getTxs(c *gin.Context) {
	if s.redirectedBecauseWalletIsLocked(c) {
		return
	}

	lastId := c.Param("lastId")
	loadMore := false
	txsPerPage := 10

	txHistory, err := s.getTxHistory(c)
	if err != nil {
		log.WithError(err).Warn("failed to get tx history")
	}

	if lastId == "0" {
		if len(txHistory) > txsPerPage {
			txHistory = txHistory[:txsPerPage]
			loadMore = true
		}
	} else {
		for i, tx := range txHistory {
			if tx.Id == lastId {
				txsOnList := i + 1
				if txsOnList+txsPerPage < len(txHistory) {
					txHistory = txHistory[:txsOnList+txsPerPage]
					loadMore = true
				}
				break
			}
		}
	}

	lastId = "0"
	if len(txHistory) > 0 {
		lastId = txHistory[len(txHistory)-1].Id
	}

	bodyContent := components.HistoryBodyContent(txHistory, lastId, loadMore)
	partialViewHandler(bodyContent, c)
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
	balance, err := s.svc.GetTotalBalance(c)
	if err != nil {
		return "", err
	}
	return strconv.FormatUint(balance, 10), nil
}

func (s *service) getNodeBalance(c *gin.Context) string {
	if s.svc.IsConnectedLN() {
		msats, err := s.svc.GetBalanceLN(c)
		if err == nil {
			sats := msats / 1000
			return strconv.FormatUint(sats, 10)
		}
	}
	return "0"
}

func (s *service) getTxHistory(c *gin.Context) (transactions []types.Transaction, err error) {
	// get tx history from Server
	transferTxns, err := s.svc.GetTransactionHistory(c)
	if err != nil {
		return nil, err
	}
	data, err := s.svc.GetConfigData(c)
	if err != nil {
		return nil, err
	}
	// TODO: use tx.ExpiresAt when it will be available
	treeExpiryValue := int64(data.VtxoTreeExpiry.Value)

	// Get Swap Transaction
	swapTxs, err := s.svc.GetSwapHistory(c)
	if err != nil {
		return nil, err
	}

	history := make([]types.Transaction, 0, len(transferTxns)+len(swapTxs))

	// add swaps to history
	for _, swap := range swapTxs {
		transformedSwap := toSwap(swap)
		swapTxn := types.Transaction{
			Kind:        "swap",
			Swap:        &transformedSwap,
			Id:          swap.Id,
			DateCreated: swap.Timestamp,
		}

		if transformedSwap.Kind == "submarine" {
			updatedTransfers, sendTransfer, ok := RemoveFind(transferTxns, func(t sdktypes.Transaction) bool {
				return swap.FundingTxId != "" && swap.FundingTxId == t.RedeemTxid
			})

			if ok {
				transferTxns = updatedTransfers
				modifiedSendTransfer := toTransfer(sendTransfer, treeExpiryValue)
				swapTxn.VHTLCTransfer = &modifiedSendTransfer
			}

			updatedTransfers, receiveTransfer, ok := RemoveFind(transferTxns, func(t sdktypes.Transaction) bool {
				return swap.RedeemTxId != "" && swap.RedeemTxId == t.RedeemTxid
			})
			if ok {
				transferTxns = updatedTransfers
				modifiedReceiveTransfer := toTransfer(receiveTransfer, treeExpiryValue)
				swapTxn.RedeemTransfer = &modifiedReceiveTransfer
			}

		} else {
			updatedTransfers, receiveTransfer, ok := RemoveFind(transferTxns, func(t sdktypes.Transaction) bool {
				return swap.RedeemTxId != "" && swap.RedeemTxId == t.RedeemTxid
			})

			if ok {
				transferTxns = updatedTransfers
				modifiedReceiveTransfer := toTransfer(receiveTransfer, treeExpiryValue)
				swapTxn.RedeemTransfer = &modifiedReceiveTransfer
			}
		}

		history = append(history, swapTxn)

	}

	// transform remaining transaction types
	for _, tx := range transferTxns {

		modifiedTansfer := toTransfer(tx, treeExpiryValue)
		transaction := types.Transaction{
			Kind:        "transfer",
			Transfer:    &modifiedTansfer,
			Id:          modifiedTansfer.Txid,
			DateCreated: tx.CreatedAt.Unix(),
		}
		history = append(history, transaction)

	}

	sort.SliceStable(history, func(i, j int) bool {
		return history[i].DateCreated > history[j].DateCreated
	})
	return history, nil
}

func (s *service) redirectedBecauseWalletIsLocked(c *gin.Context) bool {
	var shouldRedirect bool
	func() {
		defer func() {
			// redirect even if IsLocked() panics
			if r := recover(); r != nil {
				log.WithError(fmt.Errorf("%v", r)).Warn("IsLocked() panicked")
				shouldRedirect = true
			}
		}()
		shouldRedirect = s.svc.IsLocked(c)
	}()

	if shouldRedirect {
		c.Redirect(http.StatusFound, "/")
	}
	return shouldRedirect
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

func (s *service) scannerModal(c *gin.Context) {
	id := c.Param("id")
	scan := modals.Scanner(id)
	modalHandler(scan, c)
}

func (s *service) seedInfoModal(c *gin.Context) {
	seed, err := s.svc.Dump(c)
	if err != nil {
		toast := components.Toast("Unable to get seed", true)
		toastHandler(toast, c)
		return
	}
	info := modals.SeedInfo(seed)
	modalHandler(info, c)
}

func (s *service) claimTx(c *gin.Context) {
	data, err := s.svc.GetConfigData(c)
	if err != nil {
		toast := components.Toast(err.Error(), true)
		toastHandler(toast, c)
		return
	}

	transferTxns, err := s.svc.GetTransactionHistory(c)
	if err != nil {
		// nolint:all
		c.AbortWithError(http.StatusInternalServerError, err)
		return
	}

	txid := c.Param("txid")
	var tx types.Transfer
	for _, transaction := range transferTxns {
		transfer := toTransfer(transaction, int64(data.VtxoTreeExpiry.Value))
		if transfer.Txid == txid {
			tx = transfer
			break
		}
	}

	if len(tx.Txid) == 0 {
		toast := components.Toast("transaction not found", true)
		toastHandler(toast, c)
		return
	}

	if _, err := s.svc.Settle(c); err != nil {
		toast := components.Toast(err.Error(), true)
		toastHandler(toast, c)
		return
	}

	tx.Status = "success"

	partial := components.Transfer(tx, getExplorerUrl(data.Network.Name))
	partialViewHandler(partial, c)
}

func (s *service) lnConnectInfoModal(c *gin.Context) {
	info := modals.LnConnectInfo()
	modalHandler(info, c)
}

func (s *service) getHero(c *gin.Context) {
	if s.redirectedBecauseWalletIsLocked(c) {
		return
	}

	var isOnline bool

	spendableBalance, err := s.getSpendableBalance(c)
	if err == nil {
		isOnline = true
	} else {
		log.WithError(err).Warn("failed to get spendable balance")
	}

	partialContent := components.Hero(spendableBalance, isOnline)
	partialViewHandler(partialContent, c)
}

func (s *service) swapHistory(c *gin.Context) {
	if s.redirectedBecauseWalletIsLocked(c) {
		return
	}
	bodyContent := pages.SwapHistoryBodyContent()
	s.pageViewHandler(bodyContent, c)
}

func (s *service) getSwaps(c *gin.Context) {
	if s.redirectedBecauseWalletIsLocked(c) {
		return
	}

	// TODO: Fix the errors later
	swapHistory, _ := s.svc.GetSwapHistory(c)

	parsedSwapHistory := make([]types.Swap, len(swapHistory))

	for i, swap := range swapHistory {
		parsedSwapHistory[i] = toSwap(swap)
	}

	lastId := c.Param("lastId")
	loadMore := false
	txsPerPage := 2

	if lastId != "0" {
		for i, swap := range parsedSwapHistory {
			if swap.Id == lastId {
				firstIndex := i + 1
				if firstIndex+txsPerPage > len(parsedSwapHistory) {
					parsedSwapHistory = parsedSwapHistory[i+1:]
				} else {
					parsedSwapHistory = parsedSwapHistory[i+1 : i+1+txsPerPage]
					loadMore = true
				}
				break
			}
		}
	}

	if len(parsedSwapHistory) > txsPerPage {
		parsedSwapHistory = parsedSwapHistory[:txsPerPage]
		loadMore = true
	}

	bodyContent := pages.SwapHistoryListContent(parsedSwapHistory, loadMore)
	partialViewHandler(bodyContent, c)
}

// RemoveFind drops the first element in slice for which match(v) is true.
// It returns the updated slice, the removed element, and true.
// If nothing matches, it returns the original slice, the zero value, and false.
func RemoveFind[T any](slice []T, match func(T) bool) ([]T, T, bool) {
	var zero T
	for i, v := range slice {
		if match(v) {
			// remove element at i
			slice = append(slice[:i], slice[i+1:]...)
			return slice, v, true
		}
	}
	return slice, zero, false
}

func toSwap(swap domain.Swap) types.Swap {
	selectSwapType := func(swap domain.Swap) string {
		if swap.To == boltz.CurrencyBtc && swap.From == boltz.CurrencyArk {
			return "submarine"
		} else {
			return "reverse"
		}
	}

	selectSwapStatus := func(swap domain.Swap) string {
		switch swap.Status {
		case domain.SwapSuccess:
			return "success"
		case domain.SwapPending:
			return "pending"
		default:
			return "failure"
		}
	}

	return types.Swap{
		Amount: strconv.FormatUint(swap.Amount, 10),
		Date:   prettyDay(swap.Timestamp),
		Hour:   prettyHour(swap.Timestamp),
		Id:     swap.Id,
		Kind:   selectSwapType(swap),
		Status: selectSwapStatus(swap),
	}
}

func toTransfer(tx sdktypes.Transaction, treeExpiryValue int64) types.Transfer {
	// amount
	amount := strconv.FormatUint(tx.Amount, 10)
	if tx.Type == sdktypes.TxSent {
		amount = "-" + amount
	}
	// date of creation
	dateCreated := tx.CreatedAt.Unix()
	// TODO: use tx.ExpiresAt when it will be available
	expiresAt := tx.CreatedAt.Unix() + treeExpiryValue
	// status of tx
	status := "pending"
	if tx.Settled {
		status = "success"
	}
	if tx.CreatedAt.IsZero() {
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

	return types.Transfer{
		Amount:     amount,
		CreatedAt:  prettyUnixTimestamp(dateCreated),
		Day:        prettyDay(dateCreated),
		ExpiresAt:  prettyUnixTimestamp(expiresAt),
		Explorable: explorable,
		Hour:       prettyHour(dateCreated),
		Kind:       strings.ToLower(string(tx.Type)),
		Txid:       txid,
		Status:     status,
		UnixDate:   dateCreated,
	}
}
