package web

import (
	"context"
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
	"github.com/ArkLabsHQ/fulmine/pkg/swap"
	"github.com/ArkLabsHQ/fulmine/utils"
	"github.com/a-h/templ"
	"github.com/angelofallars/htmx-go"
	arklib "github.com/arkade-os/arkd/pkg/ark-lib"
	clientTypes "github.com/arkade-os/arkd/pkg/client-lib/types"
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

	if isSynced, _ := s.svc.IsSynced(); !isSynced {
		syncedCh := s.svc.GetSyncedUpdate()
		for {
			select {
			case <-s.stopCh:
				return
			case <-c.Request.Context().Done():
				return
			case event, ok := <-syncedCh:
				if !ok {
					continue
				}
				c.SSEvent("SYNCED", event)
				c.Writer.Flush()
			}
		}

	}

	txsCh := s.svc.GetTransactionEventChannel(c.Request.Context())
	for {
		select {
		case <-s.stopCh:
			return
		case <-c.Request.Context().Done():
			return
		case event, ok := <-txsCh:
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
	if s.svc.IsInitialized() {
		{
			if s.svc.IsLocked(c) {
				bodyContent = pages.Unlock()
			} else {
				bodyContent = pages.IndexBodyContent()
			}
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
	if autoPassword, ok := s.autoUnlockPassword(c); ok && !s.svc.IsInitialized() {
		// An unlocker is configured: the wallet must be created with its password
		// so the daemon can auto-unlock afterwards. The unlocker password is
		// authoritative, so any submitted value is ignored.
		password = autoPassword
	} else {
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
	}

	if err := s.svc.Setup(c, serverUrl, password, privateKey); err != nil {
		log.WithError(err).Warn("failed to initialize")
		errorContent := components.Error("Server initialization failed", "Please try again")
		partialViewHandler(errorContent, c)
		return
	}

	redirect("/done", c)
}

func (s *service) importWalletPrivateKey(c *gin.Context) {
	bodyContent := pages.ManagePrivateKeyContent("")
	s.pageViewHandler(bodyContent, c)
}

func (s *service) lock(c *gin.Context) {
	if s.redirectedBecauseWalletIsLocked(c) {
		return
	}
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
	bip21, offchainAddr, boardingAddr, invoice, _, err := s.svc.GetAddress(c, sats)

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

	bodyContent := pages.ReceiveQrCodeContent(bip21, offchainAddr, boardingAddr, invoice, encoded, fmt.Sprintf("%d", sats), s.svc.CurrentLnurl())
	s.pageViewHandler(bodyContent, c)
}

func (s *service) receiveSwap(c *gin.Context) {
	if s.redirectedBecauseWalletIsLocked(c) {
		return
	}
	sats, err := strconv.ParseUint(c.PostForm("sats"), 10, 0)
	if err != nil || sats == 0 {
		toast := components.Toast("enter an amount to swap", true)
		toastHandler(toast, c)
		return
	}
	chainSwap, err := s.svc.CreateBtcToArkChainSwap(c, sats)
	if err != nil {
		toast := components.Toast(err.Error(), true)
		toastHandler(toast, c)
		return
	}
	png, err := qrcode.Encode(chainSwap.UserBtcLockupAddress, qrcode.Medium, 256)
	if err != nil {
		// nolint:all
		c.AbortWithError(http.StatusInternalServerError, err)
		return
	}
	encoded := base64.StdEncoding.EncodeToString(png)
	bodyContent := pages.ReceiveSwapContent(chainSwap.UserBtcLockupAddress, fmt.Sprintf("%d", sats), encoded)
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
	var addr, invoice, onchainAddr, offchainAddr, offer string

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
		return
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
	if utils.IsValidInvoice(dest) {
		invoice = dest
	}
	if swap.IsBolt12Offer(dest) {
		offer = dest
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
	} else if len(invoice) > 0 {
		if config.VtxoMaxAmount != -1 && int64(total) > config.VtxoMaxAmount {
			if len(onchainAddr) > 0 && (config.UtxoMaxAmount == -1 || int64(total) <= config.UtxoMaxAmount) {
				addr = onchainAddr
			} else {
				toast := components.Toast("Amount too high", true)
				toastHandler(toast, c)
				return
			}
		} else {
			addr = invoice
		}
	} else if len(onchainAddr) > 0 {
		if config.UtxoMaxAmount != -1 && int64(total) > config.UtxoMaxAmount {
			toast := components.Toast("Amount too high", true)
			toastHandler(toast, c)
			return
		} else {
			addr = onchainAddr
		}
	} else if len(offer) > 0 {
		if config.VtxoMaxAmount != -1 && int64(total) > config.VtxoMaxAmount {
			if len(onchainAddr) > 0 && (config.UtxoMaxAmount == -1 || int64(total) <= config.UtxoMaxAmount) {
				addr = onchainAddr
			} else {
				toast := components.Toast("Amount too high", true)
				toastHandler(toast, c)
				return
			}
		} else {
			addr = offer
		}

	}

	if utils.IsLnAddressOrLnurl(dest) {
		addr = dest
	}

	if len(addr) == 0 {
		toast := components.Toast("Invalid address", true)
		toastHandler(toast, c)
		return
	}

	bodyContent := pages.SendPreviewContent(addr, strconv.Itoa(sats), strconv.Itoa(feeAmount), strconv.Itoa(total), utils.IsValidBtcAddress(addr))
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

	receivers := []clientTypes.Receiver{{To: address, Amount: value}}

	if utils.IsValidArkAddress(address) {
		for range 3 {
			txId, err = s.svc.SendOffChain(c, receivers)
			if err != nil {
				if strings.Contains(strings.ToLower(err.Error()), "vtxo_already_spent") {
					continue
				}
				toast := components.Toast(err.Error(), true)
				toastHandler(toast, c)
				return
			}
			break
		}
		if err != nil {
			log.WithError(err).Error("failed to pay to vHTLC address")
			toast := components.Toast(err.Error(), true)
			if strings.Contains(strings.ToLower(err.Error()), "vtxo_already_spent") {
				toast = components.Toast("something went wrong, please try again", true)
			}
			toastHandler(toast, c)
			return
		}
	}

	if utils.IsValidBtcAddress(address) {
		if c.PostForm("method") == "swap" {
			if _, err := s.svc.CreateChainSwapArkToBtc(c, value, address); err != nil {
				toast := components.Toast(err.Error(), true)
				toastHandler(toast, c)
				return
			}
			// the chain swap settles asynchronously; it shows up in tx history.
			redirect("/", c)
			return
		}
		txId, err = s.svc.CollaborativeExit(c, address, value)
		if err != nil {
			toast := components.Toast(err.Error(), true)
			toastHandler(toast, c)
			return
		}
	}

	if utils.IsValidInvoice(address) {
		resp, err := s.svc.PayInvoice(c, address)
		if err != nil {
			toast := components.Toast(err.Error(), true)
			toastHandler(toast, c)
			return
		}
		txId = resp.TxId

		if resp.SwapStatus == domain.SwapFailed {
			bodyContent := pages.SendFailureContent(address, sats)
			partialViewHandler(bodyContent, c)
			return
		}
	}

	if utils.IsLnAddressOrLnurl(address) {
		invoice, err := utils.ResolveLightningAddressOrLnurl(nil, address, value)
		if err != nil {
			toast := components.Toast(err.Error(), true)
			toastHandler(toast, c)
			return
		}
		resp, err := s.svc.PayInvoice(c, invoice)
		if err != nil {
			toast := components.Toast(err.Error(), true)
			toastHandler(toast, c)
			return
		}
		txId = resp.TxId

		if resp.SwapStatus == domain.SwapFailed {
			bodyContent := pages.SendFailureContent(address, sats)
			partialViewHandler(bodyContent, c)
			return
		}
	}

	if swap.IsValidBolt12Offer(address) {
		resp, err := s.svc.PayOffer(c, address)
		if err != nil {
			toast := components.Toast(err.Error(), true)
			toastHandler(toast, c)
			return
		}
		txId = resp.TxId

		if resp.SwapStatus == domain.SwapFailed {
			bodyContent := pages.SendFailureContent(address, sats)
			partialViewHandler(bodyContent, c)
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

	// When an unlocker is configured (e.g. FULMINE_UNLOCKER_PASSWORD) and the
	// wallet isn't initialized yet, the wallet must be created with the unlocker
	// password so it can auto-unlock afterwards. Asking for a password here would
	// be redundant (and a footgun), so skip straight to the server URL step.
	if _, ok := s.autoUnlockPassword(c); ok && !s.svc.IsInitialized() {
		serverUrl := c.PostForm("urlOnQuery")
		if serverUrl == "" {
			serverUrl = s.arkServer
		}
		bodyContent := pages.ServerUrlBodyContent(serverUrl, privateKey, "")
		partialViewHandler(bodyContent, c)
		return
	}

	bodyContent := pages.SetPasswordContent(privateKey)
	partialViewHandler(bodyContent, c)
}

// autoUnlockPassword returns the password from the configured unlocker (env or
// file based) and whether one is configured. It lets the onboarding flow create
// the wallet with the same password the daemon uses to auto-unlock.
func (s *service) autoUnlockPassword(ctx context.Context) (string, bool) {
	if s.unlocker == nil {
		return "", false
	}
	password, err := s.unlocker.GetPassword(ctx)
	if err != nil {
		log.WithError(err).Warn("failed to get password from unlocker")
		return "", false
	}
	if password == "" {
		// The file-based unlocker can return an empty password when its file is
		// empty or whitespace-only. Treat that as "no unlocker password" so the
		// password prompt + validation still apply instead of creating the
		// wallet with an empty password.
		log.Warn("unlocker returned an empty password; falling back to the standard password flow")
		return "", false
	}
	return password, true
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
		active, *settings, s.svc.IsLocked(c), s.svc.BuildInfo.Version,
	)
	s.pageViewHandler(bodyContent, c)
}

func (s *service) getTransfer(
	c *gin.Context, transfer types.Transfer, explorerUrl, arkExplorerUrl string,
) templ.Component {
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
				boardingTimelock := arklib.RelativeLocktime{Type: data.BoardingExitDelay.Type, Value: data.BoardingExitDelay.Value}
				closeToBoardingSettlement := time.Now().Add(time.Duration(boardingTimelock.Seconds()) * time.Second)
				nextSettlement = closeToBoardingSettlement
			}
		}

		if nextSettlementStr != "unknown" {
			nextSettlementStr = prettyUnixTimestamp(nextSettlement.Unix())
		}

		return pages.TransferTxPendingContent(transfer, explorerUrl, arkExplorerUrl, nextSettlementStr)
	} else {
		return pages.TransferTxBodyContent(transfer, explorerUrl, arkExplorerUrl)
	}
}

// TODO: Ensure the correct Content are being displayed
func (s *service) getSwap(swap types.Swap) templ.Component {
	switch swap.Status {
	case "pending":
		return pages.SwapTxPendingContent(swap)
	case "refunding":
		return pages.SwapTxRefundingContent(swap)
	case "failure":
		return pages.SwapTxFailureContent(swap)
	default:
		return pages.SwapContent(swap)
	}
}

func (s *service) getPayment(c *gin.Context, payment types.Payment) templ.Component {
	switch payment.Status {
	case "pending":
		return pages.PaymentTxPendingContent(payment)
	case "refunding":
		return pages.PaymentTxRefundingContent(payment)
	case "failure":
		return pages.PaymentTxFailureContent(payment)
	default:
		return pages.PaymentContent(payment)
	}
}

func (s *service) getChainSwap(cs types.ChainSwap) templ.Component {
	return pages.ChainSwapContent(cs)
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
	arkExplorerUrl := getArkExplorerUrl(data.Network.Name)

	txid := c.Param("txid")
	var tx types.Transaction
	for _, transaction := range txHistory {
		if transaction.Id == txid {
			tx = transaction
			break
		}

		if transaction.Kind == "swap" && transaction.Swap != nil {
			swapTx := transaction.Swap

			if swapTx.VHTLCTransfer != nil && swapTx.VHTLCTransfer.Txid == txid {
				bodyContent := s.getTransfer(c, *swapTx.VHTLCTransfer, explorerUrl, arkExplorerUrl)
				s.pageViewHandler(bodyContent, c)
				return
			}

			if swapTx.RedeemTransfer != nil && swapTx.RedeemTransfer.Txid == txid {
				bodyContent := s.getTransfer(c, *swapTx.RedeemTransfer, explorerUrl, arkExplorerUrl)
				s.pageViewHandler(bodyContent, c)
				return
			}
		}

		if transaction.Kind == "payment" && transaction.Payment != nil {
			paymentTx := transaction.Payment

			if paymentTx.PaymentTransfer != nil && paymentTx.PaymentTransfer.Txid == txid {
				bodyContent := s.getTransfer(c, *paymentTx.PaymentTransfer, explorerUrl, arkExplorerUrl)
				s.pageViewHandler(bodyContent, c)
				return
			}

			if paymentTx.ReclaimTransfer != nil && paymentTx.ReclaimTransfer.Txid == txid {
				bodyContent := s.getTransfer(c, *paymentTx.ReclaimTransfer, explorerUrl, arkExplorerUrl)
				s.pageViewHandler(bodyContent, c)
				return
			}

		}
	}

	var bodyContent templ.Component
	if len(tx.Id) == 0 {
		bodyContent = pages.TxNotFoundContent()
	} else if tx.Kind == "transfer" {
		bodyContent = s.getTransfer(c, *tx.Transfer, explorerUrl, arkExplorerUrl)
	} else if tx.Kind == "payment" {
		bodyContent = s.getPayment(c, *tx.Payment)
	} else if tx.Kind == "chainswap" {
		bodyContent = s.getChainSwap(*tx.ChainSwap)
	} else {
		bodyContent = s.getSwap(*tx.Swap)
	}
	s.pageViewHandler(bodyContent, c)
}

func (s *service) getTxs(c *gin.Context) {
	if s.redirectedBecauseWalletIsLocked(c) {
		return
	}

	isSynced, err := s.svc.IsSynced()
	// nolint
	if err != nil {
		// TODO: Render error
	}
	if !isSynced {
		// TODO: Render placeholder
		bodyContent := components.HistoryBodyContent(nil, "0", false)
		partialViewHandler(bodyContent, c)
		return
	}

	lastId := c.Param("lastId")
	loadMore := false
	txsPerPage := 10

	txHistory, err := s.getTxHistory(c)
	if err != nil {
		log.WithError(err).Warn("failed to get tx history")
	}

	// TODO: (JOSHUA) Inefficient, please optimize later
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

func (s *service) getTxHistory(c *gin.Context) (transactions []types.Transaction, err error) {
	// get tx history from Server
	transferTxns, err := s.svc.GetTransactionHistory(c)
	if err != nil {
		return nil, err
	}

	// Get Swap Transaction
	swapTxs, err := s.svc.GetSwapHistory(c)
	if err != nil {
		return nil, err
	}

	payments, regularSwaps := Partition(swapTxs, func(s domain.Swap) bool {
		return s.Type == domain.SwapPayment
	})

	history := make([]types.Transaction, 0, len(transferTxns)+len(swapTxs))

	// add swaps to history
	for _, swap := range regularSwaps {
		transformedSwap := toSwap(swap)

		if transformedSwap.Kind == "submarine" {
			updatedTransfers, sendTransfer, ok := RemoveFind(
				transferTxns, func(t clientTypes.Transaction) bool {
					return swap.FundingTxId != "" && swap.FundingTxId == t.ArkTxid
				},
			)

			if ok {
				transferTxns = updatedTransfers
				modifiedSendTransfer := toTransfer(sendTransfer)
				transformedSwap.VHTLCTransfer = &modifiedSendTransfer
			}

			updatedTransfers, receiveTransfer, ok := RemoveFind(
				transferTxns, func(t clientTypes.Transaction) bool {
					return swap.RedeemTxId != "" && swap.RedeemTxId == t.ArkTxid
				},
			)
			if ok {
				transferTxns = updatedTransfers
				modifiedReceiveTransfer := toTransfer(receiveTransfer)
				transformedSwap.RedeemTransfer = &modifiedReceiveTransfer
			}

		} else {
			updatedTransfers, receiveTransfer, ok := RemoveFind(
				transferTxns, func(t clientTypes.Transaction) bool {
					return swap.RedeemTxId != "" && swap.RedeemTxId == t.ArkTxid
				},
			)

			if ok {
				transferTxns = updatedTransfers
				modifiedReceiveTransfer := toTransfer(receiveTransfer)
				transformedSwap.RedeemTransfer = &modifiedReceiveTransfer
			}
		}

		swapTxn := types.Transaction{
			Kind:        "swap",
			Swap:        &transformedSwap,
			Id:          swap.Id,
			DateCreated: swap.Timestamp,
		}

		history = append(history, swapTxn)

	}

	for _, p := range payments {
		transformedPayment := toPayment(p)

		if transformedPayment.Kind == "send" {
			updatedTransfers, sendTransfer, ok := RemoveFind(
				transferTxns, func(t clientTypes.Transaction) bool {
					return p.FundingTxId != "" && p.FundingTxId == t.ArkTxid
				},
			)

			if ok {
				transferTxns = updatedTransfers
				modifiedSendTransfer := toTransfer(sendTransfer)
				transformedPayment.PaymentTransfer = &modifiedSendTransfer
			}

			updatedTransfers, receiveTransfer, ok := RemoveFind(
				transferTxns, func(t clientTypes.Transaction) bool {
					return p.RedeemTxId != "" && p.RedeemTxId == t.ArkTxid
				},
			)

			if ok {
				transferTxns = updatedTransfers
				modifiedReceiveTransfer := toTransfer(receiveTransfer)
				transformedPayment.ReclaimTransfer = &modifiedReceiveTransfer
			}
		} else {
			updatedTransfers, receiveTransfer, ok := RemoveFind(
				transferTxns, func(t clientTypes.Transaction) bool {
					return p.RedeemTxId != "" && p.RedeemTxId == t.ArkTxid
				},
			)

			if ok {
				transferTxns = updatedTransfers
				modifiedReceiveTransfer := toTransfer(receiveTransfer)
				transformedPayment.PaymentTransfer = &modifiedReceiveTransfer
			}
		}
		paymentTxn := types.Transaction{
			Kind:        "payment",
			Payment:     &transformedPayment,
			Id:          p.Id,
			DateCreated: p.Timestamp,
		}

		history = append(history, paymentTxn)
	}

	// add chain swaps (on-chain BTC<->ARK) to history
	chainSwaps, err := s.svc.ListChainSwaps(c, nil)
	if err != nil {
		return nil, err
	}
	for _, cs := range chainSwaps {
		transformedChainSwap := toChainSwap(cs)
		history = append(history, types.Transaction{
			Kind:        "chainswap",
			ChainSwap:   &transformedChainSwap,
			Id:          cs.Id,
			DateCreated: cs.CreatedAt,
		})
	}

	for _, tx := range transferTxns {

		modifiedTransfer := toTransfer(tx)

		transaction := types.Transaction{
			Kind:        "transfer",
			Transfer:    &modifiedTransfer,
			Id:          modifiedTransfer.Txid,
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
		transfer := toTransfer(transaction)
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

	partial := components.Transfer(tx, getExplorerUrl(data.Network.Name), getArkExplorerUrl(data.Network.Name))
	partialViewHandler(partial, c)
}

// refundTx initiates a unilateral refund of a pending submarine swap / payment
// from its detail page (the "Initiate Refund" button). It uses the same call the
// startup auto-refund uses — RefundVHTLC -> RefundSwap(submarine, withoutReceiver,
// nil outpoint) — and re-renders the detail in the "refunding" state.
func (s *service) refundTx(c *gin.Context) {
	if s.redirectedBecauseWalletIsLocked(c) {
		return
	}

	txid := c.Param("txid")

	swaps, err := s.svc.GetSwapHistory(c)
	if err != nil {
		toast := components.Toast(err.Error(), true)
		toastHandler(toast, c)
		return
	}

	var target *domain.Swap
	for i := range swaps {
		if swaps[i].Id == txid {
			target = &swaps[i]
			break
		}
	}
	if target == nil {
		toast := components.Toast("swap not found", true)
		toastHandler(toast, c)
		return
	}

	if _, err := s.svc.RefundVHTLC(c, target.Id, target.Vhtlc.Id, false, nil); err != nil {
		toast := components.Toast(err.Error(), true)
		toastHandler(toast, c)
		return
	}

	if target.Type == domain.SwapPayment {
		payment := toPayment(*target)
		payment.Status = "refunding"
		partialViewHandler(pages.PaymentTxRefundingContent(payment), c)
		return
	}

	swap := toSwap(*target)
	swap.Status = "refunding"
	partialViewHandler(pages.SwapTxRefundingContent(swap), c)
}

// refundChainSwapTx initiates a refund of a chain swap (BTC<->ARK) from its
// detail page and re-renders the result. RefundChainSwap handles both ARK->BTC
// (cooperative) and BTC->ARK (unilateral) refunds.
func (s *service) refundChainSwapTx(c *gin.Context) {
	if s.redirectedBecauseWalletIsLocked(c) {
		return
	}

	id := c.Param("id")

	if err := s.svc.RefundChainSwap(c, id); err != nil {
		toast := components.Toast(err.Error(), true)
		toastHandler(toast, c)
		return
	}

	chainSwaps, err := s.svc.ListChainSwaps(c, []string{id})
	if err != nil || len(chainSwaps) == 0 {
		toast := components.Toast("refund initiated", false)
		toastHandler(toast, c)
		return
	}

	cs := toChainSwap(chainSwaps[0])
	partialViewHandler(pages.ChainSwapContent(cs), c)
}

func (s *service) getHero(c *gin.Context) {
	if s.redirectedBecauseWalletIsLocked(c) {
		return
	}

	isSynced, err := s.svc.IsSynced()
	if err != nil {
		// TODO: Render error
		partialContent := components.Hero("ERROR", false, s.delegateEnabled)
		partialViewHandler(partialContent, c)
		return
	}
	if !isSynced {
		// TODO: Render placeholder
		partialContent := components.Hero("PLACEHOLDER", false, s.delegateEnabled)
		partialViewHandler(partialContent, c)
		return
	}

	var isOnline bool

	spendableBalance, err := s.getSpendableBalance(c)
	if err == nil {
		isOnline = true
	} else {
		log.WithError(err).Warn("failed to get spendable balance")
	}

	partialContent := components.Hero(spendableBalance, isOnline, s.delegateEnabled)
	partialViewHandler(partialContent, c)
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
			if swap.RedeemTxId == "" && swap.FundingTxId != "" {
				return "refunding"
			}
			return "failure"
		}
	}

	expiry := prettyUnixTimestamp(0)
	_, _, inv, err := utils.DecodeInvoice(swap.Invoice)
	if err == nil {
		at := swap.Timestamp + int64(inv.Expiry)
		expiry = prettyUnixTimestamp(int64(at))
	}

	var refundLocktime types.LockTime

	refundLT := swap.Vhtlc.RefundLocktime
	if refundLT.IsSeconds() {
		refundLocktime = types.LockTime{
			Timelock:  prettyUnixTimestamp(int64(refundLT)),
			IsSeconds: true,
		}
	} else {
		refundLocktime = types.LockTime{
			Timelock:  strconv.FormatUint(uint64(refundLT), 10),
			IsSeconds: false,
		}
	}

	return types.Swap{
		Amount: strconv.FormatUint(swap.Amount, 10),
		Date:   prettyDay(swap.Timestamp),
		Hour:   prettyHour(swap.Timestamp),
		Id:     swap.Id,
		Kind:   selectSwapType(swap),
		Status: selectSwapStatus(swap),

		ExpiresAt:      expiry,
		RefundLockTime: &refundLocktime,
	}
}

func toPayment(payment domain.Swap) types.Payment {
	selectPaymentType := func(swap domain.Swap) string {
		if swap.To == boltz.CurrencyBtc && swap.From == boltz.CurrencyArk {
			return "send"
		} else {
			return "receive"
		}
	}

	selectPaymentStatus := func(swap domain.Swap) string {
		switch swap.Status {
		case domain.SwapSuccess:
			return "success"
		case domain.SwapPending:
			return "pending"
		default:
			if swap.RedeemTxId == "" && swap.FundingTxId != "" {
				return "refunding"
			}
			return "failure"
		}
	}

	expiry := prettyUnixTimestamp(0)
	_, _, inv, err := utils.DecodeInvoice(payment.Invoice)
	if err == nil {
		at := payment.Timestamp + int64(inv.Expiry)
		expiry = prettyUnixTimestamp(int64(at))
	}

	var refundLocktime types.LockTime

	refundLT := payment.Vhtlc.RefundLocktime
	if refundLT.IsSeconds() {
		refundLocktime = types.LockTime{
			Timelock:  prettyUnixTimestamp(int64(refundLT)),
			IsSeconds: true,
		}
	} else {
		refundLocktime = types.LockTime{
			Timelock:  strconv.FormatUint(uint64(refundLT), 10),
			IsSeconds: false,
		}
	}

	return types.Payment{
		Amount:         strconv.FormatUint(payment.Amount, 10),
		Date:           prettyDay(payment.Timestamp),
		Hour:           prettyHour(payment.Timestamp),
		Id:             payment.Id,
		Kind:           selectPaymentType(payment),
		Status:         selectPaymentStatus(payment),
		RefundLockTime: &refundLocktime,
		ExpiresAt:      expiry,
	}

}

func toChainSwap(cs domain.ChainSwap) types.ChainSwap {
	kind := "btc_to_ark"
	if cs.From == boltz.CurrencyArk && cs.To == boltz.CurrencyBtc {
		kind = "ark_to_btc"
	}

	status := "pending"
	switch {
	case cs.Status == domain.ChainSwapClaimed:
		status = "success"
	case cs.Status == domain.ChainSwapRefunded || cs.Status == domain.ChainSwapRefundedUnilaterally:
		status = "failure"
	case domain.ShouldRefundChainSwapStatus(cs.Status):
		status = "refundable"
	}

	return types.ChainSwap{
		Amount:               strconv.FormatUint(cs.Amount, 10),
		Date:                 prettyDay(cs.CreatedAt),
		Hour:                 prettyHour(cs.CreatedAt),
		Id:                   cs.Id,
		Kind:                 kind,
		Status:               status,
		UserBtcLockupAddress: cs.UserBtcLockupAddress,
	}
}

func toTransfer(tx clientTypes.Transaction) types.Transfer {
	// amount
	amount := strconv.FormatUint(tx.Amount, 10)
	if tx.Type == clientTypes.TxSent {
		amount = "-" + amount
	}
	// date of creation
	dateCreated := tx.CreatedAt.Unix()
	// status of tx
	status := "success"
	if tx.BoardingTxid != "" && tx.SettledBy == "" {
		status = "pending"
	}
	if tx.CreatedAt.IsZero() {
		status = "unconfirmed"
		dateCreated = 0
	}
	// get one txid to identify tx
	explorable := tx.ArkTxid == ""

	return types.Transfer{
		Amount:     amount,
		CreatedAt:  prettyUnixTimestamp(dateCreated),
		Day:        prettyDay(dateCreated),
		Explorable: explorable,
		Hour:       prettyHour(dateCreated),
		Kind:       strings.ToLower(string(tx.Type)),
		Txid:       tx.TransactionKey.String(),
		Status:     status,
		UnixDate:   dateCreated,
	}
}

func (s *service) delegate(c *gin.Context) {
	if s.redirectedBecauseWalletIsLocked(c) {
		return
	}
	bodyContent := pages.DelegateBodyContent()
	s.pageViewHandler(bodyContent, c)
}

func (s *service) delegateActive(c *gin.Context) {
	if s.redirectedBecauseWalletIsLocked(c) {
		return
	}
	active := c.Param("active")
	bodyContent := pages.DelegatePartialContent(active)
	partialViewHandler(bodyContent, c)
}

func (s *service) getDelegateTasks(c *gin.Context) {
	if s.redirectedBecauseWalletIsLocked(c) {
		return
	}

	statusStr := c.Param("status")
	offsetStr := c.Param("offset")

	status, err := domain.DelegateTaskStatusFromString(statusStr)
	if err != nil {
		toast := components.Toast("Invalid status", true)
		toastHandler(toast, c)
		return
	}

	offset, err := strconv.Atoi(offsetStr)
	if err != nil {
		offset = 0
	}

	limit := 20
	tasks, err := s.svc.GetDelegateTasks(c, status, limit, offset)
	if err != nil {
		toast := components.Toast("Unable to get delegate tasks", true)
		toastHandler(toast, c)
		return
	}

	parsedTasks := make([]types.DelegateTask, len(tasks))
	for i, task := range tasks {
		parsedTasks[i] = toDelegateTask(task)
	}

	loadMore := len(tasks) == limit
	nextOffset := offset + len(tasks)

	if len(parsedTasks) == 0 && offset > 0 {
		bodyContent := templ.Component(nil)
		partialViewHandler(bodyContent, c)
		return
	}

	bodyContent := pages.DelegateTasksListContent(parsedTasks, statusStr, nextOffset, loadMore)
	partialViewHandler(bodyContent, c)
}

func (s *service) getDelegateTaskDetail(c *gin.Context) {
	if s.redirectedBecauseWalletIsLocked(c) {
		return
	}

	taskID := c.Param("id")
	if taskID == "" {
		toast := components.Toast("Task ID is required", true)
		toastHandler(toast, c)
		return
	}

	task, err := s.svc.GetDelegateTaskByID(c, taskID)
	if err != nil {
		toast := components.Toast("Unable to get task details", true)
		toastHandler(toast, c)
		return
	}

	if task == nil {
		toast := components.Toast("Task not found", true)
		toastHandler(toast, c)
		return
	}

	parsedTask := toDelegateTask(*task)
	modal := modals.DelegateTaskDetail(parsedTask)
	modalHandler(modal, c)
}

func toDelegateTask(task domain.DelegateTask) types.DelegateTask {
	unixTime := task.ScheduledAt.Unix()
	result := types.DelegateTask{
		ID:                task.ID,
		Status:            task.Status.String(),
		Fee:               strconv.FormatUint(task.Fee, 10),
		ScheduledAt:       prettyUnixTimestamp(unixTime),
		ScheduledAtUnix:   unixTime,
		ScheduledDate:     prettyDay(unixTime),  // Keep for backward compatibility
		ScheduledHour:     prettyHour(unixTime), // Keep for backward compatibility
		FailReason:        task.FailReason,
		CommitmentTxid:    task.CommitmentTxid,
		DelegatePublicKey: task.DelegatePublicKey,
	}

	// Convert Intent
	if task.Intent.Txid != "" || task.Intent.Message != "" || task.Intent.Proof != "" || len(task.Intent.Inputs) > 0 {
		intent := &types.DelegateTaskIntent{
			Txid:    task.Intent.Txid,
			Message: task.Intent.Message,
			Proof:   task.Intent.Proof,
			Inputs:  make([]string, len(task.Intent.Inputs)),
		}
		for i, input := range task.Intent.Inputs {
			intent.Inputs[i] = fmt.Sprintf("%s:%d", input.Hash.String(), input.Index)
		}
		result.Intent = intent
	}

	// Convert ForfeitTxs
	if len(task.ForfeitTxs) > 0 {
		result.Forfeits = make([]types.DelegateTaskForfeit, 0, len(task.ForfeitTxs))
		for outpoint, txid := range task.ForfeitTxs {
			result.Forfeits = append(result.Forfeits, types.DelegateTaskForfeit{
				Outpoint: fmt.Sprintf("%s:%d", outpoint.Hash.String(), outpoint.Index),
				Txid:     txid,
			})
		}
	}

	return result
}
