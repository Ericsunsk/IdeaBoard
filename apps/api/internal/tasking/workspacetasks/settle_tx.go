package workspacetasks

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"ideaboard/api/internal/credits/domain"
	"ideaboard/api/internal/db/dbutil"
	"ideaboard/api/internal/generation/routing"
	"ideaboard/api/internal/tasking/settlement"

	"github.com/jackc/pgx/v5"
)

// mapTerminalLedgerError converges a ledger rejection after the fact
// already succeeded to the accurate caller semantic: the terminal fact is
// durable, so the task must report a state conflict (409), never a
// retryable 503 that would roll the settlement back and re-enter the
// provider. Same-key replays already return Replayed with no error;
// IB006 (hold settled / command already recorded) and IB001 (same key,
// different amount or BusinessReference) both land here.
//
// The 409-vs-503 mapping itself lives in settlement.IsTerminalLedgerConflict
// (the ledger rejection predicate); this helper also adds leg context to
// other errors so callers do not repeat the classification and wrapping.
func mapTerminalLedgerError(taskID, leg string, err error) error {
	if !settlement.IsTerminalLedgerConflict(err) {
		return fmt.Errorf("workspacetasks: %s: %w", leg, err)
	}
	if errors.Is(err, domain.ErrHoldSettled) {
		return fmt.Errorf("workspacetasks: %s on task %s already settled (terminal fact durable): %w: %w", leg, taskID, ErrStateConflict, err)
	}
	return fmt.Errorf("workspacetasks: %s on task %s conflicts with the durable ledger fact: %w: %w", leg, taskID, ErrStateConflict, err)
}

// SettlementTransactor demarcates the settlement transaction. The postgres
// workspace task Store implements it over its pool. In-memory stores are
// test fixtures and are intentionally outside this production seam.
//
// The Scope is opened on the task store's pool, but every participant
// shares the same PostgreSQL database (tasks, credits ledger, billing
// catalog, river_job), so Credits SettlementInTx(tx), catalog QuoteInTx,
// and River InsertTx all join the scope's transaction. Ownership travels
// with the Scope: the caller ends it with scope.Commit/scope.Rollback.
type SettlementTransactor interface {
	BeginSettlementScope(ctx context.Context) (*dbutil.Scope, error)
}

// TxHolder reserves and converges Ideas inside the settlement transaction.
// It keeps credits.Commands as the sole ledger writer: implementations must
// delegate to the transaction-scoped Commands (credits.Service.InTx) and
// perform no ledger SQL of their own.
//
// Both ids are explicit: terminal settlement reads the funds
// account and the operator from the flipped task row, so Capture/Release
// never depend on the process-local hold map (which a restart would lose)
// and never fall back to an owner wallet.
type TxHolder interface {
	HoldInTx(ctx context.Context, tx pgx.Tx, billingAccountID, actorUserID int64, amount int64, idempotencyKey, taskID string) (string, error)
	CaptureInTx(ctx context.Context, tx pgx.Tx, billingAccountID, actorUserID int64, holdTransactionID string, amount int64, taskID string) (string, error)
	ReleaseInTx(ctx context.Context, tx pgx.Tx, billingAccountID, actorUserID int64, holdTransactionID string, amount int64, taskID string) (string, error)
}

// TxBillingAccountResolver resolves the stable funds identity inside the
// settlement transaction (#915 resolution, #916 funds identity). Production
// implements it on the postgres task Store; tests stub it. Resolution joins
// the caller transaction and fails closed on ambiguity or conflict.
type TxBillingAccountResolver interface {
	ResolveBillingAccountInTx(ctx context.Context, tx pgx.Tx, workspacePublicID string) (int64, error)
}

// TxMemberQuotaChecker enforces the member cross-workspace consumption
// quota inside the settlement transaction (#946). Production implements
// it on the memberquotas Store; tests stub it. The check joins THIS
// transaction after the Quote and before the Hold, attributing usage to
// the real operator (Billing Account + User) across every Workspace
// sharing the account. A nil checker means no quota enforcement
// (unlimited).
type TxMemberQuotaChecker interface {
	CheckQuotaInTx(ctx context.Context, tx pgx.Tx, billingAccountID, userID int64, amount int64, reservedAt time.Time) error
}

