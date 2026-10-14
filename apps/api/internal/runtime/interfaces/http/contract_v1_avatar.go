package httpapi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"log/slog"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"ideaboard/api/internal/media/derivatives/infrastructure/imgproxy"
	"ideaboard/api/internal/media/imageformat"
	storagedomain "ideaboard/api/internal/media/storage/domain"
	storageports "ideaboard/api/internal/media/storage/ports"
	"ideaboard/api/internal/media/workspacemedia"
	"ideaboard/api/internal/platform/contract"
	"ideaboard/api/internal/platform/database"
	"ideaboard/api/internal/tenancy"

	"github.com/gin-gonic/gin"
)

// V1AvatarObjects stores avatars through the active profile and removes replaced objects.
type V1AvatarObjects interface {
	StoreBytesRef(ctx context.Context, filename string, data []byte, opts storageports.StorageStoreOptions) (*storageports.StoredUploadRef, error)
	DeleteObjectByKey(ctx context.Context, objectKey string) error
}

// v1AvatarMaxBytes bounds one avatar upload (5MiB: avatars are display
// images, never originals).
const v1AvatarMaxBytes = 5 * 1024 * 1024

// The stable user URL must be revalidated after each avatar replacement.
const v1PublicAvatarCacheControl = "no-store"

// v1PublicAvatarURL renders the canonical public route for one storage
// UID (sizeless; callers append ?size= for a display variant).
func v1PublicAvatarURL(storageUID string) string {
	return ContractV1GroupPath + "/public/users/" + strings.TrimSpace(storageUID) + "/avatar"
}

// v1PublicAvatarURLForRow projects a stored users.avatar value to its
// public route URL. Empty keys (no custom avatar) project to "" so
// callers keep the omitted-avatar shape; external (non-key) URLs pass
// through untouched.
func v1PublicAvatarURLForRow(storageUID, avatarKey string) string {
	key := strings.TrimSpace(avatarKey)
	if key == "" {
		return ""
	}
	if strings.HasPrefix(key, "/") || strings.Contains(key, "://") {
		return key
	}
	if strings.TrimSpace(storageUID) == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(key))
	return v1PublicAvatarURL(storageUID) + "?v=" + hex.EncodeToString(sum[:8])
}

type v1AvatarDTO struct {
	AvatarURL string `json:"avatar_url"`
}

// uploadAvatar replaces the caller's profile avatar (user-scoped). The
// request is raw image bytes (never JSON, never multipart); the
// response is the JSON envelope with the public avatar route URL. MIME
// is sniffed from the bytes against the imageformat allowlist and size
// is capped at v1AvatarMaxBytes; the object key is the canonical
// content-hashed avatar key, so identical bytes converge on one object
// and replaced avatars orphan nothing (the superseded key is removed
// best-effort after the row update).
func (h *contractV1Handler) uploadAvatar(c *gin.Context) {
	identityPrincipal, ok := v1RequestPrincipal(c)
	if !ok {
		return
	}
	if h.deps.AvatarUID == nil {
		contract.RenderError(c, http.StatusNotImplemented,
			contract.CodeContractNotImplemented, "The v1 avatar surface is not implemented yet")
		c.Abort()
		return
	}
	if h.deps.AvatarObjects == nil || h.deps.AvatarProfile == nil {
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, msgServiceUnavailable52)
		c.Abort()
		return
	}
	data, readErr := io.ReadAll(io.LimitReader(c.Request.Body, v1AvatarMaxBytes+1))
	if readErr != nil {
		contract.RenderError(c, http.StatusBadRequest, contract.CodeBadRequest, msgRequireEmptyV1BillingActionBody)
		c.Abort()
		return
	}
	if len(data) == 0 {
		renderMediaValidationError(c, workspacemedia.NewFieldValidationError("avatar"))
		return
	}
	if len(data) > v1AvatarMaxBytes {
		renderMediaValidationError(c, &workspacemedia.ValidationError{Fields: map[string]string{"avatar": "out_of_range"}})
		return
	}
	ext := imageformat.DetectImageExtensionFromBytes(data)
	mime := imageformat.ImageMIMEFromExtension(ext)
	if ext == "" || mime == "" {
		renderMediaValidationError(c, &workspacemedia.ValidationError{Fields: map[string]string{"avatar": "unsupported"}})
		return
	}
	if raw := strings.TrimSpace(c.GetHeader("Content-Type")); raw != "" {
		if canonical, ok := imageformat.NormalizeImageMIME(raw); !ok || canonical != mime {
			renderMediaValidationError(c, workspacemedia.NewFieldValidationError("avatar"))
			return
		}
	}
	ctx := c.Request.Context()
	storageUID, err := h.deps.AvatarUID.GetStorageUIDByUserID(ctx, identityPrincipal.UserID())
	if err != nil {
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, msgServiceUnavailable52)
		c.Abort()
		return
	}
	sum := sha256.Sum256(data)
	bodyChecksum := hex.EncodeToString(sum[:])
	objectKey, err := storagedomain.AvatarObjectKey(storageUID, bodyChecksum[:16], ext)
	if err != nil {
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, msgServiceUnavailable52)
		c.Abort()
		return
	}
	externalUser := tenancy.FormatUserPublicID(identityPrincipal.PublicID())
	scope := contract.IdempotencyScope(externalUser, "avatar:upload")
	hash := contract.HashIdempotencyRequest("avatar:upload", externalUser, bodyChecksum,
		strconv.FormatInt(int64(len(data)), 10), mime)
	// Avatar bytes are naturally idempotent (sniffed format + content
	// hash dedup): same optional-key contract as the upload byte PUT.
	key, ok := v1OptionalIdempotencyKey(c, hash)
	if !ok {
		return
	}
	result, _, doErr := h.deps.Idempotency.Do(ctx, scope, identityPrincipal.UserID(), key, hash, func() (any, error) {
		return h.storeAvatarOnce(ctx, identityPrincipal.UserID(), storageUID, objectKey, "avatar"+ext, data)
	})
	if doErr != nil {
		if contract.IsIdempotencyConflict(doErr) {
			contract.RenderError(c, http.StatusConflict, contract.CodeMediaIdempotencyConflict, msgWorkspaceIdempotencyConflict)
			c.Abort()
			return
		}
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, msgServiceUnavailable52)
		c.Abort()
		return
	}
	dto, ok := result.(*v1AvatarDTO)
	if !ok || dto == nil {
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, msgServiceUnavailable52)
		c.Abort()
		return
	}
	contract.RenderSuccess(c, dto)
}

