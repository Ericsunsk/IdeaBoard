package v1

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	runtimehttp "ideaboard/api/internal/runtime/interfaces/http"
	"io"
	"net/http"
	"strconv"
	"strings"

	"ideaboard/api/internal/boards"
	"ideaboard/api/internal/events"
	"ideaboard/api/internal/identity/principal"
	identityports "ideaboard/api/internal/identity/users/ports"
	derivativedomain "ideaboard/api/internal/media/derivatives/domain"
	"ideaboard/api/internal/media/imageformat"
	storagedomain "ideaboard/api/internal/media/storage/domain"
	"ideaboard/api/internal/media/workspacemedia"
	"ideaboard/api/internal/platform/contract"
	"ideaboard/api/internal/tenancy"

	"github.com/gin-gonic/gin"
)

// Workspace upload and media endpoints enforce membership for reads and completion,
// and owner/admin access for archival. Upload completion verifies the staged bytes
// against declared size, checksum, and MIME before making the asset ready.
// Original content is read from storage; display tiers render on demand via imgproxy.

// WorkspaceMediaStore is the v1 media persistence seam (alias so
// ContractV1Deps stays in one package without import cycles).
type WorkspaceMediaStore = workspacemedia.WorkspaceMediaStore

// WorkspaceMediaObjects stores and reads workspace original bytes.
type WorkspaceMediaObjects = workspacemedia.MediaObjects

// maxCompleteDimension bounds client-declared frame dimensions at complete time.
const maxCompleteDimension = int32(32768)

// v1MediaBinaryMaxBytes bounds each original image response.
const v1MediaBinaryMaxBytes = 100 * 1024 * 1024

// maxV1MediaBatchAssets bounds one batch metadata read (bounded input:
// the per-id lookups stay a single-digit-millisecond fan-out, never an
// unbounded scan).
const maxV1MediaBatchAssets = 50

// V1AvatarUIDDirectory resolves a caller's storage UID for the
// user-scoped avatar PUT. Production adapts the identity
// UserRepository via NewV1AvatarDirectory; tests inject fakes.
type V1AvatarUIDDirectory interface {
	GetStorageUIDByUserID(ctx context.Context, userID int64) (string, error)
}

// avatarUserDirectory adapts identityports.UserRepository to
// V1AvatarUIDDirectory for composition wiring.
type avatarUserDirectory struct {
	repo identityports.UserRepository
}

// NewV1AvatarDirectory exposes the production V1AvatarUIDDirectory
// adapter for composition wiring outside this package.
func NewV1AvatarDirectory(repo identityports.UserRepository) V1AvatarUIDDirectory {
	return avatarUserDirectory{repo: repo}
}

func (a avatarUserDirectory) GetStorageUIDByUserID(ctx context.Context, userID int64) (string, error) {
	if a.repo == nil || userID <= 0 {
		return "", errV1AvatarUIDUnavailable
	}
	user, err := a.repo.GetByID(ctx, userID)
	if err != nil || user == nil || strings.TrimSpace(user.StorageUID) == "" {
		return "", errV1AvatarUIDUnavailable
	}
	return user.StorageUID, nil
}

// errV1AvatarUIDUnavailable reports avatar storage-UID resolution
// failures (503, fail closed without leaking which half failed).
var errV1AvatarUIDUnavailable = errors.New("v1 avatar uid unavailable")

// errV1MediaBytesStoreUnavailable reports byte-PUT storage failures
// (503: the bytes were valid but the backend did not persist them).
var errV1MediaBytesStoreUnavailable = errors.New("v1 upload bytes store unavailable")

// errV1MediaStagedReadUnavailable reports transient staged-byte read
// failures at complete time (503: complete-after-bytes cannot be
// proven, so no asset is materialized).
var errV1MediaStagedReadUnavailable = errors.New("v1 staged upload bytes unavailable")

