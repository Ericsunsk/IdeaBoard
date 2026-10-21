package workspacetasks

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"ideaboard/api/internal/events"
	"ideaboard/api/internal/generation/routing"
	"ideaboard/api/internal/platform/contract"
	"ideaboard/api/internal/platform/telemetry"
	"ideaboard/api/internal/workflow"

	"github.com/jackc/pgx/v5"
	"go.opentelemetry.io/otel/attribute"
)

// Service orchestrates the workspace task settlement boundary:
//
//	Quote snapshot -> Credits Hold (via Holder, never direct ledger
//	writes) -> Task insert (pending, retry_count, execution_token,
//	output_refs, billing_snapshot) -> River enqueue (via Enqueuer).
//
// Production settlement commits in ONE shared PostgreSQL transaction: a
// billing failure rolls the state flip back and a state-guard failure rolls
// the billing writes back. Terminal settle (Capture on success / Release on
// fail/cancel/timeout) follows the same tx-first rule. Events stage during
// settlement and flush ONLY after commit; staging never publishes inside the
// txn (NATS fanout itself is #807). In-memory implementations are test-only
// fixtures for exercising the service contract.
type Service struct {
	taskRead      TaskReadStore
	taskLifecycle TaskLifecycleStore
	taskBilling   TaskBillingStore
	quoter        Quoter
	holder        Holder
	queue         Enqueuer
	assets        AssetReader
	acceptance    SubmissionAuthorizer
	// resolver is the authoritative Service Model Pool decision point
	// (ADR-0012 rule 10, #848). Quote and Create consult it; a nil
	// resolver fails closed with the canonical unavailable sentinel.
	resolver *routing.RoutingResolver
	// saveSnapshot persists the pinned snapshot in the settlement transaction.
	saveSnapshot RoutingSnapshotSaver
	// submissionClaims is the durable submit idempotency fact store. See
	// ServiceDeps.SubmissionClaims: production settlement fails closed
	// without it.
	submissionClaims TxSubmissionClaimStore
	billingAccounts  TxBillingAccountResolver
	events           []TaskEvent
	now              func() time.Time
}

// ServiceDeps wires the settlement seams. TaskRead and TaskLifecycle are
// independent by design; TaskBilling is an optional projection used by the
// transactional settlement path. Output refs remain part of the atomic
// lifecycle completion operation so state and outputs commit together.
// (#847: the submit-time single-value model mint is removed; the
// RoutingSnapshot pins in this transaction instead.)
type ServiceDeps struct {
	TaskRead      TaskReadStore
	TaskLifecycle TaskLifecycleStore
	TaskBilling   TaskBillingStore
	Quoter        Quoter
	Holder        Holder
	Queue         Enqueuer
	Assets        AssetReader
	// Resolver is the shared RoutingResolver quote/submit/readiness
	// consume (same instance, never an independent lookup). Nil fails
	// closed with the canonical unavailable sentinel.
	Resolver *routing.RoutingResolver
	// SnapshotSaver pins the Resolve snapshot in the settlement transaction.
	SnapshotSaver RoutingSnapshotSaver
	// SubmissionClaims is the durable submit idempotency fact store. It is
	// REQUIRED for production settlement: without it a retried submit cannot
	// be fenced and would mint a second task, take a second Hold, and
	// enqueue a second River job. NewService refuses to settle without it
	// (ErrSettlementUnavailable), so a missing wiring fails closed instead
	// of degrading into an unfenced create.
	SubmissionClaims TxSubmissionClaimStore
	Acceptance       SubmissionAuthorizer
	// BillingAccounts resolves the stable funds identity inside the
	// settlement transaction (#915 resolution, #916 funds identity). When
	// nil, production falls back to the taskLifecycle store when it
	// implements TxBillingAccountResolver (the postgres Store does); a
	// store that cannot resolve fails closed (no seam ->
	// ErrSettlementUnavailable) rather than spending an owner wallet.
	BillingAccounts TxBillingAccountResolver
}

// SubmissionAuthorizer accepts current membership, Board capability and all input
// permissions on the settlement transaction before any Quote or Hold.
type SubmissionAuthorizer interface {
	AuthorizeSubmission(context.Context, pgx.Tx, CreateInput) (TaskBoardBinding, error)
}

// NewService builds the settlement orchestrator.
func NewService(deps ServiceDeps) *Service {
	return &Service{
		taskRead: deps.TaskRead, taskLifecycle: deps.TaskLifecycle,
		taskBilling: deps.TaskBilling,
		quoter:      deps.Quoter, holder: deps.Holder,
		queue: deps.Queue, assets: deps.Assets,
		resolver: deps.Resolver, saveSnapshot: deps.SnapshotSaver, now: time.Now,
		submissionClaims: deps.SubmissionClaims, acceptance: deps.Acceptance,
		billingAccounts: deps.BillingAccounts,
	}
}

