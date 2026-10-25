package riverpolicy

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
		{Queue: RiverQueueWorkspaceInvitationMail, MaxAttempts: RiverMaxAttemptsWorkspaceInvitationMail, Worker: "WorkspaceInvitationMailWorker", Notes: "encrypted transactional mail delivery; provider idempotency is stable across retries"},
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
			Queue:       RiverQueueCreditReconciliation,
			MaxAttempts: RiverMaxAttemptsCreditReconciliation,
			Worker:      "CreditReconciliationWorker",
			Notes:       "daily scheduled run; small budget so a stuck run pages instead of churning",
		},
		{
			Queue:       RiverQueueTaskSettlementSweep,
			MaxAttempts: RiverMaxAttemptsTaskSettlementSweep,
			Worker:      "TaskSettlementSweepWorker (task_settlement_sweep)",
			Notes:       "settlement-only convergence of abandoned Credit Holds; per-task failures are isolated, so the job only fails on a sweep-level fault",
		},
		{
			Queue:       RiverQueuePaymentReconciliation,
			MaxAttempts: RiverMaxAttemptsPaymentReconciliation,
			Worker:      "PaymentReconciliationWorker",
			Notes:       "Stripe order reconciliation; transient Stripe/DB faults retry, poisoned jobs surface after 8",
		},
		{
			Queue:       RiverQueuePaymentCheckoutShutdown,
			MaxAttempts: RiverMaxAttemptsPaymentCheckoutShutdown,
			Worker:      "PaymentCheckoutShutdownWorker",
			Notes:       "checkout session shutdown sweep; same budget as payment reconciliation",
		},
		{
			Queue:       RiverQueueVisionBilling,
			MaxAttempts: RiverMaxAttemptsVisionBilling,
			Worker:      "VisionBillingReconciliationWorker",
			Notes:       "vision-search billing reconciliation against immutable snapshots",
		},
		{
			Queue:       RiverQueueMediaReclamation,
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
		{
			Queue:       RiverQueueIdentityAdminReconciliation,
			MaxAttempts: RiverMaxAttemptsIdentityAdminReconciliation,
			Worker:      "IdentityReconciliationExecuteWorker",
			Notes:       "daily dry-run identity drift scan; bounded retries surface a poisoned run without indefinite churn",
		},
		{
			Queue:       RiverQueueIdentityAdminReconciliation,
			MaxAttempts: RiverMaxAttemptsIdentityAdminReconciliation,
			Worker:      "IdentityReconciliationScheduleWorker",
			Notes:       "schedule and catch-up writes are idempotent and share the reconciliation queue budget",
		},
	}
}
