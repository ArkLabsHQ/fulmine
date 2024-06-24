package main

import (
	"errors"
	"net/http"

	"github.com/a-h/templ"
	"github.com/angelofallars/htmx-go"

	"github.com/ArkLabsHQ/ark-wallet/templates"
	"github.com/ArkLabsHQ/ark-wallet/templates/components"
	"github.com/ArkLabsHQ/ark-wallet/templates/pages"

	"github.com/gin-gonic/gin"
)

func viewHandler(bodyContent, tabsContent templ.Component, c *gin.Context) {
	indexTemplate := templates.Layout(bodyContent, tabsContent)
	if err := htmx.NewResponse().RenderTempl(c.Request.Context(), c.Writer, indexTemplate); err != nil {
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}
}

// indexViewHandler handles a view for the index page.
func homeViewHandler(c *gin.Context) {
	bodyContent := pages.HomeBodyContent()
	tabsContent := components.Tabs("home")
	viewHandler(bodyContent, tabsContent, c)
}

func infoViewHandler(c *gin.Context) {
	bodyContent := pages.InfoBodyContent()
	tabsContent := components.Tabs("info")
	viewHandler(bodyContent, tabsContent, c)
}

func keysViewHandler(c *gin.Context) {
	bodyContent := pages.KeysBodyContent()
	tabsContent := components.Tabs("keys")
	viewHandler(bodyContent, tabsContent, c)
}

func lightningViewHandler(c *gin.Context) {
	bodyContent := pages.LightningBodyContent()
	tabsContent := components.Tabs("lightning")
	viewHandler(bodyContent, tabsContent, c)
}

func walletViewHandler(c *gin.Context) {
	bodyContent := pages.WalletBodyContent()
	tabsContent := components.Tabs("wallet")
	viewHandler(bodyContent, tabsContent, c)
}

// showContentAPIHandler handles an API endpoint to show content.
func showContentAPIHandler(c *gin.Context) {
	// Check, if the current request has a 'HX-Request' header.
	// For more information, see https://htmx.org/docs/#request-headers
	if !htmx.IsHTMX(c.Request) {
		// If not, return HTTP 400 error.
		c.AbortWithError(http.StatusBadRequest, errors.New("non-htmx request"))
		return
	}

	// Write HTML content.
	c.Writer.Write([]byte("<p>🎉 Yes, <strong>htmx</strong> is ready to use! (<code>GET /api/hello-world</code>)</p>"))

	// Send htmx response.
	htmx.NewResponse().Write(c.Writer)
}
