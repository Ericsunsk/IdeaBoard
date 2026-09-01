package config

import (
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/caarlos0/env/v11"
	"github.com/joho/godotenv"
)

type Config struct {
	Database      DatabaseConfig
	Events        EventBusConfig
	App           AppConfig
	PublicOrigins PublicOriginsConfig
	Auth          AuthServiceConfig
	Worker        WorkerConfig
	Credits       CreditsConfig
	Rewards       RewardsConfig
}

type CreditsConfig struct {
	WelcomeIdentityKey string
}

type RewardsConfig struct {
	CodeHMACKeys string
}

type DatabaseConfig struct {
	Host              string
	Port              string
	User              string
	Password          string
	MigrationUser     string
	MigrationPassword string
	Database          string
	SSLMode           string
}

type AppConfig struct {
	Port               string
	Host               string
	MetricsAddr        string
	Env                string
	CORSAllowedOrigins []string
}

// PublicOriginsConfig is immutable deployment configuration. Web owns browser
// navigation URLs; API owns webhooks and locally served assets. API defaults
// to the public origin at load time, while the separate fields preserve URL
// semantics and allow an explicit API host override when needed.
type PublicOriginsConfig struct {
	Web string
	API string
}

func (c PublicOriginsConfig) Validate() error {
	if err := validatePublicOrigin("PUBLIC_ORIGIN", c.Web); err != nil {
		return err
	}
	apiOrigin := c.API
	if strings.TrimSpace(apiOrigin) == "" {
		apiOrigin = c.Web
	}
	return validatePublicOrigin("API_BASE_URL", apiOrigin)
}

func (c PublicOriginsConfig) WebURL(path string) string {
	if validatePublicOrigin("PUBLIC_ORIGIN", c.Web) != nil {
		return ""
	}
	return joinPublicOrigin(c.Web, path)
}

func (c PublicOriginsConfig) APIURL(path string) string {
	origin := c.API
	if strings.TrimSpace(origin) == "" {
		origin = c.Web
	}
	if validatePublicOrigin("API_BASE_URL", origin) != nil {
		return ""
	}
	return joinPublicOrigin(origin, path)
}

func (c PublicOriginsConfig) APIParsedURL() *url.URL {
	origin := c.API
	if strings.TrimSpace(origin) == "" {
		origin = c.Web
	}
	if validatePublicOrigin("API_BASE_URL", origin) != nil {
		return nil
	}
	parsed, err := url.Parse(origin)
	if err != nil {
		return nil
	}
	return parsed
}

type WorkerConfig struct {
	Concurrency int
}

const (
	DefaultRiverWorkerConcurrency = 5
	minRiverWorkerConcurrency     = 1
	maxRiverWorkerConcurrency     = 64
)

func (c WorkerConfig) EffectiveConcurrency() int {
	if c.Concurrency < minRiverWorkerConcurrency || c.Concurrency > maxRiverWorkerConcurrency {
		return DefaultRiverWorkerConcurrency
	}
	return c.Concurrency
}

