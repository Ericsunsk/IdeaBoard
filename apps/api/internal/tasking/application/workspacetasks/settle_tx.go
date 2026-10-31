package workspacetasks

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	creditsdomain "ideaboard/api/internal/credits/domain"
	"ideaboard/api/internal/events"
	"ideaboard/api/internal/generation/routing"
	tasks "ideaboard/api/internal/tasking/domain"
	taskports "ideaboard/api/internal/tasking/ports/tasks"
	"ideaboard/api/internal/tasking/settlement"

	"github.com/jackc/pgx/v5"
)

// mapTerminalLedgerError converges a ledger rejection after the fact
// already succeeded to the accurate caller semantic: the terminal fact is
// durable, so the task must report a state conflict (409), never a
// retryable 503 that would roll the settlement back and re-enter the
// provider.
func mapTerminalLedgerError(taskID, leg string, err error) error {
	if !settlement.IsTerminalLedgerConflict(err) {
		return fmt.Errorf("workspacetasks: %s: %w", leg, err)
	}
	if errors.Is(err, creditsdomain.ErrHoldSettled) {
		return fmt.Errorf("workspacetasks: %s on task %s already settled (terminal fact durable): %w: %w", leg, taskID, tasks.ErrStateConflict, err)
	}
	return fmt.Errorf("workspacetasks: %s on task %s conflicts with the durable ledger fact: %w: %w", leg, taskID, tasks.ErrStateConflict, err)
}

func (s *Service) stageSettlementEvent(ctx context.Context, owned bool, taskID, workspaceID string, status tasks.Status, executionToken string, occurredAt time.Time, outputs []events.V1TaskCompletedOutput) {
	settlement.StageOrDefer(ctx, owned, func(context.Context) {
		s.stageWithTokenAt(taskID, workspaceID, status, executionToken, occurredAt, outputs)
	})
}

func captureSplit(completed *tasks.WorkspaceTask, outputCount int) (capture, release int64) {
	return settlement.CaptureSplit(completed.BillingSnapshot.TotalCredits, completed.BillingSnapshot.UnitCredits, outputCount)
}

func (s *Service) holderSettles(holdID string, total int64) bool {
	return s.holder != nil && strings.TrimSpace(holdID) != "" && total > 0
}

