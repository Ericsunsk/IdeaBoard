package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"ideaboard/api/internal/events"
	"ideaboard/api/internal/identity/principal"
	derivativedomain "ideaboard/api/internal/media/derivatives/domain"
	storagedomain "ideaboard/api/internal/media/storage/domain"
	"ideaboard/api/internal/media/workspacemedia"
	"ideaboard/api/internal/platform/contract"
	"ideaboard/api/internal/tenancy"

	"github.com/gin-gonic/gin"
)

// Workspace upload sessions + media assets (issue #803, restored in
// #820 as Phase 4-03 of #809). Additive only: legacy /api/user upload
// and media routes are untouched.
//
// Restored surface (ContractV1GroupPath + PublicContractChain):
//
//	POST /api/v1/workspaces/{workspace_id}/uploads (201)
//	GET  /api/v1/workspaces/{workspace_id}/uploads/{upload_id}
//	POST /api/v1/workspaces/{workspace_id}/uploads/{upload_id}/complete (200 + asset)
//	GET  /api/v1/workspaces/{workspace_id}/media-assets (cursor page_size)
//	GET  /api/v1/workspaces/{workspace_id}/media-assets/{asset_id}
//	GET  /api/v1/workspaces/{workspace_id}/media-assets/{asset_id}/content (bytes)
//	GET  /api/v1/workspaces/{workspace_id}/media-assets/{asset_id}/derivatives/{variant} (bytes)
//	POST /api/v1/workspaces/{workspace_id}/media-assets/{asset_id}/archive (owner/admin)
//
// Binary vs metadata separation: session/list/get/archive speak the
// JSON envelope only; content/derivative serve bytes (never the
// envelope, never storage keys). Errors on the binary endpoints are
// plain non-JSON statuses for the same reason.
//
// Auth model: workspace membership, not per-owner scoping. Any member
// reads and completes; archive needs owner/admin. Hidden workspaces
// converge to 404 workspace_not_found, visible-but-forbidden to 403.
// Resource ids are upload_/asset_ opaque segments: malformed shapes,
// unknown ids, and cross-workspace ids converge to the matching
// *_not_found 404 (Baseline 4 anti-enumeration).
//
// Envelope: contract.RenderCreated/RenderSuccess/RenderPaginated/
// RenderError; cursor pagination via cursor/page_size; create/complete/
// archive require the Idempotency-Key header (scope upload:<op> /
// media:<op>; reuse with a different request hash renders 409
// upload_idempotency_conflict / media_idempotency_conflict).
//
// Production wiring fills ContractV1Deps.Media (+ MediaObjects for
// bytes) when the postgres store lands; nil store fails closed with
// 501, nil objects with 503 on binary reads (metadata keeps working)
// while derivative generation is skipped. Tests inject the in-memory
// store plus fakes.

// WorkspaceMediaStore is the v1 media persistence seam (alias so
// ContractV1Deps stays in one package without import cycles).
type WorkspaceMediaStore = workspacemedia.WorkspaceMediaStore

// WorkspaceMediaObjects is the narrow byte seam the v1 content,
// derivative, and generation paths need. *storage.Service satisfies it
// in production; tests inject fakes. Nil fails binary reads closed
// with 503 and skips generation while metadata endpoints keep working.
type WorkspaceMediaObjects = workspacemedia.DerivativeObjects

// maxCompleteDimension bounds client-declared frame dimensions at
// complete time (no byte inspection in this slice; 0 maps to NULL and
// reads back as 0).
const maxCompleteDimension = int32(32768)

// v1MediaBinaryMaxBytes bounds one binary read (parity with the legacy
// original-bytes path).
const v1MediaBinaryMaxBytes = 100 * 1024 * 1024

func (h *contractV1Handler) registerMediaRoutes(group *gin.RouterGroup) {
	group.POST("/workspaces/:workspace_id/uploads", h.createUploadSession)
	group.GET("/workspaces/:workspace_id/uploads/:upload_id", h.getUploadSession)
	group.POST("/workspaces/:workspace_id/uploads/:upload_id/complete", h.completeUploadSession)
	group.GET("/workspaces/:workspace_id/media-assets", h.listMediaAssets)
	group.GET("/workspaces/:workspace_id/media-assets/:asset_id", h.getMediaAsset)
	group.GET("/workspaces/:workspace_id/media-assets/:asset_id/content", h.contentMediaAsset)
	group.GET("/workspaces/:workspace_id/media-assets/:asset_id/derivatives/:variant", h.derivativeMediaAsset)
	group.POST("/workspaces/:workspace_id/media-assets/:asset_id/archive", h.archiveMediaAsset)
}

