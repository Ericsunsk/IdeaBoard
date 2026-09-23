package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"ideaboard/api/internal/events"
	"ideaboard/api/internal/identity/principal"
	derivativedomain "ideaboard/api/internal/media/derivatives/domain"
	storagedomain "ideaboard/api/internal/media/storage/domain"
	"ideaboard/api/internal/media/storage/ports"
	"ideaboard/api/internal/media/workspaceuploads"
	"ideaboard/api/internal/platform/contract"
	"ideaboard/api/internal/tenancy"

	"github.com/gin-gonic/gin"
)

// Workspace media assets (issue #803). Additive only: legacy
// /api/user/media-assets routes are untouched (removal is #809).
//
// Binary vs metadata separation: list/get/archive speak the JSON
// envelope only; content/derivative serve bytes (never the envelope,
// never storage keys). Errors on the binary endpoints are plain
// non-JSON statuses for the same reason. The same-origin gateway
// serves bytes today; a presigned-302 optimization for S3-compatible
// profiles is a #806 seam (no redirect is emitted in this slice).
//
// Visibility is workspace membership, not per-owner scoping: any
// workspace member reads any workspace asset; the asset's workspace
// binding (metadata.workspace_id) must equal the path workspace or
// the asset is invisible (404 media_not_found).

// V1MediaObjectReader is the narrow storage seam the v1 content and
// derivative endpoints need. *storage.Service satisfies it in
// production; tests inject fakes. Nil fails binary reads closed
// with 503 while metadata endpoints keep working.
type V1MediaObjectReader interface {
	ReadObjectAtProfile(ctx context.Context, profileID, objectKey string, maxBytes int64) (*ports.StoredUploadObject, error)
}

// v1MediaBinaryMaxBytes bounds one binary read (parity with the legacy
// original-bytes path).
const v1MediaBinaryMaxBytes = 100 * 1024 * 1024

type v1MediaDerivativeDTO struct {
	Status string `json:"status"`
	URL    string `json:"url,omitempty"`
}

type v1MediaAssetDTO struct {
	ID             string                          `json:"id"`
	WorkspaceID    string                          `json:"workspace_id"`
	Status         string                          `json:"status"`
	MediaKind      string                          `json:"media_kind"`
	MIMEType       string                          `json:"mime_type"`
	Width          int32                           `json:"width"`
	Height         int32                           `json:"height"`
	SizeBytes      int64                           `json:"size_bytes"`
	ChecksumSHA256 string                          `json:"checksum_sha256"`
	ContentURL     string                          `json:"content_url"`
	Derivatives    map[string]v1MediaDerivativeDTO `json:"derivatives"`
	ArchivedAt     *string                         `json:"archived_at,omitempty"`
	CreatedAt      string                          `json:"created_at"`
	UpdatedAt      string                          `json:"updated_at"`
}

func v1MediaContentURL(workspacePublicID, assetID string) string {
	return ContractV1GroupPath + "/workspaces/" + workspacePublicID + "/media-assets/" + assetID + "/content"
}

func v1MediaDerivativeURL(workspacePublicID, assetID, variant string) string {
	return ContractV1GroupPath + "/workspaces/" + workspacePublicID + "/media-assets/" + assetID + "/derivatives/" + variant
}

