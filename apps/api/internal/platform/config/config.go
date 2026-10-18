package config

import (
	"fmt"
	"log/slog"
	"net"
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
	Storage       StorageConfig
	Imgproxy      ImgproxyConfig
	// AdminSeed carries the init-admin provisioning inputs. They are
	// command-scoped (only the init-admin command requires them), so
	// Validate leaves them unchecked; the command fails fast instead.
	AdminSeed AdminSeedConfig
	// Migration carries the Atlas tooling knobs for the migrate path.
	// Empty values resolve to DefaultAtlasBinary/DefaultMigrationDirectory.
	Migration MigrationToolingConfig
	// Secrets carries the raw at-rest encryption key material. Presence is
	// enforced by Validate; format (base64, 32 bytes) stays with the
	// secrets package so there is a single format validator.
	Secrets SecretsConfig
	// InvalidEnv lists env keys whose raw values failed parsing and fell
	// back to defaults (e.g. RIVER_WORKER_CONCURRENCY="lots"). Development
	// tolerates them with a warning; LoadAndValidate rejects them in
	// production so misconfiguration cannot silently run with defaults.
	InvalidEnv []string
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
	Pool              PoolConfig
}

// PoolConfig bounds the runtime pgxpool and the River pgxpool. Values are
// deploy-level tuning (see docs/runbooks/deployment.md): the combined
// runtime MaxOpenConns + River MaxConns of one API instance must stay
// under PostgreSQL max_connections with headroom for migrations and CLIs.
// Out-of-range values fall back to defaults and are recorded in
// Config.InvalidEnv so production startup fails fast instead of silently
// running with defaults.
type PoolConfig struct {
	MaxOpenConns   int
	MaxIdleConns   int
	MaxLifetimeMin int
	MaxIdleTimeMin int
	RiverMaxConns  int
	RiverMinConns  int
}

const (
	DefaultPoolMaxOpenConns   = 25
	DefaultPoolMaxIdleConns   = 10
	DefaultPoolMaxLifetimeMin = 30
	DefaultPoolMaxIdleTimeMin = 5
	DefaultRiverPoolMaxConns  = 10
	DefaultRiverPoolMinConns  = 1
)

const (
	minPoolMaxOpenConns  = 1
	maxPoolMaxOpenConns  = 64
	minPoolMaxIdleConns  = 0
	maxPoolMaxIdleConns  = 64
	minPoolLifetimeMin   = 1
	maxPoolLifetimeMin   = 1440
	minPoolIdleTimeMin   = 1
	maxPoolIdleTimeMin   = 1440
	minRiverPoolMaxConns = 1
	maxRiverPoolMaxConns = 32
	minRiverPoolMinConns = 1
	maxRiverPoolMinConns = 16
	// Leave room on the default PostgreSQL max_connections=100 for migrations,
	// CLIs and health tooling; runtime and River pools may consume at most 80.
	maxRuntimePoolBudget = 80
)

// EffectiveMaxOpenConns clamps the configured ceiling into the supported
// range, falling back to the default for unset or invalid values.
func (c PoolConfig) EffectiveMaxOpenConns() int {
	if c.MaxOpenConns < minPoolMaxOpenConns || c.MaxOpenConns > maxPoolMaxOpenConns {
		return DefaultPoolMaxOpenConns
	}
	return c.MaxOpenConns
}

// EffectiveMaxIdleConns clamps idle connections and never exceeds the
// effective open ceiling, so a misconfigured idle count cannot reserve more
// than the pool allows.
func (c PoolConfig) EffectiveMaxIdleConns() int {
	maxOpen := c.EffectiveMaxOpenConns()
	if c.MaxIdleConns < minPoolMaxIdleConns || c.MaxIdleConns > maxPoolMaxIdleConns {
		return min(DefaultPoolMaxIdleConns, maxOpen)
	}
	if c.MaxIdleConns > maxOpen {
		return maxOpen
	}
	return c.MaxIdleConns
}

