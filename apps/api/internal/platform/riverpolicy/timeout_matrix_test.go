package riverpolicy

import (
	"testing"
	"time"
)

// TestRiverTimeoutMatrixIsSelfConsistent pins the code-owned Timeout budgets
// so a tuning change is deliberate: every entry needs a positive budget, and
// the roster must stay exhaustive (one row per production worker) so the
// matrix remains proof that no worker falls back to the client-level
// JobTimeout.
func TestRiverTimeoutMatrixIsSelfConsistent(t *testing.T) {
	matrix := RiverTimeoutMatrix()
	if len(matrix) == 0 {
		t.Fatal("RiverTimeoutMatrix() is empty")
	}
	seenWorkers := map[string]struct{}{}
	seenQueues := map[string]struct{}{}
	for _, policy := range matrix {
		if policy.Queue == "" {
			t.Fatalf("matrix entry %+v has empty queue", policy)
		}
		if policy.Timeout <= 0 {
			t.Fatalf("matrix entry %+v has non-positive timeout", policy)
		}
		if policy.Worker == "" || policy.Notes == "" {
			t.Fatalf("matrix entry %+v must name its worker and rationale", policy)
		}
		if _, dup := seenWorkers[policy.Worker]; dup {
			t.Fatalf("matrix has duplicate worker row %q", policy.Worker)
		}
		seenWorkers[policy.Worker] = struct{}{}
		seenQueues[policy.Queue] = struct{}{}
	}
	for _, want := range []string{
		RiverQueueDefault,
		RiverQueueCreditReconciliation,
		RiverQueuePaymentReconciliation,
		RiverQueuePaymentCheckoutShutdown,
		RiverQueueTaskRetention,
		RiverQueueVisionBilling,
		RiverQueueMediaReclamation,
		RiverQueueWebhookDeliver,
		RiverQueueTaskSettlementSweep,
		RiverQueueIdentityAdminReconciliation,
	} {
		if _, ok := seenQueues[want]; !ok {
			t.Fatalf("matrix is missing queue %q", want)
		}
	}
	for _, want := range []string{
		"TaskWorker (drawing_generate)",
		"FinalFailureWorker (drawing_final_failure, settlement-only)",
		"CreditReconciliationWorker",
		"CreditReconciliationScheduleWorker",
		"TaskSettlementSweepWorker (task_settlement_sweep, settlement-only)",
		"PaymentReconciliationWorker",
		"PaymentCheckoutShutdownWorker",
		"PaymentCheckoutShutdownScheduleWorker",
		"TaskRetentionWorker (serial single-worker queue)",
		"VisionBillingReconciliationWorker",
		"ReclaimWorker",
		"ScheduleWorker",
		"WebhookDeliverWorker (webhook_deliver)",
		"IdentityReconciliationExecuteWorker",
		"IdentityReconciliationScheduleWorker",
	} {
		if _, ok := seenWorkers[want]; !ok {
			t.Fatalf("matrix is missing worker %q", want)
		}
	}
}

