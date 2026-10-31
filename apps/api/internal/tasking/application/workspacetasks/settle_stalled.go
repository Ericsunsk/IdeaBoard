package workspacetasks

// Final-timeout settlement convergence (the "final timeout" leg of the
// DESIGN.md contract: "success converges with Capture atomically; failure,
// cancellation, and FINAL TIMEOUT converge with Release atomically").
//
// The gap this file closes: the three terminal paths that a live worker owns
// (Complete -> Capture, Fail -> Release, Cancel -> Release) all commit the
// task flip and the Release in ONE transaction, but nothing owned the
// case where NO worker ever gets there. A worker can be SIGKILLed after its
// claim commits and after River exhausts MaxAttempts and discards the job;
// the task row then sits at status='running' with a Credit Hold that nothing
// will ever release. The Credits stale-hold reconciler does NOT converge that
// leak (execute_credit_reconciliation only INSERTs a 'stale_hold' finding), so
// before this sweep the user's Ideas were locked permanently.
//
// ConvergeStalledSettlements is the periodic convergence: discover live tasks
// whose hold was abandoned past the threshold, then fail each one through the
// SAME terminal seam the worker uses, so the task flip, the Release, the
// settlement projection, and the staged status event all behave exactly as
// they do on an ordinary worker failure.
//
// NOTE (Fix-6): moved from
// apps/api/internal/tasking/workspacetasks/settle_stalled.go (ffce04c) to
// this application/workspacetasks location during the tasking DDD split; the
// StalledSettlement / Scanner / Failer shapes now live in domain and
// ports/tasks and are aliased below. This header and the threshold rationale
// are restored verbatim to keep the audit chain intact; behavior code below
// is unchanged.

import (
	"context"
	"errors"
	"fmt"
	"time"

	tasks "ideaboard/api/internal/tasking/domain"
	taskports "ideaboard/api/internal/tasking/ports/tasks"
)

const (
	// StalledSettlementThreshold is how long a live task may hold its Credit
	// Hold with no worker touching it before the sweep may converge it.
	//
	// Why 24 hours cannot kill a healthy task: tasks.updated_at is the LIVENESS
	// clock, not the submission clock. The set_tasks_updated_at BEFORE UPDATE
	// trigger stamps now() on every write, so no writer can forge a fresh
	// stamp for an abandoned lease — and every reclaim path DOES write it: a
	// River redelivery of the SAME job carries the SAME execution token, which
	// hits the running+same-token branch of ClaimWorkspaceTask, and Retry
	// re-arms pending with a fresh write. So "updated_at older than 24h" means
	// no worker has held this lease in a full day, not "this task is slow".
	//
	// The margin over the longest possible healthy execution is enormous: one
	// River attempt is capped at RiverJobTimeoutDrawing (15min provider budget
	// + 2min overhead), and even a task burning every attempt of its queue
	// budget refreshes updated_at on each reclaim. 24h is ~84x a single
	// attempt. It is deliberately NOT minute-scale: a minute-scale bound on a
	// 17-minute task would release credits for work that later succeeds, which
	// is a real money error, not a bookkeeping nit.
	//
	// 24h also matches the Credits stale-hold window
	// (ck_credit_reconciliation_run_stale_hold_fixed pins stale_hold_after to
	// 24 hours), so the auditing signal and the converging action cross at the
	// same threshold instead of leaving a window where one has fired and the
	// other has not.
	StalledSettlementThreshold = 24 * time.Hour

	// StalledSettlementSweepBatch bounds one sweep run. The sweep converges
	// oldest-first, so a bound never starves the longest-abandoned holds; it
	// only caps how much one River attempt may do (and therefore how long one
	// attempt's transaction chain can run).
	StalledSettlementSweepBatch = 100

	// StalledSettlementFailureMessage is the terminal error recorded on a
	// swept task. It names the cause rather than the mechanism so an operator
	// reading the task row knows the hold was returned and why the task died.
	StalledSettlementFailureMessage = "execution lease abandoned past the final-timeout threshold; credit hold released"
)

// StalledSettlement is one discovered (workspace, task) pair awaiting
// convergence. Both identifiers are workspace PUBLIC ids because that is the
// scope every terminal write is fenced with: a pair the store cannot resolve
// converges nothing.
type StalledSettlement = tasks.StalledSettlement

// StalledSettlementScanner is the read-only discovery seam. The postgres store
// implements it over ListStalledHeldWorkspaceTasks; tests implement it over a
// fixed slice. It must never write and must never return a row whose
// workspace or task it could not resolve.
type StalledSettlementScanner = taskports.StalledSettlementScanner