// TxEnqueuer carries the River payload inside the settlement transaction.
// Production adapts the River client InsertTx, so the river_job row commits
// atomically with the business task row (never enqueued without task facts,
// never a task without its job).
type TxEnqueuer interface {
	EnqueueInTx(ctx context.Context, tx pgx.Tx, task *WorkspaceTask) error
}

// TxTaskLifecycleStore persists task state transitions inside the settlement
// transaction. The postgres Store implements it over dbsqlc.WithTx(tx).
type TxTaskLifecycleStore interface {
	CreateTaskInTx(ctx context.Context, tx pgx.Tx, task *WorkspaceTask) (*WorkspaceTask, error)
	CompleteTaskInTx(ctx context.Context, tx pgx.Tx, workspaceID, taskID string, outputs []OutputRef) (*WorkspaceTask, error)
	FailTaskInTx(ctx context.Context, tx pgx.Tx, workspaceID, taskID, errorMsg string) (*WorkspaceTask, error)
	CancelTaskInTx(ctx context.Context, tx pgx.Tx, workspaceID, taskID string, actorUserID int64, reason string) (*WorkspaceTask, bool, error)
	TransitionForRetryInTx(ctx context.Context, tx pgx.Tx, workspaceID, taskID, newToken string) (*WorkspaceTask, error)
	// GetTaskInTx reads a workspace-scoped task inside the settlement
	// transaction. It exists so an idempotent replay resolves the task its
	// durable claim named from the same consistent snapshot that observed
	// the claim, never from a second connection.
	GetTaskInTx(ctx context.Context, tx pgx.Tx, workspaceID, taskID string) (*WorkspaceTask, error)
}

// TaskSettlementStore requires lifecycle writes and their transaction owner
// together. Every settlement joins the store's PostgreSQL transaction.
type TaskSettlementStore interface {
	SettlementTransactor
	TxTaskLifecycleStore
}

// TaskBillingStore persists immutable billing facts and the settlement read
// model in the same transaction as task state and Credits.
type TaskBillingStore interface {
	CreateBillingFactsInTx(context.Context, pgx.Tx, *WorkspaceTask) error
	SetBillingSettlementInTx(context.Context, pgx.Tx, string, string, int64, int64, int64, string, string) error
}

// stageSettlementEvent stages one status event after its settlement unit is
// durable. The owned/borrowed deferral lives in the deep settlement kernel
// (settlement.StageOrDefer on the dbutil post-commit interface); this stays
// a thin adapter binding the service's staging closure.
func (s *Service) stageSettlementEvent(ctx context.Context, owned bool, taskID, workspaceID string, status Status, executionToken string, occurredAt time.Time) {
	settlement.StageOrDefer(ctx, owned, func(context.Context) {
		s.stageWithTokenAt(taskID, workspaceID, status, executionToken, occurredAt)
	})
}

// captureSplit derives the per-output unit billing capped at the held
// total, with the remainder released. Both legs run in the same tx.
// Owned by the deep settlement kernel; this stays a thin adapter so the
// terminal paths keep their shape.
func captureSplit(completed *WorkspaceTask, outputCount int) (capture, release int64) {
	return settlement.CaptureSplit(completed.BillingSnapshot.TotalCredits, completed.BillingSnapshot.UnitCredits, outputCount)
}

func (s *Service) holderSettles(holdID string, total int64) bool {
	return s.holder != nil && strings.TrimSpace(holdID) != "" && total > 0
}