// requireMediaStore fails closed with 501 when the v1 media store is
// not wired (parity with requireStore).
func (h *contractV1Handler) requireMediaStore(c *gin.Context) bool {
	if h.deps.Media == nil {
		contract.RenderError(c, http.StatusNotImplemented,
			contract.CodeContractNotImplemented, "The v1 contract surface is not implemented yet")
		c.Abort()
		return false
	}
	return true
}

// v1MediaWorkspaceContext resolves principal + explicit workspace
// membership for upload/media routes. needAdmin restricts to
// owner/admin (archive); plain members may create/complete/read.
// Hidden workspaces converge to 404, visible-but-forbidden to 403.
func (h *contractV1Handler) v1MediaWorkspaceContext(c *gin.Context, needAdmin bool) (string, bool) {
	identityPrincipal, ok := v1RequestPrincipal(c)
	if !ok || !h.requireStore(c) || !h.requireMediaStore(c) {
		return "", false
	}
	workspace, role, visible := h.v1WorkspaceContext(c, identityPrincipal)
	if workspace == nil {
		return "", false
	}
	allowed := visible
	if needAdmin {
		allowed = visible && (role == tenancy.RoleOwner || role == tenancy.RoleAdmin)
	}
	if !RequireWorkspaceCapability(c, visible, allowed) {
		return "", false
	}
	return workspace.PublicID, true
}

type v1UploadTargetDTO struct {
	Kind      string `json:"kind"`
	Slot      string `json:"slot"`
	ExpiresAt string `json:"expires_at"`
}

type v1UploadSessionDTO struct {
	ID             string            `json:"id"`
	WorkspaceID    string            `json:"workspace_id"`
	Status         string            `json:"status"`
	Filename       string            `json:"filename"`
	MIMEType       string            `json:"mime_type"`
	SizeBytes      int64             `json:"size_bytes"`
	ChecksumSHA256 string            `json:"checksum_sha256"`
	MediaKind      string            `json:"media_kind"`
	UploadTarget   v1UploadTargetDTO `json:"upload_target"`
	AssetID        *string           `json:"asset_id,omitempty"`
	CreatedAt      string            `json:"created_at"`
	UpdatedAt      string            `json:"updated_at"`
	ExpiresAt      string            `json:"expires_at"`
}

func toUploadSessionDTO(session *workspacemedia.Session) v1UploadSessionDTO {
	dto := v1UploadSessionDTO{
		ID:             session.ID,
		WorkspaceID:    session.WorkspaceID,
		Status:         session.Status,
		Filename:       session.Filename,
		MIMEType:       session.MIMEType,
		SizeBytes:      session.SizeBytes,
		ChecksumSHA256: session.ChecksumSHA256,
		MediaKind:      session.MediaKind,
		UploadTarget: v1UploadTargetDTO{
			Kind:      "same-origin-slot",
			Slot:      session.ID,
			ExpiresAt: v1FormatTime(session.ExpiresAt),
		},
		CreatedAt: v1FormatTime(session.CreatedAt),
		UpdatedAt: v1FormatTime(session.UpdatedAt),
		ExpiresAt: v1FormatTime(session.ExpiresAt),
	}
	if session.AssetID != "" {
		assetID := session.AssetID
		dto.AssetID = &assetID
	}
	return dto
}

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

