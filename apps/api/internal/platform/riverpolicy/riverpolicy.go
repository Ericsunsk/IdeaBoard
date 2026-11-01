// Package riverpolicy owns the process-wide River retry/timeout policy.
//
// It is a pure policy module: queue names, per-queue MaxAttempts budgets,
// and per-worker Timeout budgets plus the two matrices that list them.
// It has no database, River client, or business-context dependencies so
// Identity, Payments, Credits, Media, Runtime, and Tasking workers can all
// consume one fact source without importing Tasking concrete queue
// implementation.
//
// Tasking keeps ownership of the enqueue/runtime store seam
// (internal/tasking/infrastructure/queue.RiverQueue and the workspace
// enqueuer); provider-level requeue semantics stay with
// internal/tasking/domain (MaxProviderRequeues). This package never
// becomes a generic queue framework: no client wrapper, no enqueue helper,
// only typed policy facts.
package riverpolicy

import (
	"strings"
	"time"

	"ideaboard/api/internal/platform/accountlimits"
)

// River queue names. Every production River queue has exactly one canonical
// name here; worker packages alias these constants instead of repeating
// string literals so a rename lands in one place and the RED metric label
// set (ideaboard_river_queue_depth{queue}) stays accurate.
const (
	RiverQueueDefault = "drawing_default"
	// Workspace invitation mail is delivered through Auth after transactional enqueue.
	RiverQueueWorkspaceInvitationMail = "workspace_invitation_mail"
	// RiverQueueWebhookDeliver carries signed webhook deliveries
	// (issue #833). The events package owns the worker; this package
	// owns the queue name and its budget constants.
	RiverQueueWebhookDeliver = "webhook_deliver"
	// RiverQueueTaskSettlementSweep carries the final-timeout convergence
	// sweep (abandoned Credit Holds). The tasking worker package owns the
	// worker; this package owns the queue name and its budget constants.
	RiverQueueTaskSettlementSweep = "task_settlement_sweep"
	// RiverQueueCreditReconciliation carries the Credits reconciliation runs.
	RiverQueueCreditReconciliation = "credit_reconciliation"
	// RiverQueuePaymentReconciliation carries the Stripe order reconciliation runs.
	RiverQueuePaymentReconciliation = "payment_reconciliation"
	// RiverQueuePaymentCheckoutShutdown carries checkout session shutdown runs.
	RiverQueuePaymentCheckoutShutdown = "payment_checkout_shutdown"
	// RiverQueueVisionBilling carries vision-search billing reconciliation runs.
	RiverQueueVisionBilling = "vision_billing"
	// RiverQueueMediaReclamation carries media storage consistency scans.
	RiverQueueMediaReclamation = "media_reclamation"
	// RiverQueueIdentityAdminReconciliation carries identity drift scans.
	RiverQueueIdentityAdminReconciliation = "identity_admin_reconciliation"
)

// River retry budgets (issue #772). These constants are the single source
// of truth for per-queue MaxAttempts. Insert sites in the worker packages
// must reference them instead of repeating literals, so a tuning change
// lands in one place and the runbook table
// (docs/runbooks/backend-metrics.md) stays accurate.
//
// Backoff between attempts is River's default exponential backoff with
// jitter; the drawing queue additionally layers worker-level policy on top:
// transient provider failures (429/5xx/timeout/network after gateway-internal
// retries) requeue at most twice per task (tasks.retry_count bound) before
// converging terminally. Deterministic failures never retry.
const (
	RiverMaxAttemptsDefault                     = 25
	RiverMaxAttemptsWorkspaceInvitationMail     = 8
	RiverMaxAttemptsCreditReconciliation        = 5
	RiverMaxAttemptsPaymentReconciliation       = 8
	RiverMaxAttemptsPaymentCheckoutShutdown     = 8
	RiverMaxAttemptsVisionBilling               = 5
	RiverMaxAttemptsMediaReclamation            = 10
	RiverMaxAttemptsWebhookDeliver              = 8
	RiverMaxAttemptsIdentityAdminReconciliation = 5
	// RiverMaxAttemptsTaskSettlementSweep budgets the final-timeout
	// convergence sweep. A poisoned sweep (a task whose ledger row is
	// un-releasable, a database blip) must surface as a failed River job an
	// operator can see instead of churning for 25 attempts: the sweep is
	// idempotent, so the next scheduled run converges the same batch anyway.
	RiverMaxAttemptsTaskSettlementSweep = 5
)

