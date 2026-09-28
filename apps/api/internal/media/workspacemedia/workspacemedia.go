// Package workspacemedia owns workspace-scoped upload sessions and media
// assets for the Public Data Plane v1 (issue #803, restored in #820 as
// Phase 4-03 of #809).
//
// Protocol split (Baseline 6.3): session endpoints speak the JSON
// envelope only. Byte transfer rides the opaque same-origin slot named
// in upload_target (no bucket/secret/path ever leaves the server); the
// slot key below is server-side only.
//
// Session state machine: pending -> transferring -> completed,
// transferring -> failed, pending/transferring -> expired. Sessions are
// durable rows (unlike the pre-#809 ephemeral process-local sessions):
// abandoned sessions linger until lazily expired on read (no sweeper).
// Completion-only durability: only a successful complete creates the
// durable Media Asset row.
//
// Schema convergences (the canonical schema.sql wins over older
// assumptions; there is no masset_ prefix anywhere):
//   - Public ids are upload_ sessions and asset_ assets (kernel
//     PrefixUpload/PrefixAsset, already registered; no new kernel
//     constant was needed).
//   - upload_sessions carries no filename/media_kind/updated_at/
//     expires_at columns: the slot (storage_key) is reserved eagerly at
//     create from the declared filename, the kind derives from the
//     declared MIME (image-only in this slice), expires_at computes
//     from created_at + TTL, and updated_at falls back to the latest
//     lifecycle timestamp.
//   - media_assets carries no status column: existence means ready
//     (completion accepted declarations only; byte verification is a
//     future gate). archived_at marks the archive lifecycle: metadata
//     stays readable, bytes go 410.
//   - Derivatives live in media_asset_derivatives keyed by
//     (asset_id, variant_name), NOT separate media_asset rows.
//   - Storage keys are opaque to the legacy UploadResource parser by
//     design: v1 bytes are served only through the profile+key seam,
//     never URL-parsed. Keys embed the workspace suffix for isolation.
//   - Reads use the builtin-local storage profile constant (profile
//     selection lands with the byte backend).
package workspacemedia

import (
	"context"
	"errors"
	"path"
	"strings"
	"time"

	"ideaboard/api/internal/media/imageformat"
	"ideaboard/api/internal/platform/contract"
)

// SessionStatus is the upload session lifecycle state. The underlying
// wire and database words are unchanged ("pending", "transferring",
// "completed", "failed", "expired"); the type only documents the
// allowed transitions at compile time (see CanTransition). SQL
// literals in internal/db/queries/workspace_media.sql stay
// byte-identical.
//
// Session and asset lifecycles are DISTINCT: media_assets carries no
// status column (existence means ready), so asset readiness stays the
// untyped AssetStatusReady string below and never becomes a
// SessionStatus.
type SessionStatus string

// Session states (Baseline 6.3 uploads tree).
const (
	StatusPending      SessionStatus = "pending"
	StatusTransferring SessionStatus = "transferring"
	StatusCompleted    SessionStatus = "completed"
	StatusFailed       SessionStatus = "failed"
	StatusExpired      SessionStatus = "expired"
)

// allowedSessionTransitions mirrors the durable guards in
// internal/db/queries/workspace_media.sql: CreateUploadSession inserts
// pending; MarkUploadSessionTransferring fences pending ->
// transferring (first read); Complete/Fail/Expire flip a live
// (pending/transferring) session to completed/failed/expired.
// Terminal states (completed/failed/expired) have no outgoing
// transitions; FailSession replays an already-failed session
// idempotently at the service layer (returned row, no database flip),
// mirroring the workspacetasks.Cancel precedent.
var allowedSessionTransitions = map[SessionStatus]map[SessionStatus]struct{}{
	StatusPending: {
		StatusTransferring: {},
		StatusCompleted:    {},
		StatusFailed:       {},
		StatusExpired:      {},
	},
	StatusTransferring: {
		StatusCompleted: {},
		StatusFailed:    {},
		StatusExpired:   {},
	},
}

