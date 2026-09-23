package workspacetasks

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"ideaboard/api/internal/platform/contract"
	"ideaboard/api/internal/workflow"
	"ideaboard/api/internal/workflow/paramkeys"
)

// Task statuses (subset of the durable task lifecycle; pending -> running
// -> completed/failed/cancelled). cancelled_with_results converges to
// cancelled for the v1 surface (outputs carry the results).
const (
	StatusPending   = "pending"
	StatusRunning   = "running"
	StatusCompleted = "completed"
	StatusFailed    = "failed"
	StatusCancelled = "cancelled"
)

// MaxProviderRequeues bounds worker-level retries for transient provider
// failures (429/5xx/timeout/network) after gateway-internal retries. It
// mirrors execution.maxProviderFailureRequeues (2): deterministic failures
// go terminal directly. Retry budgeting reads the fresh retry_count row;
// a queued payload copy would go stale across redeliveries.
const MaxProviderRequeues = 2

// MaxOutputsPerTask bounds persisted output refs per task.
const MaxOutputsPerTask = 4

// MaxMediaInputs bounds media_asset_ids per create.
const MaxMediaInputs = 8

var (
	// ErrNotFound reports an invisible task (unknown id, malformed shape
	// at lookup, or cross-workspace id). Handlers render it as 404
	// task_not_found per Baseline 4 anti-enumeration.
	ErrNotFound = errors.New("workspacetasks: not found")
	// ErrForbidden reports a visible-but-forbidden task operation (403
	// task_forbidden).
	ErrForbidden = errors.New("workspacetasks: forbidden")
	// ErrStateConflict reports a terminal-state mutation (409
	// task_state_conflict).
	ErrStateConflict = errors.New("workspacetasks: illegal state transition")
	// ErrValidation reports a 422-class failure (kind/assets/params).
	ErrValidation = errors.New("workspacetasks: validation failed")
	// ErrInsufficientCredits reports a hold failure for lack of Ideas
	// (402 task_insufficient_credits).
	ErrInsufficientCredits = errors.New("workspacetasks: insufficient credits")
	// ErrQuoteUnavailable reports a server-side price snapshot failure
	// (503 billing_quote_unavailable).
	ErrQuoteUnavailable = errors.New("workspacetasks: quote unavailable")
	// ErrWalletRestricted reports a frozen/restricted wallet (403
	// billing_wallet_restricted).
	ErrWalletRestricted = errors.New("workspacetasks: wallet restricted")
	// ErrIdempotencyConflict reports key reuse with a different request
	// (409 task_idempotency_conflict).
	ErrIdempotencyConflict = errors.New("workspacetasks: idempotency conflict")
	// ErrMediaNotFound wraps an invisible input/output asset.
	ErrMediaNotFound = errors.New("workspacetasks: media not found")
	// ErrMediaNotReady wraps a non-ready asset (409 media_not_ready).
	ErrMediaNotReady = errors.New("workspacetasks: media not ready")
	// ErrMediaArchived wraps an archived asset (410 media_archived).
	ErrMediaArchived = errors.New("workspacetasks: media archived")
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

func (e *ValidationError) Error() string { return ErrValidation.Error() }

func newValidationError(pairs ...FieldError) *ValidationError {
	fields := make(map[string]string, len(pairs))
	for _, pair := range pairs {
		fields[pair.Field] = pair.Message
	}
	return &ValidationError{Fields: fields}
}

// IsValidationError reports whether err is a 422-class failure.
func IsValidationError(err error) bool {
	var target *ValidationError
	return errors.As(err, &target)
}

// BillingSnapshot is the immutable price fact persisted with a task.
// Money is integer minor units (credits) throughout; snapshots are
// INSERT-only in the durable implementation.
type BillingSnapshot struct {
	SKU               string    `json:"sku"`
	DisplayName       string    `json:"display_name"`
	PriceVersionID    int64     `json:"price_version_id"`
	PriceVersion      int64     `json:"price_version"`
	OutputResolution  string    `json:"output_resolution"`
	UnitCredits       int64     `json:"unit_credits"`
	Quantity          int64     `json:"quantity"`
	TotalCredits      int64     `json:"total_credits"`
	QuotedAt          time.Time `json:"quoted_at"`
	HoldTransactionID string    `json:"hold_transaction_id,omitempty"`
	SettlementState   string    `json:"settlement_state"`
}

// OutputRef references one task output: either a byte-verified ready
// asset_... Media Asset (via the #803 seam) or an explicit remote-output
// contract (URL + checksum + kind, no storage internals).
type OutputRef struct {
	Kind           string `json:"kind"`
	AssetID        string `json:"asset_id,omitempty"`
	URL            string `json:"url,omitempty"`
	ChecksumSHA256 string `json:"checksum_sha256,omitempty"`
	Role           string `json:"role,omitempty"`
	ResultIndex    int    `json:"result_index"`
}

const (
	// OutputKindAsset references a durable asset_... Media Asset.
	OutputKindAsset = "asset"
	// OutputKindRemote references an explicit remote output (URL +
	// checksum + kind, no storage internals).
	OutputKindRemote = "remote"
)

// WorkspaceTask is one workspace-scoped async job. RetryCount,
// ExecutionToken, OutputRefs, and BillingSnapshot persist as durable
// facts; the store never exposes internal numeric keys.
type WorkspaceTask struct {
	ID              string          `json:"id"`
	WorkspaceID     string          `json:"workspace_id"`
	OwnerUserID     int64           `json:"owner_user_id"`
	WorkflowKind    string          `json:"workflow_kind"`
	MediaAssetIDs   []string        `json:"media_asset_ids,omitempty"`
	Params          map[string]any  `json:"params,omitempty"`
	Status          string          `json:"status"`
	RetryCount      int             `json:"retry_count"`
	ExecutionToken  string          `json:"execution_token"`
	OutputRefs      []OutputRef     `json:"output_refs,omitempty"`
	BillingSnapshot BillingSnapshot `json:"billing_snapshot"`
	ErrorMsg        string          `json:"error_msg,omitempty"`
	CancelReason    string          `json:"cancel_reason,omitempty"`
	CreatedAt       time.Time       `json:"created_at"`
	UpdatedAt       time.Time       `json:"updated_at"`
	CompletedAt     *time.Time      `json:"completed_at,omitempty"`
}

// IsTerminal reports whether the task accepts no further transitions.
func (t *WorkspaceTask) IsTerminal() bool {
	switch t.Status {
	case StatusCompleted, StatusFailed, StatusCancelled:
		return true
	default:
		return false
	}
}

// TaskEvent is one staged status event. Events emit ONLY after commit
// (for #807 task.* fanout); staging never publishes inside the txn.
type TaskEvent struct {
	TaskID      string    `json:"task_id"`
	WorkspaceID string    `json:"workspace_id"`
	Status      string    `json:"status"`
	OccurredAt  time.Time `json:"occurred_at"`
}

// CreateParams carries validated task creation input. Repository methods
// take context.Context first.
type CreateParams struct {
	WorkspaceID   string
	OwnerUserID   int64
	WorkflowKind  string
	MediaAssetIDs []string
	Params        map[string]any
}

// QuoteParams carries preflight quote input (no persistence).
type QuoteParams struct {
	WorkflowKind string
	Quantity     int64
	Resolution   string
}

// Quoter mints immutable server-side price snapshots. Production adapts
// the Billing catalog; tests stub this interface.
type Quoter interface {
	Quote(ctx context.Context, workflowKind, resolution string, quantity int64) (BillingSnapshot, error)
}

// Holder reserves Ideas via Credits Commands (never direct ledger
// writes). Production adapts credits.Commands; tests stub it.
type Holder interface {
	Hold(ctx context.Context, userID int64, amount int64, idempotencyKey, taskID string) (string, error)
	Capture(ctx context.Context, holdTransactionID string, amount int64, taskID string) error
	Release(ctx context.Context, holdTransactionID string, amount int64, taskID string) error
}

// Enqueuer carries the River payload. Production adapts the River queue;
// tests stub it (including forced failures for rollback proof).
type Enqueuer interface {
	Enqueue(ctx context.Context, task *WorkspaceTask) error
}

// AssetInfo is the minimal ready-gate projection over the #803 seam.
type AssetInfo struct {
	ID          string
	WorkspaceID string
	Status      string
	Archived    bool
}

// AssetReader checks byte-verified readiness. Production adapts
// workspaceuploads.AssetStore; tests stub it.
type AssetReader interface {
	GetAsset(ctx context.Context, workspaceID, assetID string) (*AssetInfo, error)
}

// ValidateCreate enforces the discriminated input union:
// workflow_kind against the workflow features registry, media_asset_ids
// as asset_... only (no raw storage URLs), and params as a bounded map
// with no storage internals or unknown fields.
func ValidateCreate(kind string, mediaAssetIDs []string, params map[string]any) (string, error) {
	canonical, ok := workflow.ParseAIImageFeatureSlug(strings.TrimSpace(kind))
	if !ok {
		if _, perr := workflow.ValidateWorkflowFeatureSlug(strings.TrimSpace(kind)); perr != nil {
			return "", newValidationError(FieldError{Field: "workflow_kind", Message: "unsupported"})
		}
		return "", newValidationError(FieldError{Field: "workflow_kind", Message: "unsupported"})
	}
	if len(mediaAssetIDs) > MaxMediaInputs {
		return "", newValidationError(FieldError{Field: "media_asset_ids", Message: "too_many"})
	}
	for i, id := range mediaAssetIDs {
		trimmed := strings.TrimSpace(id)
		if !contract.IsAssetID(trimmed) {
			return "", newValidationError(FieldError{Field: fmt.Sprintf("media_asset_ids[%d]", i), Message: "invalid"})
		}
		if !contract.IsValidPublicID(contract.PrefixAsset, trimmed) {
			return "", newValidationError(FieldError{Field: fmt.Sprintf("media_asset_ids[%d]", i), Message: "invalid"})
		}
	}
	if err := ValidateParams(params); err != nil {
		return "", err
	}
	return canonical, nil
}

// ValidateParams rejects unknown fields and raw storage URLs. Unknown
// fields fail closed (422) per Baseline 5.1; storage URLs fail closed
// because persisted task params must never contain media URLs (the
// canonical media contract carries AssetID + role facts only).
func ValidateParams(params map[string]any) error {
	if params == nil {
		return nil
	}
	for key, value := range params {
		if !paramkeys.IsKnownSubmitTaskParamKey(key) && !paramkeys.IsCanonicalTaskParamKey(key) {
			return newValidationError(FieldError{Field: "params." + key, Message: "unknown_field"})
		}
		if paramkeys.IsLegacyPersistedMediaKey(key) {
			return newValidationError(FieldError{Field: "params." + key, Message: "raw_media_url_forbidden"})
		}
		if containsStorageURL(value) {
			return newValidationError(FieldError{Field: "params." + key, Message: "raw_storage_url_forbidden"})
		}
	}
	return nil
}

func containsStorageURL(value any) bool {
	switch typed := value.(type) {
	case string:
		lowered := strings.ToLower(strings.TrimSpace(typed))
		for _, marker := range []string{"http://", "https://", "s3://", "storage_key", "ws-uploads", "builtin-local", "bucket"} {
			if strings.Contains(lowered, marker) {
				return true
			}
		}
		return false
	case []any:
		for _, item := range typed {
			if containsStorageURL(item) {
				return true
			}
		}
		return false
	case []string:
		for _, item := range typed {
			if containsStorageURL(item) {
				return true
			}
		}
		return false
	case map[string]any:
		for _, item := range typed {
			if containsStorageURL(item) {
				return true
			}
		}
		return false
	default:
		return false
	}
}

// ValidateOutputRef enforces the output contract: asset refs carry only
// asset_... ids (byte-verified ready, enforced at settle time via
// AssetReader); remote refs carry URL + checksum + kind with no storage
// internals.
func ValidateOutputRef(ref OutputRef) error {
	switch ref.Kind {
	case OutputKindAsset:
		if !contract.IsAssetID(strings.TrimSpace(ref.AssetID)) {
			return newValidationError(FieldError{Field: "asset_id", Message: "invalid"})
		}
		if strings.TrimSpace(ref.URL) != "" || strings.TrimSpace(ref.ChecksumSHA256) != "" {
			return newValidationError(FieldError{Field: "kind", Message: "mixed_output_contract"})
		}
		return nil
	case OutputKindRemote:
		url := strings.TrimSpace(ref.URL)
		if url == "" || (!strings.HasPrefix(strings.ToLower(url), "https://") && !strings.HasPrefix(strings.ToLower(url), "http://")) {
			return newValidationError(FieldError{Field: "url", Message: "invalid"})
		}
		lowered := strings.ToLower(url)
		for _, marker := range []string{"ws-uploads", "builtin-local", "storage_key"} {
			if strings.Contains(lowered, marker) {
				return newValidationError(FieldError{Field: "url", Message: "storage_internals_forbidden"})
			}
		}
		if !isValidChecksum(ref.ChecksumSHA256) {
			return newValidationError(FieldError{Field: "checksum_sha256", Message: "invalid"})
		}
		if strings.TrimSpace(ref.AssetID) != "" {
			return newValidationError(FieldError{Field: "kind", Message: "mixed_output_contract"})
		}
		return nil
	default:
		return newValidationError(FieldError{Field: "kind", Message: "unsupported"})
	}
}

func isValidChecksum(s string) bool {
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

// NewTaskPublicID mints an opaque task_ identifier.
func NewTaskPublicID() (string, error) {
	return contract.MintPublicID(contract.PrefixTask, 12)
}

// NewExecutionToken mints a fresh execution token for retry fencing
// (reset + River backoff, new token per attempt, bounded by retry_count).
// Tokens are opaque hex with an exec_ prefix; they are never public ids
// and never enter IsValidPublicID checks.
func NewExecutionToken() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("workspacetasks: mint execution token: %w", err)
	}
	return "exec_" + hex.EncodeToString(buf), nil
}

