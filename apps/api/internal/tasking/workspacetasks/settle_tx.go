package workspacetasks

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"ideaboard/api/internal/db/dbutil"
	"ideaboard/api/internal/generation/routing"

	"github.com/jackc/pgx/v5"
)

// SettlementTransactor demarcates the settlement transaction. The postgres
// workspace task Store implements it over its pool. In-memory stores are
// test fixtures and are intentionally outside this production seam.
//
// The transaction is opened on the task store's pool, but every participant
// shares the same PostgreSQL database (tasks, credits ledger, billing
// catalog, river_job), so Credits InTx(tx), catalog QuoteAtResolutionInTx,
// and River InsertTx all join the ambient tx. The caller owns commit and
// rollback.
type SettlementTransactor interface {
	BeginSettlementTx(ctx context.Context) (pgx.Tx, error)
}

// TxQuoter mints the price snapshot inside the settlement transaction
// (repeatable-read snapshot consistency with the Hold that follows).
// Production adapts the Billing catalog QuoteAtResolutionInTx.
type TxQuoter interface {
	QuoteInTx(ctx context.Context, tx pgx.Tx, service, resolution string, quantity int64) (BillingSnapshot, error)
}

// TxHolder reserves and converges Ideas inside the settlement transaction.
// It keeps credits.Commands as the sole ledger writer: implementations must
// delegate to the transaction-scoped Commands (credits.Service.InTx) and
// perform no ledger SQL of their own.
//
// Unlike Holder, the userID is explicit: terminal settlement reads it from
// the flipped task row, so Capture/Release never depend on the
// process-local hold map (which a restart would lose).
type TxHolder interface {
	HoldInTx(ctx context.Context, tx pgx.Tx, userID int64, amount int64, idempotencyKey, taskID string) (string, error)
	CaptureInTx(ctx context.Context, tx pgx.Tx, userID int64, holdTransactionID string, amount int64, taskID string) (string, error)
	ReleaseInTx(ctx context.Context, tx pgx.Tx, userID int64, holdTransactionID string, amount int64, taskID string) (string, error)
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

// TaskBillingStore persists immutable billing facts and the settlement read
// model in the same transaction as task state and Credits.
type TaskBillingStore interface {
	CreateBillingFactsInTx(context.Context, pgx.Tx, *WorkspaceTask) error
	SetBillingSettlementInTx(context.Context, pgx.Tx, string, string, int64, int64, int64, string, string) error
}

// TxBillingFacts is retained as a descriptive alias for callers that refer to
// the transaction-only billing projection.
type TxBillingFacts = TaskBillingStore

func (s *Service) billingFacts() TxBillingFacts {
	if s == nil || s.taskBilling == nil {
		return nil
	}
	facts := s.taskBilling
	return facts
}

func (s *Service) settlementSeams() (SettlementTransactor, TxQuoter, TxHolder, TxTaskLifecycleStore, TxEnqueuer, TxSubmissionClaimStore, bool) {
	if s == nil || s.taskLifecycle == nil {
		return nil, nil, nil, nil, nil, nil, false
	}
	transactor, ok := s.taskLifecycle.(SettlementTransactor)
	if !ok {
		return nil, nil, nil, nil, nil, nil, false
	}
	txQuoter, ok := s.quoter.(TxQuoter)
	if !ok {
		return nil, nil, nil, nil, nil, nil, false
	}
	txHolder, ok := s.holder.(TxHolder)
	if !ok {
		return nil, nil, nil, nil, nil, nil, false
	}
	txStore, ok := s.taskLifecycle.(TxTaskLifecycleStore)
	if !ok {
		return nil, nil, nil, nil, nil, nil, false
	}
	txQueue, ok := s.queue.(TxEnqueuer)
	if !ok {
		return nil, nil, nil, nil, nil, nil, false
	}
	// The durable submit claim is discovered from the same lifecycle store
	// that owns the settlement transaction, for the same reason every other
	// seam above is: the claim must be written in THAT transaction, so it
	// cannot come from an independently wired component. A store that cannot
	// claim fails closed (no seam -> ErrSettlementUnavailable) rather than
	// settling an unfenced submit.
	txClaims := s.claimStore(txStore)
	if txClaims == nil {
		return nil, nil, nil, nil, nil, nil, false
	}
	return transactor, txQuoter, txHolder, txStore, txQueue, txClaims, true
}

// claimStore resolves the durable submit claim store: the explicit
// ServiceDeps override when set (tests inject a fake), otherwise the
// lifecycle store itself when it implements the seam (production: the
// postgres Store does, so no extra wiring is required).
func (s *Service) claimStore(txStore TxTaskLifecycleStore) TxSubmissionClaimStore {
	if s != nil && s.submissionClaims != nil {
		return s.submissionClaims
	}
	if claims, ok := txStore.(TxSubmissionClaimStore); ok {
		return claims
	}
	return nil
}

func (s *Service) terminalSeams() (SettlementTransactor, TxHolder, TxTaskLifecycleStore, bool) {
	if s == nil || s.taskLifecycle == nil || s.holder == nil {
		return nil, nil, nil, false
	}
	transactor, ok := s.taskLifecycle.(SettlementTransactor)
	if !ok {
		return nil, nil, nil, false
	}
	txHolder, ok := s.holder.(TxHolder)
	if !ok {
		return nil, nil, nil, false
	}
	txStore, ok := s.taskLifecycle.(TxTaskLifecycleStore)
	if !ok {
		return nil, nil, nil, false
	}
	return transactor, txHolder, txStore, true
}

func beginSettlementTx(ctx context.Context, transactor SettlementTransactor) (pgx.Tx, bool, error) {
	// Ambient guard transaction (control audit atomicity, step 3): when
	// the guard staged its mutation transaction in the context, the
	// whole settlement unit (claim/quote/hold/task/enqueue, or the
	// terminal capture/release flips) joins it INSTEAD of opening the
	// store's own settlement transaction. The bool reports ownership:
	// true means the caller opened the transaction and must commit or
	// roll it back; false means the transaction is BORROWED from the
	// guard, which alone commits or rolls it back, so the caller must do
	// neither. Nesting depth is 1: a staged carrier is reused as-is
	// (pgx has no true nested transactions and savepoints are not
	// needed, because settlement and audit share one flat unit of
	// commit). With no carrier the settlement transactor path below
	// runs unchanged.
	if ambient, ok := dbutil.AmbientTxFromContext(ctx); ok {
		return ambient, false, nil
	}
	if transactor == nil {
		return nil, false, errors.New("workspacetasks: settlement transactor is nil")
	}
	tx, err := transactor.BeginSettlementTx(ctx)
	if err != nil {
		return nil, false, fmt.Errorf("workspacetasks: begin settlement tx: %w", err)
	}
	if tx == nil {
		return nil, false, errors.New("workspacetasks: begin settlement tx returned nil")
	}
	return tx, true, nil
}

func rollbackSettlementTx(ctx context.Context, tx pgx.Tx) {
	if tx == nil {
		return
	}
	rollbackCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), dbutil.RollbackTimeout)
	defer cancel()
	_ = tx.Rollback(rollbackCtx)
}

