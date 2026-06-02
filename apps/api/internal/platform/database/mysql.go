package database

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"api/internal/platform/config"

	_ "github.com/go-sql-driver/mysql"
)

func NewMySQL(cfg config.DatabaseConfig) (*sql.DB, error) {
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=True&loc=Local",
		cfg.User,
		cfg.Password,
		cfg.Host,
		cfg.Port,
		cfg.Database,
	)

	maxRetries := getEnvInt("DB_CONNECT_MAX_RETRIES", 60)
	retryInterval := time.Duration(getEnvInt("DB_CONNECT_RETRY_INTERVAL_MS", 1000)) * time.Millisecond

	var lastErr error
	for attempt := 1; attempt <= maxRetries; attempt++ {
		db, err := sql.Open("mysql", dsn)
		if err == nil {
			db.SetMaxOpenConns(25)
			db.SetMaxIdleConns(10)
			db.SetConnMaxLifetime(5 * time.Minute)
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
		log.Printf("[Database] MySQL connect attempt %d/%d failed: %v; retrying in %s", attempt, maxRetries, err, retryInterval)
		time.Sleep(retryInterval)
	}
	return nil, lastErr
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
