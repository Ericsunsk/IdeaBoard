package queue

// River retry matrix (issue #772). Retry/timeout policy is code-owned per
// the backend context rules: these constants are the single source of truth
// for per-queue MaxAttempts. Insert sites in the worker packages must
// reference them instead of repeating literals, so a tuning change lands in
// one place and the runbook table (docs/runbooks/backend-metrics.md) stays
// accurate.
//
// Backoff between attempts is River's default exponential backoff with
// jitter; the drawing queue additionally layers worker-level policy on top:
// transient provider failures (429/5xx/timeout/network after gateway-internal
// retries) requeue at most twice per task (tasks.retry_count bound) before
// converging terminally. Deterministic failures never retry.
const (
	RiverMaxAttemptsCreditReconciliation    = 5
	RiverMaxAttemptsPaymentReconciliation   = 8
	RiverMaxAttemptsPaymentCheckoutShutdown = 8
	RiverMaxAttemptsTaskRetention           = 3
	RiverMaxAttemptsVisionBilling           = 5
	RiverMaxAttemptsMediaReclamation        = 10
	RiverMaxAttemptsWebhookDeliver          = 8
)

// QueueRetryPolicy pins the retry budget of one River queue.
type QueueRetryPolicy struct {
	// Queue is the River queue name as observed in
	// ideaboard_river_queue_depth{queue} / _lag_seconds{queue}.
	Queue string
	// MaxAttempts is the River job MaxAttempts for this queue.
	MaxAttempts int
	// Worker is the owning worker implementation.
	Worker string
	// Notes records why the budget differs from the drawing default.
	Notes string
}

// RiverRetryMatrix lists every River queue with its retry budget. The
// drawing queues share RiverMaxAttemptsDefault (queue-layer retries cover
// worker panics, process errors and transient infrastructure faults);
// periodic maintenance queues use smaller budgets so a poisoned job surfaces
// fast instead of churning for 25 attempts.
func RiverRetryMatrix() []QueueRetryPolicy {
	return []QueueRetryPolicy{
		{
			Queue:       RiverQueueDefault,
			MaxAttempts: RiverMaxAttemptsDefault,
			Worker:      "TaskWorker (drawing_generate)",
			Notes:       "queue-layer retry for panics/process errors; provider transient requeues (max 2) ride inside this budget",
		},
		{
			Queue:       RiverQueueDefault,
			MaxAttempts: RiverMaxAttemptsDefault,
			Worker:      "FinalFailureWorker (drawing_final_failure, settlement-only)",
			Notes:       "never calls a provider; retries only durable terminal convergence",
		},
		{
			Queue:       "credit_reconciliation",
			MaxAttempts: RiverMaxAttemptsCreditReconciliation,
			Worker:      "CreditReconciliationWorker",
			Notes:       "daily scheduled run; small budget so a stuck run pages instead of churning",
		},
		{
			Queue:       "payment_reconciliation",
			MaxAttempts: RiverMaxAttemptsPaymentReconciliation,
			Worker:      "PaymentReconciliationWorker",
			Notes:       "Stripe order reconciliation; transient Stripe/DB faults retry, poisoned jobs surface after 8",
		},
		{
			Queue:       "payment_checkout_shutdown",
			MaxAttempts: RiverMaxAttemptsPaymentCheckoutShutdown,
			Worker:      "PaymentCheckoutShutdownWorker",
			Notes:       "checkout session shutdown sweep; same budget as payment reconciliation",
		},
		{
			Queue:       "task_retention",
			MaxAttempts: RiverMaxAttemptsTaskRetention,
			Worker:      "TaskRetentionWorker (serial single-worker queue)",
			Notes:       "purge runs are serial by design; failures must surface immediately, not retry into the next window",
		},
		{
			Queue:       "vision_billing",
			MaxAttempts: RiverMaxAttemptsVisionBilling,
			Worker:      "VisionBillingReconciliationWorker",
			Notes:       "vision-search billing reconciliation against immutable snapshots",
		},
		{
			Queue:       "media_reclamation",
			MaxAttempts: RiverMaxAttemptsMediaReclamation,
			Worker:      "ReclaimWorker / ScheduleWorker",
			Notes:       "storage consistency scan; storage blips are transient so the budget exceeds other maintenance queues",
		},
		{
			Queue:       RiverQueueWebhookDeliver,
			MaxAttempts: RiverMaxAttemptsWebhookDeliver,
			Worker:      "WebhookDeliverWorker (webhook_deliver)",
			Notes:       "one job per delivery preserves the 8-attempt budget; discarded rows are the durable DLQ fact",
		},
	}
}
