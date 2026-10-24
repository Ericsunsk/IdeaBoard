package dbutil

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// WithReadSnapshot runs fn inside a single REPEATABLE READ READ ONLY
// transaction so every participant that joins the ambient carrier observes
// the same snapshot. Stores built on repositoryQueryer (tenancy,
// billing-account, organization) join automatically via
// AmbientTxFromContext; writers must never run inside it (the snapshot is
// read-only, so a first-touch provisioning write fails closed instead of
// forking state: callers resolve-or-provision before opening it).
//
// The snapshot is always released with Rollback (read-only: nothing to
// commit); fn must never Commit or Rollback the borrowed transaction.
func WithReadSnapshot(ctx context.Context, pool *pgxpool.Pool, fn func(ctx context.Context) error) error {
	if fn == nil {
		return errors.New("dbutil: snapshot function is nil")
	}
	if pool == nil {
		return errors.New("dbutil: snapshot pool is nil")
	}
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{
		IsoLevel:   pgx.RepeatableRead,
		AccessMode: pgx.ReadOnly,
	})
	if err != nil {
		return err
	}
	rollback := func() {
		rollbackCtx := ctx
		if rollbackCtx == nil {
			rollbackCtx = context.Background()
		}
		_ = tx.Rollback(rollbackCtx)
	}
	defer rollback()
	return fn(ContextWithAmbientTx(ctx, tx))
}
