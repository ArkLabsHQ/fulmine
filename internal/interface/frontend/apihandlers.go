package main

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/angelofallars/htmx-go"

	"github.com/gin-gonic/gin"
)

func apiHandler(c *gin.Context, r []byte) {
	if !htmx.IsHTMX(c.Request) {
		c.AbortWithError(http.StatusBadRequest, errors.New("non-htmx request"))
		return
	}
	c.Writer.Write(r)
	htmx.NewResponse().Write(c.Writer)
}

func inboundAPIHandler(c *gin.Context) {
	apiHandler(c, []byte("<p>inbound result here</p>"))
}

func outboundAPIHandler(c *gin.Context) {
	apiHandler(c, []byte("<p>outbound result here</p>"))
}

func receiveAPIHandler(c *gin.Context) {
	amount := c.PostForm("amount")
	message := fmt.Sprintf("Hello, amount is %s.", amount)
	apiHandler(c, []byte(message))
}

func sendAPIHandler(c *gin.Context) {
	address := c.PostForm("address")
	amount := c.PostForm("amount")
	message := fmt.Sprintf("Hello, address is %s and amount %s.", address, amount)
	apiHandler(c, []byte(message))
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