// SetClockForTests injects a deterministic clock.
func (s *Service) SetClockForTests(now func() time.Time) {
	s.now = now
}

func (s *Service) clock() time.Time {
	if s.now != nil {
		return s.now().UTC()
	}
	return time.Now().UTC()
}

// StagedEvents drains staged status events (for #807 fanout). Callers flush
// only after the settlement commit succeeds.
func (s *Service) StagedEvents() []TaskEvent {
	staged := append([]TaskEvent(nil), s.events...)
	s.events = nil
	return staged
}

func (s *Service) stageWithTokenAt(taskID, workspaceID string, status Status, executionToken string, occurredAt time.Time) {
	if occurredAt.IsZero() {
		occurredAt = s.clock()
	}
	s.events = append(s.events, TaskEvent{
		EventID: events.StableEventID(taskID, string(status), executionToken),
		TaskID:  taskID, WorkspaceID: workspaceID, Status: status, OccurredAt: occurredAt.UTC(),
	})
}

// resolveSnapshot runs the authoritative pool availability check
// (ADR-0012 rule 10): readiness, quote, and submit consume this same
// seam, never an independent lookup. An empty eligible set returns the
// sentinel (errors.Is ErrServiceResolutionUnavailable, fields via
// errors.As) before any quote, hold, task, or enqueue work.
// Deterministic caller bugs (empty service, non-canonical resolution)
// translate to ValidationError: they are never availability outcomes and
// never trigger model fallback. Store outages propagate unwrapped: an
// outage is not an empty pool and must never render the sentinel. A nil
// resolver fails closed with the same sentinel (no pool configured).
func (s *Service) resolveSnapshot(ctx context.Context, service, resolution string) (routing.RoutingSnapshot, error) {
	if s.resolver == nil {
		requested := routing.Resolution(strings.ToLower(strings.TrimSpace(resolution)))
		return routing.RoutingSnapshot{}, &routing.ResolutionUnavailableError{Service: service, Resolution: requested}
	}
	snap, err := s.resolver.Resolve(ctx, service, resolution)
	if err != nil {
		var inputErr *routing.InvalidRoutingInputError
		if errors.As(err, &inputErr) {
			return routing.RoutingSnapshot{}, newValidationError(FieldError{Field: inputErr.Field, Message: "invalid"})
		}
		return routing.RoutingSnapshot{}, fmt.Errorf("workspacetasks: routing unavailable: %w", err)
	}
	return snap, nil
}

// Quote mints an immutable price snapshot with no persistence (preflight,
// separate from authoritative create).
func (s *Service) Quote(ctx context.Context, params QuoteParams) (BillingSnapshot, error) {
	if s.quoter == nil {
		return BillingSnapshot{}, fmt.Errorf("workspacetasks: quoter unavailable: %w", ErrQuoteUnavailable)
	}
	kind, err := ValidateCreate(params.Service, nil, map[string]any{})
	if err != nil {
		return BillingSnapshot{}, err
	}
	quantity := params.Quantity
	if quantity <= 0 {
		quantity = 1
	}
	resolution := strings.TrimSpace(params.Resolution)
	if resolution == "" {
		resolution = workflow.OutputResolution1K
	}
	if _, err := s.resolveSnapshot(ctx, kind, resolution); err != nil {
		return BillingSnapshot{}, err
	}
	snapshot, err := s.quoter.Quote(ctx, kind, resolution, quantity)
	if err != nil {
		return BillingSnapshot{}, err
	}
	snapshot.SettlementState = "quoted"
	return snapshot, nil
}

// CreateInput is the validated creation intent for settlement. The provider
// model is deliberately absent: submit never accepts a caller-chosen model.
// (#847: the single-value admin mapping mint is removed; #848 resolves the
// model via RoutingResolver and pins the snapshot here instead.)
type CreateInput struct {
	WorkspaceID     string
	OwnerUserID     int64
	Service         string
	MediaAssetIDs   []string
	Params          map[string]any
	Quantity        int64
	Resolution      string
	CanonicalParams string
	// RequestHash fingerprints the request body for the durable submit
	// claim. Callers that already canonicalize the body (the v1 HTTP edge)
	// pass TaskSubmitRequestHash; when empty, Create derives an equivalent
	// hash from the settlement-shaping fields so the claim is never stored
	// with an empty fingerprint.
	RequestHash string
	// Prompt is the caller prompt. It feeds the request hash (a different
	// prompt with the same key is a conflict, not a replay) but is not
	// itself persisted here; the canonical compiler owns prompt storage.
	Prompt string
	// BoardID optionally binds a Canvas submit to its Board (#937): the
	// board_ public id, validated against the caller's live Board
	// posture (visible identity + generate capability + input
	// availability) before any Quote/Hold. Empty means a non-Canvas task
	// with its own workspace-level permission; it must never be forged
	// into a Board attribution.
	BoardID string
	// WorkspaceRole is the caller's workspace role string, consumed only
	// together with BoardID for Board posture resolution.
	WorkspaceRole string
}

