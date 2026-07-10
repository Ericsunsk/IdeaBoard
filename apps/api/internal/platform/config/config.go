package config

import (
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/caarlos0/env/v11"
	"github.com/joho/godotenv"
)

type Config struct {
	Database DatabaseConfig
	Queue    QueueConfig
	Events   EventBusConfig
	App      AppConfig
	Auth     AuthServiceConfig
	Credits  CreditsConfig
}

type CreditsConfig struct {
	WelcomeIdentityKey string
}

type DatabaseConfig struct {
	Host                   string
	Port                   string
	User                   string
	Password               string
	MigrationUser          string
	MigrationPassword      string
	Database               string
	SSLMode                string
	ConnectMaxRetries      int
	ConnectRetryIntervalMS int
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

type envConfig struct {
	DBHost                   string `env:"DB_HOST"`
	DBPort                   string `env:"DB_PORT"`
	DBUser                   string `env:"DB_USER"`
	DBPassword               string `env:"DB_PASSWORD"`
	DBMigrationUser          string `env:"DB_MIGRATION_USER"`
	DBMigrationPassword      string `env:"DB_MIGRATION_PASSWORD"`
	DBName                   string `env:"DB_NAME"`
	DBSSLMode                string `env:"DB_SSLMODE"`
	DBConnectMaxRetries      string `env:"DB_CONNECT_MAX_RETRIES"`
	DBConnectRetryIntervalMS string `env:"DB_CONNECT_RETRY_INTERVAL_MS"`

	RiverWorkerConcurrency           string `env:"RIVER_WORKER_CONCURRENCY"`
	RiverTaskTimeoutSeconds          string `env:"RIVER_TASK_TIMEOUT_SECONDS"`
	RiverNoCapacityRetryDelaySeconds string `env:"RIVER_NO_CAPACITY_RETRY_SECONDS"`
	RiverMaxRetry                    string `env:"RIVER_MAX_RETRY"`

	NATSEventBusEnabled string `env:"NATS_EVENT_BUS_ENABLED"`
	NATSURL             string `env:"NATS_URL"`
	NATSStreamName      string `env:"NATS_STREAM_NAME"`
	NATSSubjectPrefix   string `env:"NATS_SUBJECT_PREFIX"`
	NATSConsumerName    string `env:"NATS_CONSUMER_NAME"`
	NATSRetentionDays   string `env:"NATS_RETENTION_DAYS"`
	NATSConnectTimeout  string `env:"NATS_CONNECT_TIMEOUT_SECONDS"`
	NATSRequestTimeout  string `env:"NATS_REQUEST_TIMEOUT_SECONDS"`
	NATSDrainTimeout    string `env:"NATS_DRAIN_TIMEOUT_SECONDS"`

	AppPort               string `env:"APP_PORT"`
	AppHost               string `env:"APP_HOST"`
	AppEnv                string `env:"APP_ENV"`
	AppGinMode            string `env:"APP_GIN_MODE"`
	AppLogFile            string `env:"APP_LOG_FILE"`
	AppCORSAllowedOrigins string `env:"APP_CORS_ALLOWED_ORIGINS"`

	AuthServiceBaseURL        string `env:"AUTH_SERVICE_BASE_URL"`
	AuthServiceInternalAPIKey string `env:"AUTH_INTERNAL_KEY"`
	AuthServiceTimeoutSeconds string `env:"AUTH_SERVICE_TIMEOUT_SECONDS"`
	CreditsWelcomeIdentityKey string `env:"CREDITS_WELCOME_IDENTITY_KEY"`
}

var loadEnvOnce sync.Once

func Load() *Config {
	loadEnvFile()

	raw, err := env.ParseAs[envConfig]()
	if err != nil {
		slog.Warn("failed to parse environment config; using defaults where possible", "error", err)
	}

	raw.DBPassword = strings.TrimSpace(raw.DBPassword)
	if raw.DBPassword == "" {
		slog.Warn("DB_PASSWORD is not set; database connection may fail")
	}
	raw.DBMigrationPassword = strings.TrimSpace(raw.DBMigrationPassword)

	appEnv := normalizeAppEnv(raw.AppEnv)
	ginMode := normalizeGinMode(raw.AppGinMode)
	natsURL := strings.TrimSpace(raw.NATSURL)

	return &Config{
		Database: DatabaseConfig{
			Host:                   stringEnvDefault(raw.DBHost, "localhost"),
			Port:                   stringEnvDefault(raw.DBPort, "5432"),
			User:                   stringEnvDefault(raw.DBUser, "ideaboard"),
			Password:               raw.DBPassword,
			MigrationUser:          strings.TrimSpace(raw.DBMigrationUser),
			MigrationPassword:      raw.DBMigrationPassword,
			Database:               stringEnvDefault(raw.DBName, "ideaboard"),
			SSLMode:                stringEnvDefault(raw.DBSSLMode, "disable"),
			ConnectMaxRetries:      parsePositiveIntEnv("DB_CONNECT_MAX_RETRIES", raw.DBConnectMaxRetries, 60),
			ConnectRetryIntervalMS: parsePositiveIntEnv("DB_CONNECT_RETRY_INTERVAL_MS", raw.DBConnectRetryIntervalMS, 1000),
		},
		Queue: QueueConfig{
			WorkerConcurrency:           parsePositiveIntEnv("RIVER_WORKER_CONCURRENCY", raw.RiverWorkerConcurrency, 5),
			TaskTimeoutSeconds:          parsePositiveIntEnv("RIVER_TASK_TIMEOUT_SECONDS", raw.RiverTaskTimeoutSeconds, 180),
			NoCapacityRetryDelaySeconds: parsePositiveIntEnv("RIVER_NO_CAPACITY_RETRY_SECONDS", raw.RiverNoCapacityRetryDelaySeconds, 5),
			MaxRetry:                    parsePositiveIntEnv("RIVER_MAX_RETRY", raw.RiverMaxRetry, 25),
		},
		Events: EventBusConfig{
			Enabled:            parseBoolEnvDefault("NATS_EVENT_BUS_ENABLED", raw.NATSEventBusEnabled, natsURL != ""),
			NATSURL:            natsURL,
			NATSStreamName:     stringEnvDefault(raw.NATSStreamName, "IDEABOARD_EVENTS"),
			NATSSubjectPrefix:  stringEnvDefault(raw.NATSSubjectPrefix, "ideaboard"),
			NATSConsumerName:   strings.TrimSpace(raw.NATSConsumerName),
			NATSRetentionDays:  parsePositiveIntEnv("NATS_RETENTION_DAYS", raw.NATSRetentionDays, 7),
			NATSConnectTimeout: parsePositiveIntEnv("NATS_CONNECT_TIMEOUT_SECONDS", raw.NATSConnectTimeout, 5),
			NATSRequestTimeout: parsePositiveIntEnv("NATS_REQUEST_TIMEOUT_SECONDS", raw.NATSRequestTimeout, 5),
			NATSDrainTimeout:   parsePositiveIntEnv("NATS_DRAIN_TIMEOUT_SECONDS", raw.NATSDrainTimeout, 2),
		},
		App: AppConfig{
			Port:               stringEnvDefault(raw.AppPort, "6000"),
			Host:               strings.TrimSpace(raw.AppHost),
			Env:                appEnv,
			GinMode:            ginMode,
			LogFile:            stringEnvDefault(raw.AppLogFile, "server.log"),
			CORSAllowedOrigins: detectCORSAllowedOrigins(raw.AppCORSAllowedOrigins, appEnv),
		},
		Auth: AuthServiceConfig{
			BaseURL:        stringEnvDefault(raw.AuthServiceBaseURL, "http://127.0.0.1:6001"),
			InternalAPIKey: strings.TrimSpace(raw.AuthServiceInternalAPIKey),
			TimeoutSeconds: parsePositiveIntEnv("AUTH_SERVICE_TIMEOUT_SECONDS", raw.AuthServiceTimeoutSeconds, 5),
		},
		Credits: CreditsConfig{WelcomeIdentityKey: strings.TrimSpace(raw.CreditsWelcomeIdentityKey)},
	}
}

func normalizeAppEnv(raw string) string {
	return stringEnvDefault(raw, "development")
}

func normalizeGinMode(raw string) string {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "release"
	}

	switch strings.ToLower(value) {
	case "debug", "release", "test":
		return strings.ToLower(value)
	default:
		slog.Warn("invalid APP_GIN_MODE; fallback to release", "value", value)
		return "release"
	}
}

func detectCORSAllowedOrigins(raw, appEnv string) []string {
	origins := splitCSVEnv(raw)
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

func stringEnvDefault(raw, defaultValue string) string {
	if value := strings.TrimSpace(raw); value != "" {
		return value
	}
	return defaultValue
}

func parsePositiveIntEnv(key, raw string, defaultValue int) int {
	value := strings.TrimSpace(raw)
	if value == "" {
		return defaultValue
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		slog.Warn("invalid positive integer env; fallback to default", "key", key, "value", value, "default", defaultValue)
		return defaultValue
	}
	return parsed
}

func parseBoolEnvDefault(key, raw string, defaultValue bool) bool {
	value := strings.TrimSpace(raw)
	if value == "" {
		return defaultValue
	}
	switch strings.ToLower(value) {
	case "1", "true", "t", "yes", "y", "on":
		return true
	case "0", "false", "f", "no", "n", "off":
		return false
	default:
		slog.Warn("invalid boolean env; fallback to default", "key", key, "value", value, "default", defaultValue)
		return defaultValue
	}
}