func (h *contractV1Handler) registerMediaRoutes(group *gin.RouterGroup) {
	group.POST("/workspaces/:workspace_id/uploads", h.createUploadSession)
	group.GET("/workspaces/:workspace_id/uploads/:upload_id", h.getUploadSession)
	group.PUT("/workspaces/:workspace_id/uploads/:upload_id/bytes", h.putUploadBytes)
	group.POST("/workspaces/:workspace_id/uploads/:upload_id/complete", h.completeUploadSession)
	group.GET("/workspaces/:workspace_id/media-assets", h.listMediaAssets)
	group.POST("/workspaces/:workspace_id/media-assets/batch", h.batchGetMediaAssets)
	group.GET("/workspaces/:workspace_id/media-assets/:asset_id", h.getMediaAsset)
	group.GET("/workspaces/:workspace_id/media-assets/:asset_id/content", h.contentMediaAsset)
	group.GET("/workspaces/:workspace_id/media-assets/:asset_id/derivatives/:variant", h.derivativeMediaAsset)
	group.POST("/workspaces/:workspace_id/media-assets/:asset_id/archive", h.archiveMediaAsset)
	group.PUT("/user/avatar", h.uploadAvatar)
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
	if !runtimehttp.RequireWorkspaceCapability(c, visible, allowed) {
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
		Status:         string(session.Status),
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
	return runtimehttp.ContractV1GroupPath + "/workspaces/" + workspacePublicID + "/media-assets/" + assetID + "/content"
}

func v1MediaDerivativeURL(workspacePublicID, assetID, variant string) string {
	return runtimehttp.ContractV1GroupPath + "/workspaces/" + workspacePublicID + "/media-assets/" + assetID + "/derivatives/" + variant
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
	// Display tiers become available when their source asset is ready.
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
			contract.CodeUploadExpired, runtimehttp.MsgUploadExpired)
	case errors.Is(err, workspacemedia.ErrStateConflict):
		contract.RenderError(c, contract.StatusOf(contract.CodeUploadStateConflict),
			contract.CodeUploadStateConflict, runtimehttp.MsgUploadStateConflict)
	default:
		contract.RenderError(c, http.StatusNotFound, contract.CodeUploadNotFound, runtimehttp.MsgUploadNotFound)
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
			contract.CodeUploadValidationFailed, runtimehttp.MsgUploadValidationFailed,
			contract.FieldErrors{Fields: fields})
		c.Abort()
		return
	}
	renderUploadStoreError(c, err)
}

func renderMediaStoreError(c *gin.Context, err error) {
	contract.RenderError(c, http.StatusNotFound, contract.CodeMediaNotFound, runtimehttp.MsgMediaNotFound)
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
			contract.CodeMediaValidationFailed, runtimehttp.MsgMediaValidationFailed,
			contract.FieldErrors{Fields: fields})
		c.Abort()
		return
	}
	renderMediaStoreError(c, err)
}

// renderMediaUnsupportedType renders 415 for bytes whose sniffed format is
// unknown to the upload allowlist (heic, svg, truncated headers, …).
// Mismatched-but-known formats stay 422 through the validation renderers.
func renderMediaUnsupportedType(c *gin.Context, field string) {
	contract.RenderErrorWithDetails(c, contract.StatusOf(contract.CodeMediaUnsupportedType),
		contract.CodeMediaUnsupportedType, runtimehttp.MsgMediaValidationFailed,
		contract.FieldErrors{Fields: map[string]string{field: "unsupported"}})
	c.Abort()
}

// mediaBoards exposes the narrow #936 board-access port consumed by asset
// authorization: h.deps.Boards satisfies workspacemedia.BoardAccess
// structurally (GetBoardRole + ListBoardsReferencingAsset). A nil store
// denies board-sourced reads (fail closed) while owner/library/task
// sources still evaluate; production always wires Boards.
func (h *contractV1Handler) mediaBoards() workspacemedia.BoardAccess {
	if h.deps.Boards == nil {
		return nil
	}
	return h.deps.Boards
}

// authorizeMediaAsset enforces #936 asset authorization AFTER workspace
// membership (the withWorkspace prelude or v1BinaryWorkspaceContext) and
// BEFORE any metadata, bytes, derivative-render, shared-cache, or 304
// handling. Every denial — unknown id, cross-workspace id, non-member,
// or member without an independent source — renders media_not_found 404
// (Baseline 4: ids and URLs are not credentials, existence never leaks).
// ok=false means a status was already written.
func (h *contractV1Handler) authorizeMediaAsset(c *gin.Context, workspacePublicID, assetID string, userID int64, workspaceRole string) (*workspacemedia.Asset, bool) {
	if h.deps.Media == nil {
		contract.RenderError(c, http.StatusNotImplemented,
			contract.CodeContractNotImplemented, "The v1 contract surface is not implemented yet")
		c.Abort()
		return nil, false
	}
	asset, err := h.deps.Media.AuthorizeAsset(c.Request.Context(), workspacePublicID, assetID, userID, workspaceRole, h.mediaBoards())
	if err != nil {
		renderMediaStoreError(c, err)
		return nil, false
	}
	return asset, true
}

// authorizeMediaAssetBytes is the plain-status counterpart of
// authorizeMediaAsset for the binary endpoints: every failure (including
// authorization) renders a non-JSON status so content/derivative responses
// never carry the JSON envelope. ok=false means a status was already
// written.
func (h *contractV1Handler) authorizeMediaAssetBytes(c *gin.Context, workspacePublicID, assetID string, userID int64, workspaceRole string) (*workspacemedia.Asset, bool) {
	if h.deps.Media == nil {
		runtimehttp.RenderV1BinaryError(c, http.StatusNotImplemented)
		return nil, false
	}
	asset, err := h.deps.Media.AuthorizeAsset(c.Request.Context(), workspacePublicID, assetID, userID, workspaceRole, h.mediaBoards())
	if err != nil {
		runtimehttp.RenderV1BinaryError(c, http.StatusNotFound)
		return nil, false
	}
	return asset, true
}