// Create settles one task atomically: Quote -> Hold -> Insert -> Enqueue,
// fenced by a durable submit idempotency claim written in the same
// transaction. Any failure compensates all prior steps and rolls the claim
// back with them. On success it stages a pending status event (flushed by
// the caller after commit).
//
// idempotencyKey is REQUIRED and load-bearing: it scopes the durable claim
// (workspace + operation + key) and derives the Credit Hold ledger key. A
// repeated key returns the ORIGINAL task and stages NO event (the original
// submit already emitted it); a repeated key with a different request body
// returns ErrIdempotencyConflict. An empty key is a validation error, never
// an unfenced create.
func (s *Service) Create(ctx context.Context, input CreateInput, idempotencyKey string) (*WorkspaceTask, error) {
	ctx, span := telemetry.Start(ctx, "generation.submit.settlement",
		attribute.String("service", input.Service), attribute.String("resolution", input.Resolution))
	defer telemetry.End(span, nil)
	if s.taskLifecycle == nil {
		return nil, fmt.Errorf("workspacetasks: store unavailable: %w", ErrNotFound)
	}
	key, err := NormalizeSubmitIdempotencyKey(idempotencyKey)
	if err != nil {
		return nil, err
	}
	kind, err := ValidateCreate(input.Service, input.MediaAssetIDs, input.Params)
	if err != nil {
		return nil, err
	}
	if err := ValidateBoardID(input.BoardID); err != nil {
		return nil, err
	}
	if s.quoter == nil || s.holder == nil || s.queue == nil {
		return nil, fmt.Errorf("workspacetasks: settlement unavailable: %w", ErrQuoteUnavailable)
	}
	quantity := input.Quantity
	if quantity <= 0 {
		quantity = 1
	}
	resolution := strings.TrimSpace(input.Resolution)
	if resolution == "" {
		resolution = workflow.OutputResolution1K
	}
	// Availability resolves authoritatively inside the settlement
	// transaction below and the snapshot pins atomically with it. The
	// worker fails closed on a missing snapshot, and credits.Commands
	// stays the sole ledger writer.
	workspaceID := strings.TrimSpace(input.WorkspaceID)
	// Mint the task id up front so the claim can name it. A replay returns
	// this same id instead of minting a second one. Mint owned by
	// platform/contract; no domain wrapper.
	taskID, err := contract.MintPublicID(contract.PrefixTask, 12)
	if err != nil {
		return nil, err
	}
	token, err := NewExecutionToken()
	if err != nil {
		return nil, err
	}
	claim := SubmissionClaim{
		WorkspaceID: workspaceID, Operation: TaskSubmitOperation,
		IdempotencyKey: key, TaskPublicID: taskID,
		RequestHash: s.submitRequestHash(input, kind, resolution, quantity),
	}
	if created, handled, err := s.createInTx(ctx, input, kind, resolution, quantity, taskID, token, claim); handled {
		return created, err
	}
	return nil, ErrSettlementUnavailable
}

// pendingTaskWithBinding captures the accepted inputs and Board binding for
// transactional settlement.
func pendingTaskWithBinding(input CreateInput, kind, taskID, token string, binding TaskBoardBinding, snapshot BillingSnapshot) *WorkspaceTask {
	return &WorkspaceTask{
		ID: taskID, WorkspaceID: strings.TrimSpace(input.WorkspaceID), OwnerUserID: input.OwnerUserID,
		BillingAccountID: snapshot.BillingAccountID,
		Service:          kind, MediaAssetIDs: append([]string(nil), input.MediaAssetIDs...),
		Params: cloneParams(input.Params), CanonicalParams: input.CanonicalParams,
		BoardID: binding.BoardID, BoardAccessVersion: binding.AccessVersion,
		Status:     StatusPending,
		RetryCount: 0, ExecutionToken: token, BillingSnapshot: snapshot,
	}
}

// submitRequestHash resolves the fingerprint persisted with the durable
// claim. Callers that canonicalize the body themselves (the v1 HTTP edge)
// supply it; otherwise an equivalent hash is derived from the fields that
// shape settlement and rendering, so the claim is never stored blank.
func (s *Service) submitRequestHash(input CreateInput, kind, resolution string, quantity int64) string {
	if supplied := strings.TrimSpace(input.RequestHash); supplied != "" {
		return supplied
	}
	paramsJSON := ""
	if len(input.Params) > 0 {
		if encoded, err := json.Marshal(input.Params); err == nil {
			paramsJSON = string(encoded)
		}
	}
	return TaskSubmitRequestHash(input.WorkspaceID, kind, input.MediaAssetIDs, input.Prompt, paramsJSON, quantity, resolution, input.BoardID)
}