// storeAvatarOnce persists one avatar body under its canonical key and
// projects the public route URL. Identical bytes are already stored
// (content-addressed key), so replays skip the write; replaced keys are
// removed best-effort after the row update and never fail the upload.
func (h *contractV1Handler) storeAvatarOnce(ctx context.Context, userID int64, storageUID, objectKey, filename string, data []byte) (any, error) {
	previous, err := h.deps.AvatarProfile.GetUserByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	previousKey := ""
	if previous != nil {
		previousKey = strings.TrimSpace(previous.Avatar)
	}
	if previousKey != objectKey {
		ref, err := h.deps.AvatarObjects.StoreBytesRef(ctx, filename, data, storageports.StorageStoreOptions{ObjectKey: objectKey})
		if err != nil {
			return nil, errV1MediaBytesStoreUnavailable
		}
		storedKey := strings.TrimSpace(ref.StorageKey)
		if storedKey == "" {
			storedKey = objectKey
		}
		if writeErr := h.deps.AvatarProfile.UpdateUserAvatar(ctx, userID, storedKey); writeErr != nil {
			return nil, writeErr
		}
		if previousKey != "" && previousKey != storedKey {
			if deleteErr := h.deps.AvatarObjects.DeleteObjectByKey(ctx, previousKey); deleteErr != nil {
				slog.WarnContext(ctx, "avatar superseded key cleanup failed", "key", previousKey, "err", deleteErr)
			}
		}
	}
	return &v1AvatarDTO{AvatarURL: v1PublicAvatarURLForRow(storageUID, objectKey)}, nil
}

// publicAvatar returns a rendered size variant or the bundled default SVG.
func (h *contractV1Handler) publicAvatar(c *gin.Context) {
	uid, err := storagedomain.NormalizeStorageUserSegment(strings.TrimSpace(c.Param("uid")))
	if err != nil {
		renderV1BinaryError(c, http.StatusBadRequest)
		return
	}
	size := imgproxy.DefaultAvatarSize
	if raw := strings.TrimSpace(c.Query("size")); raw != "" {
		parsed, parseErr := strconv.Atoi(raw)
		if parseErr != nil {
			renderV1BinaryError(c, http.StatusBadRequest)
			return
		}
		size = parsed
	}
	if !slices.Contains(imgproxy.AvatarSizes, size) {
		renderV1BinaryError(c, http.StatusBadRequest)
		return
	}
	if h.deps.AvatarProfile == nil {
		renderV1BinaryError(c, http.StatusServiceUnavailable)
		return
	}
	profile, err := h.deps.AvatarProfile.GetUserByStorageUID(c.Request.Context(), uid)
	if err != nil {
		if database.IsNotFound(err) {
			renderV1BinaryError(c, http.StatusNotFound)
		} else {
			renderV1BinaryError(c, http.StatusServiceUnavailable)
		}
		return
	}
	if profile == nil {
		renderV1BinaryError(c, http.StatusNotFound)
		return
	}
	if strings.TrimSpace(profile.Avatar) == "" {
		c.Header("Cache-Control", v1PublicAvatarCacheControl)
		c.Data(http.StatusOK, storagedomain.DefaultAvatarSVGContentType, storagedomain.DefaultAvatarSVG)
		return
	}
	image, err := h.deps.MediaRenderer.Avatar(c.Request.Context(), strings.TrimSpace(profile.Avatar), size)
	writeRenderedImage(c, image, err, v1PublicAvatarCacheControl)
}
