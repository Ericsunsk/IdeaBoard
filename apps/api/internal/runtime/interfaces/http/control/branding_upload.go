package control

// Multipart bodies exceed the guard's 64KiB idempotency buffer, so this
// POST route uses the read guard and enforces mutation guarantees in-handler:
// header-only Idempotency-Key, content-identical replay, conflicting-content
// rejection, and the standard audit record.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	runtimehttp "ideaboard/api/internal/runtime/interfaces/http"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"strings"
	"sync"

	"ideaboard/api/internal/media/imageformat"
	"ideaboard/api/internal/media/storage/domain"
	"ideaboard/api/internal/media/storage/ports"
	"ideaboard/api/internal/platform/contract"
	"ideaboard/api/internal/platform/settings"

	"github.com/gin-gonic/gin"
)

// controlBrandingLogoMaxBytes bounds one branding logo upload.
const controlBrandingLogoMaxBytes = 50 * 1024 * 1024

// ControlBrandingUploadService is the Storage upload seam behind the
// control branding-logo upload. *storage.Service satisfies it; tests
// substitute a fake.
type ControlBrandingUploadService interface {
	UploadRefWithOptions(ctx context.Context, file *multipart.FileHeader, opts ports.StorageStoreOptions) (*ports.StoredUploadRef, error)
}

// controlBrandingLogoUpload is the contract-envelope payload for a stored
// branding logo: logo_url carries the canonical storage key
// (uploads/site/assets/logo_{hash}.ext) and feeds the branding setting
// update verbatim; the rest is operator-facing file evidence.
type controlBrandingLogoUpload struct {
	LogoURL string `json:"logo_url"`
	MIME    string `json:"mime,omitempty"`
	Width   int    `json:"width,omitempty"`
	Height  int    `json:"height,omitempty"`
	Ext     string `json:"ext,omitempty"`
}

var controlBrandingUploadLimiter = &controlUploadConcurrencyLimiter{}

type controlUploadConcurrencyLimiter struct {
	mu       sync.Mutex
	inFlight int
}

func (l *controlUploadConcurrencyLimiter) acquire(limit int) (func(), bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.inFlight >= limit {
		return nil, false
	}
	l.inFlight++
	return func() {
		l.mu.Lock()
		defer l.mu.Unlock()
		if l.inFlight > 0 {
			l.inFlight--
		}
	}, true
}

func controlBrandingUploadSlot(ctx context.Context, provider settings.MediaUploadSettingProvider) (func(), bool) {
	limit := settings.DefaultMediaUploadSetting().UploadMaxConcurrent
	if provider != nil {
		if setting, err := provider.GetMediaUpload(ctx); err == nil {
			if normalized, err := settings.NormalizeMediaUploadSetting(setting); err == nil {
				limit = normalized.UploadMaxConcurrent
			} else {
				slog.WarnContext(ctx, "control branding upload settings invalid; using defaults", "err", err)
			}
		} else {
			slog.WarnContext(ctx, "control branding upload settings read failed; using defaults", "err", err)
		}
	}
	return controlBrandingUploadLimiter.acquire(limit)
}

