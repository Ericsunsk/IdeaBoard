package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"ideaboard/api/internal/events"
	"ideaboard/api/internal/identity/principal"
	"ideaboard/api/internal/media/workspaceuploads"
	"ideaboard/api/internal/platform/contract"
	"ideaboard/api/internal/tenancy"

	"github.com/gin-gonic/gin"
)

// Workspace upload sessions (issue #803). Additive only: legacy
// /api/user upload and media routes are untouched (removal is #809).
//
// Binary vs metadata separation: the session endpoints below speak the
// JSON envelope only. Byte transfer rides the opaque same-origin slot
// named in upload_target (no bucket/secret/path ever leaves the server);
// the byte PUT + presigned variants land with #805.

// maxCompleteDimension bounds client-declared frame dimensions at
// complete time (no byte inspection in this slice; the storage
// integrity gate verifies real dimensions with #805).
const maxCompleteDimension = int32(32768)

// requireUploadStores fails closed with 501 when the v1 upload/media
// stores are not wired (parity with requireStore).
func (h *contractV1Handler) requireUploadStores(c *gin.Context) bool {
	if h.deps.UploadSessions == nil || h.deps.WorkspaceAssets == nil {
		contract.RenderError(c, http.StatusNotImplemented,
			contract.CodeContractNotImplemented, "The v1 contract surface is not implemented yet")
		c.Abort()
		return false
	}
	return true
}

// v1UploadWorkspaceContext resolves principal + explicit workspace
// membership for upload/media routes. needAdmin restricts to
// owner/admin (archive); plain members may create/complete/read.
// Hidden workspaces converge to 404, visible-but-forbidden to 403.
func (h *contractV1Handler) v1UploadWorkspaceContext(c *gin.Context, needAdmin bool) (string, bool) {
	identityPrincipal, ok := v1RequestPrincipal(c)
	if !ok || !h.requireStore(c) || !h.requireUploadStores(c) {
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

func toUploadSessionDTO(session *workspaceuploads.Session) v1UploadSessionDTO {
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

func renderUploadStoreError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, workspaceuploads.ErrExpired):
		contract.RenderError(c, contract.StatusOf(contract.CodeUploadExpired),
			contract.CodeUploadExpired, "上传会话已过期，请重新创建")
	case errors.Is(err, workspaceuploads.ErrStateConflict):
		contract.RenderError(c, contract.StatusOf(contract.CodeUploadStateConflict),
			contract.CodeUploadStateConflict, "上传会话状态不允许此操作")
	default:
		contract.RenderError(c, http.StatusNotFound, contract.CodeUploadNotFound, "上传会话不存在")
	}
	c.Abort()
}

