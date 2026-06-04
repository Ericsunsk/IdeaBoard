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

func NewPostgres(cfg config.DatabaseConfig) (*sql.DB, error) {
	dsn := PostgresDSN(cfg)
	maxRetries := positiveIntDefault(cfg.ConnectMaxRetries, 60)
	retryInterval := time.Duration(positiveIntDefault(cfg.ConnectRetryIntervalMS, 1000)) * time.Millisecond

	var lastErr error
	for attempt := 1; attempt <= maxRetries; attempt++ {
		db, err := sql.Open("pgx", dsn)
		if err == nil {
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
		if attempt >= maxRetries {
			break
		}
		slog.Warn("PostgreSQL connect attempt failed", "attempt", attempt, "max_retries", maxRetries, "retry_interval", retryInterval, "err", err)
		time.Sleep(retryInterval)
	}
	return nil, lastErr
}

func positiveIntDefault(value, defaultValue int) int {
	if value > 0 {
		return value
	}
	return defaultValue
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
