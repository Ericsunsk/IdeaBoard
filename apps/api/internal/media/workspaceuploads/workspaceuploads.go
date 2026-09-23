package workspaceuploads

import (
	"errors"
	"strings"
	"time"

	"ideaboard/api/internal/media/imageformat"
	"ideaboard/api/internal/platform/contract"
)

// Session states (Baseline 6.3 uploads tree).
const (
	StatusPending      = "pending"
	StatusTransferring = "transferring"
	StatusCompleted    = "completed"
	StatusFailed       = "failed"
	StatusExpired      = "expired"
)

// Declared media kinds. Only image is accepted in this slice: it is the
// only kind with a byte-inspection seam (internal/media/imageformat).
// video/audio/document/other stay 422 until their validators land;
// the kind field keeps the wire extensible for #805/#806.
const MediaKindImage = "image"

// Retention TTLs. Documented here and in the #803 report; tests pin them
// via the clock-injected in-memory stores.
const (
	// UploadPendingTTL bounds pending -> expired without transfer start.
	UploadPendingTTL = 30 * time.Minute
	// UploadTransferTTL bounds transferring -> expired without complete.
	UploadTransferTTL = 2 * time.Hour
	// WorkspaceAssetAbandonedTTL stamps reclaim_after on assets whose
	// bytes were never verified (complete accepted declarations only).
	WorkspaceAssetAbandonedTTL = 7 * 24 * time.Hour
	// WorkspaceArchivedAssetRetentionTTL stamps reclaim_after on archive.
	WorkspaceArchivedAssetRetentionTTL = 30 * 24 * time.Hour
)

// Transfer bounds. MaxUploadSizeBytes matches the legacy board-upload
// 30MiB cap so workspace uploads never accept more than the existing
// multipart path.
const (
	MaxUploadSizeBytes = 30 * 1024 * 1024
	MaxFilenameRunes   = 255
)

// DefaultStorageProfile is the reserved-slot profile until #805 owns
// profile selection. The slot key below it is server-side only.
const DefaultStorageProfile = "builtin-local"

// Metadata keys for the workspace binding on media_assets rows (no new
// columns; see doc.go).
const (
	MetadataWorkspaceKey = "workspace_id"
	MetadataArchivedKey  = "archived_at"
)

var (
	// ErrNotFound reports an invisible or missing session/asset. Handlers
	// render it as 404 (upload_not_found / media_not_found) per
	// Baseline 4 anti-enumeration.
	ErrNotFound = errors.New("workspaceuploads: not found")
	// ErrExpired reports a lazily-expired session (410 upload_expired).
	ErrExpired = errors.New("workspaceuploads: upload expired")
	// ErrStateConflict reports a terminal-state mutation attempt
	// (409 upload_state_conflict).
	ErrStateConflict = errors.New("workspaceuploads: illegal state transition")
)

// FieldError carries one field-level validation failure for
// details.fields (Baseline 5.3).
type FieldError struct {
	Field   string
	Message string
}

// ValidationError is a 422-class failure with field details.
type ValidationError struct {
	Fields map[string]string
}

func (e *ValidationError) Error() string {
	return "workspaceuploads: validation failed"
}

func newValidationError(pairs ...FieldError) *ValidationError {
	fields := make(map[string]string, len(pairs))
	for _, pair := range pairs {
		fields[pair.Field] = pair.Message
	}
	return &ValidationError{Fields: fields}
}

// NewFieldValidationError builds a single-field 422 failure for handlers
// validating completion echoes outside ValidateCreateInput.
func NewFieldValidationError(field string) *ValidationError {
	return newValidationError(FieldError{Field: field, Message: "invalid"})
}

// Session is one ephemeral upload coordination record.
type Session struct {
	ID             string
	WorkspaceID    string
	OwnerUserID    int64
	Filename       string
	MIMEType       string
	SizeBytes      int64
	ChecksumSHA256 string
	MediaKind      string
	Status         string
	AssetID        string
	FailReason     string
	CreatedAt      time.Time
	UpdatedAt      time.Time
	ExpiresAt      time.Time
}

// IsTerminal reports whether the session accepts no further transitions.
func (s *Session) IsTerminal() bool {
	switch s.Status {
	case StatusCompleted, StatusFailed, StatusExpired:
		return true
	default:
		return false
	}
}

