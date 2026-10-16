package httpapi

import (
	"context"
	"errors"
	"net/http"

	"ideaboard/api/internal/media/derivatives/infrastructure/imgproxy"

	"github.com/gin-gonic/gin"
)

const statusClientClosedRequest = 499

func writeRenderedImage(c *gin.Context, image imgproxy.Image, err error, cacheControl string) {
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
		renderV1BinaryError(c, status)
		return
	}
	writeRenderedImageBytes(c, image, cacheControl)
}

func writeRenderedImageBytes(c *gin.Context, image imgproxy.Image, cacheControl string) {
	c.Header("Cache-Control", cacheControl)
	c.Header("Content-Disposition", "inline")
	c.Header("X-Content-Type-Options", "nosniff")
	c.Data(http.StatusOK, image.MIMEType, image.Data)
}