// EffectiveMaxLifetimeMin bounds connection recycling age in minutes.
func (c PoolConfig) EffectiveMaxLifetimeMin() int {
	if c.MaxLifetimeMin < minPoolLifetimeMin || c.MaxLifetimeMin > maxPoolLifetimeMin {
		return DefaultPoolMaxLifetimeMin
	}
	return c.MaxLifetimeMin
}

// EffectiveMaxIdleTimeMin bounds idle recycling age in minutes.
func (c PoolConfig) EffectiveMaxIdleTimeMin() int {
	if c.MaxIdleTimeMin < minPoolIdleTimeMin || c.MaxIdleTimeMin > maxPoolIdleTimeMin {
		return DefaultPoolMaxIdleTimeMin
	}
	return c.MaxIdleTimeMin
}

// EffectiveRiverMaxConns clamps the River pgxpool ceiling.
func (c PoolConfig) EffectiveRiverMaxConns() int {
	if c.RiverMaxConns < minRiverPoolMaxConns || c.RiverMaxConns > maxRiverPoolMaxConns {
		return DefaultRiverPoolMaxConns
	}
	return c.RiverMaxConns
}

// EffectiveRiverMinConns clamps the River pgxpool floor and never exceeds
// the effective ceiling.
func (c PoolConfig) EffectiveRiverMinConns() int {
	maxConns := c.EffectiveRiverMaxConns()
	if c.RiverMinConns < minRiverPoolMinConns || c.RiverMinConns > maxRiverPoolMinConns {
		return DefaultRiverPoolMinConns
	}
	if c.RiverMinConns > maxConns {
		return maxConns
	}
	return c.RiverMinConns
}

type AppConfig struct {
	Port               string
	Host               string
	MetricsAddr        string
	Env                string
	CORSAllowedOrigins []string
	// TrustedProxies are the reverse proxies (Nginx, Cloudflare, load
	// balancer) whose X-Forwarded-For headers Gin may trust for ClientIP.
	// ClientIP feeds per-IP rate limiting, so an open trust breaks abuse
	// control via header spoofing. Defaults to loopback only; production
	// behind a proxy must set APP_TRUSTED_PROXIES explicitly.
	TrustedProxies []string
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
	// TaskRetentionDays bounds raw task row lifetime; statistics aggregates
	// and financial facts are retained independently of this window.
	TaskRetentionDays int
}

const (
	DefaultRiverWorkerConcurrency = 5
	minRiverWorkerConcurrency     = 1
	maxRiverWorkerConcurrency     = 64
)

const (
	DefaultTaskRetentionDays = 7
	minTaskRetentionDays     = 1
	maxTaskRetentionDays     = 90
)

func (c WorkerConfig) EffectiveConcurrency() int {
	if c.Concurrency < minRiverWorkerConcurrency || c.Concurrency > maxRiverWorkerConcurrency {
		return DefaultRiverWorkerConcurrency
	}
	return c.Concurrency
}

