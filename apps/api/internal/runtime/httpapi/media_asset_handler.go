package httpapi

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

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

type mediaAssetOriginalDTO struct {
	Status    string `json:"status"`
	MIMEType  string `json:"mime_type,omitempty"`
	Width     int32  `json:"width,omitempty"`
	Height    int32  `json:"height,omitempty"`
	SizeBytes int64  `json:"size_bytes,omitempty"`
	URL       string `json:"url,omitempty"`
}

type mediaAssetDTO struct {
	AssetID     string                             `json:"asset_id"`
	Status      string                             `json:"status"`
	MIMEType    string                             `json:"mime_type"`
	Width       int32                              `json:"width"`
	Height      int32                              `json:"height"`
	SizeBytes   int64                              `json:"size_bytes"`
	Original    mediaAssetOriginalDTO              `json:"original"`
	Derivatives map[string]mediaAssetDerivativeDTO `json:"derivatives"`
}

func (h *mediaAssetHandler) Metadata(c *gin.Context) {
	identity, ok := principal.Require(c)
	if !ok || h == nil || h.assets == nil || h.storage == nil {
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
	ensureTier := strings.TrimSpace(c.Query("ensure_tier"))
	if ensureTier != "" {
		if ensureTier != derivatives.TierLargePreview {
			response.Error(c, http.StatusBadRequest, "请求的媒体派生规格无效")
			return
		}
		if asset.Status == "ready" {
			if err := h.assets.EnsureDerivative(c.Request.Context(), identity.UserID(), asset.ID, ensureTier); err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					response.Error(c, http.StatusNotFound, "媒体不存在")
					return
				}
				response.InternalError(c, http.StatusInternalServerError, "准备媒体派生信息失败", err)
				return
			}
		}
	}
	rows, err := h.assets.ListDerivatives(c.Request.Context(), identity.UserID(), asset.ID)
	if err != nil {
		response.InternalError(c, http.StatusInternalServerError, "读取媒体派生信息失败", err)
		return
	}
	result := mediaAssetDTO{
		AssetID: asset.AssetID, Status: asset.Status, MIMEType: asset.MimeType,
		Width: asset.Width, Height: asset.Height, SizeBytes: asset.SizeBytes,
		Original: mediaAssetOriginalDTO{
			Status: asset.Status, MIMEType: asset.MimeType, Width: asset.Width,
			Height: asset.Height, SizeBytes: asset.SizeBytes,
		},
		Derivatives: make(map[string]mediaAssetDerivativeDTO, len(derivatives.TierSpecs)),
	}
	if asset.Status == "ready" {
		result.Original.URL = "/api/user/media-assets/" + asset.AssetID + "/original/content"
		signedURL, supported, err := h.storage.ResolveSignedURLAtProfile(c.Request.Context(), asset.StorageProfileID, asset.StorageKey, time.Hour)
		if err != nil {
			response.InternalError(c, http.StatusInternalServerError, "生成原图访问地址失败", err)
			return
		}
		if supported {
			result.Original.URL = signedURL
		}
	}
	for tier := range derivatives.TierSpecs {
		result.Derivatives[tier] = mediaAssetDerivativeDTO{Status: "pending"}
	}
	for _, row := range rows {
		item := mediaAssetDerivativeDTO{Status: row.Status, Format: row.Format, Width: row.Width, Height: row.Height, SizeBytes: row.SizeBytes, ErrorCode: row.ErrorCode}
		if row.Status == "ready" {
			item.URL = fmt.Sprintf("/api/user/media-assets/%s/derivatives/%s/%s/content", asset.AssetID, row.Tier, row.Format)
			signedURL, supported, err := h.storage.ResolveSignedURLAtProfile(c.Request.Context(), row.StorageProfileID, row.StorageKey, time.Hour)
			if err != nil {
				response.InternalError(c, http.StatusInternalServerError, "生成媒体派生访问地址失败", err)
				return
			}
			if supported {
				item.URL = signedURL
			}
		}
		result.Derivatives[row.Tier] = item
	}
	c.Header("Cache-Control", "no-store")
	response.SuccessDTO(c, result)
}

func (h *mediaAssetHandler) Content(c *gin.Context) {
	identity, ok := principal.Require(c)
	if !ok || h == nil || h.assets == nil || h.storage == nil {
		return
	}
	assetID := strings.TrimSpace(c.Param("asset_id"))
	tier := strings.TrimSpace(c.Param("tier"))
	format := strings.TrimSpace(c.Param("format"))
	if _, exists := derivatives.TierSpecs[tier]; !exists {
		response.Error(c, http.StatusBadRequest, "图片派生规格无效")
		return
	}
	if format != derivatives.FormatWebP && format != derivatives.FormatJPEG && format != derivatives.FormatPNG {
		response.Error(c, http.StatusBadRequest, "图片派生格式无效")
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
		if row.Tier != tier || row.Format != format || row.Status != "ready" {
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

func (h *mediaAssetHandler) Original(c *gin.Context) {
	identity, ok := principal.Require(c)
	if !ok || h == nil || h.assets == nil || h.storage == nil {
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
	if asset.Status != "ready" || asset.DeletedAt.Valid {
		response.Error(c, http.StatusAccepted, "原图正在准备中")
		return
	}
	object, err := h.storage.ReadObjectAtProfile(c.Request.Context(), asset.StorageProfileID, asset.StorageKey, 100*1024*1024)
	if err != nil {
		response.InternalError(c, http.StatusInternalServerError, "读取原图失败", err)
		return
	}
	mimeType := strings.TrimSpace(asset.MimeType)
	if mimeType == "" {
		mimeType = object.ContentType
	}
	c.Header("Cache-Control", "private, max-age=3600")
	c.Header("Content-Disposition", "inline")
	c.Data(http.StatusOK, mimeType, object.Data)
}

func (h *mediaAssetHandler) lookupAsset(c *gin.Context, ownerUserID int64, raw string) (dbsqlc.MediaAsset, error) {
	if id, err := strconv.ParseInt(raw, 10, 64); err == nil && id > 0 {
		return h.assets.GetByID(c.Request.Context(), ownerUserID, id)
	}
	return h.assets.GetByAssetID(c.Request.Context(), ownerUserID, raw)
}
