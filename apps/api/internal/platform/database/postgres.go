package database

import (
	"database/sql"
	"fmt"
	"log/slog"
	"net/url"
	"time"

	"ideaboard/api/internal/platform/config"

	_ "github.com/jackc/pgx/v5/stdlib"
)

const (
	postgresConnectMaxRetries = 60
	postgresConnectRetryDelay = time.Second
)

func NewPostgres(cfg config.DatabaseConfig) (*sql.DB, error) {
	dsn := PostgresDSN(cfg)
	var lastErr error
	for attempt := 1; attempt <= postgresConnectMaxRetries; attempt++ {
		db, err := sql.Open("pgx", dsn)
		if err == nil {
			// sql.DB pool (25 conns). The River worker maintains its own pgxpool.Pool;
			// keep the combined total under the PostgreSQL max_connections limit.
			db.SetMaxOpenConns(25)
			db.SetMaxIdleConns(10)
			db.SetConnMaxLifetime(30 * time.Minute)
			if pingErr := db.Ping(); pingErr == nil {
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
		slog.Warn("PostgreSQL connect attempt failed", "attempt", attempt, "max_retries", postgresConnectMaxRetries, "retry_interval", postgresConnectRetryDelay, "err", err)
		time.Sleep(postgresConnectRetryDelay)
	}
	return nil, lastErr
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