// settleFreeTerminal converges a free task (zero total, no hold) to its
// terminal settlement word in the same transaction as the state flip. Free
// tasks hold nothing, so there is no Capture/Release leg and no ledger
// row; the settlement read model still moves free -> captured (completed)
// or free -> released (failed/cancelled) with zero amounts and NULL
// transaction ids, so terminal free tasks never fork the read model on
// the initial state.
//
// A positive total without a hold is a torn durable fact (the initial
// held row requires a hold): it fails closed here so the task stays
// non-terminal and operator-visible instead of completing with no
// settlement. Amount<=0 Capture/Release legs are unreachable by
// construction (holderSettles gates the paid path, and the Credits ledger
// rejects non-positive commands): their idempotency keys are
// intentionally never written, and the read model records NULL tx ids.
func (s *Service) settleFreeTerminal(ctx context.Context, tx pgx.Tx, task *WorkspaceTask, state string, outputCount int64) error {
	if task == nil {
		return fmt.Errorf("workspacetasks: free terminal settlement requires a task: %w", ErrSettlementUnavailable)
	}
	if task.BillingSnapshot.TotalCredits > 0 || strings.TrimSpace(task.BillingSnapshot.HoldTransactionID) != "" {
		return fmt.Errorf("workspacetasks: terminal settlement without hold: %w", ErrSettlementUnavailable)
	}
	facts := s.taskBilling
	if facts == nil {
		return nil
	}
	if err := facts.SetBillingSettlementInTx(ctx, tx, task.ID, state, outputCount, 0, 0, "", ""); err != nil {
		return fmt.Errorf("workspacetasks: persist free billing settlement: %w", err)
	}
	task.BillingSnapshot.SettlementState = state
	return nil
}

// releaseTerminalInTx converges failed/cancelled billing after the state flip.
// Transaction commit, replay detection and event staging remain with the caller.
func (s *Service) releaseTerminalInTx(ctx context.Context, tx pgx.Tx, task *WorkspaceTask) error {
	if !s.holderSettles(task.BillingSnapshot.HoldTransactionID, task.BillingSnapshot.TotalCredits) {
		return s.settleFreeTerminal(ctx, tx, task, "released", 0)
	}
	releaseTxID, err := s.holder.ReleaseInTx(ctx, tx, task.BillingAccountID, task.OwnerUserID,
		task.BillingSnapshot.HoldTransactionID, task.BillingSnapshot.TotalCredits, task.ID)
	if err != nil {
		return mapTerminalLedgerError(task.ID, "release terminal billing", err)
	}
	task.BillingSnapshot.SettlementState = "released"
	if facts := s.taskBilling; facts != nil {
		if err := facts.SetBillingSettlementInTx(ctx, tx, task.ID, "released", 0, 0,
			task.BillingSnapshot.TotalCredits, "", releaseTxID); err != nil {
			return fmt.Errorf("workspacetasks: persist billing settlement: %w", err)
		}
	}
	return nil
}

