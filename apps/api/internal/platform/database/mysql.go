package database

import (
	"fmt"
	"os"
	"strings"

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

	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logMode),
	})
	if err != nil {
		return nil, err
	}

	return db, nil
}
