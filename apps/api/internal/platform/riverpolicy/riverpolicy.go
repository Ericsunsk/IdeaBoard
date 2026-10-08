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
// internal/tasking/workspacetasks (MaxProviderRequeues). This package never
// becomes a generic queue framework: no client wrapper, no enqueue helper,
// only typed policy facts.
package riverpolicy

import (
	"time"

	"ideaboard/api/internal/platform/accountlimits"
)

// River queue names. Every production River queue has exactly one canonical
// name here; worker packages alias these constants instead of repeating
// string literals so a rename lands in one place and the RED metric label
// set (ideaboard_river_queue_depth{queue}) stays accurate.
const (
	RiverQueueDefault = "drawing_default"
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
	// RiverQueueTaskRetention carries the serial task retention purges.
	RiverQueueTaskRetention = "task_retention"
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
	RiverMaxAttemptsCreditReconciliation        = 5
	RiverMaxAttemptsPaymentReconciliation       = 8
	RiverMaxAttemptsPaymentCheckoutShutdown     = 8
	RiverMaxAttemptsTaskRetention               = 3
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
// falls back to the client-level JobTimeout (the drawing 17min global
// budget) and the runbook table (docs/runbooks/backend-metrics.md) stays
// accurate.
//
// River semantics: a worker without an explicit Timeout() inherits the
// client JobTimeout. That inheritance is the bug this matrix closes: only
// the drawing worker legitimately needs the full 17min budget, so it now
// pins the same value explicitly instead of inheriting it.
const (
	// RiverJobTimeoutDrawing owns the global budget: the longest supported
	// task execution (accountlimits.MaxTimeoutSeconds) plus worker overhead.
	// The River client default references this constant as a pure fallback
	// for unregistered/test workers.
	RiverJobTimeoutDrawing = time.Duration(accountlimits.MaxTimeoutSeconds)*time.Second + 2*time.Minute
	// RiverJobTimeoutFinalFailure covers settlement-only convergence: the
	// orchestrator caps a converge attempt at 15s internally, so 1 minute
	// leaves ample margin for the fallback enqueue path. Never calls a provider.
	RiverJobTimeoutFinalFailure = time.Minute
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
	// RiverJobTimeoutTaskRetention covers the serial purge window.
	RiverJobTimeoutTaskRetention = 15 * time.Minute
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