// createInTx settles Authorize -> Claim -> Quote -> Hold -> Insert ->
// Enqueue in ONE PostgreSQL transaction. Any failure rolls the whole
// transaction back: no task row without its Hold, no Hold without its
// task, no River job without task facts, and no durable idempotency claim
// without all of them.
//
// Lock order is load-bearing (#917): the submission gate runs FIRST
// because it takes the workspace row FOR UPDATE, serializing
// same-workspace settlements at entry. The claim INSERT that follows
// takes FOR KEY SHARE on the same row through its workspace FK; taking
// it before the membership lock deadlocked concurrent distinct-key
// submits (both held KEY SHARE, both wanted the FOR UPDATE upgrade,
// SQLSTATE 40P01). The claim still precedes Quote and the Hold, so a
// retried submit collides on the claim's unique constraint and returns
// the original task without ever reaching the ledger, the task table, or
// River. Missing required dependencies fail closed before any write.
func (s *Service) createInTx(ctx context.Context, input CreateInput, kind, resolution string, quantity int64, taskID, token string, claim SubmissionClaim) (*WorkspaceTask, error) {
	if s == nil || s.taskLifecycle == nil || s.quoter == nil || s.holder == nil || s.queue == nil || s.submissionClaims == nil || s.billingAccounts == nil {
		return nil, fmt.Errorf("workspacetasks: create settlement seams are not configured (caller must retry): %w", ErrSettlementUnavailable)
	}
	if s.saveSnapshot == nil {
		return nil, errors.New("workspacetasks: routing snapshot saver is required for atomic settlement")
	}
	scope, err := settlement.BorrowOrBeginScope(ctx, s.taskLifecycle)
	if err != nil {
		return nil, err
	}
	tx := scope.Tx()
	owned := scope.Owned()
	committed := false
	defer func() {
		if !committed {
			_ = scope.Rollback(ctx)
		}
	}()
	// The submission gate runs before the claim (see the lock-order note
	// above): it pins current membership, the Board binding, and input
	// authorization on the settlement transaction before any Quote or
	// Hold. A rejection here writes nothing, so no claim is left behind
	// to fence its key.
	if s.acceptance == nil {
		return nil, ErrSettlementUnavailable
	}
	binding, err := s.acceptance.AuthorizeSubmission(ctx, tx, input)
	if err != nil {
		return nil, err
	}
	// Claim the client key inside the settlement transaction. This is the
	// concurrency fence: a concurrent submit of the same key blocks on the
	// unique constraint until this transaction commits or rolls back, then
	// observes the winner and replays it.
	existing, claimed, err := s.submissionClaims.ClaimSubmissionInTx(ctx, tx, claim)
	if err != nil {
		return nil, fmt.Errorf("workspacetasks: claim submission: %w", err)
	}
	if !claimed {
		replayed, err := s.replayClaimedSubmission(ctx, tx, s.taskLifecycle, existing, claim)
		if err != nil {
			return nil, err
		}
		// Owned read-only transaction: commit so the replayed task is
		// read from a consistent snapshot and no write is left open. A
		// borrowed guard transaction stays open for the guard to commit
		// (Scope.Commit is a safe no-op when borrowed).
		if err := scope.Commit(ctx); err != nil {
			return nil, err
		}
		committed = true
		// No event is staged on replay: the winning submit already emitted
		// task.created for this exact task.
		return replayed, nil
	}
	// Stable funds identity first (#915 resolution joins this transaction,
	// #916 spends it): the Hold below and every terminal settlement use
	// this account with the submitter as operator. Ambiguity or conflict
	// fails closed before any ledger, task, or queue write.
	billingAccountID, err := s.billingAccounts.ResolveBillingAccountInTx(ctx, tx, claim.WorkspaceID)
	if err != nil {
		return nil, err
	}
	// Member-quota boundary (#946: the member cross-workspace quota
	// joins THIS transaction after the Quote (so the quoted amount is
	// known) and before the Hold below, locking its row and reading the
	// same snapshot. A nil checker means unlimited (no enforcement).
	// Resolve inside the settlement transaction before Quote/Hold so
	// an empty pool fails before any ledger, task, or queue write (the
	// rollback below then removes nothing because nothing was written).
	// The snapshot pins atomically with the rows that follow. A nil
	// resolver fails closed with the canonical unavailable sentinel.
	if s.resolver == nil {
		return nil, &routing.ResolutionUnavailableError{Service: kind, Resolution: routing.Resolution(resolution)}
	}
	routingSnap, err := s.resolver.ResolveInTx(ctx, tx, kind, resolution)
	if err != nil {
		var inputErr *routing.InvalidRoutingInputError
		if errors.As(err, &inputErr) {
			return nil, newValidationError(FieldError{Field: inputErr.Field, Message: "invalid"})
		}
		return nil, fmt.Errorf("workspacetasks: routing unavailable: %w", err)
	}
	snapshot, err := s.quoter.QuoteInTx(ctx, tx, kind, resolution, quantity)
	if err != nil {
		return nil, err
	}
	snapshot.SettlementState = "quoted"
	snapshot.BillingAccountID = billingAccountID
	if s.quotas != nil && snapshot.TotalCredits > 0 {
		if err := s.quotas.CheckQuotaInTx(ctx, tx, billingAccountID, input.OwnerUserID, snapshot.TotalCredits, snapshot.QuotedAt); err != nil {
			return nil, err
		}
	}
	var holdTxID string
	if snapshot.TotalCredits > 0 {
		// The ledger key is derived from the DURABLE claim scope, never from
		// the freshly minted task id. A retried submit therefore asks the
		// ledger for the same key, so the ledger's own replay path fences a
		// duplicate hold even if the claim were somehow bypassed.
		holdTxID, err = s.holder.HoldInTx(ctx, tx, billingAccountID, input.OwnerUserID, snapshot.TotalCredits,
			settlement.TaskHoldKey(claim.WorkspaceID, claim.IdempotencyKey), taskID)
		if err != nil {
			return nil, err
		}
		snapshot.HoldTransactionID = holdTxID
		snapshot.SettlementState = "held"
	} else {
		snapshot.SettlementState = "free"
	}
	task := pendingTaskWithBinding(input, kind, taskID, token, binding, snapshot)
	created, err := s.taskLifecycle.CreateTaskInTx(ctx, tx, task)
	if err != nil {
		return nil, err
	}
	// Pin the snapshot in the same tx so it commits atomically with
	// Hold/task/enqueue. Later Admin pool edits never invalidate this
	// accepted task (the worker replays selection from the pinned fact,
	// never the live pool).
	if err := s.saveSnapshot(ctx, tx, created.ID, routingSnap); err != nil {
		return nil, fmt.Errorf("workspacetasks: pin routing snapshot: %w", err)
	}
	if facts := s.taskBilling; facts != nil {
		if err := facts.CreateBillingFactsInTx(ctx, tx, created); err != nil {
			return nil, fmt.Errorf("workspacetasks: persist billing facts: %w", err)
		}
	}
	if err := s.queue.EnqueueInTx(ctx, tx, created); err != nil {
		return nil, fmt.Errorf("workspacetasks: enqueue: %w", err)
	}
	// The settlement unit is durable once the owned transaction commits;
	// a borrowed guard transaction commits later at the guard. The event
	// stages after the successful unit either way (never inside a failed
	// one), and the caller flushes it only after the commit it can see.
	if err := scope.Commit(ctx); err != nil {
		return nil, err
	}
	committed = true
	s.stageSettlementEvent(ctx, owned, created.ID, created.WorkspaceID, StatusPending, created.ExecutionToken, created.UpdatedAt)
	return created, nil
}

