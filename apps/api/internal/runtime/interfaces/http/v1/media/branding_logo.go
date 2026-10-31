package media

import (
	"net/http"
	"strings"

	storagedomain "ideaboard/api/internal/media/storage/domain"
	runtimehttp "ideaboard/api/internal/runtime/interfaces/http"

	"github.com/gin-gonic/gin"
)

// Branding responses revalidate against the active storage profile.
const v1PublicBrandingLogoCacheControl = "no-store"

// publicBrandingLogo renders the requested canonical site asset, including unsaved drafts.
func (h *Handler) publicBrandingLogo(c *gin.Context) {
	key := c.Query("key")
	if !storagedomain.IsSiteAssetLogoKey(key) {
		runtimehttp.RenderV1BinaryError(c, http.StatusBadRequest)
		return
	}
	image, err := h.deps.Media.MediaRenderer.Logo(c.Request.Context(), strings.TrimSpace(key))
	runtimehttp.WriteRenderedImage(c, image, err, v1PublicBrandingLogoCacheControl)
}

func (h *Handler) PublicBrandingLogo(c *gin.Context) {
	h.publicBrandingLogo(c)
}
