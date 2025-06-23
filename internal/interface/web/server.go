package web

import (
	"context"
	"embed"
	"io/fs"
	"net/http"

	"github.com/ArkLabsHQ/fulmine/internal/core/application"
	"github.com/a-h/templ"
	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/render"
)

//go:embed static/*
var static embed.FS

func init() {
	gin.SetMode(gin.ReleaseMode)
}

// TemplRender implements the render.Render interface.
type TemplRender struct {
	Code int
	Data templ.Component
}

// Render implements the render.Render interface.
func (t TemplRender) Render(w http.ResponseWriter) error {
	t.WriteContentType(w)
	w.WriteHeader(t.Code)
	if t.Data != nil {
		return t.Data.Render(context.Background(), w)
	}
	return nil
}

// WriteContentType implements the render.Render interface.
func (t TemplRender) WriteContentType(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
}

// Instance implements the render.Render interface.
func (t *TemplRender) Instance(name string, data interface{}) render.Render {
	if templData, ok := data.(templ.Component); ok {
		return &TemplRender{
			Code: http.StatusOK,
			Data: templData,
		}
	}
	return nil
}

type service struct {
	*gin.Engine
	svc       *application.Service
	stopCh    chan struct{}
	arkServer string
}

func NewService(
	appSvc *application.Service,
	stopCh chan struct{},
	sentryEnabled bool,
	arkServer string,
) *service {
	// Create a new Fiber server.
	router := gin.Default()

	// Define HTML renderer for template engine.
	router.HTMLRender = &TemplRender{}
	staticFS, _ := fs.Sub(static, "static")

	svc := &service{
		Engine:    router,
		svc:       appSvc,
		stopCh:    stopCh,
		arkServer: arkServer,
	}

	// Configure Sentry for Gin (includes built-in panic recovery)
	setupMiddleware(svc.Engine, sentryEnabled)

	// Handle static files.
	// svc.Static("/static", "./static")
	svc.StaticFS("/static", http.FS(staticFS))

	// Handle index page view.
	svc.GET("/", svc.index)
	svc.GET("/backup", svc.backupInitial)
	svc.GET("/backup/secret", svc.backupSecret)
	svc.GET("/backup/ack", svc.backupAck)
	svc.GET("/backup/tab/:active", svc.backupTabActive)
	svc.GET("/done", svc.done)
	svc.GET("/events", svc.events)
	svc.GET("/hero", svc.getHero)
	svc.GET("/import", svc.importWalletPrivateKey)
	svc.GET("/lock", svc.lock)
	svc.GET("/new", svc.newWalletPrivateKey)
	svc.GET("/receive", svc.receiveQrCode)
	svc.GET("/receive/edit", svc.receiveEdit)
	svc.GET("/send", svc.send)
	svc.GET("/settings/:active", svc.settings)
	svc.GET("/swap", svc.swap)
	svc.GET("/swap/:active", svc.swapActive)
	svc.GET("/swapHistory/", svc.swapHistory)
	svc.GET("/swaps/:lastId", svc.getSwaps)
	svc.GET("/tx/:txid", svc.getTx)
	svc.GET("/txs/:lastId", svc.getTxs)
	svc.GET("/unlock", svc.unlock)
	svc.GET("/welcome", svc.welcome)

	svc.GET("/modal/feeinfo", svc.feeInfoModal)
	svc.GET("/modal/lnconnectinfo", svc.lnConnectInfoModal)
	svc.GET("/modal/reversibleinfo", svc.reversibleInfoModal)
	svc.GET("/modal/scanner/:id", svc.scannerModal)
	svc.GET("/modal/seedinfo", svc.seedInfoModal)

	svc.POST("/initialize", svc.initialize)
	svc.POST("/mnemonic", svc.setMnemonic)
	svc.POST("/password", svc.setPassword)
	svc.POST("/privatekey", svc.setPrivateKey)

	svc.POST("/note/confirm", svc.noteConfirm)
	svc.POST("/receive/preview", svc.receiveQrCode)
	svc.POST("/receive/success", svc.receiveSuccess)
	svc.POST("/send/preview", svc.sendPreview)
	svc.POST("/send/confirm", svc.sendConfirm)
	svc.POST("/swap/preview", svc.swapPreview)
	svc.POST("/swap/confirm", svc.swapConfirm)

	svc.POST("/helpers/bip21/validate", svc.validateBip21Api)
	svc.POST("/helpers/claim/:txid", svc.claimTx)
	svc.POST("/helpers/forgot", svc.forgotApi)
	svc.POST("/helpers/invoice/validate", svc.validateInvoiceApi)
	svc.POST("/helpers/lnurl/validate", svc.validateLnUrlApi)
	svc.POST("/helpers/node/connect", svc.connectLNDApi)
	svc.POST("/helpers/node/disconnect", svc.disconnectLNDApi)
	svc.POST("/helpers/note/validate", svc.validateNoteApi)
	svc.POST("/helpers/mnemonic/validate", svc.validateMnemonicApi)
	svc.POST("/helpers/privatekey/validate", svc.validatePrivateKeyApi)
	svc.POST("/helpers/settings", svc.updateSettingsApi)
	svc.POST("/helpers/unlock", svc.unlockApi)
	svc.POST("/helpers/url/validate", svc.validateUrlApi)

	svc.GET("/helpers/balance", svc.getBalanceApi)

	return svc
}
