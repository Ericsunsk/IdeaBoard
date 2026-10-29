package database

import (
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNotFound = pgx.ErrNoRows

// ErrPoolNotConfigured is the shared fail-closed sentinel for a missing
// database pool. Callers wrap it with their own %w prefix so the existing
// message surface ("<owner>: database pool not configured") is preserved
// while errors.Is converges on one value.
var ErrPoolNotConfigured = errors.New("database pool not configured")

// RequirePool fails closed when pool is nil, so every pool guard shares
// one sentinel instead of hand-writing the literal.
func RequirePool(pool *pgxpool.Pool) error {
	if pool == nil {
		return ErrPoolNotConfigured
	}
	return nil
}

func IsNotFound(err error) bool {
	return errors.Is(err, ErrNotFound)
}

func IsDuplicateConstraintError(err error) bool {
	if err == nil {
		return false
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23505"
	}
	return false
}
