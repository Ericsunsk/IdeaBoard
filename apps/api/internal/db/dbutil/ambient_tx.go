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
	"sync"

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

// ambientPostCommitKey carries the post-commit hook registry for one
// guarded mutation (#889). It is separate from ambientTxContextKey on
// purpose: the transaction carrier predates post-commit announcements
// and is constructed by scopes the guard does not control in tests, so
// the registry rides its own key and is simply absent there.
type ambientPostCommitKey struct{}

// AmbientPostCommit buffers post-commit callbacks for one guarded
// mutation (#889: task cancel transitions).
//
// POST-COMMIT SEMANTICS (load-bearing):
//   - Hooks run ONLY after the guard-owned transaction commits. The
//     guard runs them exactly once, synchronously, right after its
//     successful Commit.
//   - Hooks NEVER run on rollback, audit failure, or commit failure:
//     the guard returns before the run line on those paths, so the
//     registered callbacks are dropped with the context and publish
//     nothing. There is no explicit discard call.
//   - Hooks run OUTSIDE the transaction (it already committed), so they
//     must perform side effects only (event publication) and never issue
//     statements on the carrier.
//   - Hooks run in registration order (FIFO): a settlement that defers
//     its staging before its announcer defers the announcement observes
//     the staged fact when the announcement runs.
type AmbientPostCommit struct {
	mu    sync.Mutex
	hooks []func(context.Context)
	ran   bool
}

// ContextWithAmbientPostCommit returns ctx carrying an empty post-commit
// hook registry for the current guarded mutation. The guard is the only
// production caller (right after its TxScope.Begin); tests attach it to
// a carrier to prove deferral without the guard.
func ContextWithAmbientPostCommit(ctx context.Context) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, ambientPostCommitKey{}, &AmbientPostCommit{})
}

// DeferAmbientPostCommit registers fn to run after the guard-owned
// transaction commits. It reports false (and registers nothing) when
// ctx carries no registry — raw carriers outside the guard, legacy
// callers — so participants can fall back to their legacy behavior.
// A nil fn is rejected the same way.
func DeferAmbientPostCommit(ctx context.Context, fn func(context.Context)) bool {
	if ctx == nil || fn == nil {
		return false
	}
	reg, ok := ctx.Value(ambientPostCommitKey{}).(*AmbientPostCommit)
	if !ok || reg == nil {
		return false
	}
	reg.mu.Lock()
	defer reg.mu.Unlock()
	if reg.ran {
		return false
	}
	reg.hooks = append(reg.hooks, fn)
	return true
}

// RunAmbientPostCommit runs the registered hooks in registration order.
// It is a no-op without a registry, with no hooks, or when called twice:
// exactly-once execution is enforced here so a repeated call can never
// double-publish. The guard calls this once, immediately after its
// successful Commit; hook errors are the hooks' own concern (announcers
// warn-log and never fail the already-committed settlement).
func RunAmbientPostCommit(ctx context.Context) {
	if ctx == nil {
		return
	}
	reg, ok := ctx.Value(ambientPostCommitKey{}).(*AmbientPostCommit)
	if !ok || reg == nil {
		return
	}
	reg.mu.Lock()
	if reg.ran {
		reg.mu.Unlock()
		return
	}
	reg.ran = true
	hooks := reg.hooks
	reg.hooks = nil
	reg.mu.Unlock()
	for _, fn := range hooks {
		if fn != nil {
			fn(ctx)
		}
	}
}