func toMediaAssetDTO(workspacePublicID string, asset *workspacemedia.Asset, updatedAt string) v1MediaAssetDTO {
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
		UpdatedAt:      updatedAt,
	}
	// Display tiers resolve from stored derivative rows once generated.
	// Until then every tier is pending unless the asset itself is
	// ready, mirroring the frozen metadata shape (ready tiers carry
	// URLs, others stay pending; the binary endpoint 409s on rows
	// that were never materialized).
	for tier := range derivativedomain.TierSpecs {
		if tier == derivativedomain.TierProcessingInput {
			continue
		}
		derivative := v1MediaDerivativeDTO{Status: "pending"}
		if asset.Status == workspacemedia.AssetStatusReady {
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

func renderUploadStoreError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, workspacemedia.ErrExpired):
		contract.RenderError(c, contract.StatusOf(contract.CodeUploadExpired),
			contract.CodeUploadExpired, "上传会话已过期，请重新创建")
	case errors.Is(err, workspacemedia.ErrStateConflict):
		contract.RenderError(c, contract.StatusOf(contract.CodeUploadStateConflict),
			contract.CodeUploadStateConflict, "上传会话状态不允许此操作")
	default:
		contract.RenderError(c, http.StatusNotFound, contract.CodeUploadNotFound, "上传会话不存在")
	}
	c.Abort()
}

func renderUploadValidationError(c *gin.Context, err error) {
	var validationErr *workspacemedia.ValidationError
	if errors.As(err, &validationErr) {
		fields := make(map[string]string, len(validationErr.Fields))
		for field, message := range validationErr.Fields {
			fields[field] = message
		}
		contract.RenderErrorWithDetails(c, contract.StatusOf(contract.CodeUploadValidationFailed),
			contract.CodeUploadValidationFailed, "上传声明非法",
			contract.FieldErrors{Fields: fields})
		c.Abort()
		return
	}
	renderUploadStoreError(c, err)
}

func renderMediaStoreError(c *gin.Context, err error) {
	contract.RenderError(c, http.StatusNotFound, contract.CodeMediaNotFound, "媒体不存在")
	c.Abort()
}

func renderMediaValidationError(c *gin.Context, err error) {
	var validationErr *workspacemedia.ValidationError
	if errors.As(err, &validationErr) {
		fields := make(map[string]string, len(validationErr.Fields))
		for field, message := range validationErr.Fields {
			fields[field] = message
		}
		contract.RenderErrorWithDetails(c, contract.StatusOf(contract.CodeMediaValidationFailed),
			contract.CodeMediaValidationFailed, "媒体参数非法",
			contract.FieldErrors{Fields: fields})
		c.Abort()
		return
	}
	renderMediaStoreError(c, err)
}

// renderV1BinaryError answers binary endpoints with a plain non-JSON
// status: content/derivative endpoints never carry the JSON envelope.
func renderV1BinaryError(c *gin.Context, status int) {
	c.String(status, http.StatusText(status))
	c.Abort()
}

type v1CreateUploadBody struct {
	Data struct {
		Filename       string `json:"filename"`
		MIMEType       string `json:"mime_type"`
		SizeBytes      int64  `json:"size_bytes"`
		ChecksumSHA256 string `json:"checksum_sha256"`
		MediaKind      string `json:"media_kind"`
	} `json:"data"`
}

