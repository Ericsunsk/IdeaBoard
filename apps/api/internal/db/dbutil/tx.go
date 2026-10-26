package dbutil

// Deep transaction module: the single owner of borrow-or-begin.
//
// Every store previously repeated the same three-piece plumbing:
// dbWithContext (bind ambient carrier or pool) + withTx/WithTxDBTX
// (borrow or begin) + the "borrowed transactions must never commit or
// roll back" rule kept alive only by comments. This file owns all three
// pieces in one place:
//
//   - Begin / BeginOwned / Do decide borrow vs. begin.
//   - BorrowedQueries binds the read path to the carrier.
//   - Scope tracks ownership, and the borrowed handle itself refuses
//     Commit/Rollback with ErrBorrowedTxLifecycle, so the rule is
//     enforced by the module instead of by comments.
//
// LIFECYCLE CONTRACT (unchanged from the ambient carrier rules):
//   - The stager (guard / settlement owner) begins the transaction and
//     is the ONLY owner of Commit and Rollback.
//   - A borrowed participant runs statements only. Scope.Commit and
//     Scope.Rollback are safe no-ops on a borrowed scope (the same
//     "do nothing, the owner decides" behavior every store hand-rolled
//     before); calling Commit/Rollback directly on the borrowed handle
//     returns ErrBorrowedTxLifecycle instead of double-committing or
//     discarding the owner's unit.
//   - Nesting depth is 1: a carrier already in context is reused as-is.
//   - Owned scopes keep the historical dbutil semantics byte for byte:
//     begin with the given options, commit on success, detached
//     Background rollback with the shared RollbackTimeout budget and a
//     warn log on failure (see Do).
//
// Callers that must NEVER borrow (workspace hard delete, the committed
// refund reservation, worker lease claims) use BeginOwned, which makes
// the no-borrow intent explicit instead of relying on "we just don't
// check the carrier here" comments.

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	dbsqlc "ideaboard/api/internal/db/sqlc"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrBorrowedTxLifecycle reports a Commit/Rollback attempt on a borrowed
// transaction handle. Only the stager that began the transaction may end
// it; participants that reach this error must route lifecycle through
// their Scope (which no-ops when borrowed) instead of touching the
// handle. Match with errors.Is.
var ErrBorrowedTxLifecycle = errors.New("dbutil: borrowed transaction must not be committed or rolled back")

// TxStarter opens owned transactions. It is satisfied by *pgxpool.Pool
// and by the advisory-lock session starter some repositories carry, so
// BeginOn covers both without the module importing those packages.
type TxStarter interface {
	BeginTx(context.Context, pgx.TxOptions) (pgx.Tx, error)
}

// borrowedTx wraps a carrier transaction handed to a participant. Every
// statement method delegates to the carrier; Commit and Rollback refuse
// with ErrBorrowedTxLifecycle so a borrowed participant can never end
// the owner's unit, even when it bypasses its Scope.
type borrowedTx struct {
	pgx.Tx
}

// BorrowedTx reports whether tx is a borrowed (lifecycle-refusing)
// handle minted by this module. Scopes never double-wrap: Begin reuses
// an already-borrowed handle as-is.
func BorrowedTx(tx pgx.Tx) bool {
	if tx == nil {
		return false
	}
	_, ok := tx.(*borrowedTx)
	return ok
}

// Commit refuses: the stager owns the commit.
func (b *borrowedTx) Commit(context.Context) error {
	return fmt.Errorf("%w: commit refused", ErrBorrowedTxLifecycle)
}

// Rollback refuses: the stager owns the rollback decision.
func (b *borrowedTx) Rollback(context.Context) error {
	return fmt.Errorf("%w: rollback refused", ErrBorrowedTxLifecycle)
}

// borrow wraps tx as a participant handle, reusing it unchanged when it
// already refuses lifecycle.
func borrow(tx pgx.Tx) pgx.Tx {
	if tx == nil || BorrowedTx(tx) {
		return tx
	}
	return &borrowedTx{Tx: tx}
}

