package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/a-h/templ"
	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/render"

	"github.com/ArkLabsHQ/ark-wallet/handlers"

	gowebly "github.com/gowebly/helpers"
)

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

// runServer runs a new HTTP server with the loaded environment variables.
func runServer() error {
	// Validate environment variables.
	port, err := strconv.Atoi(gowebly.Getenv("BACKEND_PORT", "7000"))
	if err != nil {
		return err
	}

	// Create a new Fiber server.
	router := gin.Default()

	// Define HTML renderer for template engine.
	router.HTMLRender = &TemplRender{}

	// Handle static files.
	router.Static("/static", "./static")

	// Handle index page view.
	router.GET("/", handlers.Index)
	router.GET("/import", handlers.ImportWallet)
	router.GET("/locked", handlers.Locked)
	router.GET("/new", handlers.NewWallet)
	router.GET("/password", handlers.Password)
	router.GET("/send", handlers.Send)
	router.GET("/settings/:active", handlers.Settings)
	router.GET("/swap", handlers.Swap)
	router.GET("/receive", handlers.Receive)
	router.GET("/tx/:txid", handlers.Tx)
	router.GET("/welcome", handlers.Welcome)

	router.GET("/modal/info", handlers.InfoModal)

	router.POST("/password", handlers.SetPassword)
	router.POST("/send/preview", handlers.SendPreview)
	router.POST("/send/confirm", handlers.SendConfirm)
	router.POST("/receive/preview", handlers.ReceivePreview)
	router.POST("/unlock", handlers.Unlock)

	router.POST("/api/settings", handlers.SettingsApiPost)

	// Create a new server instance with options from environment variables.
	// For more information, see https://blog.cloudflare.com/the-complete-guide-to-golang-net-http-timeouts/
	server := &http.Server{
		Addr:         fmt.Sprintf(":%d", port),
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		Handler:      router,
	}

	// Send log message.
	slog.Info("Starting server...", "port", port)

	return server.ListenAndServe()
}
