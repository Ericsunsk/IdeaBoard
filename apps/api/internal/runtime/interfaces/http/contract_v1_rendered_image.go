package httpapi

import (
	"errors"
	"net/http"

	"ideaboard/api/internal/media/derivatives/infrastructure/imgproxy"

	"github.com/gin-gonic/gin"
)

func writeRenderedImage(c *gin.Context, image imgproxy.Image, err error, cacheControl string) {
	if err != nil {
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
	c.Header("Cache-Control", cacheControl)
	c.Header("Content-Disposition", "inline")
	c.Header("X-Content-Type-Options", "nosniff")
	c.Data(http.StatusOK, image.MIMEType, image.Data)
}