// IsRetryableProviderError classifies provider failures: transient
// (429/5xx/timeout/network) may consume the retry budget; deterministic
// failures go terminal directly. Category-based classification keeps the
// API slice free of provider SDK imports; the worker maps gateway errors
// to these categories.
func IsRetryableProviderError(statusCode int, err error) bool {
	if statusCode == 429 || (statusCode >= 500 && statusCode <= 599) || statusCode == 408 {
		return true
	}
	if err != nil {
		msg := strings.ToLower(err.Error())
		for _, marker := range []string{"timeout", "deadline exceeded", "network", "connection reset", "temporary", "econnreset", "econnrefused"} {
			if strings.Contains(msg, marker) {
				return true
			}
		}
	}
	return false
}

// ShouldRetry reports whether a failed attempt may requeue (reset + River
// backoff, new execution token) or must converge terminally. The budget
// is MaxProviderRequeues whole-job redeliveries; deterministic failures
// never retry regardless of budget.
func ShouldRetry(retryCount int, retryable bool) bool {
	return retryable && retryCount < MaxProviderRequeues
}

// IsTerminalStatus reports whether status is terminal.
func IsTerminalStatus(status string) bool {
	switch status {
	case StatusCompleted, StatusFailed, StatusCancelled:
		return true
	default:
		return false
	}
}