type EventBusConfig struct {
	Enabled            bool
	NATSURL            string
	NATSStreamName     string
	NATSSubjectPrefix  string
	NATSConsumerName   string
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
	DBHost              string `env:"DB_HOST"`
	DBPort              string `env:"DB_PORT"`
	DBUser              string `env:"DB_USER"`
	DBPassword          string `env:"DB_PASSWORD"`
	DBMigrationUser     string `env:"DB_MIGRATION_USER"`
	DBMigrationPassword string `env:"DB_MIGRATION_PASSWORD"`
	DBName              string `env:"DB_NAME"`
	DBSSLMode           string `env:"DB_SSLMODE"`
	NATSEventBusEnabled string `env:"NATS_EVENT_BUS_ENABLED"`
	NATSURL             string `env:"NATS_URL"`
	RiverConcurrency    string `env:"RIVER_WORKER_CONCURRENCY"`

	AppPort               string `env:"APP_PORT"`
	AppHost               string `env:"APP_HOST"`
	MetricsAddr           string `env:"METRICS_ADDR"`
	AppEnv                string `env:"APP_ENV"`
	AppCORSAllowedOrigins string `env:"APP_CORS_ALLOWED_ORIGINS"`
	PublicOrigin          string `env:"PUBLIC_ORIGIN"`
	APIBaseURL            string `env:"API_BASE_URL"`

	AuthServiceBaseURL        string `env:"AUTH_SERVICE_BASE_URL"`
	AuthServiceInternalAPIKey string `env:"AUTH_INTERNAL_KEY"`
	CreditsWelcomeIdentityKey string `env:"CREDITS_WELCOME_IDENTITY_KEY"`
	RewardsCodeHMACKeys       string `env:"REWARDS_CODE_HMAC_KEYS"`
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
	natsURL := strings.TrimSpace(raw.NATSURL)

	publicOrigin := normalizePublicOrigin(raw.PublicOrigin)
	apiOrigin := normalizePublicOrigin(raw.APIBaseURL)
	if apiOrigin == "" {
		apiOrigin = publicOrigin
	}

	return &Config{
		Database: DatabaseConfig{
			Host:              stringEnvDefault(raw.DBHost, "localhost"),
			Port:              stringEnvDefault(raw.DBPort, "5432"),
			User:              stringEnvDefault(raw.DBUser, "ideaboard"),
			Password:          raw.DBPassword,
			MigrationUser:     strings.TrimSpace(raw.DBMigrationUser),
			MigrationPassword: raw.DBMigrationPassword,
			Database:          stringEnvDefault(raw.DBName, "ideaboard"),
			SSLMode:           stringEnvDefault(raw.DBSSLMode, "disable"),
		},
		Events: EventBusConfig{
			Enabled: parseBoolEnvDefault("NATS_EVENT_BUS_ENABLED", raw.NATSEventBusEnabled, natsURL != ""),
			NATSURL: natsURL,
		},
		App: AppConfig{
			Port:               stringEnvDefault(raw.AppPort, "6000"),
			Host:               strings.TrimSpace(raw.AppHost),
			MetricsAddr:        stringEnvDefault(raw.MetricsAddr, "127.0.0.1:9091"),
			Env:                appEnv,
			CORSAllowedOrigins: detectCORSAllowedOrigins(raw.AppCORSAllowedOrigins, appEnv),
		},
		PublicOrigins: PublicOriginsConfig{
			Web: publicOrigin,
			API: apiOrigin,
		},
		Auth: AuthServiceConfig{
			BaseURL:        stringEnvDefault(raw.AuthServiceBaseURL, "http://127.0.0.1:6001"),
			InternalAPIKey: strings.TrimSpace(raw.AuthServiceInternalAPIKey),
			TimeoutSeconds: 5,
		},
		Worker: WorkerConfig{Concurrency: boundedIntEnvDefault(
			"RIVER_WORKER_CONCURRENCY",
			raw.RiverConcurrency,
			DefaultRiverWorkerConcurrency,
			minRiverWorkerConcurrency,
			maxRiverWorkerConcurrency,
		)},
		Credits: CreditsConfig{WelcomeIdentityKey: strings.TrimSpace(raw.CreditsWelcomeIdentityKey)},
		Rewards: RewardsConfig{CodeHMACKeys: strings.TrimSpace(raw.RewardsCodeHMACKeys)},
	}
}

// LoadAndValidate loads deployment configuration and rejects unsafe database
// settings before any connection attempt. Development keeps the local
// DB_SSLMODE=disable default; production/release requires an explicit,
// non-placeholder DB_PASSWORD and an allowlisted SSL mode.
func LoadAndValidate() (*Config, error) {
	cfg := Load()
	if err := cfg.ValidateDatabase(); err != nil {
		return nil, err
	}
	return cfg, nil
}

func (c *Config) ValidateDatabase() error {
	if c == nil {
		return fmt.Errorf("config is required")
	}
	if err := validateDatabaseSSLMode(c.Database.SSLMode); err != nil {
		return err
	}
	if !isProductionAppEnv(c.App.Env) {
		return nil
	}
	if isUnsafeProductionSecret(c.Database.Password) {
		return fmt.Errorf("DB_PASSWORD must be set to a non-placeholder value when APP_ENV=%s", c.App.Env)
	}
	return nil
}

func validateDatabaseSSLMode(value string) error {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "disable", "require", "verify-full":
		return nil
	default:
		return fmt.Errorf("DB_SSLMODE must be one of disable, require, or verify-full")
	}
}

func isUnsafeProductionSecret(value string) bool {
	normalized := strings.ToLower(strings.TrimSpace(value))
	if normalized == "" || strings.HasPrefix(normalized, "replace-with") || strings.HasPrefix(normalized, "replace_with") ||
		strings.Contains(normalized, "change-before-production") || strings.Contains(normalized, "placeholder") {
		return true
	}
	switch normalized {
	case "changeme", "change-me", "password", "secret":
		return true
	default:
		return false
	}
}

func validatePublicOrigin(name, raw string) error {
	if strings.TrimSpace(raw) == "" {
		return fmt.Errorf("%s is required", name)
	}
	parsed, err := url.Parse(raw)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" ||
		parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || (parsed.Path != "" && parsed.Path != "/") {
		return fmt.Errorf("%s must be an http(s) origin without path, credentials, query, or fragment", name)
	}
	return nil
}

func normalizePublicOrigin(raw string) string {
	return strings.TrimRight(strings.TrimSpace(raw), "/")
}

func joinPublicOrigin(origin, targetPath string) string {
	return strings.TrimRight(origin, "/") + "/" + strings.TrimLeft(targetPath, "/")
}

func normalizeAppEnv(raw string) string {
	return stringEnvDefault(raw, "development")
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

func boundedIntEnvDefault(key, raw string, defaultValue, minValue, maxValue int) int {
	value := strings.TrimSpace(raw)
	if value == "" {
		return defaultValue
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < minValue || parsed > maxValue {
		slog.Warn("invalid integer env; fallback to default", "key", key, "value", value, "default", defaultValue)
		return defaultValue
	}
	return parsed
}
