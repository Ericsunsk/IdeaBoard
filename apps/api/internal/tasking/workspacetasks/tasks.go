package workspacetasks

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	creditsdomain "ideaboard/api/internal/credits/domain"
	"ideaboard/api/internal/events"
	"ideaboard/api/internal/generation/routing"
	quotadomain "ideaboard/api/internal/memberquotas"
	"ideaboard/api/internal/platform/contract"
	"ideaboard/api/internal/workflow"
	"ideaboard/api/internal/workflow/paramkeys"

	"github.com/jackc/pgx/v5"
)

// Status is the workspace task lifecycle state. The underlying wire and
// database words are unchanged ("pending", "running", "completed",
// "failed", "cancelled"); the type only documents the allowed transitions
// at compile time (see CanTransition). SQL literals in
// internal/db/queries/workspace_tasks.sql stay byte-identical.
type Status string

// Task statuses (subset of the durable task lifecycle; pending -> running
// -> completed/failed/cancelled). cancelled_with_results converges to
// cancelled for the v1 surface (outputs carry the results).
const (
	StatusPending   Status = "pending"
	StatusRunning   Status = "running"
	StatusCompleted Status = "completed"
	StatusFailed    Status = "failed"
	StatusCancelled Status = "cancelled"
)

// allowedTaskTransitions mirrors the durable guards in
// internal/db/queries/workspace_tasks.sql (and the chk_tasks_status CHECK):
// Claim fences pending -> running; Complete/Fail/Cancel flip a live
// (pending/running) task to completed/failed/cancelled; Retry returns a
// live task to pending. Terminal states (completed/failed/cancelled) have
// no outgoing transitions; Cancel replays idempotently at the service
// layer (already=true) without a database flip.
var allowedTaskTransitions = map[Status]map[Status]struct{}{
	StatusPending: {
		StatusRunning:   {},
		StatusPending:   {},
		StatusCompleted: {},
		StatusFailed:    {},
		StatusCancelled: {},
	},
	StatusRunning: {
		StatusPending:   {},
		StatusCompleted: {},
		StatusFailed:    {},
		StatusCancelled: {},
	},
}

// CanTransition reports whether a from -> to flip is durably allowed. It
// is documentation, not a gate: every transition it reports true for was
// already possible through the SQL guards, and no previously possible
// transition reports false.
func CanTransition(from, to Status) bool {
	nexts, ok := allowedTaskTransitions[from]
	if !ok {
		return false
	}
	_, ok = nexts[to]
	return ok
}

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

// MaxWorkspaceTaskPromptBytes bounds the v1 data.prompt input in bytes.
// It matches the canonical submit bound (8KB):
// generation/inpaint require a prompt, render/variations/fusion/outpaint
// accept an optional prompt, and cutout/upscale ignore it (dropped at
// canonical compile).
const MaxWorkspaceTaskPromptBytes = 8 * 1024

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
	// ErrInsufficientCredits is the canonical credits hold failure
	// (402 task_insufficient_credits). The holder propagates it unchanged.
	ErrInsufficientCredits = creditsdomain.ErrInsufficientCredits
	// ErrQuoteUnavailable reports a server-side price snapshot failure
	// (503 billing_quote_unavailable). It stays a distinct routing
	// marker (never a canonical alias): adapters pair it with the
	// underlying billing cause via multi-%w, and the 422-vs-503 split in
	// renderTaskError depends on that pairing.
	ErrQuoteUnavailable = errors.New("workspacetasks: quote unavailable")
	// ErrSettlementUnavailable reports a missing atomic settlement seam.
	// Production task mutation never falls back to compensating writes.
	ErrSettlementUnavailable = errors.New("workspacetasks: settlement unavailable")
	// ErrWalletRestricted is the canonical credits restriction failure
	// (403 billing_wallet_restricted). The holder propagates it unchanged.
	ErrWalletRestricted = creditsdomain.ErrWalletRestricted
	// ErrMemberQuotaExhausted reports that a submit would exceed the
	// member's remaining cross-workspace consumption quota for the
	// current window (403 member_quota_exhausted, #946). Alias, not a
	// copy: the quota seam pairs it via multi-%w and renderTaskError
	// keeps it distinct from 402 insufficiency and 403 wallet restriction.
	ErrMemberQuotaExhausted = quotadomain.ErrQuotaExhausted
	// ErrMediaNotFound wraps an invisible input/output asset.
	ErrMediaNotFound = errors.New("workspacetasks: media not found")
	// ErrMediaNotReady wraps a non-ready asset (409 media_not_ready).
	ErrMediaNotReady = errors.New("workspacetasks: media not ready")
	// ErrMediaArchived wraps an archived asset (410 media_archived).
	ErrMediaArchived = errors.New("workspacetasks: media archived")
	// ErrBoardNotFound wraps an invisible Board binding (unknown id or no
	// effective Board role for the caller). Handlers render it as 404
	// board_not_found, never leaking Board existence.
	ErrBoardNotFound = errors.New("workspacetasks: board not found")
	// ErrBoardArchived wraps a generation attempt on an archived Board
	// (409 board_archived, matching the Boards write contract).
	ErrBoardArchived = errors.New("workspacetasks: board archived")
)