// Note (plane split): RenderV1BinaryError/WriteRenderedImage/WriteRenderedImageBytes
// live in the runtimehttp kernel (media_previews.go); v1 calls runtimehttp.*.

type v1CreateUploadBody struct {
	Data struct {
		Filename       string `json:"filename"`
		MIMEType       string `json:"mime_type"`
		SizeBytes      int64  `json:"size_bytes"`
		ChecksumSHA256 string `json:"checksum_sha256"`
		MediaKind      string `json:"media_kind"`
		// BoardID optionally binds the upload to the board it is captured
		// for (#936). Verified against the creator's board write access
		// below — never trusted from the client. Empty means no binding
		// (e.g. the board is not autosaved yet): the asset stays
		// owner-only until another source grants wider access.
		BoardID string `json:"board_id,omitempty"`
	} `json:"data"`
}

func (h *contractV1Handler) createUploadSession(c *gin.Context) {
	h.withWorkspace(c, v1WorkspaceOpts{Require: h.requireStoreAndMedia}, func(wctx v1WorkspaceCtx) {
		identityPrincipal := wctx.Principal
		workspacePublicID := wctx.Workspace.PublicID
		key, err := contract.IdempotencyKeyFromHeader(c)
		if err != nil {
			contract.RenderError(c, http.StatusBadRequest, contract.CodeIdempotencyKeyRequired, runtimehttp.MsgIdempotencyKeyRequired2)
			c.Abort()
			return
		}
		var body v1CreateUploadBody
		if bindErr := contract.BindLenientJSON(c, &body); bindErr != nil {
			writeV1InputError(c)
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
		// #936: an optional board binding stages the upload for a board the
		// creator can write. Malformed, unknown, and unreadable boards
		// converge to 404; readable-but-unwritable boards to 403 — the
		// client can neither forge a binding nor learn board existence
		// from it. Unbound uploads stay owner-only (no workspace-readable
		// window, even when the board is not autosaved yet).
		targetBoardID := strings.TrimSpace(body.Data.BoardID)
		if targetBoardID != "" {
			if _, _, ok := h.authorizeBoardByID(c, wctx.Workspace, targetBoardID, wctx.Role, identityPrincipal.UserID(), boards.CapWrite); !ok {
				return
			}
		}
		ctx := c.Request.Context()
		externalUser := tenancy.FormatUserPublicID(identityPrincipal.PublicID())
		scope := contract.IdempotencyScope(externalUser, "upload:create")
		hash := contract.HashIdempotencyRequest("upload:create", workspacePublicID, filename,
			workspacemedia.CanonicalMIME(mimeType), strconv.FormatInt(body.Data.SizeBytes, 10), checksum, kind, targetBoardID)
		ownerUserID := identityPrincipal.UserID()
		result, _, doErr := h.deps.Idempotency.Do(ctx, scope, ownerUserID, key, hash, func() (any, error) {
			session, createErr := h.deps.Media.CreateSession(ctx, workspacemedia.CreateSessionParams{
				WorkspaceID:         workspacePublicID,
				OwnerUserID:         ownerUserID,
				Filename:            filename,
				MIMEType:            mimeType,
				SizeBytes:           body.Data.SizeBytes,
				ChecksumSHA256:      checksum,
				MediaKind:           kind,
				TargetBoardPublicID: targetBoardID,
			})
			if createErr != nil {
				return nil, createErr
			}
			dto := toUploadSessionDTO(session)
			return &dto, nil
		})
		if doErr != nil {
			if contract.IsIdempotencyConflict(doErr) {
				contract.RenderError(c, http.StatusConflict, contract.CodeUploadIdempotencyConflict, runtimehttp.MsgWorkspaceIdempotencyConflict)
				c.Abort()
				return
			}
			renderUploadValidationError(c, doErr)
			return
		}
		dto, ok := result.(*v1UploadSessionDTO)
		if !ok || dto == nil {
			contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable4)
			c.Abort()
			return
		}
		contract.RenderCreated(c, dto)
	})
}

