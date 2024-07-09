package handlers

import (
	"errors"
	"net/http"

	"github.com/a-h/templ"
	"github.com/angelofallars/htmx-go"

	"github.com/gin-gonic/gin"

	"github.com/ArkLabsHQ/ark-wallet/templates/modals"
)

func modalHandler(t templ.Component, c *gin.Context) {
	if !htmx.IsHTMX(c.Request) {
		c.AbortWithError(http.StatusBadRequest, errors.New("non-htmx request"))
		return
	}
	htmx.NewResponse().RenderTempl(c, c.Writer, t)
}

func InfoModal(c *gin.Context) {
	info := modals.Info()
	modalHandler(info, c)
}
