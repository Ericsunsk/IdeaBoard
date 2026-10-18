package control

// Control Plane guard transaction scope (step 2).
//
// The control handlers own no transaction: every business write commits
// inside a domain-owned transaction, so threading one shared pgx.Tx
// through each handler would mean touching every owning context.
// Instead the guard (the single choke point every guarded mutation
// flows through) owns exactly one transaction per mutating call via
// this scope, and the atomic audit participant joins it before commit.
// No per-handler edit is needed for coverage: every Mutation:true
// control route inherits the scope structurally from the guard.
//
// Only the audit half joined at step 2 (domain seams still committed
// in their own transactions). At step 3 the domain ports accept the
// carrier via dbutil.AmbientTxFromContext (bridged as
// control.ControlTxFromContext) and join this same transaction, so the
// business fact and its audit row commit or roll back together. The
// transaction rides the request context
// (control.ContextWithControlTx), so a tx-aware seam joins the same
// commit with no guard change.

import (
	"context"
	"errors"
	"log/slog"

	"ideaboard/api/internal/db/dbutil"
	"ideaboard/api/internal/runtime/interfaces/http/control"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// errControlAuditMissingTx marks a participant write with no guard
// transaction: the scope failed to stage one, so joining is impossible
// and the mutation must fail rather than commit unaudited.
var errControlAuditMissingTx = errors.New("control atomic audit requires the guard transaction")

// ControlTxScope implements control.ControlTxScope over the shared
// pool: one pgx transaction per guarded mutation, carried on the
// request context.
type ControlTxScope struct {
	db *pgxpool.Pool
}

var _ control.ControlTxScope = (*ControlTxScope)(nil)

// NewControlTxScope builds the guard transaction scope over pool. A
// nil pool keeps every Begin fail-closed (the guard renders 503-style
// 500 instead of panicking), matching the nil-dep convention.
func NewControlTxScope(db *pgxpool.Pool) *ControlTxScope {
	return &ControlTxScope{db: db}
}

// Begin opens the mutation transaction and returns the request context
// carrying it. The caller (the guard) must Commit or Rollback it.
func (s *ControlTxScope) Begin(ctx context.Context) (context.Context, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("control tx scope: database is nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, err
	}
	return control.ContextWithControlTx(ctx, tx), nil
}

// Commit commits the mutation transaction (participants plus the audit
// row) as one unit.
func (s *ControlTxScope) Commit(ctx context.Context) error {
	tx, ok := control.ControlTxFromContext(ctx)
	if !ok {
		return errControlAuditMissingTx
	}
	return tx.Commit(ctx)
}

// Rollback discards the mutation transaction on a detached budget (the
// request already failed for another reason; rollback only releases
// locks). It never reports an error.
func (s *ControlTxScope) Rollback(ctx context.Context) {
	tx, ok := control.ControlTxFromContext(ctx)
	if !ok {
		return
	}
	rollbackCtx, cancel := context.WithTimeout(context.Background(), dbutil.RollbackTimeout)
	defer cancel()
	if err := tx.Rollback(rollbackCtx); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
		slog.Warn("failed to roll back control mutation transaction", "err", err)
	}
}
