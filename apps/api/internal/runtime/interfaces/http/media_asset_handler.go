package httpapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	dbsqlc "ideaboard/api/internal/db/sqlc"
	"ideaboard/api/internal/identity/principal"
	"ideaboard/api/internal/media/assets"
	derivativedomain "ideaboard/api/internal/media/derivatives/domain"
	"ideaboard/api/internal/media/derivatives/infrastructure/imgproxy"
	storagedomain "ideaboard/api/internal/media/storage/domain"
	storageports "ideaboard/api/internal/media/storage/ports"
	"ideaboard/api/internal/platform/database"
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

type mediaAssetBatchRequest struct {
	AssetIDs   []string `json:"asset_ids"`
	EnsureTier string   `json:"ensure_tier,omitempty"`
}

type mediaAssetBatchDTO struct {
	Items []mediaAssetDTO `json:"items"`
}

type metadataBatchFailure struct {
	status  int
	message string
	cause   error
}

func (h *mediaAssetHandler) Metadata(c *gin.Context) {
	identity, ok := principal.Require(c)
	if !ok || h == nil || h.assets == nil {
		return
	}
	assetID := strings.TrimSpace(c.Param("asset_id"))
	asset, err := h.lookupAsset(c, identity.UserID(), assetID)
	if err != nil {
		if database.IsNotFound(err) {
			response.Error(c, http.StatusNotFound, "媒体不存在")
			return
		}
		response.InternalError(c, http.StatusInternalServerError, "读取媒体信息失败", err)
		return
	}
	ensureTier := strings.TrimSpace(c.Query("ensure_tier"))
	if !validateEnsureTier(ensureTier) {
		response.Error(c, http.StatusBadRequest, "请求的媒体派生规格无效")
		return
	}
	result, err := h.buildMetadata(c, identity.UserID(), asset)
	if err != nil {
		response.InternalError(c, http.StatusInternalServerError, "读取媒体派生信息失败", err)
		return
	}
	c.Header("Cache-Control", "no-store")
	response.SuccessDTO(c, result)
}

// MetadataBatch returns metadata and same-origin delivery URLs for several owned assets
// in one request. Missing or unauthorized assets are omitted to avoid leaking
// ownership information across the batch boundary.
func (h *mediaAssetHandler) MetadataBatch(c *gin.Context) {
	identity, ok := principal.Require(c)
	if !ok || h == nil || h.assets == nil {
		return
	}
	req, ensureTier, failure := parseMetadataBatchRequest(c)
	if failure != nil {
		respondMetadataBatchFailure(c, failure)
		return
	}
	items, failure := h.collectMetadataBatch(c, identity.UserID(), req.AssetIDs, ensureTier)
	if failure != nil {
		respondMetadataBatchFailure(c, failure)
		return
	}
	c.Header("Cache-Control", "no-store")
	response.SuccessDTO(c, mediaAssetBatchDTO{Items: items})
}

func parseMetadataBatchRequest(c *gin.Context) (mediaAssetBatchRequest, string, *metadataBatchFailure) {
	var req mediaAssetBatchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		return req, "", &metadataBatchFailure{status: http.StatusBadRequest, message: "请求参数无效"}
	}
	if len(req.AssetIDs) == 0 || len(req.AssetIDs) > 100 {
		return req, "", &metadataBatchFailure{status: http.StatusBadRequest, message: "媒体数量必须在1到100之间"}
	}
	ensureTier := strings.TrimSpace(req.EnsureTier)
	if !isMetadataBatchTier(ensureTier) {
		return req, "", &metadataBatchFailure{status: http.StatusBadRequest, message: "请求的媒体派生规格无效"}
	}
	return req, ensureTier, nil
}

func isMetadataBatchTier(ensureTier string) bool {
	return validateEnsureTier(ensureTier)
}

