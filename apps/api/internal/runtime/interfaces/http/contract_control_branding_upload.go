package httpapi

// Control branding-logo upload: multipart site-asset upload behind
// control:platform.
//
// The file rides the Storage service into the site-assets prefix (the
// same delivery plane the public branding read serves), so the returned
// logo_url drops straight into the branding setting update. Validation
// mirrors the deleted legacy admin upload (50MB image cap, detected
// format allow-list, never trust the filename).
//
// Transport note: the control guard buffers mutation bodies up to 64KiB
// for reason extraction and idempotency hashing, which cannot carry
// image bytes. This route therefore registers as a POST read-guard
// (no body buffering) and enforces the mutation guarantees in-handler:
// X-Operator-Reason (or the multipart "reason" field) and a header-only
// Idempotency-Key are both required (400 otherwise), the key replays the
// first stored response on content-identical reuse and conflicts (409)
// on divergent reuse, and every call emits the standard audit record.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"strings"
	"sync"

	"ideaboard/api/internal/media/imageformat"
	"ideaboard/api/internal/media/storage/domain"
	"ideaboard/api/internal/media/storage/ports"
	"ideaboard/api/internal/platform/adminaction"
	"ideaboard/api/internal/platform/contract"
	"ideaboard/api/internal/platform/settings"

	"github.com/gin-gonic/gin"
)

// controlBrandingLogoMaxBytes bounds one branding logo upload (same 50MB
// budget as the legacy admin upload; branding marks are tiny in
// practice).
const controlBrandingLogoMaxBytes = 50 * 1024 * 1024

// ControlBrandingUploadService is the Storage upload seam behind the
// control branding-logo upload. *storage.Service satisfies it; tests
// substitute a fake.
type ControlBrandingUploadService interface {
	UploadRefWithOptions(ctx context.Context, file *multipart.FileHeader, opts ports.StorageStoreOptions) (*ports.StoredUploadRef, error)
}

// controlBrandingLogoUpload is the contract-envelope payload for a stored
// branding logo: logo_url feeds the branding setting update, the rest is
// operator-facing file evidence.
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
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, msgServiceUnavailable4)
		return
	}
	// Handler-enforced mutation guarantees (see the file header): the
	// guard cannot buffer multipart bodies, so reason and key are
	// required here instead of in the guard.
	reason := extractControlReason(c, nil)
	if reason == "" {
		if raw := strings.TrimSpace(c.PostForm("reason")); raw != "" {
			reason = raw
		} else if raw := strings.TrimSpace(c.PostForm("operator_note")); raw != "" {
			reason = raw
		}
	}
	if _, err := adminaction.New(ctx.Operator.UserID(), reason); err != nil {
		contract.RenderError(c, http.StatusBadRequest, contract.CodeControlReasonRequired, msgControlReasonRequired)
		return
	}
	setControlReason(c, reason)
	key, err := contract.IdempotencyKeyFromHeader(c)
	if err != nil {
		contract.RenderError(c, http.StatusBadRequest, contract.CodeIdempotencyKeyRequired, msgIdempotencyKeyRequired)
		return
	}
	release, ok := controlBrandingUploadSlot(c.Request.Context(), d.Settings)
	if !ok {
		contract.RenderError(c, http.StatusTooManyRequests, contract.CodeRateLimited, msgRateLimited)
		return
	}
	defer release()
	file, err := c.FormFile("file")
	if err != nil {
		contract.RenderError(c, http.StatusBadRequest, contract.CodeBadRequest, msgBadRequest6)
		return
	}
	if _, ok := validateControlUploadFile(c, file); !ok {
		return
	}
	contentHash, err := hashControlUploadFile(file)
	if err != nil {
		contract.RenderError(c, http.StatusBadRequest, contract.CodeBadRequest, msgBadRequest7)
		return
	}
	store := d.idempotencyStore()
	_, operatorID := controlOperatorIdentity(ctx.Operator)
	scope := contract.IdempotencyScope(operatorID, "control:platform.upload_branding_logo")
	requestHash := contract.HashIdempotencyRequest(c.Request.Method, c.FullPath(), contentHash)
	if existing, replayed, err := store.Lookup(scope, ctx.Operator.UserID(), key, requestHash); err != nil {
		contract.RenderError(c, http.StatusConflict, contract.CodeIdempotencyConflict, msgIdempotencyConflict)
		return
	} else if replayed {
		stored, ok := existing.(controlStoredResponse)
		if !ok {
			contract.RenderError(c, http.StatusConflict, contract.CodeIdempotencyConflict, msgIdempotencyConflict)
			return
		}
		if contentType := stored.Header["Content-Type"]; contentType != "" {
			c.Header("Content-Type", contentType)
		}
		c.Data(stored.Status, stored.Header["Content-Type"], []byte(stored.Body))
		return
	}
	stored, err := d.BrandingUpload.UploadRefWithOptions(c.Request.Context(), file, ports.StorageStoreOptions{
		DirPrefix: domain.SiteAssetUploadDirPrefix(),
	})
	if err != nil {
		contract.RenderError(c, http.StatusInternalServerError, contract.CodeServiceUnavailable, msgServiceUnavailable5)
		return
	}
	payload := controlBrandingLogoUpload{
		LogoURL: stored.URL,
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
		contract.RenderError(c, http.StatusInternalServerError, contract.CodeServiceUnavailable, msgServiceUnavailable5)
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
		contract.RenderError(c, http.StatusBadRequest, contract.CodeBadRequest, msgBadRequest6)
		return "", false
	}
	if fh.Size > controlBrandingLogoMaxBytes {
		contract.RenderError(c, http.StatusBadRequest, contract.CodeBadRequest,
			fmt.Sprintf(msgValidateControlUploadFile, controlBrandingLogoMaxBytes/1024/1024))
		return "", false
	}
	facts, err := imageformat.InspectFileHeader(fh)
	if err != nil {
		slog.Warn("control branding logo inspect failed", "err", err)
		contract.RenderError(c, http.StatusBadRequest, contract.CodeBadRequest, msgBadRequest8)
		return "", false
	}
	if !imageformat.UploadAllowedImageExtensions()[facts.Extension] {
		contract.RenderError(c, http.StatusBadRequest, contract.CodeBadRequest,
			msgValidateControlUploadFile2+imageformat.SupportedUploadImageLabel()+msgValidateControlUploadFile3)
		return "", false
	}
	return facts.Extension, true
}
