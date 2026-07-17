package credits

import (
	"testing"
	"time"
)

func TestDeriveCreditHealthStatus(t *testing.T) {
	now := time.Date(2026, 7, 17, 12, 0, 0, 0, time.UTC)
	recentSuccess := reconciliationRunAt(ReconciliationRunCompleted, now.Add(-time.Hour))
	exactlyFreshSuccess := reconciliationRunAt(ReconciliationRunCompleted, now.Add(-creditHealthFreshnessWindow))
	staleSuccess := reconciliationRunAt(ReconciliationRunCompleted, now.Add(-creditHealthFreshnessWindow-time.Second))
	recentFailure := reconciliationRunAt(ReconciliationRunFailed, now.Add(-30*time.Minute))
	oldFailure := reconciliationRunAt(ReconciliationRunFailed, now.Add(-2*time.Hour))

	tests := []struct {
		name              string
		success           *ReconciliationRun
		failure           *ReconciliationRun
		findings          CreditHealthFindingSummary
		restrictedWallets int64
		want              CreditHealthStatus
	}{
		{name: "unchecked", want: CreditHealthUnchecked},
		{name: "healthy", success: recentSuccess, want: CreditHealthHealthy},
		{name: "exactly 48 hours remains healthy", success: exactlyFreshSuccess, want: CreditHealthHealthy},
		{name: "attention", success: recentSuccess, findings: CreditHealthFindingSummary{Total: 1, Warning: 1}, want: CreditHealthAttention},
		{name: "critical severity without high risk is attention", success: recentSuccess, findings: CreditHealthFindingSummary{Total: 1, Critical: 1}, want: CreditHealthAttention},
		{name: "critical high risk overrides failed", success: recentSuccess, failure: recentFailure, findings: CreditHealthFindingSummary{Total: 1, HighRisk: 1}, want: CreditHealthCritical},
		{name: "critical active restriction overrides stale", success: staleSuccess, restrictedWallets: 1, want: CreditHealthCritical},
		{name: "failed without success", failure: recentFailure, want: CreditHealthFailed},
		{name: "failed after success", success: recentSuccess, failure: recentFailure, want: CreditHealthFailed},
		{name: "successful run after failure", success: recentSuccess, failure: oldFailure, want: CreditHealthHealthy},
		{name: "stale", success: staleSuccess, want: CreditHealthStale},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := deriveCreditHealthStatus(now, tt.success, tt.failure, tt.findings, tt.restrictedWallets); got != tt.want {
				t.Fatalf("deriveCreditHealthStatus() = %q, want %q", got, tt.want)
			}
		})
	}
}

func reconciliationRunAt(status ReconciliationRunStatus, completedAt time.Time) *ReconciliationRun {
	return &ReconciliationRun{Status: status, CompletedAt: &completedAt}
}
