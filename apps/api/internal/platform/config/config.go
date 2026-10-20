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
	Realtime      RealtimeConfig
	Imgproxy      ImgproxyConfig
	// AdminSeed carries the init-admin provisioning inputs. They are
	// command-scoped (only the init-admin command requires them), so
	// Validate leaves them unchecked; the command fails fast instead.
	AdminSeed AdminSeedConfig
	// Migration carries the Atlas clean-break opt-in for the migrate path.
	// The Atlas binary and migrations directory are hardcoded code
	// constants (DefaultAtlasBinary/DefaultMigrationDirectory).
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
	// Mode selects the process role of this binary (ADR-0021): all runs
	// HTTP + River workers in one process (local development default),
	// http-only serves API/SSE and only enqueues River jobs, worker-only
	// consumes River queues with no HTTP listeners.
	Mode ProcessMode
}

type CreditsConfig struct {
	WelcomeIdentityKey string
}

type RewardsConfig struct {
	CodeHMACKeys string
}

// RealtimeConfig carries the board realtime-ticket HMAC secret read from
// RT_TICKET_SECRET. Empty fails the ticket endpoint closed with 500
// (no fallback default); Load records the missing key in InvalidEnv so
// production startup fails fast via ValidateEnvStrict.
type RealtimeConfig struct {
	TicketSecret string
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

// Pool sizing is hardcoded code constants by design (no env knobs; see
// docs/runbooks/deployment.md): DefaultPoolMaxOpenConns bounds the runtime
// pgxpool and DefaultRiverPoolMaxConns bounds the River pgxpool, while idle
// sizing and connection recycling use the remaining DefaultPoolMax* /
// DefaultRiverPoolMinConns constants. A single API instance holds
// 25 + 10 = 35 connections against the default PostgreSQL
// max_connections=100, leaving headroom for migrations, CLIs and fixtures.
const (
	DefaultPoolMaxOpenConns   = 25
	DefaultPoolMaxIdleConns   = 10
	DefaultPoolMaxLifetimeMin = 30
	DefaultPoolMaxIdleTimeMin = 5
	DefaultRiverPoolMaxConns  = 10
	DefaultRiverPoolMinConns  = 1
)

type AppConfig struct {
	Env                string
	CORSAllowedOrigins []string
}

// IsProduction selects the repository's production Compose topology.
func (c AppConfig) IsProduction() bool {
	return isProductionAppEnv(c.Env)
}

// TrustedProxies only trusts the Docker bridge in the owned production
// topology, whose API port is published on host loopback behind Nginx.
// Host development trusts loopback proxies only and binds loopback listeners.
func (c AppConfig) TrustedProxies() []string {
	proxies := []string{"127.0.0.1", "::1"}
	if c.IsProduction() {
		proxies = append(proxies, "10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16")
	}
	return proxies
}

// PublicOriginsConfig is immutable deployment configuration. Web is the
// single canonical public origin (PUBLIC_ORIGIN): it owns browser navigation
// URLs as well as webhooks and locally served assets. APIURL and
// APIParsedURL resolve through the same origin; there is no separate API
// host override by design (same-origin Gateway).
type PublicOriginsConfig struct {
	Web string
}

func (c PublicOriginsConfig) Validate() error {
	return validatePublicOrigin("PUBLIC_ORIGIN", c.Web)
}

func (c PublicOriginsConfig) WebURL(path string) string {
	if validatePublicOrigin("PUBLIC_ORIGIN", c.Web) != nil {
		return ""
	}
	return joinPublicOrigin(c.Web, path)
}

func (c PublicOriginsConfig) APIURL(path string) string {
	return c.WebURL(path)
}

func (c PublicOriginsConfig) APIParsedURL() *url.URL {
	if validatePublicOrigin("PUBLIC_ORIGIN", c.Web) != nil {
		return nil
	}
	parsed, err := url.Parse(c.Web)
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

// ProcessMode selects the runtime role of the API process (ADR-0021). It is
// deploy-level topology, not business policy: the same binary and database
// serve every mode, only the started components differ.
type ProcessMode string

const (
	// ProcessModeAll runs HTTP listeners and River workers in one process.
	// It preserves the pre-split behavior and stays the local default.
	ProcessModeAll ProcessMode = "all"
	// ProcessModeHTTPOnly serves API/SSE and enqueues River jobs without
	// registering workers or scheduling periodic jobs.
	ProcessModeHTTPOnly ProcessMode = "http-only"
	// ProcessModeWorkerOnly consumes River queues with no HTTP listeners.
	ProcessModeWorkerOnly ProcessMode = "worker-only"
)

// ParseProcessMode normalizes a raw mode value, reporting validity
// separately so Load can record fallback defaults in Config.InvalidEnv.
// Empty means unset (valid, use the default); unknown tokens are invalid.
func ParseProcessMode(raw string) (ProcessMode, bool) {
	value := ProcessMode(strings.ToLower(strings.TrimSpace(raw)))
	if value == "" {
		return ProcessModeAll, true
	}
	switch value {
	case ProcessModeAll, ProcessModeHTTPOnly, ProcessModeWorkerOnly:
		return value, true
	default:
		return ProcessModeAll, false
	}
}

// ServesHTTP reports whether the process binds HTTP listeners (API + metrics).
func (m ProcessMode) ServesHTTP() bool {
	return m != ProcessModeWorkerOnly
}

// RunsWorkers reports whether the process registers River workers and
// schedules periodic jobs. An http-only process still enqueues jobs from
// the submit transaction; it just never consumes them.
func (m ProcessMode) RunsWorkers() bool {
	return m != ProcessModeHTTPOnly
}

type EventBusConfig struct {
	NATSURL string
}

type AuthServiceConfig struct {
	BaseURL        string
	InternalAPIKey string
	TimeoutSeconds int
}

const DefaultAuthServiceTimeoutSeconds = 5

// DefaultStorageLocalRoot resolves once from the process working directory.
// Host development starts in apps/api; container WORKDIR is /app. Both
// share the same uploads layout with imgproxy.
const DefaultStorageLocalRoot = "."

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

// MigrationToolingConfig carries the Atlas clean-break opt-in read from
// ATLAS_CLEAN_BREAK_ALLOW_RESET. The Atlas binary and migrations directory
// are hardcoded code constants (DefaultAtlasBinary/DefaultMigrationDirectory).
type MigrationToolingConfig struct {
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
	// DefaultAtlasBinary is the Atlas executable (hardcoded; no env knob).
	DefaultAtlasBinary = "atlas"
	// DefaultMigrationDirectory is the Atlas migration directory
	// (hardcoded; no env knob).
	DefaultMigrationDirectory = "file://internal/db/migrations"
)

const defaultNATSPort = "4222"

type envConfig struct {
	PGHost              string `env:"PG_HOST"`
	PGPort              string `env:"PG_PORT"`
	APIPGUser           string `env:"API_PG_USER"`
	APIPGPassword       string `env:"API_PG_PASSWORD"`
	PGMigrationUser     string `env:"PG_MIGRATION_USER"`
	PGMigrationPassword string `env:"PG_MIGRATION_PASSWORD"`
	PGDB                string `env:"PG_DB"`
	PGSSLMode           string `env:"PG_SSLMODE"`
	NATSPassword        string `env:"NATS_PASSWORD"`
	RiverConcurrency    string `env:"RIVER_WORKER_CONCURRENCY"`

	AppEnv       string `env:"APP_ENV"`
	AppMode      string `env:"API_MODE"`
	PublicOrigin string `env:"PUBLIC_ORIGIN"`

	AuthServiceBaseURL        string `env:"AUTH_INTERNAL_URL"`
	AuthServiceInternalAPIKey string `env:"AUTH_INTERNAL_KEY"`
	CreditsWelcomeIdentityKey string `env:"CREDITS_WELCOME_IDENTITY_KEY"`
	RewardsCodeHMACKeys       string `env:"REWARDS_CODE_HMAC_KEYS"`
	RealtimeTicketSecret      string `env:"RT_TICKET_SECRET"`
	ImgproxyKeyHex            string `env:"IMGPROXY_KEY"`
	ImgproxySaltHex           string `env:"IMGPROXY_SALT"`

	InitAdminEmail       string `env:"INIT_ADMIN_EMAIL"`
	InitAdminPassword    string `env:"INIT_ADMIN_PASSWORD"`
	InitAdminDisplayName string `env:"INIT_ADMIN_DISPLAY_NAME"`
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

	raw.APIPGPassword = strings.TrimSpace(raw.APIPGPassword)
	if raw.APIPGPassword == "" {
		slog.Warn("API_PG_PASSWORD is not set; database connection may fail")
	}
	raw.PGMigrationPassword = strings.TrimSpace(raw.PGMigrationPassword)

	appEnv := normalizeAppEnv(raw.AppEnv)
	imgproxyBaseURL := "http://127.0.0.1:8080"
	if isProductionAppEnv(appEnv) {
		imgproxyBaseURL = "http://imgproxy:8080"
	}
	natsHost := "127.0.0.1"
	if isProductionAppEnv(appEnv) {
		natsHost = "nats"
	}
	natsURL := ""
	natsPassword := strings.TrimSpace(raw.NATSPassword)
	if natsPassword != "" {
		natsURL = (&url.URL{
			Scheme: "nats",
			User:   url.UserPassword("ideaboard", natsPassword),
			Host:   net.JoinHostPort(natsHost, defaultNATSPort),
		}).String()
	}

	publicOrigin := normalizePublicOrigin(raw.PublicOrigin)

	var invalidEnv []string
	workerConcurrency, workerValid := boundedIntEnv(raw.RiverConcurrency, DefaultRiverWorkerConcurrency, minRiverWorkerConcurrency, maxRiverWorkerConcurrency)
	if !workerValid {
		invalidEnv = append(invalidEnv, "RIVER_WORKER_CONCURRENCY")
		slog.Warn("invalid integer env; fallback to default", "key", "RIVER_WORKER_CONCURRENCY", "value", strings.TrimSpace(raw.RiverConcurrency), "default", DefaultRiverWorkerConcurrency)
	}
	// ATLAS_CLEAN_BREAK_ALLOW_RESET gates DROP SCHEMA public CASCADE. An
	// unparseable value must fail closed (false) and be reported, never
	// silently enable the destructive path.
	cleanBreakAllowReset, cleanBreakValid := parseBoolEnv(raw.AtlasCleanBreakAllowReset, false)
	if !cleanBreakValid {
		invalidEnv = append(invalidEnv, "ATLAS_CLEAN_BREAK_ALLOW_RESET")
		slog.Warn("invalid boolean env; fallback to default", "key", "ATLAS_CLEAN_BREAK_ALLOW_RESET", "value", strings.TrimSpace(raw.AtlasCleanBreakAllowReset), "default", false)
	}
	// API_MODE selects the ADR-0021 process role. An unparseable value
	// falls back to all-process (local default) and is reported, so
	// production startup fails fast instead of silently running merged.
	processMode, processModeValid := ParseProcessMode(raw.AppMode)
	if !processModeValid {
		invalidEnv = append(invalidEnv, "API_MODE")
		slog.Warn("invalid process mode env; fallback to default", "key", "API_MODE", "value", strings.TrimSpace(raw.AppMode), "default", string(ProcessModeAll))
	}
	// RT_TICKET_SECRET has no fallback default: an empty value keeps
	// the ticket endpoint fail-closed with 500 and is recorded here so
	// production startup fails fast via ValidateEnvStrict instead of
	// silently serving presence without tickets.
	realtimeTicketSecret := strings.TrimSpace(raw.RealtimeTicketSecret)
	if realtimeTicketSecret == "" {
		invalidEnv = append(invalidEnv, "RT_TICKET_SECRET")
		slog.Warn("realtime ticket secret is not set; the realtime-ticket endpoint stays fail-closed with 500", "key", "RT_TICKET_SECRET")
	}

	return &Config{
		Database: DatabaseConfig{
			Host:              stringEnvDefault(raw.PGHost, "127.0.0.1"),
			Port:              stringEnvDefault(raw.PGPort, "5432"),
			User:              stringEnvDefault(raw.APIPGUser, "ideaboard_api"),
			Password:          raw.APIPGPassword,
			MigrationUser:     strings.TrimSpace(raw.PGMigrationUser),
			MigrationPassword: raw.PGMigrationPassword,
			Database:          stringEnvDefault(raw.PGDB, "ideaboard"),
			SSLMode:           stringEnvDefault(raw.PGSSLMode, "disable"),
		},
		Events: EventBusConfig{
			NATSURL: natsURL,
		},
		App: AppConfig{
			Env:                appEnv,
			CORSAllowedOrigins: defaultCORSAllowedOrigins(appEnv),
		},
		PublicOrigins: PublicOriginsConfig{
			Web: publicOrigin,
		},
		Auth: AuthServiceConfig{
			BaseURL:        stringEnvDefault(raw.AuthServiceBaseURL, "http://127.0.0.1:6001"),
			InternalAPIKey: strings.TrimSpace(raw.AuthServiceInternalAPIKey),
			TimeoutSeconds: DefaultAuthServiceTimeoutSeconds,
		},
		Worker:   WorkerConfig{Concurrency: workerConcurrency},
		Credits:  CreditsConfig{WelcomeIdentityKey: strings.TrimSpace(raw.CreditsWelcomeIdentityKey)},
		Rewards:  RewardsConfig{CodeHMACKeys: strings.TrimSpace(raw.RewardsCodeHMACKeys)},
		Realtime: RealtimeConfig{TicketSecret: realtimeTicketSecret},
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
			CleanBreakAllowReset: cleanBreakAllowReset,
		},
		Secrets: SecretsConfig{
			EncryptionKey: strings.TrimSpace(raw.SecretsEncryptionKey),
		},
		Mode:       processMode,
		InvalidEnv: invalidEnv,
	}
}

// LoadAndValidate loads deployment configuration for the serving process
// and rejects unsafe settings before any connection attempt. Development
// keeps the local PG_SSLMODE=disable default; production/release requires
// an explicit, non-placeholder API_PG_PASSWORD and an allowlisted SSL mode.
// Production also rejects env keys that failed parsing (see
// Config.InvalidEnv) so a typo like RIVER_WORKER_CONCURRENCY="lots" cannot
// silently run with defaults.
// Required serving secrets and origins are validated here as well
// (see ValidateRequired) so a missing API_PG_PASSWORD fails fast at boot
// with every missing key listed instead of surfacing from the first
// connection attempt.
// APP_SECRETS_ENCRYPTION_KEY presence is required in every environment,
// matching the unconditional wiring requirement for at-rest secret encryption.
func LoadAndValidate() (*Config, error) {
	cfg := Load()
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

// Validate runs every startup fail-fast check in one place: required
// secrets and origins (listed together so one run reports everything
// missing), database settings, malformed env fallbacks (production only),
// public origins, and the at-rest secrets encryption key. Callers booting
// the serving process must fail on the returned error before opening any
// connection or listener. Migration-only entrypoints (migrate,
// finalize-migrate) use ValidateForMigration instead: they legitimately run
// without the serving secrets (internal keys, ticket secret, rewards).
func (c *Config) Validate() error {
	if c == nil {
		return fmt.Errorf("config is required")
	}
	if err := c.ValidateRequired(); err != nil {
		return err
	}
	if err := c.ValidateDatabase(); err != nil {
		return err
	}
	if err := c.ValidateEnvStrict(); err != nil {
		return err
	}
	return nil
}

// ValidateRequired rejects a serving process that is missing any required
// secret or origin. Every missing key is reported in a single error so one
// startup attempt lists everything to fill (env file or exported vars).
func (c *Config) ValidateRequired() error {
	if c == nil {
		return fmt.Errorf("config is required")
	}
	var missing []string
	if strings.TrimSpace(c.Database.Password) == "" {
		missing = append(missing, "API_PG_PASSWORD")
	}
	if strings.TrimSpace(c.Auth.InternalAPIKey) == "" {
		missing = append(missing, "AUTH_INTERNAL_KEY")
	}
	if strings.TrimSpace(c.Realtime.TicketSecret) == "" {
		missing = append(missing, "RT_TICKET_SECRET")
	}
	if strings.TrimSpace(c.Secrets.EncryptionKey) == "" {
		missing = append(missing, "APP_SECRETS_ENCRYPTION_KEY")
	}
	if strings.TrimSpace(c.Credits.WelcomeIdentityKey) == "" {
		missing = append(missing, "CREDITS_WELCOME_IDENTITY_KEY")
	}
	if strings.TrimSpace(c.Rewards.CodeHMACKeys) == "" {
		missing = append(missing, "REWARDS_CODE_HMAC_KEYS")
	}
	if err := c.PublicOrigins.Validate(); err != nil {
		missing = append(missing, "PUBLIC_ORIGIN")
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing or invalid required env: %s", strings.Join(missing, ", "))
	}
	return nil
}

// ValidateForMigration runs the fail-fast checks for the migration-only
// entrypoints (migrate, finalize-migrate). Those commands never mint
// tickets, serve internal routes, or settle rewards, so they must not
// require the serving secrets; database settings and the at-rest encryption
// key are still enforced.
func (c *Config) ValidateForMigration() error {
	if c == nil {
		return fmt.Errorf("config is required")
	}
	if err := c.ValidateDatabase(); err != nil {
		return err
	}
	if strings.TrimSpace(c.Database.Password) == "" {
		return fmt.Errorf("API_PG_PASSWORD is required for migrations")
	}
	if err := c.ValidateSecrets(); err != nil {
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
		return fmt.Errorf("API_PG_PASSWORD must be set to a non-placeholder value when APP_ENV=%s", c.App.Env)
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
		return fmt.Errorf("PG_SSLMODE must be one of disable, require, or verify-full")
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

// defaultCORSAllowedOrigins hardcodes the development allowlist. Production
// is same-origin only (nil): no env knob can reopen cross-origin.
func defaultCORSAllowedOrigins(appEnv string) []string {
	if isProductionAppEnv(appEnv) {
		return nil
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
