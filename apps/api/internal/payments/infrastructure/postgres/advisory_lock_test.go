package postgres

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"ideaboard/api/internal/db/dbutil"
	"ideaboard/api/internal/testdb"

	"github.com/jackc/pgx/v5/pgxpool"
)

func boundedAdvisoryPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	fixturePool := testdb.Open(t)
	config := fixturePool.Config()
	config.MaxConns = 2
	pool, err := pgxpool.NewWithConfig(t.Context(), config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func TestWithAdvisoryLockSharesSessionAndReleases(t *testing.T) {
	callbackError := errors.New("callback failed")
	for _, outcome := range []string{"success", "error", "cancel"} {
		t.Run(outcome, func(t *testing.T) {
			pool := boundedAdvisoryPool(t)
			ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
			defer cancel()
			observer, err := pool.Acquire(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer observer.Release()
			lockedContext, cancelLocked := context.WithCancel(ctx)
			defer cancelLocked()
			var wantError error
			err = WithAdvisoryLock(lockedContext, pool, "lock-lifetime", 1, func(locked context.Context) error {
				connection, ok := dbutil.ConnectionFromContext(locked)
				if !ok {
					return fmt.Errorf("missing borrowed connection")
				}
				if connection.Conn().PgConn().TxStatus() != 'I' {
					return fmt.Errorf("session lock opened a transaction")
				}
				var sessionPID uint32
				if err := NewRepository(pool).DBWithContext(locked).Executor().QueryRow(locked, `SELECT pg_backend_pid()`).Scan(&sessionPID); err != nil {
					return err
				}
				if sessionPID != connection.Conn().PgConn().PID() {
					return fmt.Errorf("repository used another session")
				}
				tx, owned, err := NewRepository(pool).DBWithContext(locked).BeginTx()
				if err != nil {
					return err
				}
				defer func() { _ = tx.Rollback(ctx) }()
				if !owned {
					return fmt.Errorf("short transaction must be owned")
				}
				if err := tx.QueryRow(locked, `SELECT pg_backend_pid()`).Scan(&sessionPID); err != nil {
					return err
				}
				if sessionPID != connection.Conn().PgConn().PID() {
					return fmt.Errorf("short transaction used another session")
				}
				if err := tx.Commit(locked); err != nil {
					return err
				}
				var acquired bool
				if err := observer.QueryRow(ctx, `SELECT pg_try_advisory_lock(hashtextextended($1, $2))`, "lock-lifetime", 1).Scan(&acquired); err != nil {
					return err
				}
				if acquired {
					return fmt.Errorf("another session acquired the held lock")
				}
				switch outcome {
				case "error":
					wantError = callbackError
				case "cancel":
					cancelLocked()
					wantError = context.Canceled
				}
				return wantError
			})
			if !errors.Is(err, wantError) {
				t.Fatalf("callback result = %v, want %v", err, wantError)
			}
			var acquired bool
			if err := observer.QueryRow(ctx, `SELECT pg_try_advisory_lock(hashtextextended($1, $2))`, "lock-lifetime", 1).Scan(&acquired); err != nil {
				t.Fatal(err)
			}
			if !acquired {
				t.Fatal("lock remained held after callback")
			}
			if _, err := observer.Exec(ctx, `SELECT pg_advisory_unlock(hashtextextended($1, $2))`, "lock-lifetime", 1); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestWithAdvisoryLockCanceledWaiterDoesNotRunCallback(t *testing.T) {
	pool := boundedAdvisoryPool(t)
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	blocker, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer blocker.Release()
	if _, err := blocker.Exec(ctx, `SELECT pg_advisory_lock(hashtextextended($1, $2))`, "cancel-waiter", 1); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_, _ = blocker.Exec(ctx, `SELECT pg_advisory_unlock(hashtextextended($1, $2))`, "cancel-waiter", 1)
	}()
	waiter, cancelWaiter := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancelWaiter()
	called := false
	err = WithAdvisoryLock(waiter, pool, "cancel-waiter", 1, func(context.Context) error { called = true; return nil })
	if !errors.Is(err, context.DeadlineExceeded) || called {
		t.Fatalf("waiting result = %v, callback called = %t", err, called)
	}
	if _, err := blocker.Exec(ctx, `SELECT pg_advisory_unlock(hashtextextended($1, $2))`, "cancel-waiter", 1); err != nil {
		t.Fatal(err)
	}
	if err := WithAdvisoryLock(ctx, pool, "cancel-waiter", 1, func(context.Context) error { return nil }); err != nil {
		t.Fatalf("acquire after canceled waiter: %v", err)
	}
}
