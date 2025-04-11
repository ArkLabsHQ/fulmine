package web

import (
	"time"

	sentrygin "github.com/getsentry/sentry-go/gin"
	"github.com/gin-gonic/gin"
	log "github.com/sirupsen/logrus"
)

// SetupSentryMiddleware configures and adds the Sentry middleware to the Gin engine
// This middleware handles error reporting and panic recovery in a single package
func SetupSentryMiddleware(engine *gin.Engine, enabled bool) {
	if !enabled {
		engine.Use(gin.Recovery())
		return
	}

	engine.Use(sentrygin.New(sentrygin.Options{
		Repanic:         false, // Don't re-panic after recovery
		WaitForDelivery: false, // Don't wait for Sentry to deliver events (non-blocking)
		Timeout:         5 * time.Second,
	}))

	log.Info("Sentry error monitoring enabled")
}
