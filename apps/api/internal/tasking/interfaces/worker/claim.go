package worker

// Claim phase (#860): load the task, drop dirty work, validate inputs,
// resolve media refs, settle the send's routing facts, and fence
// pending -> running with the lease token IN THE SAME STATEMENT that opens
// the send's durable attempt fact.
//
// Dirty-work drops (all return (nil, nil): no error, no provider call):
//
//	load -> drop unknown id / terminal / cancelled / stale token
//
// Validation and media resolution run BEFORE claiming so malformed tasks
// fail from pending without lease churn; both converge deterministically
// through the retry+settlement phase (failTerminal: hold released, no
// retry burn). Routing resolves before the claim for the same reason: the
// claim statement writes the attempt row, so the send's model and retry
// layer have to be known up front.

import (
	"context"
	"errors"
	"fmt"
	"ideaboard/api/internal/platform/errorrender"
	"log/slog"
	"strings"
	"time"

	"ideaboard/api/internal/generation/catalog"
	"ideaboard/api/internal/generation/routing"
	tasks "ideaboard/api/internal/tasking/domain"
	taskqueue "ideaboard/api/internal/tasking/infrastructure/queue"
	taskports "ideaboard/api/internal/tasking/ports/tasks"
)

// loadDispatchableTask loads one task for this job and drops dirty work:
// unknown ids (compensating delete on enqueue-failure rollback orphaned
// the job), already-terminal rows (a redelivery in the post-Complete
// materialization gap replays the idempotent trigger to repair the link),
// cancelled rows, and stale execution tokens (an old redelivery after a
// retry rotated the token). A nil task with a nil error means drop.
func (w *WorkspaceTaskWorker) loadDispatchableTask(ctx context.Context, args taskqueue.WorkspaceTaskArgs) (*tasks.WorkspaceTask, error) {
	taskID := strings.TrimSpace(args.TaskID)
	workspaceID := strings.TrimSpace(args.WorkspaceID)

	task, err := w.store.GetTask(ctx, workspaceID, taskID)
	if err != nil {
		if errors.Is(err, tasks.ErrNotFound) {
			slog.Info("workspace task gone (rollback delete?); dropping job", "task_id", taskID)
			return nil, nil
		}
		return nil, fmt.Errorf("workspace task load: %w", err)
	}
	if task.IsTerminal() {
		if task.Status == tasks.StatusCompleted {
			// Complete commits before Library auto-save. A redelivery in
			// this gap replays the idempotent trigger and repairs the link.
			w.saveCompletedTask(ctx, task)
		}
		slog.Info("workspace task already terminal; dropping dirty job", "task_id", taskID, "status", task.Status)
		return nil, nil
	}
	if token := strings.TrimSpace(args.ExecutionToken); token != "" && task.ExecutionToken != token {
		slog.Info("workspace task job carries a stale execution token; dropping",
			"task_id", taskID, "job_token", redactToken(token), "task_token", redactToken(task.ExecutionToken))
		return nil, nil
	}
	return task, nil
}

// validateInput enforces the discriminated input union before claiming, so
// malformed tasks fail from pending without lease churn.
func (w *WorkspaceTaskWorker) validateInput(_ context.Context, task *tasks.WorkspaceTask) error {
	if _, err := tasks.ValidateCreate(task.Service, task.MediaAssetIDs, task.Params); err != nil {
		return fmt.Errorf(errorrender.MsgValidateInput, err)
	}
	return nil
}