// replayClaimedSubmission resolves an existing durable claim into the task it
// settled, or fails closed.
//
// Two outcomes, both terminal for this request:
//   - Same request hash: replay. The ORIGINAL task is returned. It is read
//     inside the caller's transaction so the read is consistent with the
//     claim that named it.
//   - Different request hash: ErrIdempotencyConflict (409). The caller must
//     NOT receive a task built from a different body.
//
// A claim that names a task which cannot be read is a torn durable fact; it
// fails closed with an error rather than settling a second task.
func (s *Service) replayClaimedSubmission(ctx context.Context, tx pgx.Tx, txStore TxTaskLifecycleStore, existing *SubmissionClaim, attempted SubmissionClaim) (*WorkspaceTask, error) {
	if existing == nil {
		return nil, fmt.Errorf("workspacetasks: claim store reported a conflict without a claim: %w", ErrSettlementUnavailable)
	}
	if existing.WorkspaceID != attempted.WorkspaceID || existing.Operation != attempted.Operation ||
		existing.IdempotencyKey != attempted.IdempotencyKey {
		return nil, fmt.Errorf("workspacetasks: claim scope mismatch: %w", ErrSettlementUnavailable)
	}
	if existing.RequestHash != attempted.RequestHash {
		return nil, ErrIdempotencyConflict
	}
	// The claim and the task commit in the same transaction, so a readable
	// claim always names a committed task. Anything else is fail-closed.
	taskID := strings.TrimSpace(existing.TaskPublicID)
	if taskID == "" {
		return nil, fmt.Errorf("workspacetasks: claim names no task: %w", ErrSettlementUnavailable)
	}
	replayed, err := txStore.GetTaskInTx(ctx, tx, existing.WorkspaceID, taskID)
	if err != nil {
		return nil, fmt.Errorf("workspacetasks: replay claimed task %s: %w", taskID, err)
	}
	return replayed, nil
}

