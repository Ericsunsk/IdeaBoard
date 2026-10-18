package http

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"ideaboard/api/internal/media/derivatives/infrastructure/imgproxy"
	"ideaboard/api/internal/media/derivatives/infrastructure/preview"

	"github.com/gin-gonic/gin"
)

func registerPublicMediaRoutes(r *gin.Engine, previews *preview.Service) {
	if previews == nil {
		return
	}
	r.GET("/api/media/previews/:token/:tier", func(c *gin.Context) {
		image, expires, err := previews.RenderCapability(c.Request.Context(), c.Param("token"), c.Param("tier"))
		writePublicPreview(c, image, expires, err)
	})
}

func writePublicPreview(c *gin.Context, image imgproxy.Image, expires time.Time, err error) {
	if err != nil {
		status := http.StatusServiceUnavailable
		switch {
		case errors.Is(err, preview.ErrCapability), errors.Is(err, imgproxy.ErrNotFound):
			status = http.StatusNotFound
		case errors.Is(err, imgproxy.ErrSource):
			status = http.StatusBadRequest
		}
		RenderV1BinaryError(c, status)
		return
	}
	maxAge := min(300, max(0, int(time.Until(expires).Seconds())))
	WriteRenderedImageBytes(c, image, fmt.Sprintf("public, max-age=%d", maxAge))
}

const statusClientClosedRequest = 499

// RenderV1BinaryError answers binary endpoints with a plain non-JSON
// status: content/derivative endpoints never carry the JSON envelope.
// (Moved to the kernel during the plane split: the kernel public preview
// route and the v1 avatar/branding/library/media bytes routes share it.)
func RenderV1BinaryError(c *gin.Context, status int) {
	c.Header("Cache-Control", "no-store")
	c.String(status, http.StatusText(status))
	c.Abort()
}

// WriteRenderedImage renders an imgproxy image or its binary error.
func WriteRenderedImage(c *gin.Context, image imgproxy.Image, err error, cacheControl string) {
	if err != nil {
		if errors.Is(err, context.Canceled) && errors.Is(c.Request.Context().Err(), context.Canceled) {
			c.Status(statusClientClosedRequest)
			c.Abort()
			return
		}
		status := http.StatusServiceUnavailable
		switch {
		case errors.Is(err, imgproxy.ErrSource):
			status = http.StatusConflict
		case errors.Is(err, imgproxy.ErrNotFound):
			status = http.StatusNotFound
		}
		RenderV1BinaryError(c, status)
		return
	}
	WriteRenderedImageBytes(c, image, cacheControl)
}

// WriteRenderedImageBytes writes raw image bytes with cache headers.
func WriteRenderedImageBytes(c *gin.Context, image imgproxy.Image, cacheControl string) {
	c.Header("Cache-Control", cacheControl)
	c.Header("Content-Disposition", "inline")
	c.Header("X-Content-Type-Options", "nosniff")
	c.Data(http.StatusOK, image.MIMEType, image.Data)
}
