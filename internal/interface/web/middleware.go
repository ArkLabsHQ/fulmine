package web

import (
	"net/http"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/gin-gonic/gin"
)

// SentryMiddleware captures errors that happened during request handling
// and reports them to Sentry
func SentryMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Start timer
		start := time.Now()

		// Process request
		c.Next()

		// Check for errors
		if len(c.Errors) > 0 {
			// Capture each error
			for _, err := range c.Errors {
				sentry.WithScope(func(scope *sentry.Scope) {
					// Add request info for context
					scope.SetTag("method", c.Request.Method)
					scope.SetTag("path", c.Request.URL.Path)
					scope.SetTag("status", http.StatusText(c.Writer.Status()))
					scope.SetTag("ip", c.ClientIP())
					scope.SetTag("user-agent", c.Request.UserAgent())

					// Add timing information
					scope.SetExtra("latency", time.Since(start).String())

					// Set HTTP context
					scope.SetRequest(c.Request)

					// Capture the error
					sentry.CaptureException(err.Err)
				})
			}
		}
	}
}