func cloneParams(in map[string]any) map[string]any {
	if in == nil {
		return nil
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

// mapAssetGateError propagates the readiness sentinel from the AssetReader
// seam: archived and non-ready stay distinct; anything else (invisible ids
// and transport faults) converges to media-not-found, matching the previous
// gate's error mapping for seam errors.
func mapAssetGateError(err error) error {
	if errors.Is(err, ErrMediaArchived) {
		return ErrMediaArchived
	}
	if errors.Is(err, ErrMediaNotReady) {
		return ErrMediaNotReady
	}
	return ErrMediaNotFound
}

// Cancel flips pending/running to cancelled atomically with Release.
// Terminal tasks replay idempotently (already=true). The caller stages
// the cancelled event only on a fresh flip.
func (s *Service) Cancel(ctx context.Context, workspaceID, taskID string, actorUserID int64, reason string) (*WorkspaceTask, bool, error) {
	ctx, span := telemetry.Start(ctx, "generation.terminal.settlement.cancel")
	defer telemetry.End(span, nil)
	if s.taskRead == nil || s.taskLifecycle == nil {
		return nil, false, ErrNotFound
	}
	if flipped, already, handled, err := s.cancelInTx(ctx, workspaceID, taskID, actorUserID, reason); handled {
		return flipped, already, err
	}
	return nil, false, ErrSettlementUnavailable
}

// Complete flips running to completed atomically with Capture. Outputs
// must reference ready assets or the explicit remote-output contract.
func (s *Service) Complete(ctx context.Context, workspaceID, taskID string, outputs []OutputRef) (*WorkspaceTask, error) {
	ctx, span := telemetry.Start(ctx, "generation.terminal.settlement.complete")
	defer telemetry.End(span, nil)
	if s.taskLifecycle == nil {
		return nil, ErrNotFound
	}
	for _, ref := range outputs {
		if err := ValidateOutputRef(ref); err != nil {
			return nil, err
		}
	}
	if err := s.verifyOutputAssets(ctx, workspaceID, outputs); err != nil {
		return nil, err
	}
	if completed, handled, err := s.completeInTx(ctx, workspaceID, taskID, outputs); handled {
		return completed, err
	}
	return nil, ErrSettlementUnavailable
}

// Fail flips running/pending to failed atomically with Release.
func (s *Service) Fail(ctx context.Context, workspaceID, taskID, errorMsg string) (*WorkspaceTask, error) {
	ctx, span := telemetry.Start(ctx, "generation.terminal.settlement.fail")
	defer telemetry.End(span, nil)
	if s.taskLifecycle == nil {
		return nil, ErrNotFound
	}
	if failed, handled, err := s.failInTx(ctx, workspaceID, taskID, errorMsg); handled {
		return failed, err
	}
	return nil, ErrSettlementUnavailable
}

// Retry consumes one retry budget entry (reset + new execution token,
// bounded by retry_count). Transient provider failures (retryable=true)
// below the budget requeue; deterministic failures must call Fail instead.
func (s *Service) Retry(ctx context.Context, workspaceID, taskID string, retryable bool) (*WorkspaceTask, error) {
	ctx, span := telemetry.Start(ctx, "generation.terminal.settlement.retry")
	defer telemetry.End(span, nil)
	if s.taskRead == nil || s.taskLifecycle == nil {
		return nil, ErrNotFound
	}
	current, err := s.taskRead.GetTask(ctx, workspaceID, taskID)
	if err != nil {
		return nil, err
	}
	if current.IsTerminal() {
		return nil, ErrStateConflict
	}
	if !ShouldRetry(current.RetryCount, retryable) {
		return nil, ErrStateConflict
	}
	token, err := NewExecutionToken()
	if err != nil {
		return nil, err
	}
	if retried, handled, err := s.retryInTx(ctx, workspaceID, taskID, token); handled {
		return retried, err
	}
	return nil, ErrSettlementUnavailable
}

func (s *Service) verifyOutputAssets(ctx context.Context, workspaceID string, outputs []OutputRef) error {
	if s.assets == nil {
		return nil
	}
	// Sentinel-only readiness (P0-4): the AssetReader seam owns the
	// archived/ready comparison and reports it through sentinels.
	for _, ref := range outputs {
		if ref.Kind != OutputKindAsset {
			continue
		}
		if _, err := s.assets.GetAsset(ctx, workspaceID, ref.AssetID); err != nil {
			return fmt.Errorf("workspacetasks: output asset: %w", mapAssetGateError(err))
		}
	}
	return nil
}
