package postgres

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"time"
)

// advisoryConnectionContextKey keeps a session-bound connection available to
// repository operations executed insideWithAdvisoryLock. The key is private
// to the adapter so application packages cannot accidentally depend on its
// representation.
type advisoryConnectionContextKey struct{}

// ContextWithAdvisoryConnection returns a context carrying the locked SQL
// connection. Callers should use this only for work that must share the
// advisory-lock session.
func ContextWithAdvisoryConnection(ctx context.Context, connection *sql.Conn) context.Context {
	return context.WithValue(ctx, advisoryConnectionContextKey{}, connection)
}

// AdvisoryConnection returns the session-bound connection, if one was
// installed byWithAdvisoryLock.
func AdvisoryConnection(ctx context.Context) (*sql.Conn, bool) {
	connection, ok := ctx.Value(advisoryConnectionContextKey{}).(*sql.Conn)
	return connection, ok
}

// WithAdvisoryLock serializes work on one PostgreSQL session advisory lock.
// The callback receives a context that uses the same *sql.Conn, preserving
// session lock semantics across all repository queries in the callback.
func WithAdvisoryLock(
	ctx context.Context,
	db *sql.DB,
	lockKey string,
	seed int64,
	run func(context.Context) error,
) (resultErr error) {
	if db == nil || run == nil || lockKey == "" {
		return fmt.Errorf("postgres advisory lock: invalid input")
	}
	connection, err := db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("open advisory lock connection: %w", err)
	}
	defer func() {
		if closeErr := connection.Close(); closeErr != nil {
			resultErr = errors.Join(resultErr, fmt.Errorf("close advisory lock connection: %w", closeErr))
		}
	}()
	if _, err := connection.ExecContext(ctx, `SELECT pg_advisory_lock(hashtextextended($1, $2))`, lockKey, seed); err != nil {
		return fmt.Errorf("acquire advisory lock: %w", err)
	}
	defer func() {
		unlockContext, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		var unlocked bool
		unlockErr := connection.QueryRowContext(unlockContext, `SELECT pg_advisory_unlock(hashtextextended($1, $2))`, lockKey, seed).Scan(&unlocked)
		if unlockErr != nil || !unlocked {
			_ = connection.Raw(func(any) error { return driver.ErrBadConn })
			if unlockErr == nil {
				unlockErr = fmt.Errorf("advisory lock was not held")
			}
			resultErr = errors.Join(resultErr, fmt.Errorf("release advisory lock: %w", unlockErr))
		}
	}()
	return run(ContextWithAdvisoryConnection(ctx, connection))
}
