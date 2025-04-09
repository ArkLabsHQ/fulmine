package web

import (
	"github.com/getsentry/sentry-go"
	"github.com/gin-gonic/gin"
)

// SentryMiddleware captures errors that happened during request handling
// and reports them to Sentry
func SentryMiddleware(enabled bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !enabled {
			c.Next()
			return
		}

		c.Next()

		if len(c.Errors) > 0 {
			for _, err := range c.Errors {
				sentry.WithScope(func(scope *sentry.Scope) {
					scope.SetTag("method", c.Request.Method)
					scope.SetTag("path", c.Request.URL.Path)
					scope.SetRequest(c.Request)
					sentry.CaptureException(err.Err)
				})
			}
		}
	}
}