func (h *contractV1Handler) createUploadSession(c *gin.Context) {
	identityPrincipal, ok := v1RequestPrincipal(c)
	if !ok {
		return
	}
	workspacePublicID, allowed := h.v1MediaWorkspaceContext(c, false)
	if !allowed {
		return
	}
	key, err := contract.IdempotencyKeyFromHeader(c)
	if err != nil {
		contract.RenderError(c, http.StatusBadRequest, contract.CodeIdempotencyKeyRequired, "缺少幂等键，请重试此次操作")
		c.Abort()
		return
	}
	var body v1CreateUploadBody
	if bindErr := contract.BindStrictJSON(c, &body); bindErr != nil {
		contract.RenderErrorWithDetails(c, http.StatusBadRequest, contract.CodeBadRequest,
			"请求体非法", contract.FieldErrors{Fields: map[string]string{"data": "invalid"}})
		c.Abort()
		return
	}
	filename := strings.TrimSpace(body.Data.Filename)
	mimeType := strings.TrimSpace(body.Data.MIMEType)
	checksum := workspacemedia.NormalizeChecksum(body.Data.ChecksumSHA256)
	kind := strings.TrimSpace(body.Data.MediaKind)
	if kind == "" {
		kind = workspacemedia.MediaKindImage
	}
	if validateErr := workspacemedia.ValidateCreateInput(filename, mimeType, body.Data.SizeBytes, checksum, kind); validateErr != nil {
		renderUploadValidationError(c, validateErr)
		return
	}
	ctx := c.Request.Context()
	externalUser := tenancy.FormatUserPublicID(identityPrincipal.PublicID())
	scope := contract.IdempotencyScope(externalUser, "upload:create")
	hash := contract.HashIdempotencyRequest("upload:create", workspacePublicID, filename,
		workspacemedia.CanonicalMIME(mimeType), strconv.FormatInt(body.Data.SizeBytes, 10), checksum, kind)
	ownerUserID := identityPrincipal.UserID()
	result, _, doErr := h.deps.Idempotency.Do(ctx, scope, ownerUserID, key, hash, func() (any, error) {
		session, createErr := h.deps.Media.CreateSession(ctx, workspacemedia.CreateSessionParams{
			WorkspaceID:    workspacePublicID,
			OwnerUserID:    ownerUserID,
			Filename:       filename,
			MIMEType:       mimeType,
			SizeBytes:      body.Data.SizeBytes,
			ChecksumSHA256: checksum,
			MediaKind:      kind,
		})
		if createErr != nil {
			return nil, createErr
		}
		dto := toUploadSessionDTO(session)
		return &dto, nil
	})
	if doErr != nil {
		if contract.IsIdempotencyConflict(doErr) {
			contract.RenderError(c, http.StatusConflict, contract.CodeUploadIdempotencyConflict, "幂等请求与原请求不一致")
			c.Abort()
			return
		}
		renderUploadValidationError(c, doErr)
		return
	}
	dto, ok := result.(*v1UploadSessionDTO)
	if !ok || dto == nil {
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, "上传服务暂不可用")
		c.Abort()
		return
	}
	contract.RenderCreated(c, dto)
}

func (h *contractV1Handler) getUploadSession(c *gin.Context) {
	workspacePublicID, allowed := h.v1MediaWorkspaceContext(c, false)
	if !allowed {
		return
	}
	uploadID := strings.TrimSpace(c.Param("upload_id"))
	if !contract.IsUploadID(uploadID) {
		contract.RenderError(c, http.StatusNotFound, contract.CodeUploadNotFound, "上传会话不存在")
		c.Abort()
		return
	}
	session, err := h.deps.Media.GetSession(c.Request.Context(), workspacePublicID, uploadID)
	if err != nil {
		renderUploadStoreError(c, err)
		return
	}
	dto := toUploadSessionDTO(session)
	contract.RenderSuccess(c, dto)
}

type v1CompleteUploadBody struct {
	Data struct {
		ChecksumSHA256 string `json:"checksum_sha256"`
		SizeBytes      int64  `json:"size_bytes"`
		MIMEType       string `json:"mime_type"`
		MediaKind      string `json:"media_kind"`
		Width          int32  `json:"width"`
		Height         int32  `json:"height"`
	} `json:"data"`
}

type v1CompleteUploadDTO struct {
	Upload v1UploadSessionDTO `json:"upload"`
	Asset  v1MediaAssetDTO    `json:"asset"`
}

