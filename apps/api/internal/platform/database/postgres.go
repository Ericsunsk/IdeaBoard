package database

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"net/url"
	"time"

	"ideaboard/api/internal/platform/config"

	_ "github.com/jackc/pgx/v5/stdlib"
)

const (
	postgresConnectMaxRetries = 60
	postgresConnectBaseDelay  = 500 * time.Millisecond
	postgresConnectMaxDelay   = 5 * time.Second
	postgresPingTimeout       = 5 * time.Second
	postgresConnectTimeout    = 60 * time.Second
)

// Connection budget: the database/sql pool below plus the River pgxpool
// (see NewRiverPgxPool) must stay under PostgreSQL max_connections with head
// room for migrations, CLIs and test fixtures. Default server limit is 100;
// a single API instance holds 25 + 10 = 35.
const (
	postgresMaxOpenConns = 25
	postgresMaxIdleConns = 10
)

// NewPostgres opens the runtime database/sql pool. Startup retries use
// exponential backoff with jitter and honor ctx cancellation so a hung
// database cannot wedge process boot forever.
func NewPostgres(ctx context.Context, cfg config.DatabaseConfig) (*sql.DB, error) {
	connectCtx, cancel := postgresConnectContext(ctx)
	defer cancel()

	dsn := PostgresDSN(cfg)
	var lastErr error
	delay := postgresConnectBaseDelay
	for attempt := 1; attempt <= postgresConnectMaxRetries; attempt++ {
		if err := connectCtx.Err(); err != nil {
			return nil, fmt.Errorf("PostgreSQL connect cancelled: %w", err)
		}
		db, err := sql.Open("pgx", dsn)
		if err == nil {
			// sql.DB pool (25 conns). The River worker maintains its own pgxpool.Pool;
			// keep the combined total under the PostgreSQL max_connections limit.
			db.SetMaxOpenConns(postgresMaxOpenConns)
			db.SetMaxIdleConns(postgresMaxIdleConns)
			db.SetConnMaxLifetime(30 * time.Minute)
			// Idle connections behind NAT/pgbouncer go stale silently;
			// recycle them instead of handing callers a dead socket.
			db.SetConnMaxIdleTime(5 * time.Minute)
			if pingErr := pingPostgres(connectCtx, db); pingErr == nil {
				return db, nil
			} else {
				err = pingErr
				_ = db.Close()
			}
		}

		lastErr = err
		if attempt >= postgresConnectMaxRetries {
			break
		}
		slog.Warn("PostgreSQL connect attempt failed", "attempt", attempt, "max_retries", postgresConnectMaxRetries, "retry_in", delay, "err", err)
		if sleepErr := sleepContext(connectCtx, delay); sleepErr != nil {
			return nil, fmt.Errorf("PostgreSQL connect cancelled: %w", sleepErr)
		}
		delay = nextConnectDelay(delay)
	}
	return nil, lastErr
}

func postgresConnectContext(parent context.Context) (context.Context, context.CancelFunc) {
	if parent == nil {
		parent = context.Background()
	}
	return context.WithTimeout(parent, postgresConnectTimeout)
}

func pingPostgres(ctx context.Context, db *sql.DB) error {
	pingCtx, cancel := context.WithTimeout(ctx, postgresPingTimeout)
	defer cancel()
	return db.PingContext(pingCtx)
}

func sleepContext(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func nextConnectDelay(current time.Duration) time.Duration {
	next := current + current/2
	if next > postgresConnectMaxDelay {
		next = postgresConnectMaxDelay
	}
	// Full-jitter guard against synchronized restarts thundering the database.
	//nolint:gosec // jitter only, not security randomness
	return next/2 + time.Duration(rand.Int64N(int64(next/2+1)))
}

func PostgresDSN(cfg config.DatabaseConfig) string {
	values := url.Values{}
	values.Set("sslmode", cfg.SSLMode)

	dsn := url.URL{
		Scheme:   "postgres",
		User:     url.UserPassword(cfg.User, cfg.Password),
		Host:     fmt.Sprintf("%s:%s", cfg.Host, cfg.Port),
		Path:     cfg.Database,
		RawQuery: values.Encode(),
	}
	return dsn.String()
}
