package httpapi

import (
	"context"
	"net/http"

	"ideaboard/api/internal/media/assets"
	"ideaboard/api/internal/media/derivatives/infrastructure/imgproxy"
	storageports "ideaboard/api/internal/media/storage/ports"
	"ideaboard/api/internal/platform/response"

	"github.com/gin-gonic/gin"
)

// mediaObjectReader is the narrow storage seam the derivative gateway needs.
// *storage.Service satisfies it in production; tests inject fakes.
type mediaObjectReader interface {
	ReadObjectAtProfile(ctx context.Context, profileID, objectKey string, maxBytes int64) (*storageports.StoredUploadObject, error)
}

type mediaAssetHandler struct {
	assets   assets.ReadRegistry
	storage  mediaObjectReader
	imgproxy imgproxy.Config
}

func newMediaAssetHandler(registry assets.ReadRegistry, storageService mediaObjectReader, imgproxyConfig imgproxy.Config) *mediaAssetHandler {
	return &mediaAssetHandler{
		assets:   registry,
		storage:  storageService,
		imgproxy: imgproxyConfig,
	}
}

// #817 (Phase 3b-08): the legacy user-scoped media_assets postgres adapter
// was deleted with the media_assets table rename to the workspace-scoped
// model (public_id asset_%). The assets registry GetByAssetID/GetByID stubs
// fail closed, so every legacy /api/user/media-assets endpoint below fails
// closed with 501 until the workspace-scoped media asset store lands in
// Phase 4. The derivative-tier catalogue (imgproxy pull model) is retained
// in the media/derivatives domain for the Phase 4 delivery path.

func (h *mediaAssetHandler) Metadata(c *gin.Context) {
	response.Error(c, http.StatusNotImplemented, "媒体服务暂不可用")
}

func (h *mediaAssetHandler) MetadataBatch(c *gin.Context) {
	response.Error(c, http.StatusNotImplemented, "媒体服务暂不可用")
}

func (h *mediaAssetHandler) Content(c *gin.Context) {
	response.Error(c, http.StatusNotImplemented, "媒体服务暂不可用")
}

func (h *mediaAssetHandler) Original(c *gin.Context) {
	response.Error(c, http.StatusNotImplemented, "媒体服务暂不可用")
}