func (h *contractV1Handler) completeUploadSession(c *gin.Context) {
	identityPrincipal, ok := v1RequestPrincipal(c)
	if !ok {
		return
	}
	workspacePublicID, allowed := h.v1MediaWorkspaceContext(c, false)
	if !allowed {
		return
	}
	uploadID := strings.TrimSpace(c.Param("upload_id"))
	if !contract.IsUploadID(uploadID) {
		contract.RenderError(c, http.StatusNotFound, contract.CodeUploadNotFound, "上传会话不存在")
		c.Abort()
		return
	}
	key, err := contract.IdempotencyKeyFromHeader(c)
	if err != nil {
		contract.RenderError(c, http.StatusBadRequest, contract.CodeIdempotencyKeyRequired, "缺少幂等键，请重试此次操作")
		c.Abort()
		return
	}
	var body v1CompleteUploadBody
	if bindErr := contract.BindStrictJSON(c, &body); bindErr != nil {
		contract.RenderErrorWithDetails(c, http.StatusBadRequest, contract.CodeBadRequest,
			"请求体非法", contract.FieldErrors{Fields: map[string]string{"data": "invalid"}})
		c.Abort()
		return
	}
	checksum := workspacemedia.NormalizeChecksum(body.Data.ChecksumSHA256)
	if !workspacemedia.IsValidChecksum(checksum) {
		renderUploadValidationError(c, workspacemedia.NewFieldValidationError("checksum_sha256"))
		return
	}
	if body.Data.SizeBytes < 1 || body.Data.SizeBytes > workspacemedia.MaxUploadSizeBytes {
		renderUploadValidationError(c, workspacemedia.NewFieldValidationError("size_bytes"))
		return
	}
	if body.Data.Width < 0 || body.Data.Width > maxCompleteDimension ||
		body.Data.Height < 0 || body.Data.Height > maxCompleteDimension {
		renderUploadValidationError(c, workspacemedia.NewFieldValidationError("dimensions"))
		return
	}
	ctx := c.Request.Context()
	externalUser := tenancy.FormatUserPublicID(identityPrincipal.PublicID())
	scope := contract.IdempotencyScope(externalUser, "upload:complete")
	hash := contract.HashIdempotencyRequest("upload:complete", workspacePublicID, uploadID, checksum,
		strconv.FormatInt(body.Data.SizeBytes, 10), strings.TrimSpace(body.Data.MIMEType), strings.TrimSpace(body.Data.MediaKind),
		strconv.FormatInt(int64(body.Data.Width), 10), strconv.FormatInt(int64(body.Data.Height), 10))
	result, _, doErr := h.deps.Idempotency.Do(ctx, scope, identityPrincipal.UserID(), key, hash, func() (any, error) {
		return h.completeUploadOnce(ctx, workspacePublicID, uploadID, identityPrincipal, workspacemedia.CompleteFacts{
			ChecksumSHA256: checksum,
			SizeBytes:      body.Data.SizeBytes,
			MIMEType:       strings.TrimSpace(body.Data.MIMEType),
			MediaKind:      strings.TrimSpace(body.Data.MediaKind),
			Width:          body.Data.Width,
			Height:         body.Data.Height,
		})
	})
	if doErr != nil {
		if contract.IsIdempotencyConflict(doErr) {
			contract.RenderError(c, http.StatusConflict, contract.CodeUploadIdempotencyConflict, "幂等请求与原请求不一致")
			c.Abort()
			return
		}
		renderUploadValidationError(c, doErr)
		return
	}
	dto, ok := result.(*v1CompleteUploadDTO)
	if !ok || dto == nil {
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, "上传服务暂不可用")
		c.Abort()
		return
	}
	contract.RenderSuccess(c, dto)
}

// v1MediaRenderer renders display derivatives during completion. The
// production renderer adapts the derivatives rendering seam; nil (or a
// missing byte backend) skips generation while the asset stays ready
// with pending tiers.
type v1MediaRenderer interface {
	Render(ctx context.Context, data []byte, sourceName string, maxSide int) ([]byte, int, int, string, error)
}

func (h *contractV1Handler) completeUploadOnce(ctx context.Context, workspacePublicID, uploadID string, identityPrincipal principal.Principal, facts workspacemedia.CompleteFacts) (any, error) {
	assetPublicID, err := workspacemedia.NewAssetPublicID()
	if err != nil {
		return nil, err
	}
	completed, asset, err := h.deps.Media.CompleteSession(ctx, workspacePublicID, uploadID, facts, workspacemedia.CreateAssetParams{
		WorkspaceID: workspacePublicID,
		OwnerUserID: identityPrincipal.UserID(),
		PublicID:    assetPublicID,
	})
	if err != nil {
		return nil, err
	}
	// #807: media.asset_ready AFTER the durable asset row + session
	// completion commit (inside the once-fn: replays never re-emit).
	h.emitWorkspaceEvent(ctx, identityPrincipal, workspacePublicID, events.EventV1MediaAssetReady,
		events.V1Resource{Type: events.V1ResourceMediaAsset, ID: asset.ID},
		events.V1MediaAssetReadyData{AssetID: asset.ID})
	// Best-effort derivative generation: failures never fail the
	// completion itself (tiers without rows stay pending). The renderer
	// seam is optional; without it (or without readable original
	// bytes) generation is skipped silently.
	h.generateMediaDerivativesBestEffort(ctx, workspacePublicID, asset)
	uploadDTO := toUploadSessionDTO(completed)
	dto := &v1CompleteUploadDTO{Upload: uploadDTO, Asset: toMediaAssetDTO(workspacePublicID, asset, v1FormatTime(completed.UpdatedAt))}
	return dto, nil
}

