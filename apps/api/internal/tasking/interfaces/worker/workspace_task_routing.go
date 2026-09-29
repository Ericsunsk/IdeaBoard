package worker

// Snapshot-only execution helpers (#848, ADR-0012 rules 6 and 9).
//
// The worker consumes ONLY the pinned submit-time RoutingSnapshot that
// rides the task row: it never reads the live pool or the admin mapping.
// Every helper here works from the snapshot bytes alone; pool edits
// after accept change nothing. (The transient ResolvedModel handoff on
// the in-memory claimed copy is stamped from the snapshot below; it is
// never a durable routing fact.)

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"ideaboard/api/internal/generation/routing"
	tasks "ideaboard/api/internal/tasking/domain"
	"ideaboard/api/internal/tasking/workspacetasks"
)

// errWorkspaceRoutingSnapshotMissing fails a task closed when no
// snapshot was pinned (pre-pool row or a submit path that bypassed the
// pin). The hold is released and no retry budget burns, mirroring the
// #847 interim behavior.
var errWorkspaceRoutingSnapshotMissing = errors.New(msgSnapshotMissing)

// errWorkspaceRoutingSnapshotCorrupt fails a task closed when the pinned
// snapshot exists but no longer replays (tampered or malformed). Like a
// missing snapshot it releases the hold without burning retries: an
// unreplayable routing fact must never spend the fallback budget.
var errWorkspaceRoutingSnapshotCorrupt = errors.New(msgSnapshotCorrupt)

// resolveExecutionSnapshot parses and Verify()s the pinned snapshot on
// the task. It needs no live pool: Verify replays the selection from
// the snapshot fields alone. A snapshot pinned for another service is
// rejected the same as a corrupt one: routing facts must belong to the
// task that carries them.
func resolveExecutionSnapshot(task *workspacetasks.WorkspaceTask) (routing.RoutingSnapshot, error) {
	if task == nil || len(task.RoutingSnapshot) == 0 {
		return routing.RoutingSnapshot{}, errWorkspaceRoutingSnapshotMissing
	}
	var snap routing.RoutingSnapshot
	if err := json.Unmarshal(task.RoutingSnapshot, &snap); err != nil {
		return routing.RoutingSnapshot{}, fmt.Errorf("%w: %w", errWorkspaceRoutingSnapshotCorrupt, err)
	}
	if err := snap.Verify(); err != nil {
		return routing.RoutingSnapshot{}, fmt.Errorf("%w: %w", errWorkspaceRoutingSnapshotCorrupt, err)
	}
	if !strings.EqualFold(strings.TrimSpace(snap.Service), strings.TrimSpace(task.Service)) {
		return routing.RoutingSnapshot{}, fmt.Errorf("%w: snapshot service %q does not match task service %q",
			errWorkspaceRoutingSnapshotCorrupt, snap.Service, task.Service)
	}
	return snap, nil
}

// selectExecutionModel resolves the gateway target for this execution
// from the pinned snapshot and the persisted fallback state. When a
// model_fallback attempt already exists (fallback consumed by an
// earlier execution), the current model is the retry candidate and no
// further fallback is allowed: at most one fallback per task.
func selectExecutionModel(snap routing.RoutingSnapshot, fallbackConsumed bool) (model string, allowFallback bool) {
	if fallbackConsumed {
		if snap.HasRetryCandidate() {
			return snap.RetryCandidate, false
		}
		return snap.SelectedModel, false
	}
	return snap.SelectedModel, snap.HasRetryCandidate()
}

