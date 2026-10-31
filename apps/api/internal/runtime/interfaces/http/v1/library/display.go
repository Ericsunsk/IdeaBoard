package library

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/url"
	"strings"

	"ideaboard/api/internal/identity/principal"
	librarysources "ideaboard/api/internal/library/application/sources"
	librarydomain "ideaboard/api/internal/library/domain"
	mediadomain "ideaboard/api/internal/media/derivatives/domain"
	"ideaboard/api/internal/platform/contract"
	runtimehttp "ideaboard/api/internal/runtime/interfaces/http"

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

func LibraryDisplay(workspaceID string, asset *librarydomain.V1Asset) *mediadomain.Display {
	return mediadomain.DisplayVariants(func(tier string) string {
		return runtimehttp.ContractV1GroupPath + "/workspaces/" + url.PathEscape(workspaceID) + "/library/assets/" + url.PathEscape(asset.PublicID) + "/derivatives/" + tier + "?v=" + libraryDisplayVersion(asset)
	})
}

func libraryDisplay(workspaceID string, asset *librarydomain.V1Asset) *mediadomain.Display {
	return LibraryDisplay(workspaceID, asset)
}

func (h *Handler) derivativeLibraryAsset(c *gin.Context) {
	workspaceID, userID, workspaceRole, allowed := h.binaryWorkspaceContext(c)
	if !allowed {
		return
	}
	if h.deps.Library.Sources == nil || h.deps.MediaPreviews == nil {
		runtimehttp.RenderV1BinaryError(c, http.StatusServiceUnavailable)
		return
	}
	asset, mediaID, sourceErr := h.deps.Library.Sources.Resolve(c.Request.Context(), workspaceID, c.Param("asset_id"))
	if asset == nil {
		runtimehttp.RenderV1BinaryError(c, http.StatusNotFound)
		return
	}
	if c.Query("v") != libraryDisplayVersion(asset) {
		runtimehttp.RenderV1BinaryError(c, http.StatusNotFound)
		return
	}
	tier := c.Param("tier")
	if _, ok := mediadomain.TierSpecs[tier]; !ok || tier == mediadomain.TierProcessingInput {
		runtimehttp.RenderV1BinaryError(c, http.StatusBadRequest)
		return
	}
	if sourceErr != nil {
		status := http.StatusNotFound
		if errors.Is(sourceErr, librarysources.ErrSourceUnavailable) {
			status = http.StatusServiceUnavailable
		}
		runtimehttp.RenderV1BinaryError(c, status)
		return
	}
	if mediaID != "" {
		h.writeLibraryManagedDisplay(c, workspaceID, mediaID, tier, userID, workspaceRole)
		return
	}
	image, err := h.deps.MediaPreviews.RenderSource(c.Request.Context(), asset.Provider, asset.PreviewURL, tier)
	runtimehttp.WriteRenderedImage(c, image, err, "private, max-age=300")
}

func (h *Handler) writeLibraryManagedDisplay(c *gin.Context, workspaceID, mediaID, tier string, userID int64, workspaceRole string) {
	// #936: managed bytes behind a library asset enforce the same asset
	// authorization (the library-link source authorizes members here);
	// a shared library reference never becomes a bypass around it.
	if h.deps.AuthorizeMediaBinaryAsset == nil || h.deps.MediaRenderer == nil {
		runtimehttp.RenderV1BinaryError(c, http.StatusServiceUnavailable)
		return
	}
	media, ok := h.deps.AuthorizeMediaBinaryAsset(c, workspaceID, mediaID, userID, workspaceRole)
	if !ok {
		return
	}
	image, err := h.deps.MediaRenderer.WorkspaceTier(c.Request.Context(), media.StorageKey, tier)
	runtimehttp.WriteRenderedImage(c, image, err, "private, max-age=300")
}

func (h *Handler) binaryWorkspaceContext(c *gin.Context) (workspacePublicID string, userID int64, workspaceRole string, allowed bool) {
	identityPrincipal, ok := principal.From(c)
	if !ok {
		runtimehttp.RenderV1BinaryError(c, http.StatusUnauthorized)
		return "", 0, "", false
	}
	if h.deps.Store == nil {
		runtimehttp.RenderV1BinaryError(c, http.StatusNotImplemented)
		return "", 0, "", false
	}
	raw := strings.TrimSpace(c.Param("workspace_id"))
	if !contract.IsWorkspaceID(raw) {
		runtimehttp.RenderV1BinaryError(c, http.StatusNotFound)
		return "", 0, "", false
	}
	ctx := c.Request.Context()
	workspace, err := h.deps.Store.GetWorkspace(ctx, raw)
	if err != nil {
		runtimehttp.RenderV1BinaryError(c, http.StatusNotFound)
		return "", 0, "", false
	}
	role, member, err := h.deps.Store.GetWorkspaceRole(ctx, raw, identityPrincipal.UserID())
	if err != nil || !member {
		runtimehttp.RenderV1BinaryError(c, http.StatusNotFound)
		return "", 0, "", false
	}
	return workspace.PublicID, identityPrincipal.UserID(), role, true
}
