package dbutil

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	dbsqlc "ideaboard/api/internal/db/sqlc"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// RollbackTimeout bounds one transaction rollback on a detached
// context. It is the single home for the pgx rollback budget duplicated
// across credits, tasking settlement, and this package's WithTx helper:
// rollback only releases locks and discards an already-doomed tx, so
// every site shares the same small budget.
const RollbackTimeout = 5 * time.Second

// WithTx runs fn inside a single transaction and commits it.
//
// Ambient guard transaction (control audit atomicity): when ctx carries
// the guard-staged transaction (see AmbientTxFromContext), fn joins it
// as a BORROWED participant instead of opening a new transaction. The
// borrowed transaction is never committed or rolled back here; the
// guard owns its lifecycle. With no carrier the legacy owned-transaction
// path runs unchanged.
func WithTx(ctx context.Context, pool *pgxpool.Pool, fn func(*dbsqlc.Queries) error) error {
	if fn == nil {
		return errors.New("dbutil: tx function is nil")
	}
	return WithTxDBTX(ctx, pool, func(q *dbsqlc.Queries, _ pgx.Tx) error {
		return fn(q)
	})
}

// WithTxDBTX runs fn inside a single transaction, exposing both the
// transaction-bound Queries and the raw pgx.Tx, then commits.
//
// Ambient guard transaction (control audit atomicity): when ctx carries
// the guard-staged transaction (see AmbientTxFromContext), fn joins it
// as a BORROWED participant: fn runs on the carrier and its error
// returns unchanged, with no begin, commit, or rollback here. On fn
// error this branch deliberately does NOT roll back: the transaction
// is not ours, and only the guard may decide its fate. With no carrier
// the legacy owned-transaction path below runs byte for byte as before.
func WithTxDBTX(ctx context.Context, pool *pgxpool.Pool, fn func(*dbsqlc.Queries, pgx.Tx) error) error {
	if fn == nil {
		return errors.New("dbutil: tx function is nil")
	}
	if ambient, ok := AmbientTxFromContext(ctx); ok {
		// dbsqlc.New(ambient) binds Queries to the carrier without
		// touching pool, so this path needs no pool connection of its
		// own (and never acquires one).
		return fn(dbsqlc.New(ambient), ambient)
	}
	if pool == nil {
		return errors.New("dbutil: database is nil")
	}
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			rollbackCtx, cancel := context.WithTimeout(context.Background(), RollbackTimeout)
			defer cancel()
			if rollbackErr := tx.Rollback(rollbackCtx); rollbackErr != nil && !errors.Is(rollbackErr, pgx.ErrTxClosed) {
				slog.Warn("failed to roll back database transaction", "err", rollbackErr)
			}
		}
	}()

	if err := fn(dbsqlc.New(pool).WithTx(tx), tx); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	committed = true
	return nil
}

// ResolveWorkspaceInternalID maps an opaque workspace public id to its
// internal id. The input is trimmed before lookup; the query error returns
// unchanged so callers map "not found" to their own operation-scoped
// sentinel (hidden workspaces must never leak a distinct status). Callers
// that fail closed on malformed ids keep their own shape pre-check.
func ResolveWorkspaceInternalID(ctx context.Context, q *dbsqlc.Queries, workspacePublicID string) (int64, error) {
	row, err := q.GetWorkspaceByPublicID(ctx, strings.TrimSpace(workspacePublicID))
	if err != nil {
		return 0, err
	}
	return row.InternalID, nil
}

func NullTime(value *time.Time) pgtype.Timestamptz {
	if value == nil {
		return pgtype.Timestamptz{}
	}
	return TimestamptzFromTime(*value)
}

func TimePtr(value pgtype.Timestamptz) *time.Time {
	if !value.Valid {
		return nil
	}
	v := value.Time
	return &v
}

func TimeValue(value pgtype.Timestamptz) time.Time {
	if !value.Valid {
		return time.Time{}
	}
	return value.Time
}

func NullInt64(value *int64) pgtype.Int8 {
	if value == nil {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: *value, Valid: true}
}

func Int64Ptr(value pgtype.Int8) *int64 {
	if !value.Valid {
		return nil
	}
	v := value.Int64
	return &v
}

func NullString(value *string) pgtype.Text {
	if value == nil {
		return pgtype.Text{}
	}
	v := strings.TrimSpace(*value)
	return pgtype.Text{String: v, Valid: v != ""}
}

func StringPtr(value pgtype.Text) *string {
	if !value.Valid {
		return nil
	}
	v := value.String
	return &v
}

func TimestamptzFromTime(value time.Time) pgtype.Timestamptz {
	if value.IsZero() {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: value, Valid: true}
}

// Int4FromPresent lifts a validated int32 into a nullable column. Unlike a
// plain pointer helper it also encodes the domain's non-negative guard:
// absent or negative values persist as NULL so half-known dimensions never
// reach the wire.
func Int4FromPresent(value int32, present bool) pgtype.Int4 {
	if !present || value < 0 {
		return pgtype.Int4{}
	}
	return pgtype.Int4{Int32: value, Valid: true}
}

func DateFromTime(value time.Time) pgtype.Date {
	if value.IsZero() {
		return pgtype.Date{}
	}
	return pgtype.Date{Time: value, Valid: true}
}

func DateValue(value pgtype.Date) time.Time {
	if !value.Valid {
		return time.Time{}
	}
	return value.Time
}
