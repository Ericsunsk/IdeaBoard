package health_test

import (
	"ideaboard/api/internal/credits/application/health"
	"ideaboard/api/internal/credits/ports"
	"testing"
	"time"
)

func TestDeriveCreditHealthStatus(t *testing.T) {
	now := time.Date(2026, 7, 17, 12, 0, 0, 0, time.UTC)
	recentSuccess := reconciliationRunAt(ports.ReconciliationRunCompleted, now.Add(-time.Hour))
	exactlyFreshSuccess := reconciliationRunAt(ports.ReconciliationRunCompleted, now.Add(-health.CreditHealthFreshnessWindow))
	staleSuccess := reconciliationRunAt(ports.ReconciliationRunCompleted, now.Add(-health.CreditHealthFreshnessWindow-time.Second))
	recentFailure := reconciliationRunAt(ports.ReconciliationRunFailed, now.Add(-30*time.Minute))
	oldFailure := reconciliationRunAt(ports.ReconciliationRunFailed, now.Add(-2*time.Hour))

	tests := []struct {
		name              string
		success           *ports.ReconciliationRun
		failure           *ports.ReconciliationRun
		findings          health.CreditHealthFindingSummary
		restrictedWallets int64
		want              health.CreditHealthStatus
	}{
		{name: "unchecked", want: health.CreditHealthUnchecked},
		{name: "healthy", success: recentSuccess, want: health.CreditHealthHealthy},
		{name: "exactly 48 hours remains healthy", success: exactlyFreshSuccess, want: health.CreditHealthHealthy},
		{name: "attention", success: recentSuccess, findings: health.CreditHealthFindingSummary{Total: 1, Warning: 1}, want: health.CreditHealthAttention},
		{name: "critical severity without high risk is attention", success: recentSuccess, findings: health.CreditHealthFindingSummary{Total: 1, Critical: 1}, want: health.CreditHealthAttention},
		{name: "critical high risk overrides failed", success: recentSuccess, failure: recentFailure, findings: health.CreditHealthFindingSummary{Total: 1, HighRisk: 1}, want: health.CreditHealthCritical},
		{name: "critical active restriction overrides stale", success: staleSuccess, restrictedWallets: 1, want: health.CreditHealthCritical},
		{name: "failed without success", failure: recentFailure, want: health.CreditHealthFailed},
		{name: "failed after success", success: recentSuccess, failure: recentFailure, want: health.CreditHealthFailed},
		{name: "successful run after failure", success: recentSuccess, failure: oldFailure, want: health.CreditHealthHealthy},
		{name: "stale", success: staleSuccess, want: health.CreditHealthStale},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := health.DeriveCreditHealthStatus(now, tt.success, tt.failure, tt.findings, tt.restrictedWallets); got != tt.want {
				t.Fatalf("DeriveCreditHealthStatus() = %q, want %q", got, tt.want)
			}
		})
	}
}

func reconciliationRunAt(status ports.ReconciliationRunStatus, completedAt time.Time) *ports.ReconciliationRun {
	return &ports.ReconciliationRun{Status: status, CompletedAt: &completedAt}
}
