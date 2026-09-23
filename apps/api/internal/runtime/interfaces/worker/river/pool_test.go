package river

import (
	"testing"
	"time"

	"ideaboard/api/internal/platform/config"

	"github.com/jackc/pgx/v5/pgxpool"
)

const riverPoolTestDSN = "postgres://ideaboard:secret@127.0.0.1:5432/ideaboard?sslmode=disable"

func parseRiverPoolTestConfig(t *testing.T) *pgxpool.Config {
	t.Helper()
	cfg, err := pgxpool.ParseConfig(riverPoolTestDSN)
	if err != nil {
		t.Fatalf("ParseConfig() error = %v", err)
	}
	return cfg
}

func TestApplyRiverPoolLimitsKeepsHistoricalDefaults(t *testing.T) {
	cfg := parseRiverPoolTestConfig(t)
	applyRiverPoolLimits(cfg, config.PoolConfig{})
	if cfg.MaxConns != config.DefaultRiverPoolMaxConns {
		t.Fatalf("MaxConns = %d, want default %d", cfg.MaxConns, config.DefaultRiverPoolMaxConns)
	}
	if cfg.MinConns != config.DefaultRiverPoolMinConns {
		t.Fatalf("MinConns = %d, want default %d", cfg.MinConns, config.DefaultRiverPoolMinConns)
	}
	if cfg.MaxConnLifetime != time.Duration(config.DefaultPoolMaxLifetimeMin)*time.Minute {
		t.Fatalf("MaxConnLifetime = %v, want %dm", cfg.MaxConnLifetime, config.DefaultPoolMaxLifetimeMin)
	}
	if cfg.MaxConnIdleTime != time.Duration(config.DefaultPoolMaxIdleTimeMin)*time.Minute {
		t.Fatalf("MaxConnIdleTime = %v, want %dm", cfg.MaxConnIdleTime, config.DefaultPoolMaxIdleTimeMin)
	}
}

func TestApplyRiverPoolLimitsHonorsConfiguredValues(t *testing.T) {
	cfg := parseRiverPoolTestConfig(t)
	applyRiverPoolLimits(cfg, config.PoolConfig{
		MaxOpenConns:   40,
		MaxIdleConns:   20,
		MaxLifetimeMin: 60,
		MaxIdleTimeMin: 15,
		RiverMaxConns:  16,
		RiverMinConns:  2,
	})
	if cfg.MaxConns != 16 {
		t.Fatalf("MaxConns = %d, want 16", cfg.MaxConns)
	}
	if cfg.MinConns != 2 {
		t.Fatalf("MinConns = %d, want 2", cfg.MinConns)
	}
	if cfg.MaxConnLifetime != 60*time.Minute {
		t.Fatalf("MaxConnLifetime = %v, want 60m", cfg.MaxConnLifetime)
	}
	if cfg.MaxConnIdleTime != 15*time.Minute {
		t.Fatalf("MaxConnIdleTime = %v, want 15m", cfg.MaxConnIdleTime)
	}
}

func TestApplyRiverPoolLimitsClampsInvalidValues(t *testing.T) {
	cfg := parseRiverPoolTestConfig(t)
	applyRiverPoolLimits(cfg, config.PoolConfig{RiverMaxConns: 500, RiverMinConns: 60})
	if cfg.MaxConns != config.DefaultRiverPoolMaxConns {
		t.Fatalf("MaxConns = %d, want default %d", cfg.MaxConns, config.DefaultRiverPoolMaxConns)
	}
	if cfg.MinConns != config.DefaultRiverPoolMinConns {
		t.Fatalf("MinConns = %d, want default %d", cfg.MinConns, config.DefaultRiverPoolMinConns)
	}
}
