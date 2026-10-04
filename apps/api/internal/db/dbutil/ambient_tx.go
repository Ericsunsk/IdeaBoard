package dbutil

// Ambient guard transaction carrier (control audit atomicity, step 3).
//
// Step 2 stages exactly one pgx transaction per guarded Control Plane
// mutation at the guard (the single choke point every guarded mutation
// flows through) and the audit row joins it before commit. Step 3 closes
// the remaining crash window by letting the domain-owned business writes
// join that SAME transaction instead of committing in their own.
//
// LIFECYCLE OWNERSHIP (load-bearing, read before touching):
//   - The GUARD begins the transaction (one per guarded mutation) and is
//     the ONLY owner of Commit and Rollback.
//   - A domain participant that finds a carrier via AmbientTxFromContext
//     MUST run its statements on it and MUST NEVER Commit or Rollback it.
//     Committing a borrowed transaction would double-commit the guard's
//     unit; rolling it back would discard the guard's audit row and every
//     sibling participant's writes.
//   - Nesting depth is 1: a carrier already in context is reused as-is.
//     pgx has no true nested transactions and savepoints are not needed
//     here, because every participant shares one flat unit of commit.
//   - With no carrier in context the caller keeps its legacy behavior
//     (begin/commit/rollback its own transaction), byte for byte.
//
// WHY THIS LIVES IN DBUTIL
// The carrier must be readable from both the Control Plane guard and the
// domain writers without creating a Go import cycle: the guard package
// already imports the credits/identity/tasking/payments/settings/tenancy
// trees, so those trees cannot import the guard back for the accessor.
// dbutil is a leaf (it imports only sqlc/pgx) already shared by the
// credits, settlement, settings, and tenancy writers, so it is the single
// home for the context key. The guard's ContextWithControlTx /
// ControlTxFromContext delegate here, so there is still exactly ONE
// carrier mechanism, not two keys that could disagree.

import (
	"context"

	"github.com/jackc/pgx/v5"
)

// ambientTxContextKey carries the guard-owned transaction for one guarded
// mutation. It is private so no other package can mint a competing key.
type ambientTxContextKey struct{}

// ContextWithAmbientTx returns ctx carrying tx for the current guarded
// mutation. The guard scope is the only production caller; tests use it
// to stage a transaction without a pool.
func ContextWithAmbientTx(ctx context.Context, tx pgx.Tx) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, ambientTxContextKey{}, tx)
}

// AmbientTxFromContext extracts the guard-owned transaction, if any. The
// second return is false when the call runs outside a guard scope (legacy
// path: the caller manages its own transaction) or when the staged value
// is nil. A returned transaction is BORROWED: the caller must use it for
// statements only and must never commit or roll it back.
func AmbientTxFromContext(ctx context.Context) (pgx.Tx, bool) {
	if ctx == nil {
		return nil, false
	}
	tx, ok := ctx.Value(ambientTxContextKey{}).(pgx.Tx)
	if !ok || tx == nil {
		return nil, false
	}
	return tx, true
}
