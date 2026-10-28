package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"ideaboard/api/internal/db/dbutil"

	"github.com/jackc/pgx/v5/pgxpool"
)

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
	return run(dbutil.ContextWithConnection(ctx, connection))
}
