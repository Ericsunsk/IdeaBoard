package config

import (
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/joho/godotenv"
)

type Config struct {
	Database DatabaseConfig
	Redis    RedisConfig
	App      AppConfig
}

type DatabaseConfig struct {
	Host     string
	Port     string
	User     string
	Password string
	Database string
}

type RedisConfig struct {
	Host string
	Port string
	Auth string
}

type AppConfig struct {
	Port string
}

var loadEnvOnce sync.Once

func Load() *Config {
	loadEnvFile()

	return &Config{
		Database: DatabaseConfig{
			Host:     getEnv("DB_HOST", "localhost"),
			Port:     getEnv("DB_PORT", "3306"),
			User:     getEnv("DB_USER", "root"),
			Password: getEnv("DB_PASSWORD", "root123"),
			Database: getEnv("DB_NAME", "feng"),
		},
		Redis: RedisConfig{
			Host: getEnv("REDIS_HOST", "localhost"),
			Port: getEnv("REDIS_PORT", "6379"),
			Auth: getEnv("REDIS_AUTH", ""),
		},
		App: AppConfig{
			Port: getEnv("APP_PORT", "6000"),
		},
	}
}

func loadEnvFile() {
	loadEnvOnce.Do(func() {
		candidates := make([]string, 0, 3)

		if envFile := strings.TrimSpace(os.Getenv("ENV_FILE")); envFile != "" {
			candidates = append(candidates, envFile)
		}

		candidates = append(candidates, ".env")

		if exePath, err := os.Executable(); err == nil {
			candidates = append(candidates, filepath.Join(filepath.Dir(exePath), ".env"))
		}

		for _, path := range candidates {
			if path == "" {
				continue
			}
			if _, err := os.Stat(path); err == nil {
				_ = godotenv.Load(path)
				return
			}
		}
	})
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