func (s *Service) settleFreeTerminal(ctx context.Context, tx pgx.Tx, task *tasks.WorkspaceTask, state string, outputCount int64) error {
	if task == nil {
		return fmt.Errorf("workspacetasks: free terminal settlement requires a task: %w", tasks.ErrSettlementUnavailable)
	}
	if task.BillingSnapshot.TotalCredits > 0 || strings.TrimSpace(task.BillingSnapshot.HoldTransactionID) != "" {
		return fmt.Errorf("workspacetasks: terminal settlement without hold: %w", tasks.ErrSettlementUnavailable)
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

func (s *Service) releaseTerminalInTx(ctx context.Context, tx pgx.Tx, task *tasks.WorkspaceTask) error {
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

func (s *Service) createInTx(ctx context.Context, input tasks.CreateInput, kind, resolution string, quantity int64, taskID, token string, claim tasks.SubmissionClaim) (*tasks.WorkspaceTask, error) {
	if s == nil || s.taskLifecycle == nil || s.quoter == nil || s.holder == nil || s.queue == nil || s.submissionClaims == nil || s.billingAccounts == nil {
		return nil, fmt.Errorf("workspacetasks: create settlement seams are not configured (caller must retry): %w", tasks.ErrSettlementUnavailable)
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
	if s.acceptance == nil {
		return nil, tasks.ErrSettlementUnavailable
	}
	binding, err := s.acceptance.AuthorizeSubmission(ctx, tx, input)
	if err != nil {
		return nil, err
	}
	existing, claimed, err := s.submissionClaims.ClaimSubmissionInTx(ctx, tx, claim)
	if err != nil {
		return nil, fmt.Errorf("workspacetasks: claim submission: %w", err)
	}
	if !claimed {
		replayed, err := s.replayClaimedSubmission(ctx, tx, s.taskLifecycle, existing, claim)
		if err != nil {
			return nil, err
		}
		if err := scope.Commit(ctx); err != nil {
			return nil, err
		}
		committed = true
		return replayed, nil
	}
	billingAccountID, err := s.billingAccounts.ResolveBillingAccountInTx(ctx, tx, claim.WorkspaceID)
	if err != nil {
		return nil, err
	}
	if s.resolver == nil {
		return nil, &routing.ResolutionUnavailableError{Service: kind, Resolution: routing.Resolution(resolution)}
	}
	routingSnap, err := s.resolver.ResolveInTx(ctx, tx, kind, resolution)
	if err != nil {
		var inputErr *routing.InvalidRoutingInputError
		if errors.As(err, &inputErr) {
			return nil, tasks.NewValidationError(tasks.FieldError{Field: inputErr.Field, Message: "invalid"})
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
	if err := scope.Commit(ctx); err != nil {
		return nil, err
	}
	committed = true
	s.stageSettlementEvent(ctx, owned, created.ID, created.WorkspaceID, tasks.StatusPending, created.ExecutionToken, created.UpdatedAt, nil)
	return created, nil
}

func (s *Service) replayClaimedSubmission(ctx context.Context, tx pgx.Tx, txStore taskports.TxTaskLifecycleStore, existing *tasks.SubmissionClaim, attempted tasks.SubmissionClaim) (*tasks.WorkspaceTask, error) {
	if existing == nil {
		return nil, fmt.Errorf("workspacetasks: claim store reported a conflict without a claim: %w", tasks.ErrSettlementUnavailable)
	}
	if existing.WorkspaceID != attempted.WorkspaceID || existing.Operation != attempted.Operation ||
		existing.IdempotencyKey != attempted.IdempotencyKey {
		return nil, fmt.Errorf("workspacetasks: claim scope mismatch: %w", tasks.ErrSettlementUnavailable)
	}
	if existing.RequestHash != attempted.RequestHash {
		return nil, tasks.ErrIdempotencyConflict
	}
	taskID := strings.TrimSpace(existing.TaskPublicID)
	if taskID == "" {
		return nil, fmt.Errorf("workspacetasks: claim names no task: %w", tasks.ErrSettlementUnavailable)
	}
	replayed, err := txStore.GetTaskInTx(ctx, tx, existing.WorkspaceID, taskID)
	if err != nil {
		return nil, fmt.Errorf("workspacetasks: replay claimed task %s: %w", taskID, err)
	}
	return replayed, nil
}

func (s *Service) completeInTx(ctx context.Context, workspaceID, taskID string, outputs []tasks.OutputRef) (*tasks.WorkspaceTask, error) {
	if s == nil || s.taskLifecycle == nil || s.holder == nil {
		return nil, fmt.Errorf("workspacetasks: terminal settlement seams are not configured (caller must retry): %w", tasks.ErrSettlementUnavailable)
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
	completedOutputs := tasks.BuildTaskCompletedOutputs(workspaceID, outputs)
	s.stageSettlementEvent(ctx, owned, taskID, workspaceID, tasks.StatusCompleted, completed.ExecutionToken, completed.UpdatedAt, completedOutputs)
	return completed, nil
}

func (s *Service) failInTx(ctx context.Context, workspaceID, taskID, errorMsg string) (*tasks.WorkspaceTask, error) {
	if s == nil || s.taskLifecycle == nil || s.holder == nil {
		return nil, fmt.Errorf("workspacetasks: terminal settlement seams are not configured (caller must retry): %w", tasks.ErrSettlementUnavailable)
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
	s.stageSettlementEvent(ctx, owned, taskID, workspaceID, tasks.StatusFailed, failed.ExecutionToken, failed.UpdatedAt, nil)
	return failed, nil
}

func (s *Service) cancelInTx(ctx context.Context, workspaceID, taskID string, actorUserID int64, reason string) (*tasks.WorkspaceTask, bool, error) {
	if s == nil || s.taskLifecycle == nil || s.holder == nil {
		return nil, false, fmt.Errorf("workspacetasks: terminal settlement seams are not configured (caller must retry): %w", tasks.ErrSettlementUnavailable)
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
		s.stageWithTokenAt(taskID, workspaceID, tasks.StatusCancelled, flipped.ExecutionToken, flipped.UpdatedAt, nil)
		return flipped, false, nil
	}
	s.stageSettlementEvent(ctx, owned, taskID, workspaceID, tasks.StatusCancelled, flipped.ExecutionToken, flipped.UpdatedAt, nil)
	return flipped, false, nil
}

func (s *Service) retryInTx(ctx context.Context, workspaceID, taskID, newToken string) (*tasks.WorkspaceTask, error) {
	if s == nil || s.taskLifecycle == nil || s.queue == nil {
		return nil, fmt.Errorf("workspacetasks: retry settlement seams are not configured (caller must retry): %w", tasks.ErrSettlementUnavailable)
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
	s.stageSettlementEvent(ctx, owned, taskID, workspaceID, tasks.StatusPending, retried.ExecutionToken, retried.UpdatedAt, nil)
	return retried, nil
}