// CreateSessionParams carries validated session creation input.
// Repository methods take context.Context first.
type CreateSessionParams struct {
	WorkspaceID    string
	OwnerUserID    int64
	Filename       string
	MIMEType       string
	SizeBytes      int64
	ChecksumSHA256 string
	MediaKind      string
}

// ValidateCreateInput enforces the declaration contract shared by create
// and complete: ownership is checked by the handler via workspace
// membership; here we check filename shape, MIME allowlist, size bounds,
// checksum shape, and declared media kind.
func ValidateCreateInput(filename, mimeType string, sizeBytes int64, checksum, kind string) error {
	var pairs []FieldError
	trimmedName := strings.TrimSpace(filename)
	if trimmedName == "" || len([]rune(trimmedName)) > MaxFilenameRunes || strings.ContainsRune(trimmedName, '/') {
		pairs = append(pairs, FieldError{Field: "filename", Message: "invalid"})
	}
	if _, ok := imageformat.NormalizeImageMIME(mimeType); !ok {
		pairs = append(pairs, FieldError{Field: "mime_type", Message: "unsupported"})
	}
	if sizeBytes < 1 || sizeBytes > MaxUploadSizeBytes {
		pairs = append(pairs, FieldError{Field: "size_bytes", Message: "out_of_range"})
	}
	if !IsValidChecksum(checksum) {
		pairs = append(pairs, FieldError{Field: "checksum_sha256", Message: "invalid"})
	}
	if strings.TrimSpace(kind) != MediaKindImage {
		pairs = append(pairs, FieldError{Field: "media_kind", Message: "unsupported"})
	}
	if len(pairs) > 0 {
		return newValidationError(pairs...)
	}
	return nil
}

// IsValidChecksum reports whether s is a lowercase-or-any-case 64-hex
// sha256 digest. Empty is invalid: create and complete must both declare.
func IsValidChecksum(s string) bool {
	trimmed := strings.TrimSpace(s)
	if len(trimmed) != 64 {
		return false
	}
	for _, r := range trimmed {
		switch {
		case r >= '0' && r <= '9', r >= 'a' && r <= 'f', r >= 'A' && r <= 'F':
		default:
			return false
		}
	}
	return true
}

// NormalizeChecksum lowercases a valid checksum for stable comparison.
func NormalizeChecksum(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

// CanonicalMIME returns the registry-canonical MIME or "" when unsupported.
func CanonicalMIME(mimeType string) string {
	normalized, ok := imageformat.NormalizeImageMIME(mimeType)
	if !ok {
		return ""
	}
	return normalized
}

// NewUploadPublicID mints an opaque upload_ session identifier.
func NewUploadPublicID() (string, error) {
	return contract.MintPublicID(contract.PrefixUpload, 12)
}

// NewAssetPublicID mints an opaque asset_ identifier for durable assets.
func NewAssetPublicID() (string, error) {
	return contract.MintPublicID(contract.PrefixAsset, 12)
}

// SlotKey reserves the server-side storage locator for a session. The key
// never leaves the server: transfer instructions carry only the opaque
// slot (the upload_ id), never bucket/secret/path.
func SlotKey(workspaceID, uploadID, filename string) string {
	wsSuffix := strings.TrimPrefix(strings.TrimSpace(workspaceID), "ws_")
	uploadSuffix := strings.TrimPrefix(strings.TrimSpace(uploadID), "upload_")
	safe := SanitizeFilename(filename)
	if wsSuffix == "" {
		wsSuffix = "unknown"
	}
	if uploadSuffix == "" {
		uploadSuffix = "unknown"
	}
	return "ws-uploads/" + wsSuffix + "/" + uploadSuffix + "/" + safe
}

// SanitizeFilename strips path separators and bounds length for key use.
// Validation already rejected '/' and overlong names; this is belt and
// suspenders for storage keys.
func SanitizeFilename(filename string) string {
	trimmed := strings.TrimSpace(filename)
	trimmed = strings.ReplaceAll(trimmed, "/", "_")
	trimmed = strings.ReplaceAll(trimmed, "\x00", "_")
	if trimmed == "" || trimmed == "." || trimmed == ".." {
		return "upload.bin"
	}
	runes := []rune(trimmed)
	if len(runes) > MaxFilenameRunes {
		trimmed = string(runes[:MaxFilenameRunes])
	}
	return trimmed
}