// resolveMediaRefs checks every media_asset_ids entry read-only through the
// TaskAssets adapter. Invisible/archived/non-ready assets fail the task
// deterministically; the worker never mutates media.
func (w *WorkspaceTaskWorker) resolveMediaRefs(ctx context.Context, task *tasks.WorkspaceTask) error {
	if len(task.MediaAssetIDs) == 0 && task.Params == nil {
		return nil
	}
	if w.assets == nil && (len(task.MediaAssetIDs) > 0 || hasWorkspaceMaskParam(task)) {
		return errors.New(errorrender.MsgResolveMediaRefs)
	}
	for i, id := range task.MediaAssetIDs {
		if _, err := w.assets.GetAsset(ctx, task.WorkspaceID, id); err != nil {
			switch {
			case errors.Is(err, tasks.ErrMediaArchived):
				return fmt.Errorf(errorrender.MsgResolveMediaRefs2, i, err)
			case errors.Is(err, tasks.ErrMediaNotReady):
				return fmt.Errorf(errorrender.MsgResolveMediaRefs3, i, err)
			default:
				return fmt.Errorf(errorrender.MsgResolveMediaRefs4, i, tasks.ErrMediaNotFound)
			}
		}
	}
	// The v1 mask reference (params.mask_asset_id) resolves through the
	// same read-only gate so a mask that vanished between submit and
	// dispatch fails deterministically instead of reaching the provider.
	if maskID, ok := tasks.MaskAssetIDFromParams(task.Params); ok {
		if w.assets == nil {
			return errors.New(errorrender.MsgResolveMediaRefs5)
		}
		if _, err := w.assets.GetAsset(ctx, task.WorkspaceID, maskID); err != nil {
			switch {
			case errors.Is(err, tasks.ErrMediaArchived):
				return fmt.Errorf(errorrender.MsgResolveMediaRefs6, err)
			case errors.Is(err, tasks.ErrMediaNotReady):
				return fmt.Errorf(errorrender.MsgResolveMediaRefs7, err)
			default:
				return fmt.Errorf(errorrender.MsgResolveMediaRefs8, tasks.ErrMediaNotFound)
			}
		}
	}
	return nil
}

func hasWorkspaceMaskParam(task *tasks.WorkspaceTask) bool {
	if task == nil {
		return false
	}
	_, ok := tasks.MaskAssetIDFromParams(task.Params)
	return ok
}

// claim fences pending -> running with the lease token AND opens the send's
// durable attempt fact, in one statement. The ordering is what removes the
// intermediate state: the attempt row is written by the SAME statement that
// takes the lease, so there is no window in which the lease token is
// committed without its attempt row (a crash, a rejected connection, or a
// failed attempt insert now rolls the lease flip back with it).
//
// Because that row is the send's own fact, it is also keyed by the lease
// token: one token, one attempt row, one attempt_no. The returned send closes
// that same row, so a send can never be recorded against a token the claim
// never took.
//
// A River redelivery reacquires the same interrupted running lease because it
// carries the same token. That resume reuses the attempt row the earlier
// attempt opened (started=false) instead of colliding on the token, so the
// send keeps one fact. Lease-guarded conflicts reconverge: terminal/cancelled
// rows drop the job, anything else is transient and rides River backoff.
func (w *WorkspaceTaskWorker) claim(
	ctx context.Context,
	args taskqueue.WorkspaceTaskArgs,
	task *tasks.WorkspaceTask,
	tracker *workspaceSendTracker,
	model, layer string,
) (*tasks.WorkspaceTask, *workspaceSend, error) {
	lease := strings.TrimSpace(args.ExecutionToken)
	if lease == "" {
		// Fail closed: Create/Retry always mint the lease, so an empty
		// lease only arrives on a hand-made job. Minting silently here
		// would execute a second send the submit path never fenced.
		return nil, nil, fmt.Errorf("workspace task job carries no execution token: %w", errEmptyLease)
	}
	claimed, started, err := w.claims.ClaimTaskWithAttempt(ctx, task.WorkspaceID, task.ID, lease, tasks.AttemptStart{
		TaskID:         task.ID,
		RiverJobID:     tracker.riverJobID,
		Service:        tracker.service,
		RequestedModel: model,
		ResolvedModel:  model,
		Provider:       workspaceAttemptProvider(model),
		RetryLayer:     layer,
		StartedAt:      time.Now(),
	})
	if err != nil {
		if errors.Is(err, tasks.ErrStateConflict) || errors.Is(err, tasks.ErrNotFound) {
			current, getErr := w.store.GetTask(ctx, task.WorkspaceID, task.ID)
			if getErr != nil {
				if errors.Is(getErr, tasks.ErrNotFound) {
					return nil, nil, nil
				}
				return nil, nil, fmt.Errorf("workspace task reload after claim conflict: %w", getErr)
			}
			if current.IsTerminal() {
				slog.Info("workspace task converged before claim; dropping", "task_id", task.ID, "status", current.Status)
				return nil, nil, nil
			}
			return nil, nil, fmt.Errorf("workspace task claim conflict: %w", err)
		}
		return nil, nil, fmt.Errorf("workspace task claim: %w", err)
	}
	if !started {
		// Same lease, same send: the attempt row already exists, so the
		// resumed execution closes that row instead of opening a new one.
		slog.Info("workspace task claim resumed an existing attempt fact",
			"task_id", claimed.ID, "retry_layer", layer, "retry_count", claimed.RetryCount)
	}
	return claimed, tracker.claimedSend(lease, model, layer), nil
}

