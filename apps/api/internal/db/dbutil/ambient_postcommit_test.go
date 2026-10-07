package dbutil

// Post-commit hook registry for the ambient guard transaction (#889).
//
// What is proven here, without a database:
//   - Defer without a registry reports false (raw carriers, legacy
//     callers); participants fall back to their legacy behavior.
//   - A nil hook is rejected the same way, never stored.
//   - Run executes hooks in registration order (FIFO) with the run
//     context: settlement staging registered before its announcement
//     is visible to the announcement when it runs.
//   - Run without a registry, without hooks, on a nil context, or a
//     second time is a no-op: exactly-once execution can never
//     double-publish.

import (
	"context"
	"testing"
)

func TestDeferAmbientPostCommitWithoutRegistryReportsFalse(t *testing.T) {
	called := false
	if DeferAmbientPostCommit(context.Background(), func(context.Context) { called = true }) {
		t.Fatal("DeferAmbientPostCommit without a registry = true, want false")
	}
	txCtx := ContextWithAmbientTx(context.Background(), nil)
	if DeferAmbientPostCommit(txCtx, func(context.Context) { called = true }) {
		t.Fatal("DeferAmbientPostCommit on a carrier without a registry = true, want false")
	}
	if DeferAmbientPostCommit(nil, func(context.Context) { called = true }) {
		t.Fatal("DeferAmbientPostCommit on a nil context = true, want false")
	}
	RunAmbientPostCommit(context.Background())
	RunAmbientPostCommit(nil)
	if called {
		t.Fatal("unregistered hook ran, want it dropped")
	}
}

func TestDeferAmbientPostCommitRejectsNilHook(t *testing.T) {
	carrier := ContextWithAmbientPostCommit(context.Background())
	if DeferAmbientPostCommit(carrier, nil) {
		t.Fatal("DeferAmbientPostCommit with a nil hook = true, want false")
	}
	RunAmbientPostCommit(carrier)
}

func TestAmbientPostCommitRunsInRegistrationOrder(t *testing.T) {
	carrier := ContextWithAmbientPostCommit(context.Background())
	var order []string
	if !DeferAmbientPostCommit(carrier, func(context.Context) { order = append(order, "stage") }) {
		t.Fatal("first DeferAmbientPostCommit = false, want true")
	}
	if !DeferAmbientPostCommit(carrier, func(ctx context.Context) {
		if ctx == nil {
			t.Error("post-commit hook received a nil context")
		}
		order = append(order, "announce")
	}) {
		t.Fatal("second DeferAmbientPostCommit = false, want true")
	}
	RunAmbientPostCommit(carrier)
	if len(order) != 2 || order[0] != "stage" || order[1] != "announce" {
		t.Fatalf("hook order = %v, want [stage announce] (FIFO)", order)
	}
}

func TestRunAmbientPostCommitRunsOnce(t *testing.T) {
	carrier := ContextWithAmbientPostCommit(context.Background())
	runs := 0
	if !DeferAmbientPostCommit(carrier, func(context.Context) { runs++ }) {
		t.Fatal("DeferAmbientPostCommit = false, want true")
	}
	RunAmbientPostCommit(carrier)
	RunAmbientPostCommit(carrier)
	if runs != 1 {
		t.Fatalf("hook runs = %d, want exactly 1 (no double-publish)", runs)
	}
	if DeferAmbientPostCommit(carrier, func(context.Context) { runs++ }) {
		t.Fatal("DeferAmbientPostCommit after the run = true, want false (registry spent)")
	}
	RunAmbientPostCommit(carrier)
	if runs != 1 {
		t.Fatalf("hook runs after late Defer = %d, want still 1", runs)
	}
}