// validateEnsureTier checks the tier catalogue contract shared by the single
// and batch metadata endpoints. Tiers resolve on demand through imgproxy, so
// the parameter only validates: there is nothing to enqueue.
func validateEnsureTier(ensureTier string) bool {
	if ensureTier == "" {
		return true
	}
	_, exists := derivativedomain.TierSpecs[ensureTier]
	return exists
}

func respondMetadataBatchFailure(c *gin.Context, failure *metadataBatchFailure) {
	if failure.status >= http.StatusInternalServerError {
		response.InternalError(c, failure.status, failure.message, failure.cause)
		return
	}
	response.Error(c, failure.status, failure.message)
}

func (h *mediaAssetHandler) collectMetadataBatch(c *gin.Context, ownerUserID int64, rawAssetIDs []string, ensureTier string) ([]mediaAssetDTO, *metadataBatchFailure) {
	items := make([]mediaAssetDTO, 0, len(rawAssetIDs))
	seen := make(map[string]struct{}, len(rawAssetIDs))
	for _, rawID := range rawAssetIDs {
		assetID := strings.TrimSpace(rawID)
		if assetID == "" || len(assetID) > 128 {
			return nil, &metadataBatchFailure{status: http.StatusBadRequest, message: "媒体标识无效"}
		}
		if _, exists := seen[assetID]; exists {
			continue
		}
		seen[assetID] = struct{}{}
		item, include, failure := h.loadMetadataBatchItem(c, ownerUserID, assetID, ensureTier)
		if failure != nil {
			return nil, failure
		}
		if include {
			items = append(items, item)
		}
	}
	return items, nil
}

func (h *mediaAssetHandler) loadMetadataBatchItem(c *gin.Context, ownerUserID int64, assetID, ensureTier string) (mediaAssetDTO, bool, *metadataBatchFailure) {
	asset, err := h.lookupAsset(c, ownerUserID, assetID)
	if database.IsNotFound(err) {
		return mediaAssetDTO{}, false, nil
	}
	if err != nil {
		return mediaAssetDTO{}, false, &metadataBatchFailure{status: http.StatusInternalServerError, message: "读取媒体信息失败", cause: err}
	}
	if ensureTier != "" && asset.Status == "ready" {
		// Tiers resolve on demand through imgproxy; nothing to enqueue.
		if !validateEnsureTier(ensureTier) {
			return mediaAssetDTO{}, false, &metadataBatchFailure{status: http.StatusBadRequest, message: "请求的媒体派生规格无效"}
		}
	}
	item, err := h.buildMetadata(c, ownerUserID, asset)
	if err != nil {
		return mediaAssetDTO{}, false, &metadataBatchFailure{status: http.StatusInternalServerError, message: "读取媒体派生信息失败", cause: err}
	}
	return item, true, nil
}

func (h *mediaAssetHandler) buildMetadata(c *gin.Context, ownerUserID int64, asset dbsqlc.MediaAsset) (mediaAssetDTO, error) {
	result := mediaAssetDTO{
		AssetID: asset.AssetID, Status: asset.Status, MIMEType: asset.MimeType,
		Width: asset.Width, Height: asset.Height, SizeBytes: asset.SizeBytes,
		Original: mediaAssetOriginalDTO{
			Status: asset.Status, MIMEType: asset.MimeType, Width: asset.Width,
			Height: asset.Height, SizeBytes: asset.SizeBytes,
		},
		Derivatives: make(map[string]mediaAssetDerivativeDTO, len(derivativedomain.TierSpecs)),
	}
	if asset.Status == "ready" {
		// Keep media URLs on the same-origin gateway. In particular, do not
		// expose S3 signed URLs to the browser; the content endpoint reads via
		// the owning storage profile and preserves the gateway security boundary.
		result.Original.URL = "/api/user/media-assets/" + asset.AssetID + "/original/content"
	}
	// Display tiers resolve on demand through imgproxy, so every known tier
	// is immediately servable. The processing-input tier keeps its
	// synchronously materialized row for provider inputs; derive the rest
	// from the pull renderer instead of stored derivative rows.
	readyDerivatives, err := h.listReadyDerivativeRows(c, ownerUserID, asset)
	if err != nil {
		return mediaAssetDTO{}, err
	}
	for tier := range derivativedomain.TierSpecs {
		if tier == derivativedomain.TierProcessingInput {
			continue
		}
		derivative := mediaAssetDerivativeDTO{Status: "ready", Format: "webp", Width: asset.Width, Height: asset.Height}
		if asset.Status == "ready" {
			derivative.URL = fmt.Sprintf("/api/user/media-assets/%s/derivatives/%s/content", asset.AssetID, tier)
		} else {
			derivative.Status = "pending"
			derivative.URL = ""
		}
		result.Derivatives[tier] = derivative
	}
	for tier, item := range readyDerivatives {
		result.Derivatives[tier] = item
	}
	return result, nil
}

