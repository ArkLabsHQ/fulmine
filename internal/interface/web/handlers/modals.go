package handlers

import (
	"errors"
	"net/http"

	"github.com/a-h/templ"
	"github.com/angelofallars/htmx-go"

	"github.com/gin-gonic/gin"

	"github.com/ArkLabsHQ/ark-node/internal/interface/web/templates/modals"
)

func modalHandler(t templ.Component, c *gin.Context) {
	if !htmx.IsHTMX(c.Request) {
		//nolint:all
		c.AbortWithError(http.StatusBadRequest, errors.New("non-htmx request"))
		return
	}
	//nolint:all
	htmx.NewResponse().RenderTempl(c, c.Writer, t)
}

func FeeInfoModal(c *gin.Context) {
	info := modals.FeeInfo()
	modalHandler(info, c)
}
