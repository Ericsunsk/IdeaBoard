package workspacetasks

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"ideaboard/api/internal/generation/routing"
	"ideaboard/api/internal/runtime/infrastructure/telemetry"
	"ideaboard/api/internal/workflow"
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
	tasks  WorkspaceTaskStore
	quoter Quoter
	holder Holder
	queue  Enqueuer
	assets AssetReader
	// resolver is the authoritative Service Model Pool decision point
	// (ADR-0012 rule 10, #848). Quote and Create consult it; a nil
	// resolver fails closed with the canonical unavailable sentinel.
	resolver *routing.RoutingResolver
	// saveSnapshot persists the pinned snapshot in the settlement transaction.
	saveSnapshot RoutingSnapshotSaver
	// allowNonTransactional is set only by NewTestServiceForTests. Production
	// settlement must always use the atomic PostgreSQL seams below.
	allowNonTransactional bool
	events                []TaskEvent
	now                   func() time.Time
}

// ServiceDeps wires the settlement seams. Tasks is required; Quoter,
// Holder, Enqueuer, and Assets are required for production creates.
// (#847: the submit-time single-value model mint is removed; the
// RoutingSnapshot pins in this transaction instead.)
type ServiceDeps struct {
	Tasks  WorkspaceTaskStore
	Quoter Quoter
	Holder Holder
	Queue  Enqueuer
	Assets AssetReader
	// Resolver is the shared RoutingResolver quote/submit/readiness
	// consume (same instance, never an independent lookup). Nil fails
	// closed with the canonical unavailable sentinel.
	Resolver *routing.RoutingResolver
	// SnapshotSaver pins the Resolve snapshot in the settlement transaction.
	SnapshotSaver RoutingSnapshotSaver
}

// NewService builds the settlement orchestrator.
func NewService(deps ServiceDeps) *Service {
	return &Service{
		tasks: deps.Tasks, quoter: deps.Quoter, holder: deps.Holder,
		queue: deps.Queue, assets: deps.Assets,
		resolver: deps.Resolver, saveSnapshot: deps.SnapshotSaver, now: time.Now,
	}
}