// CanTransition reports whether a from -> to flip is durably allowed.
// It is documentation, not a gate: every transition it reports true
// for was already possible through the SQL guards, and no previously
// possible transition reports false.
func CanTransition(from, to SessionStatus) bool {
	nexts, ok := allowedSessionTransitions[from]
	if !ok {
		return false
	}
	_, ok = nexts[to]
	return ok
}

// Asset readiness. Created assets are ready: completion accepted
// declarations only, so the durable row itself is the readiness gate
// (byte verification lands as a future gate).
const (
	AssetStatusReady = "ready"
)

// Declared media kinds. Only image is accepted in this slice: it is the
// only kind with a byte-inspection seam (internal/media/imageformat).
// video/audio/document/other stay 422 until their validators land;
// the kind field keeps the wire extensible.
const MediaKindImage = "image"

// Retention TTLs. Expiry is lazy on read (no background sweeper:
// sessions hold no durable resources beyond their row).
const (
	// UploadPendingTTL bounds pending -> expired without transfer start.
	UploadPendingTTL = 30 * time.Minute
	// UploadTransferTTL bounds transferring -> expired without complete.
	UploadTransferTTL = 2 * time.Hour
)

// Transfer bounds. MaxUploadSizeBytes matches the legacy board-upload
// 30MiB cap so workspace uploads never accept more than the existing
// multipart path.
const (
	MaxUploadSizeBytes = 30 * 1024 * 1024
	MaxFilenameRunes   = 255
)

// DefaultStorageProfile is the reserved-slot profile until the byte
// backend owns profile selection. The slot key below it is
// server-side only.
const DefaultStorageProfile = "builtin-local"

var (
	// ErrNotFound reports an invisible session/asset (unknown id,
	// malformed shape at lookup, or cross-workspace id). Handlers
	// render it as 404 (upload_not_found / media_not_found) per
	// Baseline 4 anti-enumeration.
	ErrNotFound = errors.New("workspacemedia: not found")
	// ErrExpired reports a lazily-expired session (410 upload_expired).
	ErrExpired = errors.New("workspacemedia: upload expired")
	// ErrStateConflict reports a terminal-state mutation attempt
	// (409 upload_state_conflict).
	ErrStateConflict = errors.New("workspacemedia: illegal state transition")
	// ErrDerivativeSourceMissing reports derivative generation with no
	// readable original bytes (409 at the binary endpoint, never 500).
	ErrDerivativeSourceMissing = errors.New("workspacemedia: derivative source missing")
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
	return "workspacemedia: validation failed"
}

// NewFieldValidationError builds a single-field 422 failure for callers
// validating completion echoes outside ValidateCreateInput.
func NewFieldValidationError(field string) *ValidationError {
	return newValidationError(FieldError{Field: field, Message: "invalid"})
}

func newValidationError(pairs ...FieldError) *ValidationError {
	fields := make(map[string]string, len(pairs))
	for _, pair := range pairs {
		fields[pair.Field] = pair.Message
	}
	return &ValidationError{Fields: fields}
}