func redactToken(token string) string {
	if len(token) <= 8 {
		return "***"
	}
	return token[:4] + "***" + token[len(token)-4:]
}

// --- claim-phase routing (from workspace_task_routing.go): snapshot-only model/attempt facts ---

// errWorkspaceRoutingSnapshotMissing fails a task closed when no
// snapshot was pinned (pre-pool row or a submit path that bypassed the
// pin). The hold is released and no retry budget burns, mirroring the
// #847 interim behavior.
var errWorkspaceRoutingSnapshotMissing = errors.New(errorrender.MsgSnapshotMissing)

// errEmptyLease fails a hand-made job closed: Create/Retry always mint
// the execution token, so an empty lease never arrives on a submit-fenced
// job and must never mint silently.
var errEmptyLease = errors.New("workspace task job carries no execution token")

// errWorkspaceRoutingSnapshotCorrupt fails a task closed when the pinned
// snapshot exists but no longer replays (tampered or malformed). Like a
// missing snapshot it releases the hold without burning retries: an
// unreplayable routing fact must never spend the fallback budget.
var errWorkspaceRoutingSnapshotCorrupt = errors.New(errorrender.MsgSnapshotCorrupt)

// resolveExecutionSnapshot parses and Verify()s the pinned snapshot on
// the task. It needs no live pool: the parse+verify+service-match
// sequence is owned by catalog.DecodePinnedSnapshot (shared with the
// DB-reading LoadRoutingSnapshot); this stays a thin worker-sentinel
// adapter so existing failTerminal messages and errors.Is targets never
// change. A snapshot pinned for another service is rejected the same as
// a corrupt one: routing facts must belong to the task that carries them.
func resolveExecutionSnapshot(task *tasks.WorkspaceTask) (routing.RoutingSnapshot, error) {
	if task == nil || len(task.RoutingSnapshot) == 0 {
		return routing.RoutingSnapshot{}, errWorkspaceRoutingSnapshotMissing
	}
	snap, err := catalog.DecodePinnedSnapshot(task.Service, task.RoutingSnapshot)
	if err != nil {
		if errors.Is(err, catalog.ErrRoutingSnapshotMissing) {
			return routing.RoutingSnapshot{}, errWorkspaceRoutingSnapshotMissing
		}
		return routing.RoutingSnapshot{}, fmt.Errorf("%w: %w", errWorkspaceRoutingSnapshotCorrupt, err)
	}
	return snap, nil
}

// workspaceAttemptProvider reports the provider slug prefix of a pinned
// model reference (provider/model) for attempt facts. Unknown shapes
// stay empty instead of failing: the gateway call validates the model
// itself. This mirrors gatewayModelProvider without importing runtime.
func workspaceAttemptProvider(model string) string {
	model = strings.TrimSpace(model)
	if i := strings.Index(model, "/"); i > 0 {
		return model[:i]
	}
	return ""
}