func toMediaAssetDTO(workspacePublicID string, asset *workspaceuploads.Asset) v1MediaAssetDTO {
	dto := v1MediaAssetDTO{
		ID:             asset.ID,
		WorkspaceID:    workspacePublicID,
		Status:         asset.Status,
		MediaKind:      asset.MediaKind,
		MIMEType:       asset.MIMEType,
		Width:          asset.Width,
		Height:         asset.Height,
		SizeBytes:      asset.SizeBytes,
		ChecksumSHA256: asset.ChecksumSHA256,
		ContentURL:     v1MediaContentURL(workspacePublicID, asset.ID),
		Derivatives:    make(map[string]v1MediaDerivativeDTO, len(derivativedomain.TierSpecs)),
		CreatedAt:      v1FormatTime(asset.CreatedAt),
		UpdatedAt:      v1FormatTime(asset.UpdatedAt),
	}
	// Display tiers resolve from stored derivative rows once byte
	// verification lands (#805). Until then every tier is pending
	// unless the asset itself is ready, mirroring the legacy metadata
	// shape (ready tiers carry URLs, others stay pending).
	for tier := range derivativedomain.TierSpecs {
		if tier == derivativedomain.TierProcessingInput {
			continue
		}
		derivative := v1MediaDerivativeDTO{Status: "pending"}
		if asset.Status == "ready" {
			derivative.Status = "ready"
			derivative.URL = v1MediaDerivativeURL(workspacePublicID, asset.ID, tier)
		}
		dto.Derivatives[tier] = derivative
	}
	if asset.ArchivedAt != nil {
		archived := v1FormatTime(*asset.ArchivedAt)
		dto.ArchivedAt = &archived
	}
	return dto
}

func renderMediaStoreError(c *gin.Context, err error) {
	contract.RenderError(c, http.StatusNotFound, contract.CodeMediaNotFound, "媒体不存在")
	c.Abort()
}

// renderV1BinaryError answers binary endpoints with a plain non-JSON
// status: content/derivative endpoints never carry the JSON envelope.
func renderV1BinaryError(c *gin.Context, status int) {
	c.String(status, http.StatusText(status))
	c.Abort()
}

func (h *contractV1Handler) listMediaAssets(c *gin.Context) {
	workspacePublicID, allowed := h.v1UploadWorkspaceContext(c, false)
	if !allowed {
		return
	}
	pageSize, err := contract.ParsePageSize(c.Query(contract.PageSizeParam))
	if err != nil {
		contract.RenderError(c, http.StatusBadRequest, contract.CodeBadRequest, "page_size 非法")
		c.Abort()
		return
	}
	offset := 0
	if raw := strings.TrimSpace(c.Query(contract.CursorParam)); raw != "" {
		cursor, decodeErr := contract.DecodeCursor[v1ListCursor](raw)
		if decodeErr != nil || cursor == nil || cursor.Offset < 0 {
			contract.RenderError(c, http.StatusBadRequest, contract.CodePaginationCursorInvalid, "分页游标非法")
			c.Abort()
			return
		}
		offset = cursor.Offset
	}
	// Over-fetch by one to compute has_more without a count query.
	assets, err := h.deps.WorkspaceAssets.ListAssets(c.Request.Context(), workspacePublicID, pageSize+1, offset)
	if err != nil {
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, "媒体服务暂不可用")
		c.Abort()
		return
	}
	hasMore := len(assets) > pageSize
	if hasMore {
		assets = assets[:pageSize]
	}
	page := make([]v1MediaAssetDTO, 0, len(assets))
	for _, asset := range assets {
		page = append(page, toMediaAssetDTO(workspacePublicID, asset))
	}
	meta := contract.NewPageMeta("", false)
	if hasMore {
		next, encodeErr := contract.EncodeCursor(v1ListCursor{Offset: offset + pageSize})
		if encodeErr != nil {
			contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, "媒体服务暂不可用")
			c.Abort()
			return
		}
		meta = contract.NewPageMeta(next, true)
	}
	contract.RenderPaginated(c, page, meta)
}

func (h *contractV1Handler) getMediaAsset(c *gin.Context) {
	workspacePublicID, allowed := h.v1UploadWorkspaceContext(c, false)
	if !allowed {
		return
	}
	assetID := strings.TrimSpace(c.Param("asset_id"))
	if !contract.IsAssetID(assetID) {
		contract.RenderError(c, http.StatusNotFound, contract.CodeMediaNotFound, "媒体不存在")
		c.Abort()
		return
	}
	asset, err := h.deps.WorkspaceAssets.GetAsset(c.Request.Context(), workspacePublicID, assetID)
	if err != nil {
		renderMediaStoreError(c, err)
		return
	}
	contract.RenderSuccess(c, toMediaAssetDTO(workspacePublicID, asset))
}