// Session is one upload coordination record.
type Session struct {
	ID             string
	WorkspaceID    string
	OwnerUserID    int64
	Filename       string
	MIMEType       string
	SizeBytes      int64
	ChecksumSHA256 string
	MediaKind      string
	Status         SessionStatus
	StorageKey     string
	AssetID        string
	CreatedAt      time.Time
	CompletedAt    *time.Time
	ExpiredAt      *time.Time
	ExpiresAt      time.Time
	UpdatedAt      time.Time
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

// IsLive reports whether the session may still complete or fail.
func (s *Session) IsLive() bool {
	return s.Status == StatusPending || s.Status == StatusTransferring
}

// Asset is the workspace-bound media asset projection. InternalID and
// StorageKey stay server-side and are never serialized to clients.
type Asset struct {
	InternalID     int64
	ID             string
	WorkspaceID    string
	OwnerUserID    int64
	Status         string
	MediaKind      string
	MIMEType       string
	Width          int32
	Height         int32
	SizeBytes      int64
	ChecksumSHA256 string
	StorageKey     string
	ArchivedAt     *time.Time
	CreatedAt      time.Time
}

// IsArchived reports whether the asset carries the archive lifecycle
// marker (archived_at).
func (a *Asset) IsArchived() bool {
	return a != nil && a.ArchivedAt != nil
}

// Derivative is the server-side projection of one materialized
// derivative row used by generation and the v1 derivative-bytes
// endpoint.
type Derivative struct {
	VariantName string
	MIMEType    string
	Width       int32
	Height      int32
	SizeBytes   int64
	StorageKey  string
	CreatedAt   time.Time
}

// TaskAsset is one media asset attached to a task output, with the
// output role from task_outputs.metadata (#819's mapping).
type TaskAsset struct {
	Asset       *Asset
	OutputIndex int32
	Role        string
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

// CreateAssetParams carries validated asset creation input for the
// atomic complete path. PublicID carries the handler-minted asset_ id
// (the store regenerates internally on collision).
type CreateAssetParams struct {
	WorkspaceID    string
	OwnerUserID    int64
	PublicID       string
	MediaKind      string
	MIMEType       string
	Width          int32
	Height         int32
	SizeBytes      int64
	ChecksumSHA256 string
	StorageKey     string
}

// CompleteFacts carries the client-declared completion echo validated
// against the session declarations.
type CompleteFacts struct {
	ChecksumSHA256 string
	SizeBytes      int64
	MIMEType       string
	MediaKind      string
	Width          int32
	Height         int32
}

// UpdateMetadataParams carries dimension/metadata refresh input.
type UpdateMetadataParams struct {
	Width      int32
	Height     int32
	DurationMs int32
	HasDims    bool
	HasDur     bool
	Metadata   map[string]any
}

// WorkspaceMediaStore is the workspace-bound media persistence seam:
// upload sessions (durable rows, lazy expiry) plus media assets,
// derivatives, and task-media reads. Repository methods take
// context.Context first and use the dbWithContext pattern in the
// postgres sibling; the in-memory sibling is tests-only.
type WorkspaceMediaStore interface {
	SessionStore
	AssetStore
}

// SessionStore is the upload-session half of the seam.
type SessionStore interface {
	CreateSession(ctx context.Context, params CreateSessionParams) (*Session, error)
	// GetSession applies lazy lifecycle transitions (pending ->
	// transferring on first read, pending/transferring -> expired past
	// the TTL) and returns ErrNotFound for cross-workspace ids so
	// sessions stay invisible outside their workspace.
	GetSession(ctx context.Context, workspaceID, uploadID string) (*Session, error)
	// CompleteSession atomically inserts the durable asset row and
	// flips the session to completed (completion-only durability).
	// Facts must echo the session declarations; mismatches fail the
	// session and report a ValidationError.
	CompleteSession(ctx context.Context, workspaceID, uploadID string, facts CompleteFacts, asset CreateAssetParams) (*Session, *Asset, error)
	// FailSession marks a live session failed with a server-side reason.
	FailSession(ctx context.Context, workspaceID, uploadID, reason string) (*Session, error)
}

// AssetStore is the media-asset half of the seam.
type AssetStore interface {
	// GetAsset returns ErrNotFound for cross-workspace ids (invisible,
	// never a distinct status). Archived assets stay readable here;
	// binary endpoints gate them separately.
	GetAsset(ctx context.Context, workspaceID, assetID string) (*Asset, error)
	// ListAssets returns creation-ordered (newest first) non-archived
	// assets; limit is clamped to [1,100] like the contract kernel.
	ListAssets(ctx context.Context, workspaceID string, limit, offset int) ([]*Asset, error)
	// ArchiveAsset stamps the archive lifecycle marker. Repeat
	// archives converge on the stored row (idempotent per asset).
	ArchiveAsset(ctx context.Context, workspaceID, assetID string) (*Asset, error)
	// UpdateAssetMetadata refreshes dimensions and merges metadata.
	UpdateAssetMetadata(ctx context.Context, workspaceID, assetID string, params UpdateMetadataParams) (*Asset, error)
	// RecordDerivative upserts one materialized derivative variant.
	RecordDerivative(ctx context.Context, workspaceID, assetID string, derivative Derivative) (*Derivative, error)
	// ListDerivatives returns stored variants for one asset.
	ListDerivatives(ctx context.Context, workspaceID, assetID string) ([]*Derivative, error)
	// ListTaskAssets queries media by task via task_outputs
	// (media_asset outputs only, ordered by output_index).
	ListTaskAssets(ctx context.Context, workspaceID, taskID string) ([]*TaskAsset, error)
}

// ValidateCreateInput enforces the declaration contract shared by
// create and complete: filename shape, MIME allowlist, size bounds,
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

// IsValidChecksum reports whether s is a 64-hex sha256 digest (any
// case). Empty is invalid: create and complete must both declare.
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

// MediaKindForMIME derives the durable media kind from a validated
// MIME. Only image resolves today; everything else is empty (the
// caller 422s before persisting).
func MediaKindForMIME(mimeType string) string {
	if CanonicalMIME(mimeType) == "" {
		return ""
	}
	return MediaKindImage
}

// NewUploadPublicID mints an opaque upload_ session identifier.
func NewUploadPublicID() (string, error) {
	return contract.MintPublicID(contract.PrefixUpload, 12)
}

// NewAssetPublicID mints an opaque asset_ identifier for durable assets.
func NewAssetPublicID() (string, error) {
	return contract.MintPublicID(contract.PrefixAsset, 12)
}

// IsUploadID reports upload_ shape (opaque URL segment for {upload_id}).
func IsUploadID(id string) bool { return contract.IsUploadID(id) }

// IsAssetID reports asset_ shape (opaque URL segment for {asset_id}).
func IsAssetID(id string) bool { return contract.IsAssetID(id) }

// SlotKey reserves the server-side storage locator for a session. The
// key never leaves the server: transfer instructions carry only the
// opaque slot (the upload_ id), never bucket/secret/path. The workspace
// suffix isolates tenants at the key level.
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

// SlotFilename recovers the reserved basename from a slot key (the
// sanitized original declared at create).
func SlotFilename(slotKey string) string {
	base := path.Base(strings.TrimSpace(slotKey))
	if base == "." || base == "" || base == "/" {
		return "upload.bin"
	}
	return base
}

// SanitizeFilename strips path separators and bounds length for key
// use. Validation already rejected '/' and overlong names; this is
// belt and suspenders for storage keys.
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

// DerivativeSlotKey reserves the storage locator for one derivative
// variant under its parent asset.
func DerivativeSlotKey(workspaceID, assetID, variant, ext string) string {
	wsSuffix := strings.TrimPrefix(strings.TrimSpace(workspaceID), "ws_")
	assetSuffix := strings.TrimPrefix(strings.TrimSpace(assetID), "asset_")
	cleanExt := strings.TrimSpace(ext)
	if !strings.HasPrefix(cleanExt, ".") {
		cleanExt = "." + cleanExt
	}
	if wsSuffix == "" {
		wsSuffix = "unknown"
	}
	if assetSuffix == "" {
		assetSuffix = "unknown"
	}
	safeVariant := SanitizeFilename(strings.TrimSpace(variant))
	return "ws-uploads/" + wsSuffix + "/" + assetSuffix + "/derivatives/" + safeVariant + cleanExt
}