func (c WorkerConfig) EffectiveTaskRetentionDays() int {
	if c.TaskRetentionDays < minTaskRetentionDays || c.TaskRetentionDays > maxTaskRetentionDays {
		return DefaultTaskRetentionDays
	}
	return c.TaskRetentionDays
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

const DefaultAuthServiceTimeoutSeconds = 5

type StorageConfig struct {
	// LocalRoot is the canonical filesystem root for the builtin-local
	// storage driver. Empty resolves to the process working directory,
	// canonicalized once at startup.
	LocalRoot string
}

// ImgproxyConfig binds the on-demand derivative renderer. Display tiers
// resolve through signed imgproxy URLs; the async precompute chain is gone.
type ImgproxyConfig struct {
	BaseURL string
	KeyHex  string
	SaltHex string
}

// AdminSeedConfig carries the init-admin provisioning inputs read from
// INIT_ADMIN_EMAIL, INIT_ADMIN_PASSWORD and INIT_ADMIN_DISPLAY_NAME.
type AdminSeedConfig struct {
	Email       string
	Password    string
	DisplayName string
}

// MigrationToolingConfig carries the Atlas tooling knobs read from
// ATLAS_BINARY, ATLAS_MIGRATIONS_DIR and ATLAS_CLEAN_BREAK_ALLOW_RESET.
type MigrationToolingConfig struct {
	AtlasBinary   string
	MigrationsDir string
	// CleanBreakAllowReset is the explicit opt-in for the destructive clean
	// break path that drops and recreates the public schema. It defaults to
	// false: a database whose Atlas revisions are absent from the shipped
	// migration directory must fail the deploy, never silently reset itself.
	// See wiring.reconcileCleanBreak (runtime/composition/wiring).
	CleanBreakAllowReset bool
}

// SecretsConfig carries the raw at-rest encryption key material read from
// APP_SECRETS_ENCRYPTION_KEY.
type SecretsConfig struct {
	EncryptionKey string
}

const (
	// DefaultAtlasBinary is the Atlas executable used when ATLAS_BINARY is unset.
	DefaultAtlasBinary = "atlas"
	// DefaultMigrationDirectory is the Atlas migration directory used when
	// ATLAS_MIGRATIONS_DIR is unset.
	DefaultMigrationDirectory = "file://internal/db/migrations"
)

type envConfig struct {
	DBHost              string `env:"DB_HOST"`
	DBPort              string `env:"DB_PORT"`
	DBUser              string `env:"DB_USER"`
	DBPassword          string `env:"DB_PASSWORD"`
	DBMigrationUser     string `env:"DB_MIGRATION_USER"`
	DBMigrationPassword string `env:"DB_MIGRATION_PASSWORD"`
	DBName              string `env:"DB_NAME"`
	DBSSLMode           string `env:"DB_SSLMODE"`
	DBPoolMaxOpen       string `env:"DB_POOL_MAX_OPEN"`
	DBPoolMaxIdle       string `env:"DB_POOL_MAX_IDLE"`
	DBPoolMaxLifetime   string `env:"DB_POOL_MAX_LIFETIME_MINUTES"`
	DBPoolMaxIdleTime   string `env:"DB_POOL_MAX_IDLE_MINUTES"`
	RiverPoolMaxConns   string `env:"RIVER_POOL_MAX_CONNS"`
	RiverPoolMinConns   string `env:"RIVER_POOL_MIN_CONNS"`
	NATSEventBusEnabled string `env:"NATS_EVENT_BUS_ENABLED"`
	NATSPassword        string `env:"NATS_PASSWORD"`
	NATSHost            string `env:"NATS_HOST"`
	NATSPort            string `env:"NATS_PORT"`
	RiverConcurrency    string `env:"RIVER_WORKER_CONCURRENCY"`
	TaskRetentionDays   string `env:"TASK_RETENTION_DAYS"`

	AppPort               string `env:"APP_PORT"`
	AppHost               string `env:"APP_HOST"`
	MetricsAddr           string `env:"METRICS_ADDR"`
	AppEnv                string `env:"APP_ENV"`
	AppCORSAllowedOrigins string `env:"APP_CORS_ALLOWED_ORIGINS"`
	AppTrustedProxies     string `env:"APP_TRUSTED_PROXIES"`
	PublicOrigin          string `env:"PUBLIC_ORIGIN"`
	APIBaseURL            string `env:"API_BASE_URL"`

	AuthServiceBaseURL        string `env:"AUTH_SERVICE_BASE_URL"`
	AuthServiceInternalAPIKey string `env:"AUTH_INTERNAL_KEY"`
	StorageLocalRoot          string `env:"STORAGE_LOCAL_ROOT"`
	CreditsWelcomeIdentityKey string `env:"CREDITS_WELCOME_IDENTITY_KEY"`
	RewardsCodeHMACKeys       string `env:"REWARDS_CODE_HMAC_KEYS"`
	ImgproxyKeyHex            string `env:"IMGPROXY_KEY"`
	ImgproxySaltHex           string `env:"IMGPROXY_SALT"`

	InitAdminEmail       string `env:"INIT_ADMIN_EMAIL"`
	InitAdminPassword    string `env:"INIT_ADMIN_PASSWORD"`
	InitAdminDisplayName string `env:"INIT_ADMIN_DISPLAY_NAME"`
	AtlasBinary          string `env:"ATLAS_BINARY"`
	AtlasMigrationsDir   string `env:"ATLAS_MIGRATIONS_DIR"`
	// AtlasCleanBreakAllowReset gates DROP SCHEMA public CASCADE. It is a
	// deliberate, self-describing opt-in rather than a bare boolean so the
	// blast radius is obvious to whoever sets it in a deploy environment.
	AtlasCleanBreakAllowReset string `env:"ATLAS_CLEAN_BREAK_ALLOW_RESET"`
	SecretsEncryptionKey      string `env:"APP_SECRETS_ENCRYPTION_KEY"`
}

var loadEnvOnce sync.Once

func Load() *Config {
	loadEnvFile()

	raw, err := env.ParseAs[envConfig]()
	if err != nil {
		slog.Warn("failed to parse environment config; using defaults where possible", "err", err)
	}

	raw.DBPassword = strings.TrimSpace(raw.DBPassword)
	if raw.DBPassword == "" {
		slog.Warn("DB_PASSWORD is not set; database connection may fail")
	}
	raw.DBMigrationPassword = strings.TrimSpace(raw.DBMigrationPassword)

	appEnv := normalizeAppEnv(raw.AppEnv)
	imgproxyBaseURL := "http://127.0.0.1:8080"
	if isProductionAppEnv(appEnv) {
		imgproxyBaseURL = "http://imgproxy:8080"
	}
	natsURL := ""
	natsPassword := strings.TrimSpace(raw.NATSPassword)
	natsHost := stringEnvDefault(raw.NATSHost, "127.0.0.1")
	natsPort := stringEnvDefault(raw.NATSPort, "4222")
	if natsPassword != "" {
		natsURL = (&url.URL{
			Scheme: "nats",
			User:   url.UserPassword("ideaboard", natsPassword),
			Host:   net.JoinHostPort(natsHost, natsPort),
		}).String()
	}

	publicOrigin := normalizePublicOrigin(raw.PublicOrigin)
	apiOrigin := normalizePublicOrigin(raw.APIBaseURL)
	if apiOrigin == "" {
		apiOrigin = publicOrigin
	}

	var invalidEnv []string
	eventBusEnabled, eventBusValid := parseBoolEnv(raw.NATSEventBusEnabled, natsURL != "")
	if !eventBusValid {
		invalidEnv = append(invalidEnv, "NATS_EVENT_BUS_ENABLED")
		slog.Warn("invalid boolean env; fallback to default", "key", "NATS_EVENT_BUS_ENABLED", "value", strings.TrimSpace(raw.NATSEventBusEnabled), "default", natsURL != "")
	}
	workerConcurrency, workerValid := boundedIntEnv(raw.RiverConcurrency, DefaultRiverWorkerConcurrency, minRiverWorkerConcurrency, maxRiverWorkerConcurrency)
	if !workerValid {
		invalidEnv = append(invalidEnv, "RIVER_WORKER_CONCURRENCY")
		slog.Warn("invalid integer env; fallback to default", "key", "RIVER_WORKER_CONCURRENCY", "value", strings.TrimSpace(raw.RiverConcurrency), "default", DefaultRiverWorkerConcurrency)
	}
	retentionDays, retentionValid := boundedIntEnv(raw.TaskRetentionDays, DefaultTaskRetentionDays, minTaskRetentionDays, maxTaskRetentionDays)
	if !retentionValid {
		invalidEnv = append(invalidEnv, "TASK_RETENTION_DAYS")
		slog.Warn("invalid integer env; fallback to default", "key", "TASK_RETENTION_DAYS", "value", strings.TrimSpace(raw.TaskRetentionDays), "default", DefaultTaskRetentionDays)
	}
	poolMaxOpen, poolMaxOpenValid := boundedIntEnv(raw.DBPoolMaxOpen, DefaultPoolMaxOpenConns, minPoolMaxOpenConns, maxPoolMaxOpenConns)
	if !poolMaxOpenValid {
		invalidEnv = append(invalidEnv, "DB_POOL_MAX_OPEN")
		slog.Warn("invalid integer env; fallback to default", "key", "DB_POOL_MAX_OPEN", "value", strings.TrimSpace(raw.DBPoolMaxOpen), "default", DefaultPoolMaxOpenConns)
	}
	poolMaxIdle, poolMaxIdleValid := boundedIntEnv(raw.DBPoolMaxIdle, DefaultPoolMaxIdleConns, minPoolMaxIdleConns, maxPoolMaxIdleConns)
	if !poolMaxIdleValid {
		invalidEnv = append(invalidEnv, "DB_POOL_MAX_IDLE")
		slog.Warn("invalid integer env; fallback to default", "key", "DB_POOL_MAX_IDLE", "value", strings.TrimSpace(raw.DBPoolMaxIdle), "default", DefaultPoolMaxIdleConns)
	}
	poolLifetime, poolLifetimeValid := boundedIntEnv(raw.DBPoolMaxLifetime, DefaultPoolMaxLifetimeMin, minPoolLifetimeMin, maxPoolLifetimeMin)
	if !poolLifetimeValid {
		invalidEnv = append(invalidEnv, "DB_POOL_MAX_LIFETIME_MINUTES")
		slog.Warn("invalid integer env; fallback to default", "key", "DB_POOL_MAX_LIFETIME_MINUTES", "value", strings.TrimSpace(raw.DBPoolMaxLifetime), "default", DefaultPoolMaxLifetimeMin)
	}
	poolIdleTime, poolIdleTimeValid := boundedIntEnv(raw.DBPoolMaxIdleTime, DefaultPoolMaxIdleTimeMin, minPoolIdleTimeMin, maxPoolIdleTimeMin)
	if !poolIdleTimeValid {
		invalidEnv = append(invalidEnv, "DB_POOL_MAX_IDLE_MINUTES")
		slog.Warn("invalid integer env; fallback to default", "key", "DB_POOL_MAX_IDLE_MINUTES", "value", strings.TrimSpace(raw.DBPoolMaxIdleTime), "default", DefaultPoolMaxIdleTimeMin)
	}
	riverMaxConns, riverMaxValid := boundedIntEnv(raw.RiverPoolMaxConns, DefaultRiverPoolMaxConns, minRiverPoolMaxConns, maxRiverPoolMaxConns)
	if !riverMaxValid {
		invalidEnv = append(invalidEnv, "RIVER_POOL_MAX_CONNS")
		slog.Warn("invalid integer env; fallback to default", "key", "RIVER_POOL_MAX_CONNS", "value", strings.TrimSpace(raw.RiverPoolMaxConns), "default", DefaultRiverPoolMaxConns)
	}
	riverMinConns, riverMinValid := boundedIntEnv(raw.RiverPoolMinConns, DefaultRiverPoolMinConns, minRiverPoolMinConns, maxRiverPoolMinConns)
	if !riverMinValid {
		invalidEnv = append(invalidEnv, "RIVER_POOL_MIN_CONNS")
		slog.Warn("invalid integer env; fallback to default", "key", "RIVER_POOL_MIN_CONNS", "value", strings.TrimSpace(raw.RiverPoolMinConns), "default", DefaultRiverPoolMinConns)
	}
	// ATLAS_CLEAN_BREAK_ALLOW_RESET gates DROP SCHEMA public CASCADE. An
	// unparseable value must fail closed (false) and be reported, never
	// silently enable the destructive path.
	cleanBreakAllowReset, cleanBreakValid := parseBoolEnv(raw.AtlasCleanBreakAllowReset, false)
	if !cleanBreakValid {
		invalidEnv = append(invalidEnv, "ATLAS_CLEAN_BREAK_ALLOW_RESET")
		slog.Warn("invalid boolean env; fallback to default", "key", "ATLAS_CLEAN_BREAK_ALLOW_RESET", "value", strings.TrimSpace(raw.AtlasCleanBreakAllowReset), "default", false)
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
			Pool: PoolConfig{
				MaxOpenConns:   poolMaxOpen,
				MaxIdleConns:   poolMaxIdle,
				MaxLifetimeMin: poolLifetime,
				MaxIdleTimeMin: poolIdleTime,
				RiverMaxConns:  riverMaxConns,
				RiverMinConns:  riverMinConns,
			},
		},
		Events: EventBusConfig{
			Enabled: eventBusEnabled,
			NATSURL: natsURL,
		},
		App: AppConfig{
			Port:               stringEnvDefault(raw.AppPort, "6000"),
			Host:               strings.TrimSpace(raw.AppHost),
			MetricsAddr:        stringEnvDefault(raw.MetricsAddr, "127.0.0.1:9091"),
			Env:                appEnv,
			CORSAllowedOrigins: detectCORSAllowedOrigins(raw.AppCORSAllowedOrigins, appEnv),
			TrustedProxies:     detectTrustedProxies(raw.AppTrustedProxies),
		},
		PublicOrigins: PublicOriginsConfig{
			Web: publicOrigin,
			API: apiOrigin,
		},
		Auth: AuthServiceConfig{
			BaseURL:        stringEnvDefault(raw.AuthServiceBaseURL, "http://127.0.0.1:6001"),
			InternalAPIKey: strings.TrimSpace(raw.AuthServiceInternalAPIKey),
			TimeoutSeconds: DefaultAuthServiceTimeoutSeconds,
		},
		Worker:  WorkerConfig{Concurrency: workerConcurrency, TaskRetentionDays: retentionDays},
		Credits: CreditsConfig{WelcomeIdentityKey: strings.TrimSpace(raw.CreditsWelcomeIdentityKey)},
		Rewards: RewardsConfig{CodeHMACKeys: strings.TrimSpace(raw.RewardsCodeHMACKeys)},
		Storage: StorageConfig{LocalRoot: strings.TrimSpace(raw.StorageLocalRoot)},
		Imgproxy: ImgproxyConfig{
			BaseURL: imgproxyBaseURL,
			KeyHex:  strings.TrimSpace(raw.ImgproxyKeyHex),
			SaltHex: strings.TrimSpace(raw.ImgproxySaltHex),
		},
		AdminSeed: AdminSeedConfig{
			Email:       strings.TrimSpace(raw.InitAdminEmail),
			Password:    strings.TrimSpace(raw.InitAdminPassword),
			DisplayName: strings.TrimSpace(raw.InitAdminDisplayName),
		},
		Migration: MigrationToolingConfig{
			AtlasBinary:          stringEnvDefault(raw.AtlasBinary, DefaultAtlasBinary),
			MigrationsDir:        stringEnvDefault(raw.AtlasMigrationsDir, DefaultMigrationDirectory),
			CleanBreakAllowReset: cleanBreakAllowReset,
		},
		Secrets: SecretsConfig{
			EncryptionKey: strings.TrimSpace(raw.SecretsEncryptionKey),
		},
		InvalidEnv: invalidEnv,
	}
}

