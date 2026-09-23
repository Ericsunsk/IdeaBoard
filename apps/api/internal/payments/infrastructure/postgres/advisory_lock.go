package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// advisoryConnectionContextKey keeps a session-bound connection available to
// repository operations executed inside WithAdvisoryLock. The key is private
// to the adapter so application packages cannot accidentally depend on its
// representation.
type advisoryConnectionContextKey struct{}

// ContextWithAdvisoryConnection returns a context carrying the locked pgx
// connection. Callers should use this only for work that must share the
// advisory-lock session.
func ContextWithAdvisoryConnection(ctx context.Context, connection *pgxpool.Conn) context.Context {
	return context.WithValue(ctx, advisoryConnectionContextKey{}, connection)
}

// AdvisoryConnection returns the session-bound connection, if one was
// installed by WithAdvisoryLock.
func AdvisoryConnection(ctx context.Context) (*pgxpool.Conn, bool) {
	connection, ok := ctx.Value(advisoryConnectionContextKey{}).(*pgxpool.Conn)
	return connection, ok
}

// WithAdvisoryLock serializes work on one PostgreSQL session advisory lock.
// The callback receives a context that uses the same *pgxpool.Conn, preserving
// session lock semantics across all repository queries in the callback.
func WithAdvisoryLock(
	ctx context.Context,
	pool *pgxpool.Pool,
	lockKey string,
	seed int64,
	run func(context.Context) error,
) (resultErr error) {
	if pool == nil || run == nil || lockKey == "" {
		return fmt.Errorf("postgres advisory lock: invalid input")
	}
	connection, err := pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("open advisory lock connection: %w", err)
	}
	defer connection.Release()
	if _, err := connection.Exec(ctx, `SELECT pg_advisory_lock(hashtextextended($1, $2))`, lockKey, seed); err != nil {
		return fmt.Errorf("acquire advisory lock: %w", err)
	}
	defer func() {
		unlockContext, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		var unlocked bool
		unlockErr := connection.QueryRow(unlockContext, `SELECT pg_advisory_unlock(hashtextextended($1, $2))`, lockKey, seed).Scan(&unlocked)
		if unlockErr != nil || !unlocked {
			if unlockErr == nil {
				unlockErr = fmt.Errorf("advisory lock was not held")
			}
			// Destroy the pooled connection so a leaked session lock cannot
			// block later holders when the connection is reused.
			_ = connection.Conn().Close(unlockContext)
			resultErr = errors.Join(resultErr, fmt.Errorf("release advisory lock: %w", unlockErr))
		}
	}()
	return run(ContextWithAdvisoryConnection(ctx, connection))
}