// workspaceFailureStageCategory maps one classified failure onto the
// attempt-fact stage/category vocabulary. Gateway sends record the
// provider stage; local pre-send failures (no status, no marker) record
// assigned/unknown.
func workspaceFailureStageCategory(failure routing.AttemptFailure) (stage, category string) {
	switch {
	case failure.InvalidParams:
		return tasks.ExecutionStageProvider, tasks.FailureCategoryInputInvalid
	case failure.AuthFailed:
		return tasks.ExecutionStageProvider, tasks.FailureCategoryAuth
	case failure.InsufficientCredits:
		return tasks.ExecutionStageProvider, tasks.FailureCategoryQuota
	case failure.Timeout:
		return tasks.ExecutionStageProvider, tasks.FailureCategoryTimeout
	case failure.Retryable():
		return tasks.ExecutionStageProvider, tasks.FailureCategoryProviderError
	case failure.Deterministic():
		return tasks.ExecutionStageProvider, tasks.FailureCategoryProviderError
	default:
		return tasks.ExecutionStageAssigned, tasks.FailureCategoryUnknown
	}
}

// truncateWorkspaceError keeps the attempt error fact inspectable
// without persisting unbounded provider bodies.
func truncateWorkspaceError(msg string) string {
	const maxLen = 500
	msg = strings.TrimSpace(msg)
	if len(msg) > maxLen {
		msg = msg[:maxLen] + "...(truncated)"
	}
	return msg
}

// workspaceSendTracker opens and closes per-send attempt facts for one
// worker execution. All recorder failures degrade to warn-logs except
// the fallback start, which gates the second send.
type workspaceSendTracker struct {
	recorder   taskports.AttemptRecorder
	taskID     string
	riverJobID int64
	service    string
}

// workspaceAttemptLayer classifies the retry layer of the send that a claim
// is about to open.
//
// gateway is only the INITIAL schedule. Every later send is a river_task
// send: either Service.Retry rotated the token after a transient provider
// failure (retryCount > 0), or River redelivered the same job
// (jobAttempt > 0). Deriving this from retryCount ALONE mis-attributed
// redeliveries: the claim's same-token branch lets a redelivery re-acquire the
// lease without any retry_count increment, so those sends were recorded as
// fresh gateway sends and the layer histogram under-counted whole-task
// retries. jobAttempt is River's own durable signal for the redelivery case,
// so the two together cover every non-initial send.
func workspaceAttemptLayer(retryCount int32, jobAttempt int) string {
	if retryCount > 0 || jobAttempt > 0 {
		return tasks.AttemptRetryLayerRiverTask
	}
	return tasks.AttemptRetryLayerGateway
}

// workspaceSend is one started gateway send's open fact. A zero token
// means the fact was never recorded (nil or degraded recorder): every
// finish is a no-op and execution continues.
type workspaceSend struct {
	tracker   *workspaceSendTracker
	token     string
	attemptNo int32
	model     string
	layer     string
}

func newWorkspaceSendTracker(recorder taskports.AttemptRecorder, taskID string, riverJobID int64, service string) *workspaceSendTracker {
	return &workspaceSendTracker{recorder: recorder, taskID: taskID, riverJobID: riverJobID, service: service}
}

// claimedSend binds the send to the attempt fact its claim already wrote.
// token IS the lease token, so finish() closes exactly the row the claim
// opened: the split claim-then-StartAttempt pair used to mint a second,
// unrelated token here, which is what let a lease and an attempt fact drift
// apart.
func (t *workspaceSendTracker) claimedSend(token, model, layer string) *workspaceSend {
	return &workspaceSend{
		tracker: t, token: strings.TrimSpace(token),
		model: strings.TrimSpace(model), layer: strings.TrimSpace(layer),
	}
}

// fallbackConsumed reports whether a model_fallback row already exists
// for the task. A recorder failure fails closed to consumed: without
// proof that the budget remains, no second send.
func (t *workspaceSendTracker) fallbackConsumed(ctx context.Context) bool {
	if t == nil || t.recorder == nil {
		return false
	}
	consumed, err := t.recorder.HasModelFallbackAttempt(ctx, t.taskID)
	if err != nil {
		slog.Warn("workspace task fallback check failed; skipping fallback",
			"task_id", t.taskID, "err", err)
		return true
	}
	return consumed
}