// StalledSettlementFailer is the terminal settlement seam the sweep converges
// through. It is deliberately the SAME interface the workspace worker settles
// through (workspacetasks.Service, wrapped by the post-commit event bridge in
// composition), so a swept task takes the identical single-transaction path —
// task -> failed + hold Release + settlement projection — and emits its
// task.failed event through the identical post-commit bridge.
//
// Fail is workspace-scoped by contract, which is what makes the sweep safe to
// run cross-workspace: a (workspace, task) pair that does not belong together
// matches zero rows inside the settle transaction and releases nothing.
type StalledSettlementFailer = taskports.StalledSettlementFailer

// StalledConvergenceFailure is one task the sweep could NOT converge. The
// settle transaction rolled back, so nothing about that task changed: no task
// flip, no Release, no settlement projection.
type StalledConvergenceFailure struct {
	Task StalledSettlement
	Err  error
}

func (f StalledConvergenceFailure) Error() string {
	return fmt.Sprintf("stalled settlement %s/%s: %v", f.Task.WorkspaceID, f.Task.TaskID, f.Err)
}

func (f StalledConvergenceFailure) Unwrap() error { return f.Err }

// StalledConvergence reports one sweep run.
//
//	Scanned   rows the scanner returned
//	Converged rows flipped to failed with their hold released
//	Skipped   rows that were already terminal (or not visible in their
//	          workspace) when the sweep reached them — the normal race with a
//	          worker or an operator cancel that won first
//	Failures  rows whose settle transaction rolled back for a real reason
type StalledConvergence struct {
	Scanned   int
	Converged int
	Skipped   int
	Failures  []StalledConvergenceFailure
}

// StalledConvergenceDeps wires one sweep run.
type StalledConvergenceDeps struct {
	// Scanner discovers candidates. Required.
	Scanner StalledSettlementScanner
	// Failer converges them. Required.
	Failer StalledSettlementFailer
	// StalledBefore is the liveness cut: only rows whose last worker touch is
	// older than this are candidates. Callers pass
	// time.Now().Add(-StalledSettlementThreshold) so the policy constant stays
	// the single source of truth while the sweep stays clock-injectable.
	StalledBefore time.Time
	// Limit bounds one run; <= 0 fails closed (no unbounded scan).
	Limit int
}

// ConvergeStalledSettlements runs one bounded final-timeout sweep.
//
// Fail-closed and idempotent, per task:
//
//   - Discovery only ever NAMES candidates; every mutation goes through the
//     workspace-scoped Failer.
//   - Fail opens one transaction: FailTaskInTx (task -> failed, fenced to
//     pending/running) then ReleaseInTx, then the settlement projection, then
//     commit. Any error rolls all three back, so a failed attempt leaves the
//     hold exactly as it was.
//   - Repeating the sweep cannot double-release. Three independent fences: the
//     task UPDATE matches zero rows once the task is terminal (no Release
//     runs); the settlement UPDATE requires exactly one live row; and the
//     ledger itself rejects a second Release of a settled hold while the
//     "task-release:<task_id>" idempotency key replays the original command.
func ConvergeStalledSettlements(ctx context.Context, deps StalledConvergenceDeps) (StalledConvergence, error) {
	if deps.Scanner == nil {
		return StalledConvergence{}, errors.New("workspacetasks: stalled settlement scanner is required")
	}
	if deps.Failer == nil {
		return StalledConvergence{}, errors.New("workspacetasks: stalled settlement failer is required")
	}
	if deps.Limit <= 0 {
		return StalledConvergence{}, fmt.Errorf("workspacetasks: stalled settlement limit must be positive: %w", tasks.ErrValidation)
	}
	if deps.StalledBefore.IsZero() {
		return StalledConvergence{}, fmt.Errorf("workspacetasks: stalled settlement cutoff required: %w", tasks.ErrValidation)
	}
	candidates, err := deps.Scanner.ListStalledHeldSettlements(ctx, deps.StalledBefore, deps.Limit)
	if err != nil {
		return StalledConvergence{}, fmt.Errorf("workspacetasks: discover stalled settlements: %w", err)
	}
	report := StalledConvergence{Scanned: len(candidates)}
	for _, candidate := range candidates {
		if err := ctx.Err(); err != nil {
			// The caller cancelled mid-batch: every task already attempted
			// has committed or rolled back on its own, so returning here
			// loses nothing and releases nothing twice.
			report.Failures = append(report.Failures, StalledConvergenceFailure{Task: candidate, Err: err})
			return report, err
		}
		_, failErr := deps.Failer.Fail(ctx, candidate.WorkspaceID, candidate.TaskID, StalledSettlementFailureMessage)
		switch {
		case failErr == nil:
			report.Converged++
		case errors.Is(failErr, tasks.ErrStateConflict), errors.Is(failErr, tasks.ErrNotFound):
			// Someone else converged this task first (worker terminal settle,
			// operator cancel, or a prior sweep run): its hold is already
			// settled, so there is nothing left to release.
			report.Skipped++
		default:
			report.Failures = append(report.Failures, StalledConvergenceFailure{Task: candidate, Err: failErr})
		}
	}
	return report, nil
}