func (h *contractV1Handler) getUploadSession(c *gin.Context) {
	h.withWorkspace(c, v1WorkspaceOpts{Require: h.requireStoreAndMedia}, func(wctx v1WorkspaceCtx) {
		workspacePublicID := wctx.Workspace.PublicID
		uploadID := strings.TrimSpace(c.Param("upload_id"))
		if !contract.IsUploadID(uploadID) {
			contract.RenderError(c, http.StatusNotFound, contract.CodeUploadNotFound, runtimehttp.MsgUploadNotFound)
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
	})
}

// v1OptionalIdempotencyKey resolves the idempotency key for
// naturally-idempotent byte uploads (upload byte PUT, avatar upload):
// a client key is still honored for cross-request dedup, but when
// absent the server mints a deterministic key from the content hash in
// the same scope, so replay protection holds without client friction.
// A present-but-malformed key fails closed with 400
// idempotency_key_required.
func v1OptionalIdempotencyKey(c *gin.Context, hash string) (string, bool) {
	key := strings.TrimSpace(c.GetHeader(contract.IdempotencyKeyHeader))
	if key == "" {
		return "auto-" + hash, true
	}
	parsed, err := contract.ParseIdempotencyKey(key)
	if err != nil {
		contract.RenderError(c, http.StatusBadRequest, contract.CodeIdempotencyKeyRequired, runtimehttp.MsgIdempotencyKeyRequired2)
		c.Abort()
		return "", false
	}
	return parsed, true
}

// putUploadBytes stages the declared bytes at the session slot. The
// slot was reserved at create time (upload_target.slot carries only
// the opaque upload_ id, never the key); this endpoint is the single
// writer that materializes bytes there, so complete-after-bytes is
// real. Request is raw bytes (Content-Type should echo the declared
// MIME); the response stays the JSON session envelope (bytes never
// ride JSON). Validation is retryable and changes no state: size must
// equal the declaration, the sniffed format must equal the declared
// MIME, and the sha256 must equal the declared checksum.
func (h *contractV1Handler) putUploadBytes(c *gin.Context) {
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
		contract.RenderError(c, http.StatusNotFound, contract.CodeUploadNotFound, runtimehttp.MsgUploadNotFound)
		c.Abort()
		return
	}
	if h.deps.MediaObjects == nil {
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable50)
		c.Abort()
		return
	}
	data, readErr := io.ReadAll(io.LimitReader(c.Request.Body, workspacemedia.MaxUploadSizeBytes+1))
	if readErr != nil {
		contract.RenderError(c, http.StatusBadRequest, contract.CodeBadRequest, runtimehttp.MsgRequireEmptyV1BillingActionBody)
		c.Abort()
		return
	}
	if len(data) == 0 {
		renderUploadValidationError(c, &workspacemedia.ValidationError{Fields: map[string]string{"bytes": "missing"}})
		return
	}
	if len(data) > int(workspacemedia.MaxUploadSizeBytes) {
		renderUploadValidationError(c, &workspacemedia.ValidationError{Fields: map[string]string{"size_bytes": "out_of_range"}})
		return
	}
	session, err := h.deps.Media.GetSession(c.Request.Context(), workspacePublicID, uploadID)
	if err != nil {
		renderUploadStoreError(c, err)
		return
	}
	if !session.IsLive() {
		renderUploadStoreError(c, workspacemedia.ErrStateConflict)
		return
	}
	if int64(len(data)) != session.SizeBytes {
		renderUploadValidationError(c, workspacemedia.NewFieldValidationError("size_bytes"))
		return
	}
	if raw := strings.TrimSpace(c.GetHeader("Content-Type")); raw != "" {
		if canonical, ok := imageformat.NormalizeImageMIME(raw); !ok || canonical != session.MIMEType {
			renderUploadValidationError(c, workspacemedia.NewFieldValidationError("mime_type"))
			return
		}
	}
	sniffed := v1SniffedImageMIME(data)
	if sniffed == "" {
		renderMediaUnsupportedType(c, "mime_type")
		return
	}
	if sniffed != session.MIMEType {
		renderUploadValidationError(c, workspacemedia.NewFieldValidationError("mime_type"))
		return
	}
	sum := sha256.Sum256(data)
	bodyChecksum := hex.EncodeToString(sum[:])
	if workspacemedia.NormalizeChecksum(bodyChecksum) != session.ChecksumSHA256 {
		renderUploadValidationError(c, workspacemedia.NewFieldValidationError("checksum_sha256"))
		return
	}
	ctx := c.Request.Context()
	externalUser := tenancy.FormatUserPublicID(identityPrincipal.PublicID())
	scope := contract.IdempotencyScope(externalUser, "upload:bytes")
	hash := contract.HashIdempotencyRequest("upload:bytes", workspacePublicID, uploadID, bodyChecksum,
		strconv.FormatInt(int64(len(data)), 10), session.MIMEType)
	// The byte PUT is naturally idempotent (validated size/checksum/MIME
	// echo): see v1OptionalIdempotencyKey.
	key, ok := v1OptionalIdempotencyKey(c, hash)
	if !ok {
		return
	}
	result, _, doErr := h.deps.Idempotency.Do(ctx, scope, identityPrincipal.UserID(), key, hash, func() (any, error) {
		if putErr := h.deps.MediaObjects.PutObjectAtProfile(ctx, workspacemedia.DefaultStorageProfile, session.StorageKey, data, session.MIMEType); putErr != nil {
			return nil, errV1MediaBytesStoreUnavailable
		}
		staged, getErr := h.deps.Media.GetSession(ctx, workspacePublicID, uploadID)
		if getErr != nil {
			return nil, getErr
		}
		dto := toUploadSessionDTO(staged)
		return &dto, nil
	})
	if doErr != nil {
		if contract.IsIdempotencyConflict(doErr) {
			contract.RenderError(c, http.StatusConflict, contract.CodeUploadIdempotencyConflict, runtimehttp.MsgWorkspaceIdempotencyConflict)
			c.Abort()
			return
		}
		if errors.Is(doErr, errV1MediaBytesStoreUnavailable) {
			contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable50)
			c.Abort()
			return
		}
		renderUploadStoreError(c, doErr)
		return
	}
	dto, ok := result.(*v1UploadSessionDTO)
	if !ok || dto == nil {
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable4)
		c.Abort()
		return
	}
	contract.RenderSuccess(c, dto)
}