// NewTestServiceForTests builds a service for unit fixtures that do not have
// PostgreSQL transaction seams. It is intentionally separate from NewService
// so production wiring cannot silently use compensating settlement writes.
func NewTestServiceForTests(deps ServiceDeps) *Service {
	service := NewService(deps)
	service.allowNonTransactional = true
	return service
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

func (s *Service) stage(taskID, workspaceID string, status Status) {
	s.events = append(s.events, TaskEvent{
		TaskID: taskID, WorkspaceID: workspaceID, Status: status, OccurredAt: s.clock(),
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
}

// Create settles one task atomically: Quote -> Hold -> Insert -> Enqueue.
// Any failure compensates all prior steps. On success it stages a
// pending status event (flushed by the caller after commit).
func (s *Service) Create(ctx context.Context, input CreateInput, idempotencyKey string) (*WorkspaceTask, error) {
	ctx, span := telemetry.Start(ctx, "generation.submit.settlement",
		attribute.String("service", input.Service), attribute.String("resolution", input.Resolution))
	defer telemetry.End(span, nil)
	if s.tasks == nil {
		return nil, fmt.Errorf("workspacetasks: store unavailable: %w", ErrNotFound)
	}
	kind, err := ValidateCreate(input.Service, input.MediaAssetIDs, input.Params)
	if err != nil {
		return nil, err
	}
	if err := s.verifyInputAssets(ctx, input.WorkspaceID, input.MediaAssetIDs, input.Params); err != nil {
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
	// Availability resolves authoritatively below and the snapshot pins
	// atomically with the settlement transaction. The worker fails closed on a
	// missing snapshot, and credits.Commands stays the sole ledger writer.
	var snapshot BillingSnapshot
	taskID, err := NewTaskPublicID()
	if err != nil {
		return nil, err
	}
	token, err := NewExecutionToken()
	if err != nil {
		return nil, err
	}
	if created, handled, err := s.createInTx(ctx, input, kind, resolution, quantity, taskID, token); handled {
		return created, err
	}
	if !s.allowNonTransactional {
		return nil, ErrSettlementUnavailable
	}
	var routingSnap routing.RoutingSnapshot
	// Test-only fixtures without transaction seams resolve before Quote/Hold so
	// an empty pool reserves nothing. Production cannot enter this branch.
	routingSnap, err = s.resolveSnapshot(ctx, kind, resolution)
	if err != nil {
		return nil, err
	}
	snapshot, err = s.quoter.Quote(ctx, kind, resolution, quantity)
	if err != nil {
		return nil, err
	}
	var holdTxID string
	if snapshot.TotalCredits > 0 {
		holdTxID, err = s.holder.Hold(ctx, input.OwnerUserID, snapshot.TotalCredits, "task-hold:"+taskID, taskID)
		if err != nil {
			return nil, err
		}
		snapshot.HoldTransactionID = holdTxID
		snapshot.SettlementState = "held"
	} else {
		snapshot.SettlementState = "free"
	}
	task := &WorkspaceTask{
		ID: taskID, WorkspaceID: strings.TrimSpace(input.WorkspaceID), OwnerUserID: input.OwnerUserID,
		Service: kind, MediaAssetIDs: append([]string(nil), input.MediaAssetIDs...),
		Params: cloneParams(input.Params), CanonicalParams: input.CanonicalParams,
		Status:     StatusPending,
		RetryCount: 0, ExecutionToken: token, BillingSnapshot: snapshot,
	}
	created, err := s.tasks.CreateTask(ctx, task)
	if err != nil {
		if holdTxID != "" {
			_ = s.holder.Release(ctx, holdTxID, snapshot.TotalCredits, taskID)
		}
		return nil, err
	}
	if s.saveSnapshot != nil {
		if err := s.saveSnapshot(ctx, nil, created.ID, routingSnap); err != nil {
			_ = s.tasks.DeleteTask(ctx, created.WorkspaceID, created.ID)
			if holdTxID != "" {
				_ = s.holder.Release(ctx, holdTxID, snapshot.TotalCredits, taskID)
			}
			return nil, fmt.Errorf("workspacetasks: pin routing snapshot: %w", err)
		}
	}
	if err := s.queue.Enqueue(ctx, created); err != nil {
		_ = s.tasks.DeleteTask(ctx, created.WorkspaceID, created.ID)
		if holdTxID != "" {
			_ = s.holder.Release(ctx, holdTxID, snapshot.TotalCredits, taskID)
		}
		return nil, fmt.Errorf("workspacetasks: enqueue: %w", err)
	}
	s.stage(created.ID, created.WorkspaceID, StatusPending)
	return created, nil
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

func (s *Service) verifyInputAssets(ctx context.Context, workspaceID string, ids []string, params map[string]any) error {
	if s.assets == nil {
		return nil
	}
	for i, id := range ids {
		info, err := s.assets.GetAsset(ctx, workspaceID, id)
		if err != nil {
			return fmt.Errorf("workspacetasks: input asset %d: %w", i, ErrMediaNotFound)
		}
		if info.Archived {
			return fmt.Errorf("workspacetasks: input asset %d archived: %w", i, ErrMediaArchived)
		}
		if strings.ToLower(strings.TrimSpace(info.Status)) != "ready" {
			return fmt.Errorf("workspacetasks: input asset %d not ready: %w", i, ErrMediaNotReady)
		}
	}
	// The v1 mask reference (params.mask_asset_id) gates on the same
	// ready/archived semantics as image inputs: invisible, non-ready, or
	// archived masks fail the submit (404/409/410), never a late worker
	// failure.
	if maskID, ok := MaskAssetIDFromParams(params); ok {
		info, err := s.assets.GetAsset(ctx, workspaceID, maskID)
		if err != nil {
			return fmt.Errorf("workspacetasks: mask asset: %w", ErrMediaNotFound)
		}
		if info.Archived {
			return fmt.Errorf("workspacetasks: mask asset archived: %w", ErrMediaArchived)
		}
		if strings.ToLower(strings.TrimSpace(info.Status)) != "ready" {
			return fmt.Errorf("workspacetasks: mask asset not ready: %w", ErrMediaNotReady)
		}
	}
	return nil
}

// Cancel flips pending/running to cancelled atomically with Release.
// Terminal tasks replay idempotently (already=true). The caller stages
// the cancelled event only on a fresh flip.
func (s *Service) Cancel(ctx context.Context, workspaceID, taskID string, actorUserID int64, reason string) (*WorkspaceTask, bool, error) {
	ctx, span := telemetry.Start(ctx, "generation.terminal.settlement.cancel")
	defer telemetry.End(span, nil)
	if s.tasks == nil {
		return nil, false, ErrNotFound
	}
	if flipped, already, handled, err := s.cancelInTx(ctx, workspaceID, taskID, actorUserID, reason); handled {
		return flipped, already, err
	}
	if !s.allowNonTransactional {
		return nil, false, ErrSettlementUnavailable
	}
	before, err := s.tasks.GetTask(ctx, workspaceID, taskID)
	if err != nil {
		return nil, false, err
	}
	if before.IsTerminal() {
		return before, true, nil
	}
	flipped, already, err := s.tasks.CancelTask(ctx, workspaceID, taskID, actorUserID, reason)
	if err != nil {
		return nil, false, err
	}
	if already {
		return flipped, true, nil
	}
	if s.holder != nil && flipped.BillingSnapshot.HoldTransactionID != "" && flipped.BillingSnapshot.TotalCredits > 0 {
		if releaseErr := s.holder.Release(ctx, flipped.BillingSnapshot.HoldTransactionID, flipped.BillingSnapshot.TotalCredits, taskID); releaseErr != nil {
			return nil, false, fmt.Errorf("workspacetasks: release on cancel: %w", releaseErr)
		}
		flipped.BillingSnapshot.SettlementState = "released"
	}
	s.stage(taskID, workspaceID, StatusCancelled)
	return flipped, false, nil
}

// Complete flips running to completed atomically with Capture. Outputs
// must reference ready assets or the explicit remote-output contract.
func (s *Service) Complete(ctx context.Context, workspaceID, taskID string, outputs []OutputRef) (*WorkspaceTask, error) {
	ctx, span := telemetry.Start(ctx, "generation.terminal.settlement.complete")
	defer telemetry.End(span, nil)
	if s.tasks == nil {
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
	if !s.allowNonTransactional {
		return nil, ErrSettlementUnavailable
	}
	completed, err := s.tasks.CompleteTask(ctx, workspaceID, taskID, outputs)
	if err != nil {
		return nil, err
	}
	if s.holder != nil && completed.BillingSnapshot.HoldTransactionID != "" && completed.BillingSnapshot.TotalCredits > 0 {
		capture := int64(len(outputs)) * completed.BillingSnapshot.UnitCredits
		if capture > completed.BillingSnapshot.TotalCredits {
			capture = completed.BillingSnapshot.TotalCredits
		}
		release := completed.BillingSnapshot.TotalCredits - capture
		if capture > 0 {
			if captureErr := s.holder.Capture(ctx, completed.BillingSnapshot.HoldTransactionID, capture, taskID); captureErr != nil {
				return nil, fmt.Errorf("workspacetasks: capture on complete: %w", captureErr)
			}
		}
		if release > 0 {
			if releaseErr := s.holder.Release(ctx, completed.BillingSnapshot.HoldTransactionID, release, taskID); releaseErr != nil {
				return nil, fmt.Errorf("workspacetasks: release remainder on complete: %w", releaseErr)
			}
		}
		completed.BillingSnapshot.SettlementState = "captured"
	}
	s.stage(taskID, workspaceID, StatusCompleted)
	return completed, nil
}

// Fail flips running/pending to failed atomically with Release.
func (s *Service) Fail(ctx context.Context, workspaceID, taskID, errorMsg string) (*WorkspaceTask, error) {
	ctx, span := telemetry.Start(ctx, "generation.terminal.settlement.fail")
	defer telemetry.End(span, nil)
	if s.tasks == nil {
		return nil, ErrNotFound
	}
	if failed, handled, err := s.failInTx(ctx, workspaceID, taskID, errorMsg); handled {
		return failed, err
	}
	if !s.allowNonTransactional {
		return nil, ErrSettlementUnavailable
	}
	failed, err := s.tasks.FailTask(ctx, workspaceID, taskID, errorMsg)
	if err != nil {
		return nil, err
	}
	if s.holder != nil && failed.BillingSnapshot.HoldTransactionID != "" && failed.BillingSnapshot.TotalCredits > 0 {
		if releaseErr := s.holder.Release(ctx, failed.BillingSnapshot.HoldTransactionID, failed.BillingSnapshot.TotalCredits, taskID); releaseErr != nil {
			return nil, fmt.Errorf("workspacetasks: release on fail: %w", releaseErr)
		}
		failed.BillingSnapshot.SettlementState = "released"
	}
	s.stage(taskID, workspaceID, StatusFailed)
	return failed, nil
}

// Retry consumes one retry budget entry (reset + new execution token,
// bounded by retry_count). Transient provider failures (retryable=true)
// below the budget requeue; deterministic failures must call Fail instead.
func (s *Service) Retry(ctx context.Context, workspaceID, taskID string, retryable bool) (*WorkspaceTask, error) {
	ctx, span := telemetry.Start(ctx, "generation.terminal.settlement.retry")
	defer telemetry.End(span, nil)
	if s.tasks == nil {
		return nil, ErrNotFound
	}
	current, err := s.tasks.GetTask(ctx, workspaceID, taskID)
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
	if !s.allowNonTransactional {
		return nil, ErrSettlementUnavailable
	}
	retried, err := s.tasks.TransitionForRetry(ctx, workspaceID, taskID, token)
	if err != nil {
		return nil, err
	}
	if s.queue != nil {
		if enqueueErr := s.queue.Enqueue(ctx, retried); enqueueErr != nil {
			return nil, fmt.Errorf("workspacetasks: requeue: %w", enqueueErr)
		}
	}
	return retried, nil
}

func (s *Service) verifyOutputAssets(ctx context.Context, workspaceID string, outputs []OutputRef) error {
	if s.assets == nil {
		return nil
	}
	for _, ref := range outputs {
		if ref.Kind != OutputKindAsset {
			continue
		}
		info, err := s.assets.GetAsset(ctx, workspaceID, ref.AssetID)
		if err != nil {
			return fmt.Errorf("workspacetasks: output asset: %w", ErrMediaNotFound)
		}
		if info.Archived {
			return fmt.Errorf("workspacetasks: output asset archived: %w", ErrMediaArchived)
		}
		if strings.ToLower(strings.TrimSpace(info.Status)) != "ready" {
			return fmt.Errorf("workspacetasks: output asset not ready: %w", ErrMediaNotReady)
		}
	}
	return nil
}