// Scope owns one borrow-or-begin unit: the bound Queries, the
// transaction handle, and the ownership bit that decides who may end
// it. The zero value is invalid; build Scopes with Begin/BeginOwned.
// ScopeForTest wraps an already-owned raw transaction for settlement
// fakes and fixture helpers that stage their own pgx.Tx (in-memory
// fakeTx shims, pool.Begin fixtures): the Scope carries the same
// handle with the given ownership, so Scope-based participants run
// statements on it without changing commit boundaries. Production
// code never uses this; it exists so the Tx-to-Scope migration does
// not fork fixture commit paths.
func ScopeForTest(ctx context.Context, tx pgx.Tx, owned bool) *Scope {
	if ctx == nil {
		ctx = context.Background()
	}
	var queries *dbsqlc.Queries
	if tx != nil {
		queries = dbsqlc.New(tx)
	}
	return &Scope{ctx: ctx, tx: tx, queries: queries, owned: owned}
}

type Scope struct {
	ctx     context.Context
	tx      pgx.Tx
	queries *dbsqlc.Queries
	owned   bool
}

// Begin borrows the ambient carrier when one is staged in ctx, or begins
// an owned transaction on pool otherwise. A nil pool is fine when
// borrowing (no pool connection is needed); without a carrier it fails
// with the historical "dbutil: database is nil" error.
func Begin(ctx context.Context, pool *pgxpool.Pool, opts pgx.TxOptions) (*Scope, error) {
	return begin(ctx, pool, opts, true)
}

// BeginOwned always begins an owned transaction and never borrows, even
// when a carrier is staged. Use it for units whose commit boundary must
// not join the ambient unit (hard deletes, committed refund
// reservations, worker lease claims).
func BeginOwned(ctx context.Context, pool *pgxpool.Pool, opts pgx.TxOptions) (*Scope, error) {
	return begin(ctx, pool, opts, false)
}

// BeginOn borrows the ambient carrier when one is staged in ctx, or
// begins an owned transaction on starter otherwise. Unlike Begin it
// accepts any starter (for example an advisory-lock session); callers
// with a possibly-nil starter must nil-check before calling so their
// own "unavailable" error (not a panic) surfaces.
func BeginOn(ctx context.Context, starter TxStarter, opts pgx.TxOptions) (*Scope, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if ambient, ok := AmbientTxFromContext(ctx); ok {
		handle := borrow(ambient)
		return &Scope{ctx: ctx, tx: handle, queries: dbsqlc.New(handle), owned: false}, nil
	}
	if starter == nil {
		return nil, errors.New("dbutil: database is nil")
	}
	tx, err := starter.BeginTx(ctx, opts)
	if err != nil {
		return nil, err
	}
	return &Scope{ctx: ctx, tx: tx, queries: dbsqlc.New(tx), owned: true}, nil
}

func begin(ctx context.Context, pool *pgxpool.Pool, opts pgx.TxOptions, allowBorrow bool) (*Scope, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if allowBorrow {
		if ambient, ok := AmbientTxFromContext(ctx); ok {
			handle := borrow(ambient)
			return &Scope{ctx: ctx, tx: handle, queries: dbsqlc.New(handle), owned: false}, nil
		}
	}
	if pool == nil {
		return nil, errors.New("dbutil: database is nil")
	}
	tx, err := pool.BeginTx(ctx, opts)
	if err != nil {
		return nil, err
	}
	return &Scope{ctx: ctx, tx: tx, queries: dbsqlc.New(tx), owned: true}, nil
}

// Context returns the scope's context (the caller's context, unchanged).
func (s *Scope) Context() context.Context {
	if s == nil || s.ctx == nil {
		return context.Background()
	}
	return s.ctx
}

// Queries returns the transaction-bound queries: the carrier when
// borrowed, the owned transaction otherwise.
func (s *Scope) Queries() *dbsqlc.Queries {
	if s == nil {
		return nil
	}
	return s.queries
}

// Tx returns the transaction handle for raw-SQL participants and
// downstream InTx composition. On a borrowed scope the handle refuses
// Commit/Rollback with ErrBorrowedTxLifecycle; route lifecycle through
// Commit/Rollback on the Scope instead.
func (s *Scope) Tx() pgx.Tx {
	if s == nil {
		return nil
	}
	return s.tx
}