// v1SniffedImageMIME returns the registry-canonical MIME sniffed from
// raw bytes, or "" when the bytes match no allowlisted image format.
func v1SniffedImageMIME(data []byte) string {
	ext := imageformat.DetectImageExtensionFromBytes(data)
	if ext == "" {
		return ""
	}
	return imageformat.ImageMIMEFromExtension(ext)
}

// v1StagedChecksum returns the normalized hex sha256 of staged bytes
// for declaration comparison.
func v1StagedChecksum(data []byte) string {
	sum := sha256.Sum256(data)
	return workspacemedia.NormalizeChecksum(hex.EncodeToString(sum[:]))
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
	h.withWorkspace(c, v1WorkspaceOpts{Require: h.requireStoreAndMedia}, func(wctx v1WorkspaceCtx) {
		identityPrincipal := wctx.Principal
		workspacePublicID := wctx.Workspace.PublicID
		uploadID := strings.TrimSpace(c.Param("upload_id"))
		if !contract.IsUploadID(uploadID) {
			contract.RenderError(c, http.StatusNotFound, contract.CodeUploadNotFound, runtimehttp.MsgUploadNotFound)
			c.Abort()
			return
		}
		key, err := contract.IdempotencyKeyFromHeader(c)
		if err != nil {
			contract.RenderError(c, http.StatusBadRequest, contract.CodeIdempotencyKeyRequired, runtimehttp.MsgIdempotencyKeyRequired2)
			c.Abort()
			return
		}
		var body v1CompleteUploadBody
		if bindErr := contract.BindLenientJSON(c, &body); bindErr != nil {
			writeV1InputError(c)
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
				contract.RenderError(c, http.StatusConflict, contract.CodeUploadIdempotencyConflict, runtimehttp.MsgWorkspaceIdempotencyConflict)
				c.Abort()
				return
			}
			if errors.Is(doErr, errV1MediaStagedReadUnavailable) {
				contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable4)
				c.Abort()
				return
			}
			renderUploadValidationError(c, doErr)
			return
		}
		dto, ok := result.(*v1CompleteUploadDTO)
		if !ok || dto == nil {
			contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable4)
			c.Abort()
			return
		}
		contract.RenderSuccess(c, dto)
	})
}