// completeInTx flips running -> completed (persisting outputs) atomically
// with Capture (+ Release of the remainder) in ONE transaction. A billing
// failure rolls the state flip back (task unchanged); a state-guard failure
// rolls the billing writes back (no capture/release artifacts).
//
// A missing terminal seam is a retryable wiring fault, never a silent
// drop: it returns ErrSettlementUnavailable so the
// caller (HTTP 503 / River retry) must retry. No state flips and no
// billing writes run on this path.
func (s *Service) completeInTx(ctx context.Context, workspaceID, taskID string, outputs []OutputRef) (*WorkspaceTask, error) {
	if s == nil || s.taskLifecycle == nil || s.holder == nil {
		return nil, fmt.Errorf("workspacetasks: terminal settlement seams are not configured (caller must retry): %w", ErrSettlementUnavailable)
	}
	scope, err := settlement.BorrowOrBeginScope(ctx, s.taskLifecycle)
	if err != nil {
		return nil, err
	}
	tx := scope.Tx()
	owned := scope.Owned()
	committed := false
	defer func() {
		if !committed {
			_ = scope.Rollback(ctx)
		}
	}()
	completed, err := s.taskLifecycle.CompleteTaskInTx(ctx, tx, workspaceID, taskID, outputs)
	if err != nil {
		return nil, err
	}
	if s.holderSettles(completed.BillingSnapshot.HoldTransactionID, completed.BillingSnapshot.TotalCredits) {
		capture, release := captureSplit(completed, len(outputs))
		var captureTxID, releaseTxID string
		if capture > 0 {
			var captureErr error
			captureTxID, captureErr = s.holder.CaptureInTx(ctx, tx, completed.BillingAccountID, completed.OwnerUserID, completed.BillingSnapshot.HoldTransactionID, capture, taskID)
			if captureErr != nil {
				return nil, mapTerminalLedgerError(taskID, "capture on complete", captureErr)
			}
		}
		if release > 0 {
			var releaseErr error
			releaseTxID, releaseErr = s.holder.ReleaseInTx(ctx, tx, completed.BillingAccountID, completed.OwnerUserID, completed.BillingSnapshot.HoldTransactionID, release, taskID)
			if releaseErr != nil {
				return nil, mapTerminalLedgerError(taskID, "release remainder on complete", releaseErr)
			}
		}
		if capture > 0 {
			completed.BillingSnapshot.SettlementState = "captured"
		} else if release > 0 {
			completed.BillingSnapshot.SettlementState = "released"
		}
		if facts := s.taskBilling; facts != nil && completed.BillingSnapshot.SettlementState != "free" {
			if err := facts.SetBillingSettlementInTx(ctx, tx, taskID, completed.BillingSnapshot.SettlementState, int64(len(outputs)), capture, release, captureTxID, releaseTxID); err != nil {
				return nil, fmt.Errorf("workspacetasks: persist billing settlement: %w", err)
			}
		}
	} else if err := s.settleFreeTerminal(ctx, tx, completed, "captured", int64(len(outputs))); err != nil {
		return nil, err
	}
	if err := scope.Commit(ctx); err != nil {
		return nil, err
	}
	committed = true
	s.stageSettlementEvent(ctx, owned, taskID, workspaceID, StatusCompleted, completed.ExecutionToken, completed.UpdatedAt)
	return completed, nil
}

// failInTx flips to failed atomically with Release in ONE transaction.
//
// A missing terminal seam is retryable (caller must retry) with
// ErrSettlementUnavailable; nothing flips and nothing releases.
func (s *Service) failInTx(ctx context.Context, workspaceID, taskID, errorMsg string) (*WorkspaceTask, error) {
	if s == nil || s.taskLifecycle == nil || s.holder == nil {
		return nil, fmt.Errorf("workspacetasks: terminal settlement seams are not configured (caller must retry): %w", ErrSettlementUnavailable)
	}
	scope, err := settlement.BorrowOrBeginScope(ctx, s.taskLifecycle)
	if err != nil {
		return nil, err
	}
	tx := scope.Tx()
	owned := scope.Owned()
	committed := false
	defer func() {
		if !committed {
			_ = scope.Rollback(ctx)
		}
	}()
	failed, err := s.taskLifecycle.FailTaskInTx(ctx, tx, workspaceID, taskID, errorMsg)
	if err != nil {
		return nil, err
	}
	if err := s.releaseTerminalInTx(ctx, tx, failed); err != nil {
		return nil, err
	}
	if err := scope.Commit(ctx); err != nil {
		return nil, err
	}
	committed = true
	s.stageSettlementEvent(ctx, owned, taskID, workspaceID, StatusFailed, failed.ExecutionToken, failed.UpdatedAt)
	return failed, nil
}