func renderUploadValidationError(c *gin.Context, err error) {
	var validationErr *workspaceuploads.ValidationError
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
	workspacePublicID, allowed := h.v1UploadWorkspaceContext(c, false)
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
	checksum := workspaceuploads.NormalizeChecksum(body.Data.ChecksumSHA256)
	kind := strings.TrimSpace(body.Data.MediaKind)
	if kind == "" {
		kind = workspaceuploads.MediaKindImage
	}
	if validateErr := workspaceuploads.ValidateCreateInput(filename, mimeType, body.Data.SizeBytes, checksum, kind); validateErr != nil {
		renderUploadValidationError(c, validateErr)
		return
	}
	ctx := c.Request.Context()
	externalUser := tenancy.FormatUserPublicID(identityPrincipal.PublicID())
	scope := contract.IdempotencyScope(externalUser, "upload:create")
	hash := contract.HashIdempotencyRequest("upload:create", workspacePublicID, filename,
		workspaceuploads.CanonicalMIME(mimeType), strconv.FormatInt(body.Data.SizeBytes, 10), checksum, kind)
	ownerUserID := identityPrincipal.UserID()
	result, _, doErr := h.deps.Idempotency.Do(ctx, scope, ownerUserID, key, hash, func() (any, error) {
		session, createErr := h.deps.UploadSessions.CreateSession(ctx, workspaceuploads.CreateSessionParams{
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
	workspacePublicID, allowed := h.v1UploadWorkspaceContext(c, false)
	if !allowed {
		return
	}
	uploadID := strings.TrimSpace(c.Param("upload_id"))
	if !contract.IsUploadID(uploadID) {
		contract.RenderError(c, http.StatusNotFound, contract.CodeUploadNotFound, "上传会话不存在")
		c.Abort()
		return
	}
	session, err := h.deps.UploadSessions.GetSession(c.Request.Context(), workspacePublicID, uploadID)
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
	workspacePublicID, allowed := h.v1UploadWorkspaceContext(c, false)
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
	checksum := workspaceuploads.NormalizeChecksum(body.Data.ChecksumSHA256)
	if !workspaceuploads.IsValidChecksum(checksum) {
		renderUploadValidationError(c, workspaceuploads.NewFieldValidationError("checksum_sha256"))
		return
	}
	if body.Data.SizeBytes < 1 || body.Data.SizeBytes > workspaceuploads.MaxUploadSizeBytes {
		renderUploadValidationError(c, workspaceuploads.NewFieldValidationError("size_bytes"))
		return
	}
	if body.Data.Width < 0 || body.Data.Width > maxCompleteDimension ||
		body.Data.Height < 0 || body.Data.Height > maxCompleteDimension {
		renderUploadValidationError(c, workspaceuploads.NewFieldValidationError("dimensions"))
		return
	}
	ctx := c.Request.Context()
	externalUser := tenancy.FormatUserPublicID(identityPrincipal.PublicID())
	scope := contract.IdempotencyScope(externalUser, "upload:complete")
	hash := contract.HashIdempotencyRequest("upload:complete", workspacePublicID, uploadID, checksum,
		strconv.FormatInt(body.Data.SizeBytes, 10), strings.TrimSpace(body.Data.MIMEType), strings.TrimSpace(body.Data.MediaKind),
		strconv.FormatInt(int64(body.Data.Width), 10), strconv.FormatInt(int64(body.Data.Height), 10))
	result, _, doErr := h.deps.Idempotency.Do(ctx, scope, identityPrincipal.UserID(), key, hash, func() (any, error) {
		return h.completeUploadOnce(ctx, workspacePublicID, uploadID, identityPrincipal, v1CompleteFacts{
			checksum:  checksum,
			sizeBytes: body.Data.SizeBytes,
			mimeType:  strings.TrimSpace(body.Data.MIMEType),
			kind:      strings.TrimSpace(body.Data.MediaKind),
			width:     body.Data.Width,
			height:    body.Data.Height,
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

// v1CompleteFacts carries the client-declared completion facts validated
// against the session declarations.
type v1CompleteFacts struct {
	checksum  string
	sizeBytes int64
	mimeType  string
	kind      string
	width     int32
	height    int32
}

func (h *contractV1Handler) completeUploadOnce(ctx context.Context, workspacePublicID, uploadID string, identityPrincipal principal.Principal, facts v1CompleteFacts) (any, error) {
	session, err := h.deps.UploadSessions.GetSession(ctx, workspacePublicID, uploadID)
	if err != nil {
		return nil, err
	}
	if session.Status != workspaceuploads.StatusPending && session.Status != workspaceuploads.StatusTransferring {
		return nil, workspaceuploads.ErrStateConflict
	}
	if mismatch := matchCompletionFacts(session, facts); mismatch != nil {
		_, _ = h.deps.UploadSessions.FailSession(ctx, workspacePublicID, uploadID, "completion facts do not match session declarations")
		return nil, mismatch
	}
	asset, err := h.deps.WorkspaceAssets.CreateAsset(ctx, workspaceuploads.CreateAssetParams{
		WorkspaceID:      workspacePublicID,
		OwnerUserID:      session.OwnerUserID,
		MediaKind:        session.MediaKind,
		MIMEType:         session.MIMEType,
		Width:            facts.width,
		Height:           facts.height,
		SizeBytes:        session.SizeBytes,
		ChecksumSHA256:   session.ChecksumSHA256,
		StorageProfileID: workspaceuploads.DefaultStorageProfile,
		StorageKey:       workspaceuploads.SlotKey(workspacePublicID, uploadID, session.Filename),
	})
	if err != nil {
		_, _ = h.deps.UploadSessions.FailSession(ctx, workspacePublicID, uploadID, "media asset creation failed")
		return nil, err
	}
	completed, err := h.deps.UploadSessions.CompleteSession(ctx, workspacePublicID, uploadID, asset.ID)
	if err != nil {
		return nil, err
	}
	// #807: media.asset_ready AFTER the durable asset row + session
	// completion commit (inside the once-fn: replays never re-emit).
	h.emitWorkspaceEvent(ctx, identityPrincipal, workspacePublicID, events.EventV1MediaAssetReady,
		events.V1Resource{Type: events.V1ResourceMediaAsset, ID: asset.ID},
		events.V1MediaAssetReadyData{AssetID: asset.ID})
	uploadDTO := toUploadSessionDTO(completed)
	dto := &v1CompleteUploadDTO{Upload: uploadDTO, Asset: toMediaAssetDTO(workspacePublicID, asset)}
	return dto, nil
}

// matchCompletionFacts enforces the checksum/size/kind contract: the
// completion echo must equal the session declarations byte-for-byte
// (after canonicalization). MIME/kind echoes must match when present.
func matchCompletionFacts(session *workspaceuploads.Session, facts v1CompleteFacts) error {
	if facts.checksum != session.ChecksumSHA256 {
		return workspaceuploads.NewFieldValidationError("checksum_sha256")
	}
	if facts.sizeBytes != session.SizeBytes {
		return workspaceuploads.NewFieldValidationError("size_bytes")
	}
	if facts.mimeType != "" && workspaceuploads.CanonicalMIME(facts.mimeType) != session.MIMEType {
		return workspaceuploads.NewFieldValidationError("mime_type")
	}
	if facts.kind != "" && strings.TrimSpace(facts.kind) != session.MediaKind {
		return workspaceuploads.NewFieldValidationError("media_kind")
	}
	return nil
}