func (h *contractV1Handler) completeUploadOnce(ctx context.Context, workspacePublicID, uploadID string, identityPrincipal principal.Principal, facts workspacemedia.CompleteFacts) (any, error) {
	assetPublicID, err := contract.MintPublicID(contract.PrefixAsset, 12)
	if err != nil {
		return nil, err
	}
	// Complete-after-bytes: with a byte backend wired, the staged
	// object must exist at the session slot and match the declaration
	// before anything durable is written. Completion without staged
	// bytes fails loudly here and never materializes a broken asset.
	session, err := h.deps.Media.GetSession(ctx, workspacePublicID, uploadID)
	if err != nil {
		return nil, err
	}
	if !session.IsLive() {
		return nil, workspacemedia.ErrStateConflict
	}
	if gateErr := h.verifyStagedUploadBytes(ctx, session, &facts); gateErr != nil {
		return nil, gateErr
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
	uploadDTO := toUploadSessionDTO(completed)
	dto := &v1CompleteUploadDTO{Upload: uploadDTO, Asset: toMediaAssetDTO(workspacePublicID, asset, v1FormatTime(completed.UpdatedAt))}
	return dto, nil
}

// verifyStagedUploadBytes enforces complete-after-bytes against the
// staged slot object. Missing bytes report 422 (retryable: the session
// stays live for a byte PUT); mismatched bytes fail the session —
// mirroring completion-echo semantics — and report 422. Transient
// backend failures report 503 without writing anything. Without a byte
// backend the gate is skipped (degraded metadata mode); production
// wiring always sets MediaObjects.
func (h *contractV1Handler) verifyStagedUploadBytes(ctx context.Context, session *workspacemedia.Session, facts *workspacemedia.CompleteFacts) error {
	if h == nil || h.deps.MediaObjects == nil || session == nil {
		return nil
	}
	object, err := h.deps.MediaObjects.ReadObjectAtProfile(ctx, workspacemedia.DefaultStorageProfile, session.StorageKey, workspacemedia.MaxUploadSizeBytes)
	if err != nil {
		if errors.Is(err, storagedomain.ErrStoredObjectNotFound) {
			return &workspacemedia.ValidationError{Fields: map[string]string{"bytes": "missing"}}
		}
		return errV1MediaStagedReadUnavailable
	}
	if object == nil || len(object.Data) == 0 {
		return &workspacemedia.ValidationError{Fields: map[string]string{"bytes": "missing"}}
	}
	if int64(len(object.Data)) != session.SizeBytes {
		_, _ = h.deps.Media.FailSession(ctx, session.WorkspaceID, session.ID, "staged size mismatch")
		return &workspacemedia.ValidationError{Fields: map[string]string{"size_bytes": "invalid"}}
	}
	if sniffed := v1SniffedImageMIME(object.Data); sniffed != session.MIMEType {
		_, _ = h.deps.Media.FailSession(ctx, session.WorkspaceID, session.ID, "staged mime mismatch")
		return &workspacemedia.ValidationError{Fields: map[string]string{"mime_type": "invalid"}}
	}
	if v1StagedChecksum(object.Data) != session.ChecksumSHA256 {
		_, _ = h.deps.Media.FailSession(ctx, session.WorkspaceID, session.ID, "staged checksum mismatch")
		return &workspacemedia.ValidationError{Fields: map[string]string{"checksum_sha256": "invalid"}}
	}
	imageFacts, err := imageformat.InspectDetectedBytes(object.Data)
	if err != nil {
		_, _ = h.deps.Media.FailSession(ctx, session.WorkspaceID, session.ID, "invalid staged image")
		return &workspacemedia.ValidationError{Fields: map[string]string{"bytes": "invalid"}}
	}
	facts.Width = int32(imageFacts.Width)
	facts.Height = int32(imageFacts.Height)
	return nil
}

func (h *contractV1Handler) listMediaAssets(c *gin.Context) {
	h.withWorkspace(c, v1WorkspaceOpts{Require: h.requireStoreAndMedia}, func(wctx v1WorkspaceCtx) {
		workspacePublicID := wctx.Workspace.PublicID
		pageSize, ok := v1PageSize(c)
		if !ok {
			return
		}
		offset, ok := v1OffsetCursor(c)
		if !ok {
			return
		}
		// #936: authorization precedes pagination. The offset counts
		// AUTHORIZED items, so pages and has_more never leak unauthorized
		// rows. Over-fetch by one to compute has_more without a count query.
		assets, err := h.deps.Media.ListAuthorizedAssets(c.Request.Context(), workspacePublicID,
			wctx.Principal.UserID(), wctx.Role, h.mediaBoards(), pageSize+1, offset)
		if err != nil {
			contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable51)
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
		meta, ok := v1OffsetMeta(c, offset+pageSize, hasMore, runtimehttp.MsgServiceUnavailable51)
		if !ok {
			return
		}
		contract.RenderPaginated(c, page, meta)
	})
}

func (h *contractV1Handler) getMediaAsset(c *gin.Context) {
	h.withWorkspace(c, v1WorkspaceOpts{Require: h.requireStoreAndMedia}, func(wctx v1WorkspaceCtx) {
		workspacePublicID := wctx.Workspace.PublicID
		assetID := strings.TrimSpace(c.Param("asset_id"))
		// Library read-through: the Canvas import path reads managed media
		// through this endpoint with the owned lasset_ id (poll-free single
		// attempt; the Canvas pending-fact loop owns the retry). A
		// completed materialization link serves the managed asset; a miss
		// triggers one best-effort async fetch and 404s now.
		if contract.IsLibraryAssetID(assetID) {
			h.getLibraryMaterializedMediaAsset(c, workspacePublicID, assetID, wctx.Principal.UserID(), wctx.Role)
			return
		}
		if !contract.IsAssetID(assetID) {
			contract.RenderError(c, http.StatusNotFound, contract.CodeMediaNotFound, runtimehttp.MsgMediaNotFound)
			c.Abort()
			return
		}
		// #936: metadata reads enforce the same authorization as bytes.
		asset, ok := h.authorizeMediaAsset(c, workspacePublicID, assetID, wctx.Principal.UserID(), wctx.Role)
		if !ok {
			return
		}
		contract.RenderSuccess(c, toMediaAssetDTO(workspacePublicID, asset, v1FormatTime(asset.CreatedAt)))
	})
}

type v1BatchMediaAssetsBody struct {
	Data struct {
		AssetIDs []string `json:"asset_ids"`
	} `json:"data"`
}

type v1BatchMediaAssetsDTO struct {
	Items []v1MediaAssetDTO `json:"items"`
}

// batchGetMediaAssets reads up to maxV1MediaBatchAssets assets in one
// call (bounded input). Auth, workspace scoping, and per-id 404
// convergence match the single-read endpoint; the response wraps the
// same asset DTOs in request order. Read-only, so no Idempotency-Key
// (library asset-resolutions precedent).
func (h *contractV1Handler) batchGetMediaAssets(c *gin.Context) {
	withV1WorkspaceBody(h, c, v1WorkspaceOpts{Require: h.requireStoreAndMedia}, func(wctx v1WorkspaceCtx, body v1BatchMediaAssetsBody) {
		workspacePublicID := wctx.Workspace.PublicID
		ids := body.Data.AssetIDs
		if len(ids) < 1 || len(ids) > maxV1MediaBatchAssets {
			renderMediaValidationError(c, &workspacemedia.ValidationError{Fields: map[string]string{"asset_ids": "out_of_range"}})
			return
		}
		items := make([]v1MediaAssetDTO, 0, len(ids))
		for _, raw := range ids {
			assetID := strings.TrimSpace(raw)
			if !contract.IsAssetID(assetID) {
				renderMediaStoreError(c, workspacemedia.ErrNotFound)
				return
			}
			// #936: batch items enforce per-id authorization with the same
			// 404 convergence as the single read; one unauthorized id fails
			// the batch without naming which source was missing.
			asset, ok := h.authorizeMediaAsset(c, workspacePublicID, assetID, wctx.Principal.UserID(), wctx.Role)
			if !ok {
				return
			}
			items = append(items, toMediaAssetDTO(workspacePublicID, asset, v1FormatTime(asset.CreatedAt)))
		}
		contract.RenderSuccess(c, v1BatchMediaAssetsDTO{Items: items})
	})
}

func (h *contractV1Handler) archiveMediaAsset(c *gin.Context) {
	h.withWorkspace(c, v1WorkspaceOpts{Require: h.requireStoreAndMedia, Allow: v1AllowOwnerOrAdmin}, func(wctx v1WorkspaceCtx) {
		identityPrincipal := wctx.Principal
		workspacePublicID := wctx.Workspace.PublicID
		assetID := strings.TrimSpace(c.Param("asset_id"))
		if !contract.IsAssetID(assetID) {
			contract.RenderError(c, http.StatusNotFound, contract.CodeMediaNotFound, runtimehttp.MsgMediaNotFound)
			c.Abort()
			return
		}
		// #936: archiving an unreadable asset reads as 404 (the admin
		// prelude already gated workspace owner/admin; authorization runs
		// before any state change or idempotency claim).
		if _, ok := h.authorizeMediaAsset(c, workspacePublicID, assetID, identityPrincipal.UserID(), wctx.Role); !ok {
			return
		}
		key, err := contract.IdempotencyKeyFromHeader(c)
		if err != nil {
			contract.RenderError(c, http.StatusBadRequest, contract.CodeIdempotencyKeyRequired, runtimehttp.MsgIdempotencyKeyRequired2)
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
				contract.RenderError(c, http.StatusConflict, contract.CodeMediaIdempotencyConflict, runtimehttp.MsgWorkspaceIdempotencyConflict)
				c.Abort()
				return
			}
			renderMediaStoreError(c, doErr)
			return
		}
		dto, ok := result.(*v1MediaAssetDTO)
		if !ok || dto == nil {
			contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable51)
			c.Abort()
			return
		}
		contract.RenderSuccess(c, dto)
	})
}