// startSend opens the attempt fact for a SECOND send inside an already
// claimed execution (today: the model_fallback send). It is best-effort: a
// failing recorder warns and the send proceeds without facts.
//
// The primary send does NOT come through here. Its fact is written by the
// claim statement, keyed by the lease token (claimedSend); only a send that
// happens after the claim needs a freshly minted token, because
// uq_task_execution_attempts_token gives one row per send.
func (t *workspaceSendTracker) startSend(ctx context.Context, model, layer string) *workspaceSend {
	send := &workspaceSend{tracker: t, model: strings.TrimSpace(model), layer: strings.TrimSpace(layer)}
	if t == nil || t.recorder == nil {
		return send
	}
	token, err := tasks.NewExecutionToken()
	if err != nil {
		slog.Warn("workspace task attempt token mint failed; continuing without facts",
			"task_id", t.taskID, "err", err)
		return send
	}
	attemptNo, err := t.recorder.StartAttempt(ctx, tasks.AttemptStart{
		TaskID:         t.taskID,
		RiverJobID:     t.riverJobID,
		ExecutionToken: token,
		Service:        t.service,
		RequestedModel: send.model,
		ResolvedModel:  send.model,
		Provider:       workspaceAttemptProvider(send.model),
		RetryLayer:     send.layer,
		StartedAt:      time.Now(),
	})
	if err != nil {
		slog.Warn("workspace task attempt start failed; continuing without facts",
			"task_id", t.taskID, "retry_layer", send.layer, "err", err)
		return send
	}
	send.token = token
	send.attemptNo = attemptNo
	return send
}

// startFallbackSend opens the model_fallback fact BEFORE the second
// send. Any failure denies the fallback (ok=false): at-most-once is
// enforced by the persisted row, never by memory.
func (t *workspaceSendTracker) startFallbackSend(ctx context.Context, candidate string) (*workspaceSend, bool) {
	if t == nil || t.recorder == nil {
		return nil, false
	}
	send := t.startSend(ctx, candidate, tasks.AttemptRetryLayerModelFallback)
	if send.token == "" {
		return nil, false
	}
	return send, true
}

func (s *workspaceSend) finishCompleted(model string) {
	s.finish(tasks.AttemptStatusCompleted, "", "", "", strings.TrimSpace(model))
}

func (s *workspaceSend) finishFailed(stage, category, reason string) {
	s.finish(tasks.AttemptStatusFailed, stage, category, reason, s.model)
}

func (s *workspaceSend) finishRequeued(stage, category, reason string) {
	s.finish(tasks.AttemptStatusRequeued, stage, category, reason, s.model)
}

func (s *workspaceSend) finish(status, stage, category, reason, resolvedModel string) {
	if s == nil || s.tracker == nil || s.tracker.recorder == nil || s.token == "" {
		return
	}
	// Detached cleanup context: callers often finalize after the
	// execution context was cancelled, and using it would cancel the
	// writeback too.
	cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	rows, err := s.tracker.recorder.FinishAttempt(cleanupCtx, tasks.AttemptFinish{
		ExecutionToken:  s.token,
		TaskID:          s.tracker.taskID,
		Status:          status,
		ResolvedModel:   resolvedModel,
		FailureStage:    stage,
		FailureCategory: category,
		ErrorMsg:        truncateWorkspaceError(reason),
		FinishedAt:      time.Now(),
	})
	if err != nil {
		slog.Warn("workspace task attempt finish failed",
			"task_id", s.tracker.taskID, "retry_layer", s.layer, "err", err)
		return
	}
	if rows == 0 {
		slog.Warn("workspace task attempt finish matched no row",
			"task_id", s.tracker.taskID, "retry_layer", s.layer)
	}
}
