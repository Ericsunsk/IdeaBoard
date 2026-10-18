package control

import (
	"context"
	"errors"
	"testing"
)

// ControlTxScope base pairing test for tx_scope.go.
// The scope owns exactly one pgx transaction per guarded mutation;
// a nil pool or a context without a staged transaction must fail
// closed (never panic, never commit unaudited).
func TestControlTxScopeRequiresPool(t *testing.T) {
	scope := NewControlTxScope(nil)
	if scope == nil {
		t.Fatal("NewControlTxScope(nil) returned nil, want non-nil fail-closed scope")
	}
	if _, err := scope.Begin(t.Context()); err == nil {
		t.Fatal("Begin with nil pool succeeded, want fail-closed error")
	}
}

func TestControlTxScopeCommitWithoutTxFails(t *testing.T) {
	scope := NewControlTxScope(nil)
	if err := scope.Commit(context.Background()); !errors.Is(err, errControlAuditMissingTx) {
		t.Fatalf("Commit without staged tx error = %v, want errControlAuditMissingTx", err)
	}
}

func TestControlTxScopeRollbackWithoutTxIsNoop(t *testing.T) {
	scope := NewControlTxScope(nil)
	// Must not panic; rollback only releases locks on the detached budget.
	// Nil-context defense lives in dbutil.AmbientTxFromContext, covered
	// by its own tests; here a TODO context stages the no-tx noop.
	scope.Rollback(context.Background())
	scope.Rollback(context.TODO())
}