func (d *ContractControlDeps) uploadControlBrandingLogo(c *gin.Context, ctx controlContext) {
	if d == nil || d.BrandingUpload == nil {
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable4)
		return
	}
	// Handler-enforced mutation guarantees (see the file header): the
	// guard cannot buffer multipart bodies, so the key is
	// required here instead of in the guard.
	key, err := contract.IdempotencyKeyFromHeader(c)
	if err != nil {
		contract.RenderError(c, http.StatusBadRequest, contract.CodeIdempotencyKeyRequired, runtimehttp.MsgIdempotencyKeyRequired)
		return
	}
	release, ok := controlBrandingUploadSlot(c.Request.Context(), d.Settings)
	if !ok {
		contract.RenderError(c, http.StatusTooManyRequests, contract.CodeRateLimited, runtimehttp.MsgRateLimited)
		return
	}
	defer release()
	file, err := c.FormFile("file")
	if err != nil {
		contract.RenderError(c, http.StatusBadRequest, contract.CodeBadRequest, runtimehttp.MsgBadRequest6)
		return
	}
	ext, ok := validateControlUploadFile(c, file)
	if !ok {
		return
	}
	contentHash, err := hashControlUploadFile(file)
	if err != nil {
		contract.RenderError(c, http.StatusBadRequest, contract.CodeBadRequest, runtimehttp.MsgBadRequest7)
		return
	}
	// Content-hashed canonical key (same shape as profile avatars):
	// identical bytes converge on one object and the key
	// self-invalidates caches on replacement.
	objectKey, err := domain.SiteAssetLogoObjectKey(contentHash[:16], ext)
	if err != nil {
		contract.RenderError(c, http.StatusInternalServerError, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable5)
		return
	}
	store := d.idempotencyStore()
	_, operatorID := controlOperatorIdentity(ctx.Operator)
	scope := contract.IdempotencyScope(operatorID, "control:platform.upload_branding_logo")
	requestHash := contract.HashIdempotencyRequest(c.Request.Method, c.FullPath(), contentHash)
	if existing, replayed, err := store.Lookup(scope, ctx.Operator.UserID(), key, requestHash); err != nil {
		contract.RenderError(c, http.StatusConflict, contract.CodeIdempotencyConflict, runtimehttp.MsgIdempotencyConflict)
		return
	} else if replayed {
		stored, ok := existing.(controlStoredResponse)
		if !ok {
			contract.RenderError(c, http.StatusConflict, contract.CodeIdempotencyConflict, runtimehttp.MsgIdempotencyConflict)
			return
		}
		if contentType := stored.Header["Content-Type"]; contentType != "" {
			c.Header("Content-Type", contentType)
		}
		c.Data(stored.Status, stored.Header["Content-Type"], []byte(stored.Body))
		return
	}
	stored, err := d.BrandingUpload.UploadRefWithOptions(c.Request.Context(), file, ports.StorageStoreOptions{
		ObjectKey: objectKey,
	})
	if err != nil {
		contract.RenderError(c, http.StatusInternalServerError, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable5)
		return
	}
	// Settings and upload responses use the canonical object key.
	logoURL := strings.TrimSpace(stored.StorageKey)
	if logoURL == "" {
		logoURL = objectKey
	}
	payload := controlBrandingLogoUpload{
		LogoURL: logoURL,
		MIME:    stored.MIMEType,
		Width:   stored.Width,
		Height:  stored.Height,
	}
	if ext := imageformat.ImageExtensionFromMIME(stored.MIMEType); ext != "" {
		payload.Ext = strings.TrimPrefix(ext, ".")
	}
	// Only 2xx responses enter the idempotency store (mirroring the
	// guard), so a transient storage failure never poisons the key.
	body, err := json.Marshal(contract.SuccessEnvelope[controlBrandingLogoUpload]{Data: payload})
	if err != nil {
		contract.RenderError(c, http.StatusInternalServerError, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable5)
		return
	}
	store.StoreResult(scope, ctx.Operator.UserID(), key, requestHash, controlStoredResponse{
		Status:      http.StatusOK,
		Header:      map[string]string{"Content-Type": "application/json; charset=utf-8"},
		Body:        string(body),
		RequestHash: requestHash,
	})
	c.Data(http.StatusOK, "application/json; charset=utf-8", body)
}

// hashControlUploadFile fingerprints the uploaded bytes for the
// idempotency request hash. It opens its own handle so the storage
// upload below still sees the full file.
func hashControlUploadFile(fh *multipart.FileHeader) (string, error) {
	if fh == nil {
		return "", fmt.Errorf("control branding upload: missing file")
	}
	src, err := fh.Open()
	if err != nil {
		return "", err
	}
	defer src.Close()
	sum := sha256.New()
	if _, err := io.Copy(sum, io.LimitReader(src, controlBrandingLogoMaxBytes+1)); err != nil {
		return "", err
	}
	return hex.EncodeToString(sum.Sum(nil)), nil
}

// validateControlUploadFileHeader centralises branding-logo validation:
// size cap, detected-format inspection from actual bytes, and allow-list
// enforcement (never trust the filename). Failures render directly.
func validateControlUploadFile(c *gin.Context, fh *multipart.FileHeader) (string, bool) {
	if fh == nil {
		contract.RenderError(c, http.StatusBadRequest, contract.CodeBadRequest, runtimehttp.MsgBadRequest6)
		return "", false
	}
	if fh.Size > controlBrandingLogoMaxBytes {
		contract.RenderError(c, http.StatusBadRequest, contract.CodeBadRequest,
			fmt.Sprintf(runtimehttp.MsgValidateControlUploadFile, controlBrandingLogoMaxBytes/1024/1024))
		return "", false
	}
	facts, err := imageformat.InspectFileHeader(fh)
	if err != nil {
		slog.Warn("control branding logo inspect failed", "err", err)
		if errors.Is(err, imageformat.ErrUnsupportedImageFormat) {
			contract.RenderError(c, http.StatusUnsupportedMediaType, contract.CodeMediaUnsupportedType,
				runtimehttp.MsgValidateControlUploadFile2+imageformat.SupportedUploadImageLabel()+runtimehttp.MsgValidateControlUploadFile3)
			return "", false
		}
		contract.RenderError(c, http.StatusBadRequest, contract.CodeBadRequest, runtimehttp.MsgBadRequest8)
		return "", false
	}
	if !imageformat.UploadAllowedImageExtensions()[facts.Extension] {
		contract.RenderError(c, http.StatusBadRequest, contract.CodeBadRequest,
			runtimehttp.MsgValidateControlUploadFile2+imageformat.SupportedUploadImageLabel()+runtimehttp.MsgValidateControlUploadFile3)
		return "", false
	}
	return facts.Extension, true
}
