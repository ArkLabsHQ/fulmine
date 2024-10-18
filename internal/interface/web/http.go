package web

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/ArkLabsHQ/ark-node/internal/interface/web/templates"
	"github.com/a-h/templ"
	"github.com/angelofallars/htmx-go"
	"github.com/gin-gonic/gin"
)

func modalHandler(t templ.Component, c *gin.Context) {
	if !htmx.IsHTMX(c.Request) {
		// nolint:all
		c.AbortWithError(http.StatusBadRequest, fmt.Errorf("non-htmx request"))
		return
	}
	// nolint:all
	htmx.NewResponse().RenderTempl(c, c.Writer, t)
}

func (s *service) pageViewHandler(bodyContent templ.Component, c *gin.Context) {
	settings, err := s.svc.GetSettings(c)
	if err != nil {
		// nolint:all
		c.AbortWithError(http.StatusInternalServerError, err)
		return
	}
	indexTemplate := templates.Layout(bodyContent, *settings)
	if err := htmx.NewResponse().RenderTempl(c.Request.Context(), c.Writer, indexTemplate); err != nil {
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}
}

func partialViewHandler(bodyContent templ.Component, c *gin.Context) {
	if err := htmx.NewResponse().RenderTempl(c.Request.Context(), c.Writer, bodyContent); err != nil {
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}
}

func redirect(path string, c *gin.Context) {
	c.Header("HX-Redirect", path)
	c.Status(303)
}

func reload(c *gin.Context) {
	c.Header("HX-Refresh", "true")
}

func toastHandler(t templ.Component, c *gin.Context) {
	if !htmx.IsHTMX(c.Request) {
		// nolint:all
		c.AbortWithError(http.StatusBadRequest, errors.New("non-htmx request"))
		return
	}
	htmx.NewResponse().
		Retarget("#toast").
		AddTrigger(htmx.Trigger("toast")).
		// nolint:all
		RenderTempl(c, c.Writer, t)
}
