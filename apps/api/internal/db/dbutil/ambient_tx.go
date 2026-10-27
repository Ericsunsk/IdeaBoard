package dbutil

// Ambient transaction carrier shared by transaction owners and participants.
// Control guards and Invitations own outer transactions; domain writes join
// the same transaction so their facts commit atomically.
//
// LIFECYCLE OWNERSHIP (load-bearing, read before touching):
//   - The scope that begins the transaction is its ONLY owner of Commit
//     and Rollback.
//   - A domain participant that finds a carrier via AmbientTxFromContext
//     MUST run its statements on it and MUST NEVER Commit or Rollback it.
//     Committing a borrowed transaction would double-commit the owner's
//     unit; rolling it back would discard every participant's writes.
//   - Nesting depth is 1: a carrier already in context is reused as-is.
//     pgx has no true nested transactions and savepoints are not needed
//     here, because every participant shares one flat unit of commit.
//   - With no carrier in context the caller manages its own persistence
//     scope, including any required transaction and post-commit registry.
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

// ambientTxContextKey carries the owner-managed transaction for one mutation.
// It is private so no other package can mint a competing key.
type ambientTxContextKey struct{}

// ContextWithAmbientTx returns ctx carrying the caller-owned transaction.
// Control guards and atomic collaboration services own the outer transaction;
// participating repositories borrow it without changing its lifecycle.
func ContextWithAmbientTx(ctx context.Context, tx pgx.Tx) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, ambientTxContextKey{}, tx)
}

// AmbientTxFromContext extracts the owner-managed transaction, if any. The
// second return is false when no transaction is carried or the value is nil.
// A returned transaction is BORROWED: the caller must use it for
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

// ambientPostCommitKey carries the post-commit hook registry independently
// from the transaction carrier. Owners install both when participants need
// deferred publication; a transaction carrier alone does not imply a registry.
type ambientPostCommitKey struct{}

// AmbientPostCommit buffers post-commit callbacks for one transaction.
//
// POST-COMMIT SEMANTICS (load-bearing):
//   - Hooks run ONLY after the owner-managed transaction commits. The
//     owner runs them exactly once, synchronously, right after its
//     successful Commit.
//   - Hooks NEVER run on rollback, audit failure, or commit failure:
//     the owner returns before the run line on those paths, so the
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
// hook registry for the current transaction. Control guards and Invitations
// transaction owners install it after beginning their transaction; tests
// may attach it to a carrier to prove deferral.
func ContextWithAmbientPostCommit(ctx context.Context) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, ambientPostCommitKey{}, &AmbientPostCommit{})
}

// DeferAmbientPostCommit registers fn to run after the outer transaction
// commits. It reports false without registering when the registry is absent
// or already run, or fn is nil. Participants must enforce their own contract:
// Notifications rejects ambient writes before persistence if registration fails.
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
// double-publish. The transaction owner calls this once, immediately after its
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
