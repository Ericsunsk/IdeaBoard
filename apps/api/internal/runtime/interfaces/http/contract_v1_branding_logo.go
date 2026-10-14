package httpapi

import (
	"net/http"
	"strings"

	storagedomain "ideaboard/api/internal/media/storage/domain"

	"github.com/gin-gonic/gin"
)

// Branding responses revalidate against the active storage profile.
const v1PublicBrandingLogoCacheControl = "no-store"

// publicBrandingLogo renders the requested canonical site asset, including unsaved drafts.
func (h *contractV1Handler) publicBrandingLogo(c *gin.Context) {
	key := c.Query("key")
	if !storagedomain.IsSiteAssetLogoKey(key) {
		renderV1BinaryError(c, http.StatusBadRequest)
		return
	}
	image, err := h.deps.MediaRenderer.Logo(c.Request.Context(), strings.TrimSpace(key))
	writeRenderedImage(c, image, err, v1PublicBrandingLogoCacheControl)
}