// v1BinaryWorkspaceContext is the plain-status counterpart of
// v1MediaWorkspaceContext for the binary endpoints: every failure
// (including auth and fail-closed paths) renders a non-JSON status so
// content/derivative responses never carry the JSON envelope. Only
// workspace members pass; hidden workspaces and non-members converge
// to 404 (Baseline 4 anti-enumeration). It returns the caller's user id
// and workspace role for #936 asset authorization downstream.
func (h *contractV1Handler) v1BinaryWorkspaceContext(c *gin.Context) (workspacePublicID string, userID int64, workspaceRole string, allowed bool) {
	identityPrincipal, ok := principal.From(c)
	if !ok {
		runtimehttp.RenderV1BinaryError(c, http.StatusUnauthorized)
		return "", 0, "", false
	}
	if h.deps.Store == nil || h.deps.Media == nil {
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

// v1MediaBinaryAsset resolves the asset for binary endpoints with plain
// (non-envelope) failures. ok=false means a status was already written.
// #936: authorization runs on the loaded asset BEFORE the archived (410)
// and readiness (409) gates — and therefore before the ETag/304 shortcut
// and the derivative renderer — so unauthorized callers always read 404
// and never learn whether the bytes exist, are archived, or are ready.
func (h *contractV1Handler) v1MediaBinaryAsset(c *gin.Context, workspacePublicID, assetID string, userID int64, workspaceRole string) (*workspacemedia.Asset, bool) {
	asset, ok := h.authorizeMediaAssetBytes(c, workspacePublicID, assetID, userID, workspaceRole)
	if !ok {
		return nil, false
	}
	if asset.IsArchived() {
		runtimehttp.RenderV1BinaryError(c, http.StatusGone)
		return nil, false
	}
	if asset.Status != workspacemedia.AssetStatusReady {
		runtimehttp.RenderV1BinaryError(c, http.StatusConflict)
		return nil, false
	}
	return asset, true
}

func (h *contractV1Handler) writeV1MediaBytes(c *gin.Context, objectKey, mimeType string) {
	if h.deps.MediaObjects == nil {
		runtimehttp.RenderV1BinaryError(c, http.StatusServiceUnavailable)
		return
	}
	object, err := h.deps.MediaObjects.ReadObjectAtProfile(c.Request.Context(), workspacemedia.DefaultStorageProfile, objectKey, v1MediaBinaryMaxBytes)
	if err != nil {
		if errors.Is(err, storagedomain.ErrStoredObjectNotFound) {
			runtimehttp.RenderV1BinaryError(c, http.StatusNotFound)
			return
		}
		runtimehttp.RenderV1BinaryError(c, http.StatusServiceUnavailable)
		return
	}
	if object == nil || len(object.Data) == 0 {
		runtimehttp.RenderV1BinaryError(c, http.StatusNotFound)
		return
	}
	mime := strings.TrimSpace(mimeType)
	if mime == "" {
		mime = strings.TrimSpace(object.ContentType)
	}
	if mime == "" {
		mime = "application/octet-stream"
	}
	// Content-addressed ETag: bytes are immutable per storage key, so a
	// strong validator is exact and safe. Conditional requests revalidate
	// to 304 without resending bytes.
	sum := sha256.Sum256(object.Data)
	etag := `"` + hex.EncodeToString(sum[:]) + `"`
	if matchV1MediaETag(c.GetHeader("If-None-Match"), etag) {
		c.Header("ETag", etag)
		c.Status(http.StatusNotModified)
		return
	}
	c.Header("Cache-Control", "private, max-age=3600")
	c.Header("ETag", etag)
	c.Header("Content-Disposition", v1MediaDisposition(mime, object.Filename))
	c.Data(http.StatusOK, mime, object.Data)
}

// matchV1MediaETag reports whether an If-None-Match header value covers a
// ETag, using weak comparison as required for If-None-Match.
func matchV1MediaETag(header, etag string) bool {
	for _, candidate := range strings.Split(header, ",") {
		candidate = strings.TrimSpace(candidate)
		candidate = strings.TrimPrefix(candidate, "W/")
		if candidate == "*" || candidate == etag {
			return true
		}
	}
	return false
}

// v1MediaDisposition keeps `inline` for verified raster image MIME types
// (the only types the upload allowlist admits) and fails closed to
// `attachment` for anything else, so a future MIME can never execute as
// active content on the app origin.
func v1MediaDisposition(mime, filename string) string {
	switch strings.ToLower(strings.TrimSpace(strings.Split(mime, ";")[0])) {
	case "image/jpeg", "image/png", "image/gif", "image/bmp", "image/webp", "image/avif":
		return "inline"
	default:
		name := strings.TrimSpace(filename)
		if name == "" {
			name = "asset"
		}
		return `attachment; filename="` + strings.ReplaceAll(name, `"`, "") + `"`
	}
}

func (h *contractV1Handler) contentMediaAsset(c *gin.Context) {
	workspacePublicID, userID, workspaceRole, allowed := h.v1BinaryWorkspaceContext(c)
	if !allowed {
		return
	}
	assetID := strings.TrimSpace(c.Param("asset_id"))
	asset, ok := h.v1MediaBinaryAsset(c, workspacePublicID, assetID, userID, workspaceRole)
	if !ok {
		return
	}
	h.writeV1MediaBytes(c, asset.StorageKey, asset.MIMEType)
}

func (h *contractV1Handler) derivativeMediaAsset(c *gin.Context) {
	workspacePublicID, userID, workspaceRole, allowed := h.v1BinaryWorkspaceContext(c)
	if !allowed {
		return
	}
	variant := strings.TrimSpace(c.Param("variant"))
	if _, exists := derivativedomain.TierSpecs[variant]; !exists {
		runtimehttp.RenderV1BinaryError(c, http.StatusBadRequest)
		return
	}
	if variant == derivativedomain.TierProcessingInput {
		runtimehttp.RenderV1BinaryError(c, http.StatusBadRequest)
		return
	}
	assetID := strings.TrimSpace(c.Param("asset_id"))
	asset, ok := h.v1MediaBinaryAsset(c, workspacePublicID, assetID, userID, workspaceRole)
	if !ok {
		return
	}
	image, err := h.deps.MediaRenderer.WorkspaceTier(c.Request.Context(), asset.StorageKey, variant)
	runtimehttp.WriteRenderedImage(c, image, err, "private, max-age=300")
}