// River timeout budgets (issue #783). These constants are the single source
// of truth for per-worker River Timeout. Every worker's Timeout() method
// must reference them instead of repeating literals, so no worker silently
// falls back to the client-level JobTimeout and the runbook table
// (docs/runbooks/backend-metrics.md) stays
// accurate.
//
// River semantics: a worker without an explicit Timeout() inherits the
// client JobTimeout. That inheritance is the bug this matrix closes: the
// drawing worker pins RiverJobTimeoutDrawing for its outer job context,
// then bounds provider execution separately with WorkspaceExecutionBudget.
const (
	// RiverJobTimeoutDrawing owns the global budget: the longest supported
	// task execution (accountlimits.MaxTimeoutSeconds) plus worker overhead.
	// The drawing worker and the River client default share this outer bound.
	RiverJobTimeoutDrawing = time.Duration(accountlimits.MaxTimeoutSeconds)*time.Second + 2*time.Minute
	// RiverJobTimeoutFinalFailure covers settlement-only convergence: the
	// orchestrator caps a converge attempt at 15s internally, so 1 minute
	// leaves ample margin for the fallback enqueue path. Never calls a provider.
	RiverJobTimeoutFinalFailure            = time.Minute
	RiverJobTimeoutWorkspaceInvitationMail = time.Minute
	// RiverJobTimeoutCreditReconciliation covers one scheduled reconciliation
	// run (bounded per-run batch); same class as the payment sweep budget.
	RiverJobTimeoutCreditReconciliation = 4 * time.Minute
	// RiverJobTimeoutTaskSettlementSweep covers one bounded final-timeout
	// sweep: a capped discovery query plus a capped batch of per-task settle
	// transactions, none of which calls a provider. It stays well under the
	// sweep's periodic interval so a stuck run times out instead of piling up
	// behind the next one.
	RiverJobTimeoutTaskSettlementSweep = 4 * time.Minute
	// RiverJobTimeoutCreditSchedule covers a single idempotent schedule write.
	RiverJobTimeoutCreditSchedule = time.Minute
	// RiverJobTimeoutPaymentReconciliation covers the Stripe/DB sweep window.
	RiverJobTimeoutPaymentReconciliation = 4 * time.Minute
	// RiverJobTimeoutPaymentCheckoutShutdown covers one shutdown sweep or a
	// single shutdown run; same budget as payment reconciliation.
	RiverJobTimeoutPaymentCheckoutShutdown = 4 * time.Minute
	// RiverJobTimeoutVisionBilling covers one snapshot reconciliation batch.
	// It stays below the 5-minute periodic interval so a stuck run times out
	// instead of piling up behind the next scheduled run.
	RiverJobTimeoutVisionBilling = 4 * time.Minute
	// RiverJobTimeoutMediaReclamation covers one asset reclaim (claim, object
	// deletes, finalize). It must stay below the 5-minute claim lease so a
	// timed-out attempt never outlives its lease.
	RiverJobTimeoutMediaReclamation = 2 * time.Minute
	// RiverJobTimeoutMediaReclamationSchedule covers one due-asset scan
	// (capped batch) plus the enqueue fan-out.
	RiverJobTimeoutMediaReclamationSchedule = 2 * time.Minute
	// RiverJobTimeoutWebhookDeliver covers one signed delivery attempt
	// (one outbound POST under a 5s send budget plus store bookkeeping).
	RiverJobTimeoutWebhookDeliver = time.Minute
	// RiverJobTimeoutIdentityAdminReconciliation covers one bounded dry-run
	// scan and its finding persistence.
	RiverJobTimeoutIdentityAdminReconciliation = 4 * time.Minute
	// RiverJobTimeoutIdentityAdminReconciliationSchedule covers one idempotent
	// schedule/catch-up write and transactional enqueue.
	RiverJobTimeoutIdentityAdminReconciliationSchedule = time.Minute
)

// Timeout layering for drawing execution (ADR-0010 rule 4): one gateway
// attempt is bounded by the gateway 300s total budget, one worker execution
// (all provider sends in a single claim, including the bounded model
// fallback) is bounded by WorkspaceExecutionBudget (360s), and the River job
// stays bounded by RiverJobTimeoutDrawing (17min). The 360s middle layer is
// the slow-task cap: a provider send that outlives it converges as a
// retryable timeout (worker requeue with backoff, bounded by the retry
// budget) instead of holding the worker slot until the River timeout.
// Settlement after a billed send never depends on the execution budget: it
// runs on the parent (River) context or a detached bounded context.
//
// Frozen budget table (values frozen, converge + assert only, never retune
// here without syncing the mirrors):
//
// | Layer | Owner | Budget | Notes |
// | --- | --- | --- | --- |
// | Worker single attempt | runtime/ports WorkerDefaultTimeout | 300s | nests inside the 360s execution budget |
// | Worker single execution | WorkspaceExecutionBudget() | 360s = min(360s, drawing) | slow-task cap; clamped never above the River drawing budget |
// | River job outer bound | RiverJobTimeoutDrawing | 17min = 900s MaxTimeoutSeconds + 2min overhead | global bound + client fallback for unregistered/test workers |
// | Frontend quote/submit shared retry | canvas canvasBudgets SHARED_TASK_REQUEST_ATTEMPTS | 2 attempts | assert + submit each; retryable errors only (mirror, owned by web) |
// | Frontend background continuation | canvas canvasBudgets BACKGROUND_* | 8 rounds, 5s init exp backoff capped at 60s, total 315s after the 6min waiter (~675s end-to-end < 17min River) | terminal state materializes inline; exhaustion ends with progressTimeout (mirror, owned by web) |
// | Frontend board materialization | canvas canvasBudgets PENDING_IMAGE_MATERIALIZATION_ATTEMPTS | 3 attempts | in-place retry of transient graph writes; contract violations fail fast (mirror, owned by web) |
const WorkspaceSlowTaskBudget = 6 * time.Minute

// WithDefaults converges the River default template: empty queue names fall
// back to RiverQueueDefault and non-positive budgets to
// RiverMaxAttemptsDefault. Enqueuer constructors share this single seam so
// the default template cannot drift per call site.
func WithDefaults(queue string, maxAttempts int) (string, int) {
	if strings.TrimSpace(queue) == "" {
		queue = RiverQueueDefault
	}
	if maxAttempts <= 0 {
		maxAttempts = RiverMaxAttemptsDefault
	}
	return queue, maxAttempts
}

// WorkspaceExecutionBudget returns the effective per-execution provider cap:
// the 360s slow-task budget, clamped never above the River drawing budget so
// a future drawing shrink cannot leave execution unbounded.
func WorkspaceExecutionBudget() time.Duration {
	if RiverJobTimeoutDrawing < WorkspaceSlowTaskBudget {
		return RiverJobTimeoutDrawing
	}
	return WorkspaceSlowTaskBudget
}