// listReadyDerivativeRows reports synchronously materialized tiers
// (currently only processing-input) from stored derivative rows.
func (h *mediaAssetHandler) listReadyDerivativeRows(c *gin.Context, ownerUserID int64, asset dbsqlc.MediaAsset) (map[string]mediaAssetDerivativeDTO, error) {
	items := make(map[string]mediaAssetDerivativeDTO)
	rows, err := h.assets.ListDerivatives(c.Request.Context(), ownerUserID, asset.ID)
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		if row.Status != "ready" {
			continue
		}
		items[row.Tier] = mediaAssetDerivativeDTO{
			Status: row.Status, Format: row.Format, Width: row.Width, Height: row.Height,
			SizeBytes: row.SizeBytes, ErrorCode: row.ErrorCode,
			URL: fmt.Sprintf("/api/user/media-assets/%s/derivatives/%s/content", asset.AssetID, row.Tier),
		}
	}
	return items, nil
}

// Content serves a display tier by redirecting to a signed imgproxy URL
// after ownership is verified. The gateway security boundary stays intact:
// browsers never learn storage internals or imgproxy signing keys.
func (h *mediaAssetHandler) Content(c *gin.Context) {
	identity, ok := principal.Require(c)
	if !ok || h == nil || h.assets == nil {
		return
	}
	assetID := strings.TrimSpace(c.Param("asset_id"))
	tier := strings.TrimSpace(c.Param("tier"))
	if _, exists := derivativedomain.TierSpecs[tier]; !exists {
		response.Error(c, http.StatusBadRequest, "图片派生规格无效")
		return
	}
	h.serveTierRedirect(c, identity.UserID(), assetID, tier)
}

func (h *mediaAssetHandler) serveTierRedirect(c *gin.Context, ownerUserID int64, assetID, tier string) {
	asset, err := h.lookupAsset(c, ownerUserID, assetID)
	if err != nil {
		if database.IsNotFound(err) {
			response.Error(c, http.StatusNotFound, "媒体不存在")
			return
		}
		response.InternalError(c, http.StatusInternalServerError, "读取媒体信息失败", err)
		return
	}
	if asset.Status != "ready" || asset.DeletedAt.Valid {
		response.Error(c, http.StatusAccepted, "图片预览正在准备中")
		return
	}
	// Prefer a generation-matched stored derivative so same-origin fetch
	// consumers (retouch export, canvas downscale) receive bytes directly
	// instead of following a cross-origin imgproxy redirect that browsers
	// block without CORS headers. The pull renderer stays as fallback for
	// tiers without stored rows.
	if h.serveStoredDerivative(c, ownerUserID, asset, tier) {
		return
	}
	source, err := imgproxySourceForAsset(asset)
	if err != nil {
		response.Error(c, http.StatusServiceUnavailable, "图片派生服务暂不可用")
		return
	}
	target, err := h.imgproxy.TierURL(tier, source)
	if err != nil {
		response.InternalError(c, http.StatusInternalServerError, "生成图片派生地址失败", err)
		return
	}
	c.Header("Cache-Control", "private, max-age=3600")
	c.Redirect(http.StatusFound, target)
}