// generateMediaDerivativesBestEffort runs sequential derivative
// generation after a successful completion. Every failure mode (nil
// seams, missing bytes, render errors) is absorbed: the asset is
// already ready and unmaterialized tiers stay pending.
func (h *contractV1Handler) generateMediaDerivativesBestEffort(ctx context.Context, workspacePublicID string, asset *workspacemedia.Asset) {
	if h == nil || h.deps.Media == nil || h.deps.MediaObjects == nil || h.deps.MediaRenderer == nil {
		return
	}
	_, _ = workspacemedia.EnsureDerivatives(ctx, h.deps.Media, h.deps.MediaObjects, h.deps.MediaRenderer, workspacePublicID, asset)
}

func (h *contractV1Handler) listMediaAssets(c *gin.Context) {
	workspacePublicID, allowed := h.v1MediaWorkspaceContext(c, false)
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
	assets, err := h.deps.Media.ListAssets(c.Request.Context(), workspacePublicID, pageSize+1, offset)
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
		page = append(page, toMediaAssetDTO(workspacePublicID, asset, v1FormatTime(asset.CreatedAt)))
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
	workspacePublicID, allowed := h.v1MediaWorkspaceContext(c, false)
	if !allowed {
		return
	}
	assetID := strings.TrimSpace(c.Param("asset_id"))
	if !contract.IsAssetID(assetID) {
		contract.RenderError(c, http.StatusNotFound, contract.CodeMediaNotFound, "媒体不存在")
		c.Abort()
		return
	}
	asset, err := h.deps.Media.GetAsset(c.Request.Context(), workspacePublicID, assetID)
	if err != nil {
		renderMediaStoreError(c, err)
		return
	}
	contract.RenderSuccess(c, toMediaAssetDTO(workspacePublicID, asset, v1FormatTime(asset.CreatedAt)))
}