// Owned reports whether the scope began its transaction (true: the scope
// must commit or roll it back) or borrowed it (false: the stager owns
// the lifecycle and Scope.Commit/Scope.Rollback are safe no-ops).
func (s *Scope) Owned() bool {
	return s != nil && s.owned
}

// Commit commits an owned transaction and is a safe no-op on a borrowed
// scope (the stager commits it with its own unit). Direct Commit calls
// on a borrowed handle still refuse with ErrBorrowedTxLifecycle.
func (s *Scope) Commit(ctx context.Context) error {
	if s == nil {
		return errors.New("dbutil: commit on nil transaction scope")
	}
	if !s.owned {
		return nil
	}
	return s.tx.Commit(ctx)
}

// Rollback releases an owned transaction on a detached Background
// context with the shared RollbackTimeout budget, swallowing the
// already-closed case like every historical call site. It is a safe
// no-op on a borrowed scope (the stager owns the rollback decision).
func (s *Scope) Rollback(ctx context.Context) error {
	if s == nil {
		return errors.New("dbutil: rollback on nil transaction scope")
	}
	if !s.owned {
		return nil
	}
	_ = ctx
	rollbackCtx, cancel := context.WithTimeout(context.Background(), RollbackTimeout)
	defer cancel()
	if err := s.tx.Rollback(rollbackCtx); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
		return err
	}
	return nil
}

// Do runs fn inside one borrow-or-begin unit and settles it: fn's error
// returns unchanged (a borrowed unit is never rolled back here; the
// stager decides its fate), an owned unit commits on success and rolls
// back on failure with the historical warn-and-continue semantics.
//
// Do is the single canonical executor behind WithTx/WithTxDBTX, which
// remain as signature-stable wrappers.
func Do(ctx context.Context, pool *pgxpool.Pool, opts pgx.TxOptions, fn func(*Scope) error) error {
	if fn == nil {
		return errors.New("dbutil: tx function is nil")
	}
	scope, err := Begin(ctx, pool, opts)
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			if rollbackErr := scope.Rollback(ctx); rollbackErr != nil {
				slog.Warn("failed to roll back database transaction", "err", rollbackErr)
			}
		}
	}()
	if err := fn(scope); err != nil {
		return err
	}
	if err := scope.Commit(ctx); err != nil {
		return err
	}
	committed = true
	return nil
}

// BorrowedQueries binds the read path to the ambient carrier when one is
// staged, or returns base unchanged otherwise. Stores whose reads join
// the guard unit use this as their dbWithContext body; stores whose
// reads stay on the pool (library, media, tasking, audit) keep
// returning their pool-bound queries untouched.
func BorrowedQueries(ctx context.Context, base *dbsqlc.Queries) *dbsqlc.Queries {
	if ambient, ok := AmbientTxFromContext(ctx); ok {
		return dbsqlc.New(borrow(ambient))
	}
	return base
}

// BorrowOrPool returns the ambient carrier for raw single-statement
// writes when one is staged, or the pool otherwise. Both satisfy the
// statement surface; neither carries lifecycle (DBTX has no
// Commit/Rollback), so no wrapper is needed.
func BorrowOrPool(ctx context.Context, pool *pgxpool.Pool) dbsqlc.DBTX {
	if ambient, ok := AmbientTxFromContext(ctx); ok {
		return ambient
	}
	return pool
}

// Compile-time surface checks: the borrowed handle stays a pgx.Tx (so
// sqlc WithTx and InTx composition keep working), the pool stays a
// starter, and raw handles satisfy the statement surface.
var (
	_ pgx.Tx      = (*borrowedTx)(nil)
	_ TxStarter   = (*pgxpool.Pool)(nil)
	_ dbsqlc.DBTX = (*pgxpool.Pool)(nil)
	_ dbsqlc.DBTX = (pgx.Tx)(nil)
	_ dbsqlc.DBTX = (*borrowedTx)(nil)
)
