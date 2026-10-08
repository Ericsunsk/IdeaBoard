package riverpolicy

import "time"

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
// constant in riverpolicy.go and a row here, plus a Timeout() method referencing it.
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
			Queue:   RiverQueueCreditReconciliation,
			Timeout: RiverJobTimeoutCreditReconciliation,
			Worker:  "CreditReconciliationWorker",
			Notes:   "one scheduled reconciliation run; same sweep class as payment reconciliation",
		},
		{
			Queue:   RiverQueueTaskSettlementSweep,
			Timeout: RiverJobTimeoutTaskSettlementSweep,
			Worker:  "TaskSettlementSweepWorker (task_settlement_sweep, settlement-only)",
			Notes:   "final-timeout convergence of abandoned Credit Holds; bounded batch, never calls a provider; stays below the sweep interval",
		},
		{
			Queue:   RiverQueueCreditReconciliation,
			Timeout: RiverJobTimeoutCreditSchedule,
			Worker:  "CreditReconciliationScheduleWorker",
			Notes:   "single idempotent schedule write; no batch work",
		},
		{
			Queue:   RiverQueuePaymentReconciliation,
			Timeout: RiverJobTimeoutPaymentReconciliation,
			Worker:  "PaymentReconciliationWorker",
			Notes:   "Stripe/DB sweep window",
		},
		{
			Queue:   RiverQueuePaymentCheckoutShutdown,
			Timeout: RiverJobTimeoutPaymentCheckoutShutdown,
			Worker:  "PaymentCheckoutShutdownWorker",
			Notes:   "single shutdown run; same budget as payment reconciliation",
		},
		{
			Queue:   RiverQueuePaymentCheckoutShutdown,
			Timeout: RiverJobTimeoutPaymentCheckoutShutdown,
			Worker:  "PaymentCheckoutShutdownScheduleWorker",
			Notes:   "shutdown sweep; same budget as payment reconciliation",
		},
		{
			Queue:   RiverQueueTaskRetention,
			Timeout: RiverJobTimeoutTaskRetention,
			Worker:  "TaskRetentionWorker (serial single-worker queue)",
			Notes:   "serial purge window; matches the historical 15min budget",
		},
		{
			Queue:   RiverQueueVisionBilling,
			Timeout: RiverJobTimeoutVisionBilling,
			Worker:  "VisionBillingReconciliationWorker",
			Notes:   "snapshot reconciliation batch; below the 5min periodic interval",
		},
		{
			Queue:   RiverQueueMediaReclamation,
			Timeout: RiverJobTimeoutMediaReclamation,
			Worker:  "ReclaimWorker",
			Notes:   "single asset reclaim; stays below the 5min claim lease",
		},
		{
			Queue:   RiverQueueMediaReclamation,
			Timeout: RiverJobTimeoutMediaReclamationSchedule,
			Worker:  "ScheduleWorker",
			Notes:   "due-asset scan (capped batch) plus enqueue fan-out",
		},
		{
			Queue:   RiverQueueWebhookDeliver,
			Timeout: RiverJobTimeoutWebhookDeliver,
			Worker:  "WebhookDeliverWorker (webhook_deliver)",
			Notes:   "single signed POST attempt; short budget keeps slow endpoints from head-blocking the delivery queue",
		},
		{
			Queue:   RiverQueueIdentityAdminReconciliation,
			Timeout: RiverJobTimeoutIdentityAdminReconciliation,
			Worker:  "IdentityReconciliationExecuteWorker",
			Notes:   "bounded dry-run scan and finding persistence",
		},
		{
			Queue:   RiverQueueIdentityAdminReconciliation,
			Timeout: RiverJobTimeoutIdentityAdminReconciliationSchedule,
			Worker:  "IdentityReconciliationScheduleWorker",
			Notes:   "idempotent schedule/catch-up write and transactional enqueue",
		},
	}
}
