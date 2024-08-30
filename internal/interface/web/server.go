package web

import (
	"context"
	"embed"
	"io/fs"
	"net/http"

	"github.com/a-h/templ"
	arksdk "github.com/ark-network/ark/pkg/client-sdk"
	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/render"
	"github.com/lightninglabs/lndclient"

	"github.com/ArkLabsHQ/ark-node/internal/interface/web/handlers"
)

var TEMPORARY lndclient.LightningClient // TODO

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
}

func NewService() *service {
	// Create a new Fiber server.
	router := gin.Default()

	// global variable with arkClient
	var arkClient arksdk.ArkClient

	// Middleware to set a variable in the context
	router.Use(func(c *gin.Context) {
		// first access, arkClient will be nil
		if arkClient == nil {
			arkClient, _ = handlers.LoadArkClient()
		} else {
			// if meanwhile /storage/state.json was removed,
			// loadArkClient will return nil and we want to
			// reset the main variable arkClient
			if ac, _ := handlers.LoadArkClient(); ac == nil {
				arkClient = nil
			}
		}
		c.Set("arkClient", arkClient)
		c.Next() // Call the next handler
	})

	// Define HTML renderer for template engine.
	router.HTMLRender = &TemplRender{}
	staticFS, _ := fs.Sub(static, "static")

	svc := &service{router}

	// Handle static files.
	// svc.Static("/static", "./static")
	svc.StaticFS("/static", http.FS(staticFS))

	// Handle index page view.
	svc.GET("/", handlers.Index)
	svc.GET("/done", handlers.Done)
	svc.GET("/forgot", handlers.Forgot)
	svc.GET("/import", handlers.ImportWallet)
	svc.GET("/lock", handlers.Lock)
	svc.GET("/modal/feeinfo", handlers.FeeInfoModal)
	svc.GET("/new", handlers.NewWallet)
	svc.GET("/receive", handlers.ReceiveQrCode)
	svc.GET("/receive/edit", handlers.ReceiveEdit)
	svc.GET("/send", handlers.Send)
	svc.GET("/settings/:active", handlers.Settings)
	svc.GET("/swap", handlers.Swap)
	svc.GET("/swap/:active", handlers.SwapActive)
	svc.GET("/tx/:txid", handlers.Tx)
	svc.GET("/unlock", handlers.Unlock)
	svc.GET("/welcome", handlers.Welcome)

	svc.POST("/initialize", handlers.Initialize)
	svc.POST("/mnemonic", handlers.SetMnemonic)
	svc.POST("/password", handlers.SetPassword)

	svc.POST("/receive/preview", handlers.ReceiveQrCode)
	svc.POST("/receive/success", handlers.ReceiveSuccess)
	svc.POST("/send/preview", handlers.SendPreview)
	svc.POST("/send/confirm", handlers.SendConfirm)
	svc.POST("/swap/preview", handlers.SwapPreview)
	svc.POST("/swap/confirm", handlers.SwapConfirm)

	svc.POST("/api/lock", handlers.LockApiPost)
	svc.POST("/api/settings", handlers.SettingsApiPost)
	svc.POST("/api/node/connect", handlers.NodeConnectApiPost)
	svc.POST("/api/node/disconnect", handlers.NodeDisconnectApiPost)
	svc.POST("/api/mnemonic/validate", handlers.ValidateMnemonicApiPost)
	svc.POST("/api/unlock", handlers.UnlockApiPost)

	svc.GET("/api/balance", handlers.BalanceApiGet)

	return svc
}
