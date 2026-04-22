package config

import (
	"fmt"
	"log"
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
	JWT      JWTConfig
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
	Env  string
}

type JWTConfig struct {
	Secret string
}

var loadEnvOnce sync.Once

func Load() *Config {
	loadEnvFile()

	dbPassword := getEnv("DB_PASSWORD", "")
	if dbPassword == "" {
		log.Println("[Config] WARNING: DB_PASSWORD 未设置，数据库连接可能失败")
	}

	jwtSecret := strings.TrimSpace(getEnv("JWT_SECRET", ""))
	if jwtSecret == "" {
		if IsProductionEnv() {
			log.Println("[Config] ERROR: JWT_SECRET 未设置；生产环境启动将被拒绝")
		} else {
			log.Println("[Config] WARNING: JWT_SECRET 未设置；当前仅允许非生产环境使用一次性开发密钥，重启后旧 token 会失效")
		}
	}

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
		App: AppConfig{
			Port: getEnv("APP_PORT", "6000"),
			Env:  detectAppEnv(),
		},
		JWT: JWTConfig{
			Secret: jwtSecret,
		},
	}
}

func ValidateJWTSecret(cfg *Config) error {
	if cfg == nil {
		return nil
	}
	if strings.TrimSpace(cfg.JWT.Secret) != "" {
		return nil
	}
	if !isProductionAppEnv(cfg.App.Env) {
		return nil
	}
	return fmt.Errorf("JWT_SECRET is required when APP_ENV/GO_ENV/ENV or GIN_MODE indicates production (current: %s)", cfg.App.Env)
}

func IsProductionEnv() bool {
	return isProductionAppEnv(detectAppEnv())
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
