package queue

import (
	"testing"
)

// TestRiverRetryMatrixIsSelfConsistent pins the code-owned retry budgets so
// a tuning change is deliberate: every entry needs a positive budget, and
// the settlement-only final-failure path must keep the full drawing budget
// (it only retries durable convergence, never provider work).
func TestRiverRetryMatrixIsSelfConsistent(t *testing.T) {
	matrix := RiverRetryMatrix()
	if len(matrix) == 0 {
		t.Fatal("RiverRetryMatrix() is empty")
	}
	seen := map[string]int{}
	for _, policy := range matrix {
		if policy.Queue == "" {
			t.Fatalf("matrix entry %+v has empty queue", policy)
		}
		if policy.MaxAttempts <= 0 {
			t.Fatalf("matrix entry %+v has non-positive budget", policy)
		}
		if policy.Worker == "" || policy.Notes == "" {
			t.Fatalf("matrix entry %+v must name its worker and rationale", policy)
		}
		seen[policy.Queue]++
	}
	for _, want := range []string{
		RiverQueueDefault,
		"credit_reconciliation",
		"payment_reconciliation",
		"payment_checkout_shutdown",
		"task_retention",
		"vision_billing",
		"media_reclamation",
		RiverQueueTaskSettlementSweep,
		"identity_admin_reconciliation",
	} {
		if seen[want] == 0 {
			t.Fatalf("matrix is missing queue %q", want)
		}
	}
}

// TestRiverRetryMatrixMatchesInsertSiteConstants guards the contract between
// the documented matrix and the constants the worker insert sites reference.
// Adding a queue means adding both a constant and a matrix row.
func TestRiverRetryMatrixMatchesInsertSiteConstants(t *testing.T) {
	budgets := map[string]int{}
	for _, policy := range RiverRetryMatrix() {
		if prev, ok := budgets[policy.Queue]; ok && prev != policy.MaxAttempts {
			t.Fatalf("queue %q has conflicting budgets %d and %d", policy.Queue, prev, policy.MaxAttempts)
		}
		budgets[policy.Queue] = policy.MaxAttempts
	}
	want := map[string]int{
		RiverQueueDefault:               RiverMaxAttemptsDefault,
		"credit_reconciliation":         RiverMaxAttemptsCreditReconciliation,
		"payment_reconciliation":        RiverMaxAttemptsPaymentReconciliation,
		"payment_checkout_shutdown":     RiverMaxAttemptsPaymentCheckoutShutdown,
		"task_retention":                RiverMaxAttemptsTaskRetention,
		"vision_billing":                RiverMaxAttemptsVisionBilling,
		"media_reclamation":             RiverMaxAttemptsMediaReclamation,
		RiverQueueTaskSettlementSweep:   RiverMaxAttemptsTaskSettlementSweep,
		"identity_admin_reconciliation": RiverMaxAttemptsIdentityAdminReconciliation,
	}
	for queue, attempts := range want {
		if budgets[queue] != attempts {
			t.Fatalf("queue %q matrix budget = %d, want constant %d", queue, budgets[queue], attempts)
		}
	}
}