func commitSettlementTx(ctx context.Context, tx pgx.Tx) error {
	if tx == nil {
		return errors.New("workspacetasks: commit settlement tx is nil")
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("workspacetasks: commit settlement tx: %w", err)
	}
	return nil
}

// captureSplit mirrors the compensating path: per-output unit billing capped
// at the held total, remainder released. Both legs run in the same tx.
func captureSplit(completed *WorkspaceTask, outputCount int) (capture, release int64) {
	total := completed.BillingSnapshot.TotalCredits
	unit := completed.BillingSnapshot.UnitCredits
	capture = int64(outputCount) * unit
	if capture > total {
		capture = total
	}
	if capture < 0 {
		capture = 0
	}
	release = total - capture
	if release < 0 {
		release = 0
	}
	return capture, release
}

func (s *Service) holderSettles(holdID string, total int64) bool {
	return s.holder != nil && strings.TrimSpace(holdID) != "" && total > 0
}

// createInTx settles Claim -> Quote -> Hold -> Insert -> Enqueue in ONE
// PostgreSQL transaction. Any failure rolls the whole transaction back: no
// task row without its Hold, no Hold without its task, no River job without
// task facts, and no durable idempotency claim without all of them.
//
// The claim is written FIRST, before Quote and before the Hold. That
// ordering is the fix: a retried submit now collides on the claim's unique
// constraint and returns the original task, so it never reaches the ledger,
// the task table, or River. It reports handled=false for test-only service
// fixtures that do not expose PostgreSQL transaction seams.
func (s *Service) createInTx(ctx context.Context, input CreateInput, kind, resolution string, quantity int64, taskID, token string, claim SubmissionClaim) (*WorkspaceTask, bool, error) {
	transactor, txQuoter, txHolder, txStore, txQueue, txClaims, ok := s.settlementSeams()
	if !ok {
		return nil, false, nil
	}
	if s.saveSnapshot == nil {
		return nil, true, errors.New("workspacetasks: routing snapshot saver is required for atomic settlement")
	}
	tx, owned, err := beginSettlementTx(ctx, transactor)
	if err != nil {
		return nil, true, err
	}
	committed := false
	defer func() {
		if !committed && owned {
			rollbackSettlementTx(ctx, tx)
		}
	}()
	// Claim the client key inside the settlement transaction. This is the
	// concurrency fence: a concurrent submit of the same key blocks on the
	// unique constraint until this transaction commits or rolls back, then
	// observes the winner and replays it.
	existing, claimed, err := txClaims.ClaimSubmissionInTx(ctx, tx, claim)
	if err != nil {
		return nil, true, fmt.Errorf("workspacetasks: claim submission: %w", err)
	}
	if !claimed {
		replayed, err := s.replayClaimedSubmission(ctx, tx, txStore, existing, claim)
		if err != nil {
			return nil, true, err
		}
		// Owned read-only transaction: commit so the replayed task is
		// read from a consistent snapshot and no write is left open. A
		// borrowed guard transaction stays open for the guard to commit.
		if owned {
			if err := commitSettlementTx(ctx, tx); err != nil {
				return nil, true, err
			}
			committed = true
		}
		// No event is staged on replay: the winning submit already emitted
		// task.created for this exact task.
		return replayed, true, nil
	}
	// Resolve inside the settlement transaction before Quote/Hold so
	// an empty pool fails before any ledger, task, or queue write (the
	// rollback below then removes nothing because nothing was written).
	// The snapshot pins atomically with the rows that follow. A nil
	// resolver fails closed with the canonical unavailable sentinel.
	if s.resolver == nil {
		return nil, true, &routing.ResolutionUnavailableError{Service: kind, Resolution: routing.Resolution(resolution)}
	}
	routingSnap, err := s.resolver.ResolveInTx(ctx, tx, kind, resolution)
	if err != nil {
		return nil, true, err
	}
	snapshot, err := txQuoter.QuoteInTx(ctx, tx, kind, resolution, quantity)
	if err != nil {
		return nil, true, err
	}
	snapshot.SettlementState = "quoted"
	var holdTxID string
	if snapshot.TotalCredits > 0 {
		// The ledger key is derived from the DURABLE claim scope, never from
		// the freshly minted task id. A retried submit therefore asks the
		// ledger for the same key, so the ledger's own replay path fences a
		// duplicate hold even if the claim were somehow bypassed.
		holdTxID, err = txHolder.HoldInTx(ctx, tx, input.OwnerUserID, snapshot.TotalCredits,
			TaskSubmitHoldKey(claim.WorkspaceID, claim.IdempotencyKey), taskID)
		if err != nil {
			return nil, true, err
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
	created, err := txStore.CreateTaskInTx(ctx, tx, task)
	if err != nil {
		return nil, true, err
	}
	// Pin the snapshot in the same tx so it commits atomically with
	// Hold/task/enqueue. Later Admin pool edits never invalidate this
	// accepted task (the worker replays selection from the pinned fact,
	// never the live pool).
	if err := s.saveSnapshot(ctx, tx, created.ID, routingSnap); err != nil {
		return nil, true, fmt.Errorf("workspacetasks: pin routing snapshot: %w", err)
	}
	if facts := s.billingFacts(); facts != nil {
		if err := facts.CreateBillingFactsInTx(ctx, tx, created); err != nil {
			return nil, true, fmt.Errorf("workspacetasks: persist billing facts: %w", err)
		}
	}
	if err := txQueue.EnqueueInTx(ctx, tx, created); err != nil {
		return nil, true, fmt.Errorf("workspacetasks: enqueue: %w", err)
	}
	// The settlement unit is durable once the owned transaction commits;
	// a borrowed guard transaction commits later at the guard. The event
	// stages after the successful unit either way (never inside a failed
	// one), and the caller flushes it only after the commit it can see.
	if owned {
		if err := commitSettlementTx(ctx, tx); err != nil {
			return nil, true, err
		}
		committed = true
	}
	s.stage(created.ID, created.WorkspaceID, StatusPending)
	return created, true, nil
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
func (s *Service) completeInTx(ctx context.Context, workspaceID, taskID string, outputs []OutputRef) (*WorkspaceTask, bool, error) {
	transactor, txHolder, txStore, ok := s.terminalSeams()
	if !ok {
		return nil, false, nil
	}
	tx, owned, err := beginSettlementTx(ctx, transactor)
	if err != nil {
		return nil, true, err
	}
	committed := false
	defer func() {
		if !committed && owned {
			rollbackSettlementTx(ctx, tx)
		}
	}()
	completed, err := txStore.CompleteTaskInTx(ctx, tx, workspaceID, taskID, outputs)
	if err != nil {
		return nil, true, err
	}
	if s.holderSettles(completed.BillingSnapshot.HoldTransactionID, completed.BillingSnapshot.TotalCredits) {
		capture, release := captureSplit(completed, len(outputs))
		var captureTxID, releaseTxID string
		if capture > 0 {
			var captureErr error
			captureTxID, captureErr = txHolder.CaptureInTx(ctx, tx, completed.OwnerUserID, completed.BillingSnapshot.HoldTransactionID, capture, taskID)
			if captureErr != nil {
				return nil, true, fmt.Errorf("workspacetasks: capture on complete: %w", captureErr)
			}
		}
		if release > 0 {
			var releaseErr error
			releaseTxID, releaseErr = txHolder.ReleaseInTx(ctx, tx, completed.OwnerUserID, completed.BillingSnapshot.HoldTransactionID, release, taskID)
			if releaseErr != nil {
				return nil, true, fmt.Errorf("workspacetasks: release remainder on complete: %w", releaseErr)
			}
		}
		if capture > 0 {
			completed.BillingSnapshot.SettlementState = "captured"
		} else if release > 0 {
			completed.BillingSnapshot.SettlementState = "released"
		}
		if facts := s.billingFacts(); facts != nil && completed.BillingSnapshot.SettlementState != "free" {
			if err := facts.SetBillingSettlementInTx(ctx, tx, taskID, completed.BillingSnapshot.SettlementState, int64(len(outputs)), capture, release, captureTxID, releaseTxID); err != nil {
				return nil, true, fmt.Errorf("workspacetasks: persist billing settlement: %w", err)
			}
		}
	}
	if owned {
		if err := commitSettlementTx(ctx, tx); err != nil {
			return nil, true, err
		}
		committed = true
	}
	s.stage(taskID, workspaceID, StatusCompleted)
	return completed, true, nil
}

// failInTx flips to failed atomically with Release in ONE transaction.
func (s *Service) failInTx(ctx context.Context, workspaceID, taskID, errorMsg string) (*WorkspaceTask, bool, error) {
	transactor, txHolder, txStore, ok := s.terminalSeams()
	if !ok {
		return nil, false, nil
	}
	tx, owned, err := beginSettlementTx(ctx, transactor)
	if err != nil {
		return nil, true, err
	}
	committed := false
	defer func() {
		if !committed && owned {
			rollbackSettlementTx(ctx, tx)
		}
	}()
	failed, err := txStore.FailTaskInTx(ctx, tx, workspaceID, taskID, errorMsg)
	if err != nil {
		return nil, true, err
	}
	if s.holderSettles(failed.BillingSnapshot.HoldTransactionID, failed.BillingSnapshot.TotalCredits) {
		releaseTxID, releaseErr := txHolder.ReleaseInTx(ctx, tx, failed.OwnerUserID, failed.BillingSnapshot.HoldTransactionID, failed.BillingSnapshot.TotalCredits, taskID)
		if releaseErr != nil {
			return nil, true, fmt.Errorf("workspacetasks: release on fail: %w", releaseErr)
		}
		failed.BillingSnapshot.SettlementState = "released"
		if facts := s.billingFacts(); facts != nil {
			if err := facts.SetBillingSettlementInTx(ctx, tx, taskID, failed.BillingSnapshot.SettlementState, 0, 0, failed.BillingSnapshot.TotalCredits, "", releaseTxID); err != nil {
				return nil, true, fmt.Errorf("workspacetasks: persist billing settlement: %w", err)
			}
		}
	}
	if owned {
		if err := commitSettlementTx(ctx, tx); err != nil {
			return nil, true, err
		}
		committed = true
	}
	s.stage(taskID, workspaceID, StatusFailed)
	return failed, true, nil
}

// cancelInTx flips to cancelled atomically with Release in ONE transaction.
// An already-terminal row replays idempotently without billing or events,
// matching the compensating path.
func (s *Service) cancelInTx(ctx context.Context, workspaceID, taskID string, actorUserID int64, reason string) (*WorkspaceTask, bool, bool, error) {
	transactor, txHolder, txStore, ok := s.terminalSeams()
	if !ok {
		return nil, false, false, nil
	}
	tx, owned, err := beginSettlementTx(ctx, transactor)
	if err != nil {
		return nil, false, true, err
	}
	committed := false
	defer func() {
		if !committed && owned {
			rollbackSettlementTx(ctx, tx)
		}
	}()
	flipped, already, err := txStore.CancelTaskInTx(ctx, tx, workspaceID, taskID, actorUserID, reason)
	if err != nil {
		return nil, false, true, err
	}
	if already {
		return flipped, true, true, nil
	}
	if s.holderSettles(flipped.BillingSnapshot.HoldTransactionID, flipped.BillingSnapshot.TotalCredits) {
		releaseTxID, releaseErr := txHolder.ReleaseInTx(ctx, tx, flipped.OwnerUserID, flipped.BillingSnapshot.HoldTransactionID, flipped.BillingSnapshot.TotalCredits, taskID)
		if releaseErr != nil {
			return nil, false, true, fmt.Errorf("workspacetasks: release on cancel: %w", releaseErr)
		}
		flipped.BillingSnapshot.SettlementState = "released"
		if facts := s.billingFacts(); facts != nil {
			if err := facts.SetBillingSettlementInTx(ctx, tx, taskID, flipped.BillingSnapshot.SettlementState, 0, 0, flipped.BillingSnapshot.TotalCredits, "", releaseTxID); err != nil {
				return nil, false, true, fmt.Errorf("workspacetasks: persist billing settlement: %w", err)
			}
		}
	}
	if owned {
		if err := commitSettlementTx(ctx, tx); err != nil {
			return nil, false, true, err
		}
		committed = true
	}
	s.stage(taskID, workspaceID, StatusCancelled)
	return flipped, false, true, nil
}

// retryInTx consumes one retry budget entry (reset + new execution token)
// atomically with the River requeue in ONE transaction, so a requeue failure
// cannot strand a token-bumped pending task without its job.
func (s *Service) retryInTx(ctx context.Context, workspaceID, taskID, newToken string) (*WorkspaceTask, bool, error) {
	if s == nil || s.taskLifecycle == nil || s.queue == nil {
		return nil, false, nil
	}
	transactor, ok := s.taskLifecycle.(SettlementTransactor)
	if !ok {
		return nil, false, nil
	}
	txStore, ok := s.taskLifecycle.(TxTaskLifecycleStore)
	if !ok {
		return nil, false, nil
	}
	txQueue, ok := s.queue.(TxEnqueuer)
	if !ok {
		return nil, false, nil
	}
	tx, owned, err := beginSettlementTx(ctx, transactor)
	if err != nil {
		return nil, true, err
	}
	committed := false
	defer func() {
		if !committed && owned {
			rollbackSettlementTx(ctx, tx)
		}
	}()
	retried, err := txStore.TransitionForRetryInTx(ctx, tx, workspaceID, taskID, newToken)
	if err != nil {
		return nil, true, err
	}
	if err := txQueue.EnqueueInTx(ctx, tx, retried); err != nil {
		return nil, true, fmt.Errorf("workspacetasks: requeue: %w", err)
	}
	if owned {
		if err := commitSettlementTx(ctx, tx); err != nil {
			return nil, true, err
		}
		committed = true
	}
	// running -> pending is a real status transition, so it needs an event
	// like every other flip (this was the one settlement that staged
	// nothing: a requeued task went dark on the event channel because the
	// UI had no pending signal to re-render against). Staged AFTER the
	// commit, for the same reason as the terminal settlements above: a
	// staged event means a committed fact, and publishing it is the
	// caller's post-commit job (never inside this transaction).
	s.stage(taskID, workspaceID, StatusPending)
	return retried, true, nil
}