func (h *mediaAssetHandler) writeOriginalBytes(c *gin.Context, asset dbsqlc.MediaAsset, cacheControl string) {
	if h == nil || h.storage == nil {
		response.InternalError(c, http.StatusInternalServerError, "读取原图失败", fmt.Errorf("media storage is unavailable"))
		return
	}
	object, err := h.storage.ReadObjectAtProfile(c.Request.Context(), asset.StorageProfileID, asset.StorageKey, 100*1024*1024)
	if err != nil {
		if errors.Is(err, storagedomain.ErrStoredObjectNotFound) {
			response.Error(c, http.StatusNotFound, "媒体不存在")
			return
		}
		response.InternalError(c, http.StatusInternalServerError, "读取原图失败", err)
		return
	}
	mimeType := strings.TrimSpace(asset.MimeType)
	if mimeType == "" {
		mimeType = object.ContentType
	}
	c.Header("Cache-Control", cacheControl)
	c.Header("Content-Disposition", "inline")
	c.Data(http.StatusOK, mimeType, object.Data)
}

func (h *mediaAssetHandler) serveStoredDerivative(c *gin.Context, ownerUserID int64, asset dbsqlc.MediaAsset, tier string) bool {
	if h == nil || h.assets == nil || h.storage == nil {
		return false
	}
	rows, err := h.assets.ListDerivatives(c.Request.Context(), ownerUserID, asset.ID)
	if err != nil {
		response.InternalError(c, http.StatusInternalServerError, "读取图片派生信息失败", err)
		return true
	}
	for _, row := range rows {
		if row.Tier != tier || row.Status != "ready" || row.SourceObjectGeneration != asset.ObjectGeneration {
			continue
		}
		object, err := h.storage.ReadObjectAtProfile(c.Request.Context(), row.StorageProfileID, row.StorageKey, 100*1024*1024)
		// A missing or unreadable stored object falls through to the next row
		// or the pull renderer, which derives the tier from the original:
		// never fail a tier that imgproxy may still be able to serve.
		if err != nil || object == nil || len(object.Data) == 0 {
			continue
		}
		mimeType := strings.TrimSpace(row.MimeType)
		if mimeType == "" {
			mimeType = object.ContentType
		}
		if mimeType == "" {
			mimeType = "application/octet-stream"
		}
		c.Header("Cache-Control", "private, max-age=3600")
		c.Header("Content-Disposition", "inline")
		c.Data(http.StatusOK, mimeType, object.Data)
		return true
	}
	return false
}

// imgproxySourceForAsset maps an asset to its imgproxy source URL.
// Only the builtin-local profile resolves today. Other profiles must use a
// materialized derivative or fail explicitly; they must never masquerade the
// original object as a requested tier.
func imgproxySourceForAsset(asset dbsqlc.MediaAsset) (string, error) {
	if strings.TrimSpace(asset.StorageProfileID) != "builtin-local" {
		return "", fmt.Errorf("storage profile %q has no imgproxy source", asset.StorageProfileID)
	}
	return imgproxy.LocalSource(asset.StorageKey)
}

func (h *mediaAssetHandler) Original(c *gin.Context) {
	identity, ok := principal.Require(c)
	if !ok || h == nil || h.assets == nil || h.storage == nil {
		return
	}
	assetID := strings.TrimSpace(c.Param("asset_id"))
	asset, err := h.lookupAsset(c, identity.UserID(), assetID)
	if err != nil {
		if database.IsNotFound(err) {
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
	h.writeOriginalBytes(c, asset, "private, max-age=3600")
}

func (h *mediaAssetHandler) lookupAsset(c *gin.Context, ownerUserID int64, raw string) (dbsqlc.MediaAsset, error) {
	// String asset identifiers (`asset_...`) are the only accepted identity.
	// Numeric database IDs are never exposed over this gateway.
	return h.assets.GetByAssetID(c.Request.Context(), ownerUserID, strings.TrimSpace(raw))
}
