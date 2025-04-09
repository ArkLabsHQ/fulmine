package web

import (
	"fmt"
	"github.com/getsentry/sentry-go"
	"github.com/gin-gonic/gin"
	log "github.com/sirupsen/logrus"
	"net/http"
	"runtime/debug"
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

// RecoveryMiddleware catches any panics in HTTP handlers, logs them, and reports
// them to Sentry if enabled
func RecoveryMiddleware(sentryEnabled bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if r := recover(); r != nil {
				stackTrace := string(debug.Stack())
				var err error
				switch v := r.(type) {
				case error:
					err = v
				default:
					err = &panicError{value: r}
				}

				if sentryEnabled {
					sentry.WithScope(func(scope *sentry.Scope) {
						scope.SetTag("method", c.Request.Method)
						scope.SetTag("path", c.Request.URL.Path)
						scope.SetRequest(c.Request)
						scope.SetExtra("stack_trace", stackTrace)
						sentry.CaptureException(err)
					})

					log.WithFields(log.Fields{
						"method":      c.Request.Method,
						"path":        c.Request.URL.Path,
						"panic":       r,
						"stack_trace": stackTrace,
						"skip_sentry": true, // Signal that we already sent to Sentry
					}).Error("Panic recovered in HTTP handler")
				} else {
					log.WithFields(log.Fields{
						"method":      c.Request.Method,
						"path":        c.Request.URL.Path,
						"panic":       r,
						"stack_trace": stackTrace,
					}).Error("Panic recovered in HTTP handler")
				}

				c.AbortWithStatus(http.StatusInternalServerError)
			}
		}()

		c.Next()
	}
}

type panicError struct {
	value interface{}
}

func (pe *panicError) Error() string {
	return fmt.Sprintf("panic: %v", pe.value)
}