// LoadAndValidate loads deployment configuration and rejects unsafe database
// settings before any connection attempt. Development keeps the local
// DB_SSLMODE=disable default; production/release requires an explicit,
// non-placeholder DB_PASSWORD and an allowlisted SSL mode. Production also
// rejects env keys that failed parsing (see Config.InvalidEnv) so a typo like
// RIVER_WORKER_CONCURRENCY="lots" cannot silently run with defaults.
// Public origins are validated here as well so a missing PUBLIC_ORIGIN fails
// fast at boot instead of surfacing from the first URL-building call site.
// APP_SECRETS_ENCRYPTION_KEY presence is required in every environment,
// matching the unconditional wiring requirement for at-rest secret encryption.
func LoadAndValidate() (*Config, error) {
	cfg := Load()
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

// Validate runs every startup fail-fast check in one place: database
// settings, malformed env fallbacks (production only), public origins, and
// the at-rest secrets encryption key. Callers booting the process must fail
// on the returned error before opening any connection or listener.
func (c *Config) Validate() error {
	if c == nil {
		return fmt.Errorf("config is required")
	}
	if err := c.ValidateDatabase(); err != nil {
		return err
	}
	if err := c.ValidateEnvStrict(); err != nil {
		return err
	}
	if err := c.ValidateSecrets(); err != nil {
		return err
	}
	if max := c.Database.Pool.EffectiveMaxOpenConns() + c.Database.Pool.EffectiveRiverMaxConns(); max > maxRuntimePoolBudget {
		return fmt.Errorf("database pool budget %d exceeds maximum %d", max, maxRuntimePoolBudget)
	}
	if err := c.PublicOrigins.Validate(); err != nil {
		return err
	}
	return nil
}

// ValidateEnvStrict rejects fallback defaults from malformed env values in
// production. Development keeps warn-and-default so local iteration stays fast.
func (c *Config) ValidateEnvStrict() error {
	if c == nil {
		return fmt.Errorf("config is required")
	}
	if !isProductionAppEnv(c.App.Env) {
		return nil
	}
	if len(c.InvalidEnv) > 0 {
		return fmt.Errorf("invalid env values in production (fix or unset): %s", strings.Join(c.InvalidEnv, ", "))
	}
	return nil
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

// ValidateSecrets enforces the APP_SECRETS_ENCRYPTION_KEY presence check in
// every environment, matching the unconditional wiring requirement in
// runtime/composition (secrets.LoadEncryptionKey). Format validation
// (base64, exactly 32 bytes) stays in the secrets package.
func (c *Config) ValidateSecrets() error {
	if c == nil {
		return fmt.Errorf("config is required")
	}
	if strings.TrimSpace(c.Secrets.EncryptionKey) == "" {
		return fmt.Errorf("APP_SECRETS_ENCRYPTION_KEY is required")
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

func detectTrustedProxies(raw string) []string {
	if proxies := splitCSVEnv(raw); len(proxies) > 0 {
		return proxies
	}
	return []string{"127.0.0.1", "::1"}
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

// parseBoolEnv parses a boolean env value, reporting validity separately so
// Load can record fallback defaults in Config.InvalidEnv. Empty means unset
// (valid, use default); unknown tokens are invalid.
func parseBoolEnv(raw string, defaultValue bool) (bool, bool) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return defaultValue, true
	}
	switch strings.ToLower(value) {
	case "1", "true", "t", "yes", "y", "on":
		return true, true
	case "0", "false", "f", "no", "n", "off":
		return false, true
	default:
		return defaultValue, false
	}
}

// boundedIntEnv parses an integer env value within [minValue, maxValue],
// reporting validity separately for Config.InvalidEnv tracking.
func boundedIntEnv(raw string, defaultValue, minValue, maxValue int) (int, bool) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return defaultValue, true
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < minValue || parsed > maxValue {
		return defaultValue, false
	}
	return parsed, true
}