func (h *contractV1Handler) archiveMediaAsset(c *gin.Context) {
	identityPrincipal, ok := v1RequestPrincipal(c)
	if !ok {
		return
	}
	workspacePublicID, allowed := h.v1UploadWorkspaceContext(c, true)
	if !allowed {
		return
	}
	assetID := strings.TrimSpace(c.Param("asset_id"))
	if !contract.IsAssetID(assetID) {
		contract.RenderError(c, http.StatusNotFound, contract.CodeMediaNotFound, "媒体不存在")
		c.Abort()
		return
	}
	key, err := contract.IdempotencyKeyFromHeader(c)
	if err != nil {
		contract.RenderError(c, http.StatusBadRequest, contract.CodeIdempotencyKeyRequired, "缺少幂等键，请重试此次操作")
		c.Abort()
		return
	}
	ctx := c.Request.Context()
	externalUser := tenancy.FormatUserPublicID(identityPrincipal.PublicID())
	scope := contract.IdempotencyScope(externalUser, "media:archive")
	hash := contract.HashIdempotencyRequest("media:archive", workspacePublicID, assetID)
	result, _, doErr := h.deps.Idempotency.Do(ctx, scope, identityPrincipal.UserID(), key, hash, func() (any, error) {
		asset, archiveErr := h.deps.WorkspaceAssets.ArchiveAsset(ctx, workspacePublicID, assetID)
		if archiveErr != nil {
			return nil, archiveErr
		}
		// #807: media.asset_archived AFTER the archive commit (inside
		// the once-fn: replays never re-emit).
		h.emitWorkspaceEvent(ctx, identityPrincipal, workspacePublicID, events.EventV1MediaAssetArchived,
			events.V1Resource{Type: events.V1ResourceMediaAsset, ID: asset.ID},
			events.V1MediaAssetArchivedData{AssetID: asset.ID})
		dto := toMediaAssetDTO(workspacePublicID, asset)
		return &dto, nil
	})
	if doErr != nil {
		if contract.IsIdempotencyConflict(doErr) {
			contract.RenderError(c, http.StatusConflict, contract.CodeMediaIdempotencyConflict, "幂等请求与原请求不一致")
			c.Abort()
			return
		}
		renderMediaStoreError(c, doErr)
		return
	}
	dto, ok := result.(*v1MediaAssetDTO)
	if !ok || dto == nil {
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, "媒体服务暂不可用")
		c.Abort()
		return
	}
	contract.RenderSuccess(c, dto)
}

// v1BinaryWorkspaceContext is the plain-status counterpart of
// v1UploadWorkspaceContext for the binary endpoints: every failure
// (including auth and fail-closed paths) renders a non-JSON status so
// content/derivative responses never carry the JSON envelope. Only
// workspace members pass; hidden workspaces and non-members converge
// to 404 (Baseline 4 anti-enumeration).
func (h *contractV1Handler) v1BinaryWorkspaceContext(c *gin.Context) (string, bool) {
	identityPrincipal, ok := principal.From(c)
	if !ok {
		renderV1BinaryError(c, http.StatusUnauthorized)
		return "", false
	}
	if h.deps.Store == nil || h.deps.WorkspaceAssets == nil {
		renderV1BinaryError(c, http.StatusNotImplemented)
		return "", false
	}
	raw := strings.TrimSpace(c.Param("workspace_id"))
	if !contract.IsWorkspaceID(raw) {
		renderV1BinaryError(c, http.StatusNotFound)
		return "", false
	}
	ctx := c.Request.Context()
	workspace, err := h.deps.Store.GetWorkspace(ctx, raw)
	if err != nil {
		renderV1BinaryError(c, http.StatusNotFound)
		return "", false
	}
	_, member, err := h.deps.Store.GetWorkspaceRole(ctx, raw, identityPrincipal.UserID())
	if err != nil || !member {
		renderV1BinaryError(c, http.StatusNotFound)
		return "", false
	}
	return workspace.PublicID, true
}

