package httpapi

import (
	"database/sql"
	"net/http"
	"strconv"
	"strings"

	dbsqlc "ideaboard/api/internal/db/sqlc"
	"ideaboard/api/internal/identity/principal"
	"ideaboard/api/internal/media/assets"
	"ideaboard/api/internal/media/derivatives"
	"ideaboard/api/internal/media/storage"
	"ideaboard/api/internal/platform/response"

	"github.com/gin-gonic/gin"
)

type mediaAssetHandler struct {
	assets  *assets.MediaAssetRegistry
	storage *storage.StorageService
}

func newMediaAssetHandler(registry *assets.MediaAssetRegistry, storageService *storage.StorageService) *mediaAssetHandler {
	return &mediaAssetHandler{assets: registry, storage: storageService}
}

type mediaAssetDerivativeDTO struct {
	Status    string `json:"status"`
	Format    string `json:"format,omitempty"`
	URL       string `json:"url,omitempty"`
	Width     int32  `json:"width,omitempty"`
	Height    int32  `json:"height,omitempty"`
	SizeBytes int64  `json:"size_bytes,omitempty"`
	ErrorCode string `json:"error_code,omitempty"`
}

type mediaAssetDTO struct {
	AssetID     string                             `json:"asset_id"`
	Status      string                             `json:"status"`
	MIMEType    string                             `json:"mime_type"`
	Width       int32                              `json:"width"`
	Height      int32                              `json:"height"`
	SizeBytes   int64                              `json:"size_bytes"`
	Derivatives map[string]mediaAssetDerivativeDTO `json:"derivatives"`
}

func (h *mediaAssetHandler) Metadata(c *gin.Context) {
	identity, ok := principal.Require(c)
	if !ok || h == nil || h.assets == nil {
		return
	}
	assetID := strings.TrimSpace(c.Param("asset_id"))
	asset, err := h.lookupAsset(c, identity.UserID(), assetID)
	if err != nil {
		if err == sql.ErrNoRows {
			response.Error(c, http.StatusNotFound, "媒体不存在")
			return
		}
		response.InternalError(c, http.StatusInternalServerError, "读取媒体信息失败", err)
		return
	}
	rows, err := h.assets.ListDerivatives(c.Request.Context(), identity.UserID(), asset.ID)
	if err != nil {
		response.InternalError(c, http.StatusInternalServerError, "读取媒体派生信息失败", err)
		return
	}
	result := mediaAssetDTO{
		AssetID: asset.AssetID, Status: asset.Status, MIMEType: asset.MimeType,
		Width: asset.Width, Height: asset.Height, SizeBytes: asset.SizeBytes,
		Derivatives: make(map[string]mediaAssetDerivativeDTO, len(derivatives.TierSpecs)),
	}
	for tier := range derivatives.TierSpecs {
		result.Derivatives[tier] = mediaAssetDerivativeDTO{Status: "pending"}
	}
	for _, row := range rows {
		item := mediaAssetDerivativeDTO{Status: row.Status, Format: row.Format, Width: row.Width, Height: row.Height, SizeBytes: row.SizeBytes, ErrorCode: row.ErrorCode}
		if row.Status == "ready" {
			item.URL = "/api/user/media-assets/" + asset.AssetID + "/derivatives/" + row.Tier + "/content"
		}
		result.Derivatives[row.Tier] = item
	}
	response.SuccessDTO(c, result)
}

func (h *mediaAssetHandler) Content(c *gin.Context) {
	identity, ok := principal.Require(c)
	if !ok || h == nil || h.assets == nil || h.storage == nil {
		return
	}
	assetID := strings.TrimSpace(c.Param("asset_id"))
	tier := strings.TrimSpace(c.Param("tier"))
	if _, exists := derivatives.TierSpecs[tier]; !exists {
		response.Error(c, http.StatusBadRequest, "图片派生规格无效")
		return
	}
	asset, err := h.lookupAsset(c, identity.UserID(), assetID)
	if err != nil {
		if err == sql.ErrNoRows {
			response.Error(c, http.StatusNotFound, "媒体不存在")
			return
		}
		response.InternalError(c, http.StatusInternalServerError, "读取媒体信息失败", err)
		return
	}
	rows, err := h.assets.ListDerivatives(c.Request.Context(), identity.UserID(), asset.ID)
	if err != nil {
		response.InternalError(c, http.StatusInternalServerError, "读取媒体派生信息失败", err)
		return
	}
	for _, row := range rows {
		if row.Tier != tier || row.Status != "ready" {
			continue
		}
		object, readErr := h.storage.ReadObjectAtProfile(c.Request.Context(), row.StorageProfileID, row.StorageKey, 50*1024*1024)
		if readErr != nil {
			response.InternalError(c, http.StatusInternalServerError, "读取媒体派生文件失败", readErr)
			return
		}
		c.Header("Cache-Control", "private, max-age=3600")
		c.Header("Content-Disposition", "inline")
		c.Data(http.StatusOK, row.MimeType, object.Data)
		return
	}
	response.Error(c, http.StatusAccepted, "图片预览正在准备中")
}

func (h *mediaAssetHandler) lookupAsset(c *gin.Context, ownerUserID int64, raw string) (dbsqlc.MediaAsset, error) {
	if id, err := strconv.ParseInt(raw, 10, 64); err == nil && id > 0 {
		return h.assets.GetByID(c.Request.Context(), ownerUserID, id)
	}
	return h.assets.GetByAssetID(c.Request.Context(), ownerUserID, raw)
}
