package main

import (
	"context"
	"net/http"
	"os"
	"strings"
	"time"

	"ideaboard/api/internal/platform/config"
	"ideaboard/api/internal/platform/database"

	"github.com/jackc/pgx/v5/pgxpool"
)

const defaultHealthcheckURL = "http://127.0.0.1:6000/api/health"

func main() {
	os.Exit(run())
}

func run() int {
	if probeKind() == probeWorkerDB {
		return runWorkerProbe()
	}
	url := healthcheckURL()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 1
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 1
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusBadRequest {
		return 1
	}
	return 0
}

type healthProbeKind string

const (
	probeHTTP     healthProbeKind = "http"
	probeWorkerDB healthProbeKind = "worker-db"
)

// probeKind selects the health signal for this container. A worker-only
// process (ADR-0021) binds no HTTP listeners, so the HTTP probe has nothing
// to hit and the worker falls back to a database liveness round-trip. An
// explicit HEALTHCHECK_URL always wins, so ad-hoc probing keeps working in
// every mode.
func probeKind() healthProbeKind {
	if strings.TrimSpace(os.Getenv("HEALTHCHECK_URL")) != "" {
		return probeHTTP
	}
	if mode, _ := config.ParseProcessMode(os.Getenv("API_MODE")); mode == config.ProcessModeWorkerOnly {
		return probeWorkerDB
	}
	return probeHTTP
}

// runWorkerProbe checks the worker's lifeline: one PostgreSQL round-trip
// with the same DB_* env as the server. It is a liveness signal only;
// consumption progress is observed via River queue depth/lag gauges, not here.
func runWorkerProbe() int {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, database.PostgresDSN(config.Load().Database))
	if err != nil {
		return 1
	}
	defer pool.Close()

	var one int
	if err := pool.QueryRow(ctx, "SELECT 1").Scan(&one); err != nil || one != 1 {
		return 1
	}
	return 0
}

// healthcheckURL resolves the HTTP probe target. Only HEALTHCHECK_URL is
// honored; the server listener is a hardcoded code constant, so the default
// stays in one place here.
func healthcheckURL() string {
	url := strings.TrimSpace(os.Getenv("HEALTHCHECK_URL"))
	if url == "" {
		url = defaultHealthcheckURL
	}
	return url
}
