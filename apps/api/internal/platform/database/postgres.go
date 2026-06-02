package database

import (
	"database/sql"
	"fmt"
	"log"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"api/internal/platform/config"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func NewPostgres(cfg config.DatabaseConfig) (*sql.DB, error) {
	dsn := buildPostgresDSN(cfg)
	maxRetries := getEnvInt("DB_CONNECT_MAX_RETRIES", 60)
	retryInterval := time.Duration(getEnvInt("DB_CONNECT_RETRY_INTERVAL_MS", 1000)) * time.Millisecond

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
		log.Printf("[Database] PostgreSQL connect attempt %d/%d failed: %v; retrying in %s", attempt, maxRetries, err, retryInterval)
		time.Sleep(retryInterval)
	}
	return nil, lastErr
}

func buildPostgresDSN(cfg config.DatabaseConfig) string {
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

func getEnvInt(key string, defaultValue int) int {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return defaultValue
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return defaultValue
	}
	return value
}