// TestRiverTimeoutMatrixMatchesWorkerConstants guards the contract between
// the documented matrix and the constants the workers' Timeout() methods
// reference. Adding a worker means adding both a constant and a matrix row.
func TestRiverTimeoutMatrixMatchesWorkerConstants(t *testing.T) {
	budgets := map[string]time.Duration{}
	for _, policy := range RiverTimeoutMatrix() {
		if prev, ok := budgets[policy.Worker]; ok && prev != policy.Timeout {
			t.Fatalf("worker %q has conflicting budgets %s and %s", policy.Worker, prev, policy.Timeout)
		}
		budgets[policy.Worker] = policy.Timeout
	}
	want := map[string]time.Duration{
		"TaskWorker (drawing_generate)":                                      RiverJobTimeoutDrawing,
		"FinalFailureWorker (drawing_final_failure, settlement-only)":        RiverJobTimeoutFinalFailure,
		"CreditReconciliationWorker":                                         RiverJobTimeoutCreditReconciliation,
		"CreditReconciliationScheduleWorker":                                 RiverJobTimeoutCreditSchedule,
		"TaskSettlementSweepWorker (task_settlement_sweep, settlement-only)": RiverJobTimeoutTaskSettlementSweep,
		"PaymentReconciliationWorker":                                        RiverJobTimeoutPaymentReconciliation,
		"PaymentCheckoutShutdownWorker":                                      RiverJobTimeoutPaymentCheckoutShutdown,
		"PaymentCheckoutShutdownScheduleWorker":                              RiverJobTimeoutPaymentCheckoutShutdown,
		"TaskRetentionWorker (serial single-worker queue)":                   RiverJobTimeoutTaskRetention,
		"VisionBillingReconciliationWorker":                                  RiverJobTimeoutVisionBilling,
		"ReclaimWorker":                                                      RiverJobTimeoutMediaReclamation,
		"ScheduleWorker":                                                     RiverJobTimeoutMediaReclamationSchedule,
		"WebhookDeliverWorker (webhook_deliver)":                             RiverJobTimeoutWebhookDeliver,
		"IdentityReconciliationExecuteWorker":                                RiverJobTimeoutIdentityAdminReconciliation,
		"IdentityReconciliationScheduleWorker":                               RiverJobTimeoutIdentityAdminReconciliationSchedule,
	}
	for worker, timeout := range want {
		if budgets[worker] != timeout {
			t.Fatalf("worker %q matrix timeout = %s, want constant %s", worker, budgets[worker], timeout)
		}
	}
}

// TestRiverTimeoutMatrixPreservesSafetyBounds pins the cross-cutting
// constraints the Notes claim: the drawing budget owns the 17min global
// value, settlement-only and schedule work stays short, the vision budget
// stays below its 5-minute period, and reclaim stays below the claim lease.
func TestRiverTimeoutMatrixPreservesSafetyBounds(t *testing.T) {
	if RiverJobTimeoutDrawing != 17*time.Minute {
		t.Fatalf("drawing timeout = %s, want 17m (15min MaxTimeout + 2min overhead)", RiverJobTimeoutDrawing)
	}
	if RiverJobTimeoutFinalFailure > 2*time.Minute {
		t.Fatalf("final failure timeout = %s, want a short settlement-only budget", RiverJobTimeoutFinalFailure)
	}
	if RiverJobTimeoutVisionBilling >= 5*time.Minute {
		t.Fatalf("vision billing timeout = %s, must stay below the 5min periodic interval", RiverJobTimeoutVisionBilling)
	}
	if RiverJobTimeoutMediaReclamation >= 5*time.Minute {
		t.Fatalf("reclaim timeout = %s, must stay below the 5min claim lease", RiverJobTimeoutMediaReclamation)
	}
}

// TestRiverMatricesShareQueueRoster guards the cross-matrix contract: retry
// and timeout matrices must cover the same queue set so a queue never has a
// retry budget without a timeout budget (or vice versa). A new queue means a
// row in both matrices.
func TestRiverMatricesShareQueueRoster(t *testing.T) {
	retryQueues := map[string]struct{}{}
	for _, policy := range RiverRetryMatrix() {
		retryQueues[policy.Queue] = struct{}{}
	}
	timeoutQueues := map[string]struct{}{}
	for _, policy := range RiverTimeoutMatrix() {
		timeoutQueues[policy.Queue] = struct{}{}
	}
	for queue := range retryQueues {
		if _, ok := timeoutQueues[queue]; !ok {
			t.Fatalf("queue %q has a retry row but no timeout row", queue)
		}
	}
	for queue := range timeoutQueues {
		if _, ok := retryQueues[queue]; !ok {
			t.Fatalf("queue %q has a timeout row but no retry row", queue)
		}
	}
}