func (h *contractV1Handler) archiveMediaAsset(c *gin.Context) {
	identityPrincipal, ok := v1RequestPrincipal(c)
	if !ok {
		return
	}
	workspacePublicID, allowed := h.v1MediaWorkspaceContext(c, true)
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
		asset, archiveErr := h.deps.Media.ArchiveAsset(ctx, workspacePublicID, assetID)
		if archiveErr != nil {
			return nil, archiveErr
		}
		// #807: media.asset_archived AFTER the archive commit (inside
		// the once-fn: replays never re-emit).
		h.emitWorkspaceEvent(ctx, identityPrincipal, workspacePublicID, events.EventV1MediaAssetArchived,
			events.V1Resource{Type: events.V1ResourceMediaAsset, ID: asset.ID},
			events.V1MediaAssetArchivedData{AssetID: asset.ID})
		dto := toMediaAssetDTO(workspacePublicID, asset, v1FormatTime(asset.CreatedAt))
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
// v1MediaWorkspaceContext for the binary endpoints: every failure
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
	if h.deps.Store == nil || h.deps.Media == nil {
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
func (h *contractV1Handler) v1MediaBinaryAsset(c *gin.Context, workspacePublicID, assetID string) (*workspacemedia.Asset, bool) {
	if h.deps.Media == nil {
		renderV1BinaryError(c, http.StatusNotImplemented)
		return nil, false
	}
	if !contract.IsAssetID(assetID) {
		renderV1BinaryError(c, http.StatusNotFound)
		return nil, false
	}
	asset, err := h.deps.Media.GetAsset(c.Request.Context(), workspacePublicID, assetID)
	if err != nil {
		renderV1BinaryError(c, http.StatusNotFound)
		return nil, false
	}
	if asset.IsArchived() {
		renderV1BinaryError(c, http.StatusGone)
		return nil, false
	}
	if asset.Status != workspacemedia.AssetStatusReady {
		renderV1BinaryError(c, http.StatusConflict)
		return nil, false
	}
	return asset, true
}

func (h *contractV1Handler) writeV1MediaBytes(c *gin.Context, objectKey, mimeType string) {
	if h.deps.MediaObjects == nil {
		renderV1BinaryError(c, http.StatusServiceUnavailable)
		return
	}
	object, err := h.deps.MediaObjects.ReadObjectAtProfile(c.Request.Context(), workspacemedia.DefaultStorageProfile, objectKey, v1MediaBinaryMaxBytes)
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
	h.writeV1MediaBytes(c, asset.StorageKey, asset.MIMEType)
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
	if variant == derivativedomain.TierProcessingInput {
		renderV1BinaryError(c, http.StatusBadRequest)
		return
	}
	assetID := strings.TrimSpace(c.Param("asset_id"))
	asset, ok := h.v1MediaBinaryAsset(c, workspacePublicID, assetID)
	if !ok {
		return
	}
	// Serve the generation-matched stored derivative row. Tiers without
	// a stored row stay 409 until generation materializes them; there
	// is no imgproxy pull-through on the v1 path in this slice.
	derivatives, err := h.deps.Media.ListDerivatives(c.Request.Context(), workspacePublicID, asset.ID)
	if err != nil {
		renderV1BinaryError(c, http.StatusServiceUnavailable)
		return
	}
	for _, derivative := range derivatives {
		if derivative.VariantName != variant {
			continue
		}
		h.writeV1MediaBytes(c, derivative.StorageKey, derivative.MIMEType)
		return
	}
	renderV1BinaryError(c, http.StatusConflict)
}

// OpenAPI deltas for the merger (DO NOT edit apps/api/openapi*.yaml here).
// Add under paths (tags: WorkspaceMedia), reusing components parameters
// Cursor/PageSize/WorkspaceId/UploadId/AssetId/IdempotencyKey/RequestId and responses
// BadRequest/Unauthorized/Forbidden/NotFound/Conflict/Gone/UnprocessableEntity:
//
//	/workspaces/{workspace_id}/uploads:
//	  post: {operationId: createWorkspaceUpload, parameters: [WorkspaceId, IdempotencyKey]}
//	/workspaces/{workspace_id}/uploads/{upload_id}:
//	  get: {operationId: getWorkspaceUpload, parameters: [WorkspaceId, UploadId]}
//	/workspaces/{workspace_id}/uploads/{upload_id}/complete:
//	  post: {operationId: completeWorkspaceUpload, parameters: [WorkspaceId, UploadId, IdempotencyKey]}
//	/workspaces/{workspace_id}/media-assets:
//	  get: {operationId: listWorkspaceMediaAssets, parameters: [WorkspaceId, Cursor, PageSize]}
//	/workspaces/{workspace_id}/media-assets/{asset_id}:
//	  get: {operationId: getWorkspaceMediaAsset, parameters: [WorkspaceId, AssetId]}
//	/workspaces/{workspace_id}/media-assets/{asset_id}/content:
//	  get: {operationId: contentWorkspaceMediaAsset, parameters: [WorkspaceId, AssetId]}
//	/workspaces/{workspace_id}/media-assets/{asset_id}/derivatives/{variant}:
//	  get: {operationId: derivativeWorkspaceMediaAsset, parameters: [WorkspaceId, AssetId, {name: variant, in: path, required: true, schema: {type: string, enum: [thumb, preview, large-preview]}}]}
//	/workspaces/{workspace_id}/media-assets/{asset_id}/archive:
//	  post: {operationId: archiveWorkspaceMediaAsset, parameters: [WorkspaceId, AssetId, IdempotencyKey]}
//
// Schemas (UploadSession/MediaAsset/UploadId/MediaAssetId and request
// bodies) already exist in openapi.public.yaml from #803; the paths
// above were removed with the surface in #813 and return here unchanged.
//
// Error codes frozen: upload_not_found (404), upload_expired (410),
// upload_state_conflict (409), upload_idempotency_conflict (409),
// upload_validation_failed (422), media_not_found (404),
// media_not_ready (409), media_archived (410),
// media_idempotency_conflict (409), media_validation_failed (422);
// reused kernel codes workspace_not_found/workspace_forbidden/
// pagination_cursor_invalid/bad_request/idempotency_key_required/
// service_unavailable.
