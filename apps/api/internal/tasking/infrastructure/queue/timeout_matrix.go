package queue

import (
	"time"

	"ideaboard/api/internal/platform/accountlimits"
)

// River timeout matrix (issue #783). Timeout policy is code-owned per the
// backend context rules: these constants are the single source of truth for
// per-worker River Timeout. Every worker's Timeout() method must reference
// them instead of repeating literals, so no worker silently falls back to the
// client-level JobTimeout (the drawing 17min global budget) and the runbook
// table (docs/runbooks/backend-metrics.md) stays accurate.
//
// River semantics: a worker without an explicit Timeout() inherits the
// client JobTimeout. That inheritance is the bug this matrix closes: only
// the drawing worker legitimately needs the full 17min budget, so it now
// pins the same value explicitly instead of inheriting it.
const (
	// RiverJobTimeoutDrawing owns the global budget: the longest supported
	// task execution (accountlimits.MaxTimeoutSeconds) plus worker overhead.
	// The River client default (normalizeRiverServerOptions) references this
	// constant as a pure fallback for unregistered/test workers.
	RiverJobTimeoutDrawing = time.Duration(accountlimits.MaxTimeoutSeconds)*time.Second + 2*time.Minute
	// RiverJobTimeoutFinalFailure covers settlement-only convergence: the
	// orchestrator caps a converge attempt at 15s internally, so 1 minute
	// leaves ample margin for the fallback enqueue path. Never calls a provider.
	RiverJobTimeoutFinalFailure = time.Minute
	// RiverJobTimeoutCreditReconciliation covers one scheduled reconciliation
	// run (bounded per-run batch); same class as the payment sweep budget.
	RiverJobTimeoutCreditReconciliation = 4 * time.Minute
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
)

// QueueTimeoutPolicy pins the River Timeout of one worker.
type QueueTimeoutPolicy struct {
	// Queue is the River queue name as observed in
	// ideaboard_river_queue_depth{queue} / _lag_seconds{queue}.
	Queue string
	// Timeout is the worker's River Timeout.
	Timeout time.Duration
	// Worker is the owning worker implementation.
	Worker string
	// Notes records why the budget differs from the drawing default.
	Notes string
}

// RiverTimeoutMatrix lists every production River worker with its Timeout.
// The roster is exhaustive on purpose: it is the proof that no worker falls
// back to the client-level JobTimeout. Adding a worker means adding both a
// constant above and a row here, plus a Timeout() method referencing it.
func RiverTimeoutMatrix() []QueueTimeoutPolicy {
	return []QueueTimeoutPolicy{
		{
			Queue:   RiverQueueDefault,
			Timeout: RiverJobTimeoutDrawing,
			Worker:  "TaskWorker (drawing_generate)",
			Notes:   "owns the global budget (15min MaxTimeout + 2min overhead); pinned explicitly instead of inheriting the client default",
		},
		{
			Queue:   RiverQueueDefault,
			Timeout: RiverJobTimeoutFinalFailure,
			Worker:  "FinalFailureWorker (drawing_final_failure, settlement-only)",
			Notes:   "never calls a provider; converge attempt capped at 15s internally",
		},
		{
			Queue:   "credit_reconciliation",
			Timeout: RiverJobTimeoutCreditReconciliation,
			Worker:  "CreditReconciliationWorker",
			Notes:   "one scheduled reconciliation run; same sweep class as payment reconciliation",
		},
		{
			Queue:   "credit_reconciliation",
			Timeout: RiverJobTimeoutCreditSchedule,
			Worker:  "CreditReconciliationScheduleWorker",
			Notes:   "single idempotent schedule write; no batch work",
		},
		{
			Queue:   "payment_reconciliation",
			Timeout: RiverJobTimeoutPaymentReconciliation,
			Worker:  "PaymentReconciliationWorker",
			Notes:   "Stripe/DB sweep window",
		},
		{
			Queue:   "payment_checkout_shutdown",
			Timeout: RiverJobTimeoutPaymentCheckoutShutdown,
			Worker:  "PaymentCheckoutShutdownWorker",
			Notes:   "single shutdown run; same budget as payment reconciliation",
		},
		{
			Queue:   "payment_checkout_shutdown",
			Timeout: RiverJobTimeoutPaymentCheckoutShutdown,
			Worker:  "PaymentCheckoutShutdownScheduleWorker",
			Notes:   "shutdown sweep; same budget as payment reconciliation",
		},
		{
			Queue:   "task_retention",
			Timeout: RiverJobTimeoutTaskRetention,
			Worker:  "TaskRetentionWorker (serial single-worker queue)",
			Notes:   "serial purge window; matches the historical 15min budget",
		},
		{
			Queue:   "vision_billing",
			Timeout: RiverJobTimeoutVisionBilling,
			Worker:  "VisionBillingReconciliationWorker",
			Notes:   "snapshot reconciliation batch; below the 5min periodic interval",
		},
		{
			Queue:   "media_reclamation",
			Timeout: RiverJobTimeoutMediaReclamation,
			Worker:  "ReclaimWorker",
			Notes:   "single asset reclaim; stays below the 5min claim lease",
		},
		{
			Queue:   "media_reclamation",
			Timeout: RiverJobTimeoutMediaReclamationSchedule,
			Worker:  "ScheduleWorker",
			Notes:   "due-asset scan (capped batch) plus enqueue fan-out",
		},
	}
}