// cancelInTx flips to cancelled atomically with Release in ONE transaction.
// An already-terminal row replays idempotently without billing or events.
//
// A missing terminal seam is retryable (caller must retry) with
// ErrSettlementUnavailable; nothing flips and nothing releases.
func (s *Service) cancelInTx(ctx context.Context, workspaceID, taskID string, actorUserID int64, reason string) (*WorkspaceTask, bool, error) {
	if s == nil || s.taskLifecycle == nil || s.holder == nil {
		return nil, false, fmt.Errorf("workspacetasks: terminal settlement seams are not configured (caller must retry): %w", ErrSettlementUnavailable)
	}
	scope, err := settlement.BorrowOrBeginScope(ctx, s.taskLifecycle)
	if err != nil {
		return nil, false, err
	}
	tx := scope.Tx()
	owned := scope.Owned()
	committed := false
	defer func() {
		if !committed {
			_ = scope.Rollback(ctx)
		}
	}()
	flipped, already, err := s.taskLifecycle.CancelTaskInTx(ctx, tx, workspaceID, taskID, actorUserID, reason)
	if err != nil {
		return nil, false, err
	}
	if already {
		return flipped, true, nil
	}
	if err := s.releaseTerminalInTx(ctx, tx, flipped); err != nil {
		return nil, false, err
	}
	if owned {
		if err := scope.Commit(ctx); err != nil {
			return nil, false, err
		}
		committed = true
		s.stageWithTokenAt(taskID, workspaceID, StatusCancelled, flipped.ExecutionToken, flipped.UpdatedAt)
		return flipped, false, nil
	}
	// Borrowed (ambient) transaction (#889): the guard commits later
	// together with the audit row, so the stage defers through the same
	// post-commit hook every other settlement uses (see
	// stageSettlementEvent).
	s.stageSettlementEvent(ctx, owned, taskID, workspaceID, StatusCancelled, flipped.ExecutionToken, flipped.UpdatedAt)
	return flipped, false, nil
}

// retryInTx consumes one retry budget entry (reset + new execution token)
// atomically with the River requeue in ONE transaction, so a requeue failure
// cannot strand a token-bumped pending task without its job.
//
// A missing retry seam is retryable (caller must retry) with
// ErrSettlementUnavailable; no budget is consumed and nothing requeues.
func (s *Service) retryInTx(ctx context.Context, workspaceID, taskID, newToken string) (*WorkspaceTask, error) {
	if s == nil || s.taskLifecycle == nil || s.queue == nil {
		return nil, fmt.Errorf("workspacetasks: retry settlement seams are not configured (caller must retry): %w", ErrSettlementUnavailable)
	}
	scope, err := settlement.BorrowOrBeginScope(ctx, s.taskLifecycle)
	if err != nil {
		return nil, err
	}
	tx := scope.Tx()
	owned := scope.Owned()
	committed := false
	defer func() {
		if !committed {
			_ = scope.Rollback(ctx)
		}
	}()
	retried, err := s.taskLifecycle.TransitionForRetryInTx(ctx, tx, workspaceID, taskID, newToken)
	if err != nil {
		return nil, err
	}
	if err := s.queue.EnqueueInTx(ctx, tx, retried); err != nil {
		return nil, fmt.Errorf("workspacetasks: requeue: %w", err)
	}
	if err := scope.Commit(ctx); err != nil {
		return nil, err
	}
	committed = true
	// running -> pending is a real status transition, so it needs an event
	// like every other flip (this was the one settlement that staged
	// nothing: a requeued task went dark on the event channel because the
	// UI had no pending signal to re-render against). Staged AFTER the
	// commit, through the same owned/borrowed deferral every other flip
	// uses: a staged event means a committed fact, and publishing it is
	// the caller's post-commit job (never inside this transaction).
	s.stageSettlementEvent(ctx, owned, taskID, workspaceID, StatusPending, retried.ExecutionToken, retried.UpdatedAt)
	return retried, nil
}
