package httpapi

import (
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
		renderV1BinaryError(c, status)
		return
	}
	maxAge := min(300, max(0, int(time.Until(expires).Seconds())))
	writeRenderedImageBytes(c, image, fmt.Sprintf("public, max-age=%d", maxAge))
}
