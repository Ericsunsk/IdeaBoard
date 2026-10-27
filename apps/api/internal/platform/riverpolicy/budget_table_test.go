package riverpolicy

import (
	"testing"
	"time"

	"ideaboard/api/internal/runtime/ports"
)

// TestWorkspaceBudgetLayeringIsFrozen pins the timeout layering the budget
// table documents (ADR-0010 rule 4): gateway 300s nests inside the 360s
// worker execution budget, which nests inside the 17min River outer bound.
// Values are frozen: this test only converges the layers into one assertion,
// never retunes them.
func TestWorkspaceBudgetLayeringIsFrozen(t *testing.T) {
	if ports.GatewayDefaultTimeout != 300*time.Second {
		t.Fatalf("ports.GatewayDefaultTimeout = %s, want 300s (frozen canvas budget table)", ports.GatewayDefaultTimeout)
	}
	if got := WorkspaceExecutionBudget(); got != 6*time.Minute {
		t.Fatalf("WorkspaceExecutionBudget() = %s, want 360s (frozen slow-task cap)", got)
	}
	if RiverJobTimeoutDrawing != 17*time.Minute {
		t.Fatalf("RiverJobTimeoutDrawing = %s, want 17m (900s MaxTimeout + 2min overhead)", RiverJobTimeoutDrawing)
	}
	if !(ports.GatewayDefaultTimeout < WorkspaceExecutionBudget()) {
		t.Fatalf("layering broken: gateway %s must nest inside execution %s", ports.GatewayDefaultTimeout, WorkspaceExecutionBudget())
	}
	if !(WorkspaceExecutionBudget() < RiverJobTimeoutDrawing) {
		t.Fatalf("layering broken: execution %s must nest inside River %s", WorkspaceExecutionBudget(), RiverJobTimeoutDrawing)
	}
}

// TestCanvasBackgroundContinuationFitsRiverBound mirrors the frozen frontend
// canvasBudgets backoff (8 rounds, 5s init exponential, 60s cap) and pins
// its total inside the River outer bound. The frontend owns the numbers;
// this test only guards the cross-language invariant end-to-end (6min
// waiter + 315s background ≈ 675s) stays under the 17min River job bound.
// Numeric mirrors: update together with canvasBudgets.ts, never retune here.
func TestCanvasBackgroundContinuationFitsRiverBound(t *testing.T) {
	const (
		maxAttempts = 8
		initial     = 5 * time.Second
		maxDelay    = 60 * time.Second
		waiter      = 6 * time.Minute
	)
	total := time.Duration(0)
	for attempt := 0; attempt < maxAttempts; attempt++ {
		delay := initial << attempt
		if delay > maxDelay || delay <= 0 {
			delay = maxDelay
		}
		total += delay
	}
	if total != 315*time.Second {
		t.Fatalf("canvas background continuation total = %s, want 315s (5+10+20+40+60*4 mirror of canvasBudgets.ts)", total)
	}
	if total >= RiverJobTimeoutDrawing {
		t.Fatalf("background total %s must stay below River bound %s", total, RiverJobTimeoutDrawing)
	}
	if waiter+total >= RiverJobTimeoutDrawing {
		t.Fatalf("waiter %s + background %s must stay below River bound %s", waiter, total, RiverJobTimeoutDrawing)
	}
}

// TestDrawingWorkerUsesOuterRiverBudget guards the budget published for River.
func TestDrawingWorkerUsesOuterRiverBudget(t *testing.T) {
	for _, policy := range RiverTimeoutMatrix() {
		if policy.Worker == "TaskWorker (drawing_generate)" {
			if policy.Timeout != RiverJobTimeoutDrawing || policy.Timeout <= WorkspaceExecutionBudget() {
				t.Fatalf("drawing timeout = %s, want outer River budget %s above execution budget", policy.Timeout, RiverJobTimeoutDrawing)
			}
			return
		}
	}
	t.Fatal("drawing worker is missing from RiverTimeoutMatrix")
}