// v1MediaBinaryAsset resolves the asset for binary endpoints with plain
// (non-envelope) failures. ok=false means a status was already written.
func (h *contractV1Handler) v1MediaBinaryAsset(c *gin.Context, workspacePublicID, assetID string) (*workspaceuploads.Asset, bool) {
	if h.deps.WorkspaceAssets == nil {
		renderV1BinaryError(c, http.StatusNotImplemented)
		return nil, false
	}
	if !contract.IsAssetID(assetID) {
		renderV1BinaryError(c, http.StatusNotFound)
		return nil, false
	}
	asset, err := h.deps.WorkspaceAssets.GetAsset(c.Request.Context(), workspacePublicID, assetID)
	if err != nil {
		renderV1BinaryError(c, http.StatusNotFound)
		return nil, false
	}
	if asset.IsArchived() {
		renderV1BinaryError(c, http.StatusGone)
		return nil, false
	}
	if asset.Status != "ready" {
		renderV1BinaryError(c, http.StatusConflict)
		return nil, false
	}
	return asset, true
}

func (h *contractV1Handler) writeV1MediaBytes(c *gin.Context, profileID, objectKey, mimeType string) {
	if h.deps.MediaObjects == nil {
		renderV1BinaryError(c, http.StatusServiceUnavailable)
		return
	}
	object, err := h.deps.MediaObjects.ReadObjectAtProfile(c.Request.Context(), profileID, objectKey, v1MediaBinaryMaxBytes)
	if err != nil {
		if errors.Is(err, storagedomain.ErrStoredObjectNotFound) {
			renderV1BinaryError(c, http.StatusNotFound)
			return
		}
		renderV1BinaryError(c, http.StatusServiceUnavailable)
		return
	}
	if object == nil || len(object.Data) == 0 {
		renderV1BinaryError(c, http.StatusNotFound)
		return
	}
	mime := strings.TrimSpace(mimeType)
	if mime == "" {
		mime = strings.TrimSpace(object.ContentType)
	}
	if mime == "" {
		mime = "application/octet-stream"
	}
	c.Header("Cache-Control", "private, max-age=3600")
	c.Header("Content-Disposition", "inline")
	c.Data(http.StatusOK, mime, object.Data)
}

func (h *contractV1Handler) contentMediaAsset(c *gin.Context) {
	workspacePublicID, allowed := h.v1BinaryWorkspaceContext(c)
	if !allowed {
		return
	}
	assetID := strings.TrimSpace(c.Param("asset_id"))
	asset, ok := h.v1MediaBinaryAsset(c, workspacePublicID, assetID)
	if !ok {
		return
	}
	h.writeV1MediaBytes(c, asset.StorageProfileID, asset.StorageKey, asset.MIMEType)
}

func (h *contractV1Handler) derivativeMediaAsset(c *gin.Context) {
	workspacePublicID, allowed := h.v1BinaryWorkspaceContext(c)
	if !allowed {
		return
	}
	variant := strings.TrimSpace(c.Param("variant"))
	if _, exists := derivativedomain.TierSpecs[variant]; !exists {
		renderV1BinaryError(c, http.StatusBadRequest)
		return
	}
	assetID := strings.TrimSpace(c.Param("asset_id"))
	asset, ok := h.v1MediaBinaryAsset(c, workspacePublicID, assetID)
	if !ok {
		return
	}
	// Serve the generation-matched stored derivative row. Tiers without
	// a stored row stay 409 until #805 materializes them; there is no
	// imgproxy pull-through on the v1 path in this slice.
	derivatives, err := h.deps.WorkspaceAssets.ListDerivatives(c.Request.Context(), asset.OwnerUserID, asset.InternalID)
	if err != nil {
		renderV1BinaryError(c, http.StatusServiceUnavailable)
		return
	}
	for _, row := range derivatives {
		if row.Tier != variant || row.Status != "ready" || row.SourceObjectGeneration != asset.ObjectGeneration {
			continue
		}
		h.writeV1MediaBytes(c, row.StorageProfileID, row.StorageKey, row.MIMEType)
		return
	}
	renderV1BinaryError(c, http.StatusConflict)
}
