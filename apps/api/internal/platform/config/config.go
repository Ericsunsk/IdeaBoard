package config

import (
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/joho/godotenv"
)

type Config struct {
	Database DatabaseConfig
	Redis    RedisConfig
	Queue    QueueConfig
	App      AppConfig
	Auth     AuthServiceConfig
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
	Port    string
	Host    string
	Env     string
	GinMode string
}

type QueueConfig struct {
	WorkerConcurrency       int
	AsynqTaskTimeoutSeconds int
	AsynqNoCapacityDelaySec int
	AsynqMaxRetry           int
}

type AuthServiceConfig struct {
	BaseURL        string
	InternalAPIKey string
	TimeoutSeconds int
}

var loadEnvOnce sync.Once

func Load() *Config {
	loadEnvFile()

	dbPassword := getEnv("DB_PASSWORD", "")
	if dbPassword == "" {
		log.Println("[Config] WARNING: DB_PASSWORD 未设置，数据库连接可能失败")
	}

	appEnv := detectAppEnv()
	ginMode := detectGinMode()

	return &Config{
		Database: DatabaseConfig{
			Host:     getEnv("DB_HOST", "localhost"),
			Port:     getEnv("DB_PORT", "3306"),
			User:     getEnv("DB_USER", "ideaboard"),
			Password: dbPassword,
			Database: getEnv("DB_NAME", "ideaboard"),
		},
		Redis: RedisConfig{
			Host: getEnv("REDIS_HOST", "localhost"),
			Port: getEnv("REDIS_PORT", "6379"),
			Auth: getEnv("REDIS_AUTH", ""),
		},
		Queue: QueueConfig{
			WorkerConcurrency:       getEnvInt("TASK_QUEUE_WORKER_CONCURRENCY", 5),
			AsynqTaskTimeoutSeconds: getEnvInt("ASYNQ_TASK_TIMEOUT_SECONDS", 180),
			AsynqNoCapacityDelaySec: getEnvInt("ASYNQ_NO_CAPACITY_RETRY_SECONDS", 5),
			AsynqMaxRetry:           getEnvInt("ASYNQ_MAX_RETRY", 25),
		},
		App: AppConfig{
			Port:    getEnv("APP_PORT", "6000"),
			Host:    getFirstEnv("APP_HOST", "APP_BIND_HOST"),
			Env:     appEnv,
			GinMode: ginMode,
		},
		Auth: AuthServiceConfig{
			BaseURL:        getEnv("AUTH_SERVICE_BASE_URL", "http://127.0.0.1:6001"),
			InternalAPIKey: strings.TrimSpace(getEnv("AUTH_SERVICE_INTERNAL_KEY", "")),
			TimeoutSeconds: getEnvInt("AUTH_SERVICE_TIMEOUT_SECONDS", 5),
		},
	}
}

func detectAppEnv() string {
	for _, key := range []string{"APP_ENV", "GO_ENV", "ENV"} {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			return value
		}
	}
	if ginMode := strings.TrimSpace(os.Getenv("GIN_MODE")); ginMode != "" {
		return ginMode
	}
	return "development"
}

func detectGinMode() string {
	raw := strings.TrimSpace(getFirstEnv("APP_GIN_MODE", "GIN_MODE"))
	if raw == "" {
		// Default to release so local startup keeps request logging but skips Gin's route dump.
		return "release"
	}

	switch strings.ToLower(raw) {
	case "debug", "release", "test":
		return strings.ToLower(raw)
	default:
		log.Printf("[Config] WARNING: APP_GIN_MODE/GIN_MODE=%q 无效，已回退为 release", raw)
		return "release"
	}
}

func isProductionAppEnv(env string) bool {
	switch strings.ToLower(strings.TrimSpace(env)) {
	case "prod", "production", "release":
		return true
	default:
		return false
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

func getFirstEnv(keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			return value
		}
	}
	return ""
}

func getEnvInt(key string, defaultValue int) int {
	raw := strings.TrimSpace(getEnv(key, ""))
	if raw == "" {
		return defaultValue
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		log.Printf("[Config] WARNING: %s=%q 不是有效正整数，已回退默认值 %d", key, raw, defaultValue)
		return defaultValue
	}
	return value
}
