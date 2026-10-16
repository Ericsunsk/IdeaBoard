package httpapi

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/url"

	libraryapplication "ideaboard/api/internal/library/application"
	librarydomain "ideaboard/api/internal/library/domain"
	mediadomain "ideaboard/api/internal/media/derivatives/domain"

	"github.com/gin-gonic/gin"
)

func libraryDisplayVersion(asset *librarydomain.V1Asset) string {
	if asset.Source == librarydomain.AssetSourceMedia {
		sum := sha256.Sum256([]byte("media\x00" + asset.MediaAssetID))
		return hex.EncodeToString(sum[:12])
	}
	sum := sha256.Sum256([]byte(asset.Provider + "\x00" + asset.PreviewURL))
	return hex.EncodeToString(sum[:12])
}

func libraryDisplay(workspaceID string, asset *librarydomain.V1Asset) *mediadomain.Display {
	return mediadomain.DisplayVariants(func(tier string) string {
		return ContractV1GroupPath + "/workspaces/" + url.PathEscape(workspaceID) + "/library/assets/" + url.PathEscape(asset.PublicID) + "/derivatives/" + tier + "?v=" + libraryDisplayVersion(asset)
	})
}

func (h *contractV1Handler) derivativeLibraryAsset(c *gin.Context) {
	workspaceID, allowed := h.v1BinaryWorkspaceContext(c)
	if !allowed {
		return
	}
	if h.deps.Library.Sources == nil || h.deps.MediaPreviews == nil {
		renderV1BinaryError(c, http.StatusServiceUnavailable)
		return
	}
	asset, mediaID, sourceErr := h.deps.Library.Sources.Resolve(c.Request.Context(), workspaceID, c.Param("asset_id"))
	if asset == nil {
		renderV1BinaryError(c, http.StatusNotFound)
		return
	}
	if c.Query("v") != libraryDisplayVersion(asset) {
		renderV1BinaryError(c, http.StatusNotFound)
		return
	}
	tier := c.Param("tier")
	if _, ok := mediadomain.TierSpecs[tier]; !ok || tier == mediadomain.TierProcessingInput {
		renderV1BinaryError(c, http.StatusBadRequest)
		return
	}
	if sourceErr != nil {
		status := http.StatusNotFound
		if errors.Is(sourceErr, libraryapplication.ErrSourceUnavailable) {
			status = http.StatusServiceUnavailable
		}
		renderV1BinaryError(c, status)
		return
	}
	if mediaID != "" {
		h.writeLibraryManagedDisplay(c, workspaceID, mediaID, tier)
		return
	}
	image, err := h.deps.MediaPreviews.RenderSource(c.Request.Context(), asset.Provider, asset.PreviewURL, tier)
	writeRenderedImage(c, image, err, "private, max-age=300")
}

func (h *contractV1Handler) writeLibraryManagedDisplay(c *gin.Context, workspaceID, mediaID, tier string) {
	media, ok := h.v1MediaBinaryAsset(c, workspaceID, mediaID)
	if !ok {
		return
	}
	image, err := h.deps.MediaRenderer.WorkspaceTier(c.Request.Context(), media.StorageKey, tier)
	writeRenderedImage(c, image, err, "private, max-age=300")
}
