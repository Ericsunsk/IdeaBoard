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
	Queue    QueueConfig
	Events   EventBusConfig
	App      AppConfig
	Auth     AuthServiceConfig
}

type DatabaseConfig struct {
	Host     string
	Port     string
	User     string
	Password string
	Database string
	SSLMode  string
}

type AppConfig struct {
	Port               string
	Host               string
	Env                string
	GinMode            string
	LogFile            string
	CORSAllowedOrigins []string
}

type QueueConfig struct {
	WorkerConcurrency           int
	TaskTimeoutSeconds          int
	NoCapacityRetryDelaySeconds int
	MaxRetry                    int
}

type EventBusConfig struct {
	Enabled            bool
	NATSURL            string
	NATSStreamName     string
	NATSSubjectPrefix  string
	NATSConsumerName   string
	NATSRetentionDays  int
	NATSConnectTimeout int
	NATSRequestTimeout int
	NATSDrainTimeout   int
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
	natsURL := strings.TrimSpace(getEnv("NATS_URL", ""))

	return &Config{
		Database: DatabaseConfig{
			Host:     getEnv("DB_HOST", "localhost"),
			Port:     getEnv("DB_PORT", "5432"),
			User:     getEnv("DB_USER", "ideaboard"),
			Password: dbPassword,
			Database: getEnv("DB_NAME", "ideaboard"),
			SSLMode:  getEnv("DB_SSLMODE", "disable"),
		},
		Queue: QueueConfig{
			WorkerConcurrency:           getEnvInt("TASK_QUEUE_WORKER_CONCURRENCY", 5),
			TaskTimeoutSeconds:          getFirstEnvInt(180, "RIVER_TASK_TIMEOUT_SECONDS", "TASK_QUEUE_TASK_TIMEOUT_SECONDS"),
			NoCapacityRetryDelaySeconds: getFirstEnvInt(5, "RIVER_NO_CAPACITY_RETRY_SECONDS", "TASK_QUEUE_NO_CAPACITY_RETRY_SECONDS"),
			MaxRetry:                    getFirstEnvInt(25, "RIVER_MAX_RETRY", "TASK_QUEUE_MAX_RETRY"),
		},
		Events: EventBusConfig{
			Enabled:            getEnvBoolDefault("NATS_EVENT_BUS_ENABLED", natsURL != ""),
			NATSURL:            natsURL,
			NATSStreamName:     getEnv("NATS_STREAM_NAME", "IDEABOARD_EVENTS"),
			NATSSubjectPrefix:  getEnv("NATS_SUBJECT_PREFIX", "ideaboard"),
			NATSConsumerName:   getEnv("NATS_CONSUMER_NAME", ""),
			NATSRetentionDays:  getEnvInt("NATS_RETENTION_DAYS", 7),
			NATSConnectTimeout: getEnvInt("NATS_CONNECT_TIMEOUT_SECONDS", 5),
			NATSRequestTimeout: getEnvInt("NATS_REQUEST_TIMEOUT_SECONDS", 5),
			NATSDrainTimeout:   getEnvInt("NATS_DRAIN_TIMEOUT_SECONDS", 2),
		},
		App: AppConfig{
			Port:               getEnv("APP_PORT", "6000"),
			Host:               getFirstEnv("APP_HOST", "APP_BIND_HOST"),
			Env:                appEnv,
			GinMode:            ginMode,
			LogFile:            getEnv("APP_LOG_FILE", "server.log"),
			CORSAllowedOrigins: detectCORSAllowedOrigins(appEnv),
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

func detectCORSAllowedOrigins(appEnv string) []string {
	origins := splitCSVEnv(getEnv("APP_CORS_ALLOWED_ORIGINS", ""))
	if len(origins) > 0 || isProductionAppEnv(appEnv) {
		return origins
	}
	return []string{
		"http://localhost:5173",
		"http://127.0.0.1:5173",
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

func splitCSVEnv(raw string) []string {
	parts := strings.Split(raw, ",")
	result := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for _, part := range parts {
		value := strings.TrimSpace(part)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
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

func getFirstEnvInt(defaultValue int, keys ...string) int {
	for _, key := range keys {
		raw := strings.TrimSpace(os.Getenv(key))
		if raw == "" {
			continue
		}
		value, err := strconv.Atoi(raw)
		if err != nil || value <= 0 {
			log.Printf("[Config] WARNING: %s=%q 不是有效正整数，已回退默认值 %d", key, raw, defaultValue)
			return defaultValue
		}
		return value
	}
	return defaultValue
}

func getEnvBoolDefault(key string, defaultValue bool) bool {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return defaultValue
	}
	switch strings.ToLower(raw) {
	case "1", "true", "t", "yes", "y", "on":
		return true
	case "0", "false", "f", "no", "n", "off":
		return false
	default:
		log.Printf("[Config] WARNING: %s=%q 不是有效布尔值，已回退默认值 %t", key, raw, defaultValue)
		return defaultValue
	}
}