// workspaceAttemptFailure maps one provider-seam error onto the
// routing.AttemptFailure taxonomy (ADR-0012 rule 6) without importing
// the runtime layer: the HTTP status arrives through the structural
// HTTPStatus() int seam, timeout/network markers through the same
// message families IsRetryableProviderError already sniffs.
//
// Deterministic markers take precedence (a 500 carrying a business
// rejection is terminal); a failure with no marker at all fails closed
// to terminal and never spends the fallback budget.
func workspaceAttemptFailure(err error) routing.AttemptFailure {
	var failure routing.AttemptFailure
	if err == nil {
		return failure
	}
	status := workspaceProviderHTTPStatus(err)
	failure.HTTPStatus = status
	lowered := strings.ToLower(err.Error())
	switch {
	case status == 400 || status == 422:
		failure.InvalidParams = true
	case status == 401 || status == 403:
		failure.AuthFailed = true
	case status == 402:
		failure.InsufficientCredits = true
	case status == 404:
		failure.NotFound = true
	}
	if containsWorkspaceFailureMarker(lowered, workspaceContentSafetyMarkers) {
		failure.ContentSafetyRejected = true
	}
	if failure.Deterministic() {
		return failure
	}
	if containsWorkspaceFailureMarker(lowered, workspaceTimeoutMarkers) {
		failure.Timeout = true
	}
	if containsWorkspaceFailureMarker(lowered, workspaceNetworkMarkers) {
		failure.NetworkError = true
	}
	return failure
}

// workspaceProviderHTTPStatus extracts the upstream HTTP status through
// the structural seam (UpstreamHTTPError implements HTTPStatus() int),
// the same structural seam the worker previously used.
func workspaceProviderHTTPStatus(err error) int {
	var coder workspaceHTTPStatusCoder
	if errors.As(err, &coder) {
		return coder.HTTPStatus()
	}
	return 0
}

func containsWorkspaceFailureMarker(haystack string, markers []string) bool {
	for _, marker := range markers {
		if strings.Contains(haystack, marker) {
			return true
		}
	}
	return false
}

// workspaceTimeoutMarkers mirrors the timeout family
// IsRetryableProviderError sniffs, split out so the taxonomy records
// Timeout distinctly from NetworkError.
var workspaceTimeoutMarkers = []string{"timeout", "deadline exceeded"}

// workspaceNetworkMarkers mirrors the network family
// IsRetryableProviderError sniffs.
var workspaceNetworkMarkers = []string{
	"network", "connection reset", "temporary", "econnreset", "econnrefused",
}

// workspaceContentSafetyMarkers forces terminal convergence for
// content-safety rejections regardless of status code. Matching is a
// fail-closed direction only (terminal, never fallback), so the list
// stays tight to canonical provider phrasing.
var workspaceContentSafetyMarkers = []string{
	"content safety", "content-safety", "content_safety", "safety rejection",
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
	recorder   workspacetasks.AttemptRecorder
	taskID     string
	riverJobID int64
	service    string
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

func newWorkspaceSendTracker(recorder workspacetasks.AttemptRecorder, taskID string, riverJobID int64, service string) *workspaceSendTracker {
	return &workspaceSendTracker{recorder: recorder, taskID: taskID, riverJobID: riverJobID, service: service}
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

// startSend opens one attempt fact, best-effort: a failing recorder
// warns and the send proceeds without facts (telemetry failure never
// changes task or billing state).
func (t *workspaceSendTracker) startSend(ctx context.Context, model, layer string) *workspaceSend {
	send := &workspaceSend{tracker: t, model: strings.TrimSpace(model), layer: strings.TrimSpace(layer)}
	if t == nil || t.recorder == nil {
		return send
	}
	token, err := workspacetasks.NewExecutionToken()
	if err != nil {
		slog.Warn("workspace task attempt token mint failed; continuing without facts",
			"task_id", t.taskID, "err", err)
		return send
	}
	attemptNo, err := t.recorder.StartAttempt(ctx, workspacetasks.AttemptStart{
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
	send := t.startSend(ctx, candidate, workspacetasks.AttemptRetryLayerModelFallback)
	if send.token == "" {
		return nil, false
	}
	return send, true
}

func (s *workspaceSend) finishCompleted(model string) {
	s.finish(workspacetasks.AttemptStatusCompleted, "", "", "", strings.TrimSpace(model))
}

func (s *workspaceSend) finishFailed(stage, category, reason string) {
	s.finish(workspacetasks.AttemptStatusFailed, stage, category, reason, s.model)
}

func (s *workspaceSend) finishRequeued(stage, category, reason string) {
	s.finish(workspacetasks.AttemptStatusRequeued, stage, category, reason, s.model)
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
	rows, err := s.tracker.recorder.FinishAttempt(cleanupCtx, workspacetasks.AttemptFinish{
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
