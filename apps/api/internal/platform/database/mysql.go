package database

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"api/internal/platform/config"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func NewMySQL(cfg config.DatabaseConfig) (*gorm.DB, error) {
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=True&loc=Local",
		cfg.User,
		cfg.Password,
		cfg.Host,
		cfg.Port,
		cfg.Database,
	)

	logMode := logger.Warn
	switch strings.ToLower(os.Getenv("DB_LOG_MODE")) {
	case "info":
		logMode = logger.Info
	case "error":
		logMode = logger.Error
	case "silent":
		logMode = logger.Silent
	}

	maxRetries := getEnvInt("DB_CONNECT_MAX_RETRIES", 60)
	retryInterval := time.Duration(getEnvInt("DB_CONNECT_RETRY_INTERVAL_MS", 1000)) * time.Millisecond

	var lastErr error
	for attempt := 1; attempt <= maxRetries; attempt++ {
		db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{
			Logger: logger.Default.LogMode(logMode),
		})
		if err == nil {
			sqlDB, dbErr := db.DB()
			if dbErr == nil {
				if pingErr := sqlDB.Ping(); pingErr == nil {
					return db, nil
				} else {
					err = pingErr
				}
			} else {
				err = dbErr
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
