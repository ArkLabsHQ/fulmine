package web

import (
	"context"
	"embed"
	"io/fs"
	"net/http"

	"github.com/a-h/templ"
	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/render"

	"github.com/ArkLabsHQ/ark-node/internal/core/application"
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
	svc *application.Service
}

func NewService(appSvc *application.Service) *service {
	// Create a new Fiber server.
	router := gin.Default()

	// Define HTML renderer for template engine.
	router.HTMLRender = &TemplRender{}
	staticFS, _ := fs.Sub(static, "static")

	svc := &service{router, appSvc}

	// Handle static files.
	// svc.Static("/static", "./static")
	svc.StaticFS("/static", http.FS(staticFS))

	// Handle index page view.
	svc.GET("/", svc.index)
	svc.GET("/done", svc.done)
	svc.GET("/forgot", svc.forgot)
	svc.GET("/import", svc.importWallet)
	svc.GET("/lock", svc.lock)
	svc.GET("/modal/feeinfo", svc.feeInfoModal)
	svc.GET("/new", svc.newWallet)
	svc.GET("/receive", svc.receiveQrCode)
	svc.GET("/receive/edit", svc.receiveEdit)
	svc.GET("/send", svc.send)
	svc.GET("/settings/:active", svc.settings)
	svc.GET("/swap", svc.swap)
	svc.GET("/swap/:active", svc.swapActive)
	svc.GET("/tx/:txid", svc.getTx)
	svc.GET("/unlock", svc.unlock)
	svc.GET("/welcome", svc.welcome)

	svc.POST("/initialize", svc.initialize)
	svc.POST("/mnemonic", svc.setMnemonic)
	svc.POST("/password", svc.setPassword)

	svc.POST("/receive/preview", svc.receiveQrCode)
	svc.POST("/receive/success", svc.receiveSuccess)
	svc.POST("/send/preview", svc.sendPreview)
	svc.POST("/send/confirm", svc.sendConfirm)
	svc.POST("/swap/preview", svc.swapPreview)
	svc.POST("/swap/confirm", svc.swapConfirm)

	svc.POST("/api/claim", svc.claimApi)
	svc.POST("/api/lock", svc.lockApi)
	svc.POST("/api/settings", svc.updateSettingsApi)
	svc.POST("/api/node/connect", svc.connectNodeApi)
	svc.POST("/api/node/disconnect", svc.disconnectNodeApi)
	svc.POST("/api/mnemonic/validate", svc.validateMnemonicApi)
	svc.POST("/api/url/validate", svc.validateUrlApi)
	svc.POST("/api/unlock", svc.unlockApi)

	svc.GET("/api/balance", svc.getBalanceApi)

	return svc
}