// RoutingSnapshotSaver persists the immutable submit-time RoutingSnapshot.
// Production joins the settlement transaction (tx non-nil) via
// catalog.SaveRoutingSnapshot so the snapshot commits atomically with the
// Credit Hold, task insert, and River enqueue. Tests stub it to capture the
// pinned fact. A nil saver is only valid for test fixtures; production wiring
// always supplies the saver and the worker fails closed on a missing snapshot.
type RoutingSnapshotSaver func(ctx context.Context, tx pgx.Tx, taskID string, snap routing.RoutingSnapshot) error

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
//
// BillingAccountID is the funds identity (#916): the stable Billing Account
// whose Wallet was held. It never derives from the current Workspace Owner;
// the submitter rides OwnerUserID as the real operator (Actor).
type BillingSnapshot struct {
	BillingAccountID  int64     `json:"billing_account_id"`
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
	Width          int32  `json:"width,omitempty"`
	Height         int32  `json:"height,omitempty"`
	MIMEType       string `json:"mime_type,omitempty"`
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
//
// ResolvedModel is the transient execution-target handoff inside the
// worker only (#854: the durable workflow_input envelope key is
// removed). It is never set at submit, never persisted, and never read
// back from a store: the worker stamps the snapshot-derived target onto
// its in-memory claimed copy before calling the provider bridge, which
// consumes only this field. Readers must treat any stored value as
// absent; the snapshot is the sole routing fact.
type WorkspaceTask struct {
	ID          string `json:"id"`
	WorkspaceID string `json:"workspace_id"`
	OwnerUserID int64  `json:"owner_user_id"`
	// BillingAccountID is the stable funds identity resolved at submit
	// (#916): terminal Capture/Release converge on this account with
	// OwnerUserID as the operator Actor. Internal only, never on the wire.
	BillingAccountID int64          `json:"-"`
	Service          string         `json:"service"`
	MediaAssetIDs    []string       `json:"media_asset_ids,omitempty"`
	Params           map[string]any `json:"params,omitempty"`
	CanonicalParams  string         `json:"canonical_params,omitempty"`
	ResolvedModel    string         `json:"resolved_model,omitempty"`
	// RoutingSnapshot carries the immutable submit-time RoutingSnapshot
	// JSONB pinned on the task row (#848, ADR-0012 rule 9): the exact
	// routing.RoutingSnapshot JSON shape, raw so readers that do not
	// execute (HTTP views, Admin) never parse it. The worker is the only
	// consumer: it parses and Verify()s these bytes and executes the
	// pinned selected model, never the live pool and never the transient
	// ResolvedModel handoff. Nil means "never pinned" and fails
	// the worker closed.
	RoutingSnapshot json.RawMessage `json:"routing_snapshot,omitempty"`
	// BoardID is the durable Canvas Board linkage (#937): the board_
	// public id the Task was submitted from. Empty means a non-Canvas
	// task, which keeps its own workspace-level permission and is
	// submitter-only on reads (omitting the Board ID never widens
	// visibility). Persisted in the workflow_input envelope in the same
	// settlement transaction as the insert, Hold, and enqueue.
	BoardID string `json:"board_id,omitempty"`
	// BoardAccessVersion records the authorization version accepted with the Task.
	BoardAccessVersion int64           `json:"board_access_version,omitempty"`
	Status             Status          `json:"status"`
	RetryCount         int             `json:"retry_count"`
	ExecutionToken     string          `json:"execution_token"`
	OutputRefs         []OutputRef     `json:"output_refs,omitempty"`
	BillingSnapshot    BillingSnapshot `json:"billing_snapshot"`
	ErrorMsg           string          `json:"error_msg,omitempty"`
	CancelReason       string          `json:"cancel_reason,omitempty"`
	CreatedAt          time.Time       `json:"created_at"`
	UpdatedAt          time.Time       `json:"updated_at"`
	CompletedAt        *time.Time      `json:"completed_at,omitempty"`
}

// IsTerminal reports whether the task accepts no further transitions.
// Single source: delegates to IsTerminalStatus so adding a state only
// touches the status predicate below.
func (t *WorkspaceTask) IsTerminal() bool {
	return IsTerminalStatus(t.Status)
}

// TaskEvent is one staged status event. Events emit ONLY after commit
// (for #807 task.* fanout); staging never publishes inside the txn.
type TaskEvent struct {
	EventID     string                         `json:"event_id,omitempty"`
	TaskID      string                         `json:"task_id"`
	WorkspaceID string                         `json:"workspace_id"`
	Status      Status                         `json:"status"`
	OccurredAt  time.Time                      `json:"occurred_at"`
	Outputs     []events.V1TaskCompletedOutput `json:"outputs,omitempty"`
}

// BuildTaskCompletedOutputs constructs self-contained output facts for task.completed events.
func BuildTaskCompletedOutputs(workspaceID string, outputs []OutputRef) []events.V1TaskCompletedOutput {
	if len(outputs) == 0 {
		return nil
	}
	res := make([]events.V1TaskCompletedOutput, 0, len(outputs))
	workspaceID = strings.TrimSpace(workspaceID)
	for _, ref := range outputs {
		role := strings.TrimSpace(ref.Role)
		if role == "" {
			role = "result"
		}
		url := strings.TrimSpace(ref.URL)
		assetID := strings.TrimSpace(ref.AssetID)
		if assetID != "" && url == "" {
			url = "/api/v1/workspaces/" + workspaceID + "/media-assets/" + assetID + "/content"
		}
		res = append(res, events.V1TaskCompletedOutput{
			AssetID:      assetID,
			Role:         role,
			URL:          url,
			Width:        ref.Width,
			Height:       ref.Height,
			MIMEType:     strings.TrimSpace(ref.MIMEType),
			PreviewReady: true,
		})
	}
	return res
}

// CreateParams carries validated task creation input. Repository methods
// take context.Context first.
type CreateParams struct {
	WorkspaceID   string
	OwnerUserID   int64
	Service       string
	MediaAssetIDs []string
	Params        map[string]any
}

// QuoteParams carries preflight quote input (no persistence).
type QuoteParams struct {
	Service    string
	Quantity   int64
	Resolution string
}

// Quoter mints immutable server-side price snapshots. Production adapts
// the Billing catalog; tests stub this interface.
type Quoter interface {
	Quote(ctx context.Context, service, resolution string, quantity int64) (BillingSnapshot, error)
	QuoteInTx(ctx context.Context, tx pgx.Tx, service, resolution string, quantity int64) (BillingSnapshot, error)
}

// AssetInfo is the minimal ready-gate projection over the #803 seam.
type AssetInfo struct {
	ID          string
	WorkspaceID string
	Status      string
	Archived    bool
	Width       int32
	Height      int32
	MIMEType    string
}

// AssetReader checks byte-verified readiness. Production adapts the
// workspace media asset store (#813 removed the legacy postgres
// sibling; the replacement re-wires this seam); tests stub it.
//
// Seam contract (P0-4): implementations own the readiness comparison
// (media GateAsset) and report it ONLY through sentinels —
// ErrMediaArchived for archived assets, ErrMediaNotReady for non-ready
// assets, ErrMediaNotFound for invisible ids. Callers propagate those
// sentinels without inspecting AssetInfo status fields.
type AssetReader interface {
	GetAsset(ctx context.Context, workspaceID, assetID string) (*AssetInfo, error)
}

// ValidateCreate enforces the discriminated input union:
// service against the workflow features registry, media_asset_ids
// as asset_... only (no raw storage URLs), and params as a bounded map
// with no storage internals or unknown fields. Slug validation reuses
// the canonical workflow.ParseAIImageFeatureSlug.
func ValidateCreate(kind string, mediaAssetIDs []string, params map[string]any) (string, error) {
	canonical, ok := workflow.ParseAIImageFeatureSlug(strings.TrimSpace(kind))
	if !ok {
		return "", newValidationError(FieldError{Field: "service", Message: "unsupported"})
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

// ValidateWorkspacePrompt enforces the explicit v1 prompt input: an empty
// (all-whitespace) prompt means "no prompt" and is valid here; feature
// policy (required/optional/ignored) is enforced at canonical compile
// time, where the execution plan is known. Over-long prompts fail closed.
func ValidateWorkspacePrompt(prompt string) error {
	if len([]byte(prompt)) > MaxWorkspaceTaskPromptBytes {
		return newValidationError(FieldError{Field: "prompt", Message: "too_long"})
	}
	return nil
}

// IsValidMaskAssetID reports whether id is a well-formed workspace public
// asset_ mask reference. Single owner for the mask shape shared by
// ValidateParams and canonical_intake.normalizeSubmitExecutionMap.
func IsValidMaskAssetID(id string) bool {
	trimmed := strings.TrimSpace(id)
	return contract.IsAssetID(trimmed) &&
		contract.IsValidPublicID(contract.PrefixAsset, trimmed)
}

// MaskAssetIDFromParams extracts the v1 mask reference
// (params.mask_asset_id, a workspace public asset_... id minted by the
// existing upload-session flow). The second return reports presence; shape
// validation stays in ValidateParams so direct callers fail closed too.
func MaskAssetIDFromParams(params map[string]any) (string, bool) {
	if len(params) == 0 {
		return "", false
	}
	raw, ok := params[paramkeys.MaskAssetID]
	if !ok {
		return "", false
	}
	id, ok := raw.(string)
	if !ok {
		return "", false
	}
	if trimmed := strings.TrimSpace(id); trimmed != "" {
		return trimmed, true
	}
	return "", false
}

// ValidateParams rejects unknown fields and raw storage URLs. Unknown
// fields fail closed (422) per Baseline 5.1; storage URLs fail closed
// because persisted task params must never contain media URLs (the
// canonical media contract carries AssetID + role facts only).
// Persisted-media detection is owned by paramkeys.IsPersistedMediaURL.
func ValidateParams(params map[string]any) error {
	if params == nil {
		return nil
	}
	for key, value := range params {
		if !paramkeys.IsKnownSubmitTaskParamKey(key) && !paramkeys.IsCanonicalTaskParamKey(key) {
			return newValidationError(FieldError{Field: "params." + key, Message: "unknown_field"})
		}
		if key == paramkeys.MaskAssetID {
			// Contract-typed mask reference: asset_... only. Data-URL and
			// storage-URL masks cannot ride durable task params; the mask
			// bytes must be uploaded through the existing upload-session
			// flow first and linked here by asset id.
			id, _ := value.(string)
			if !IsValidMaskAssetID(id) {
				return newValidationError(FieldError{Field: "params." + key, Message: "invalid"})
			}
			continue
		}
		if paramkeys.IsPersistedMediaURL(key, value) {
			if paramkeys.IsPersistedMediaURLKey(key) {
				return newValidationError(FieldError{Field: "params." + key, Message: "raw_media_url_forbidden"})
			}
			return newValidationError(FieldError{Field: "params." + key, Message: "raw_storage_url_forbidden"})
		}
	}
	return nil
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
		for _, marker := range []string{"ws-uploads", "uploads/ws", "builtin-local", "storage_key"} {
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

// NewExecutionToken mints a fresh execution token for retry fencing
// (reset + River backoff, new token per attempt, bounded by retry_count).
func NewExecutionToken() (string, error) {
	token, err := contract.MintPublicID("exec_", 16)
	if err != nil {
		return "", fmt.Errorf("workspacetasks: mint execution token: %w", err)
	}
	return token, nil
}

// ShouldRetry reports whether a failed attempt may requeue (reset + River
// backoff, new execution token) or must converge terminally. The budget
// is MaxProviderRequeues whole-job redeliveries; deterministic failures
// never retry regardless of budget.
//
// Retryability comes only from routing.AttemptFailure.Retryable (P0-4):
// the worker maps provider errors through routing.ClassifyError and passes
// the result here. There is no second provider-error taxonomy.
func ShouldRetry(retryCount int, retryable bool) bool {
	return retryable && retryCount < MaxProviderRequeues
}

// IsTerminalStatus reports whether status is terminal.
func IsTerminalStatus(status Status) bool {
	switch status {
	case StatusCompleted, StatusFailed, StatusCancelled:
		return true
	default:
		return false
	}
}
