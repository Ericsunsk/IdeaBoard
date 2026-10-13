package river

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"ideaboard/api/internal/platform/config"
	"ideaboard/api/internal/platform/riverpolicy"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	riverqueue "github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivermigrate"
)

const (
	// defaultRiverJobTimeout is the client-level fallback for unregistered or
	// test-only workers. Production workers pin their budget explicitly via
	// Timeout() against riverpolicy.RiverTimeoutMatrix.
	defaultRiverJobTimeout   = riverpolicy.RiverJobTimeoutDrawing
	riverSchemaLookupTimeout = 5 * time.Second
)

type ServerOptions struct {
	QueueName        string
	Concurrency      int
	JobTimeout       time.Duration
	MaxRetry         int
	AdditionalQueues map[string]riverqueue.QueueConfig
	RegisterWorkers  func(*riverqueue.Workers)
	PeriodicJobs     []*riverqueue.PeriodicJob
}

type RiverServerOptions = ServerOptions

func NewRiverProducer(dbPool *pgxpool.Pool) (*riverqueue.Client[pgx.Tx], error) {
	if dbPool == nil {
		return nil, errors.New("river db pool is nil")
	}
	schemaCtx, cancel := context.WithTimeout(context.Background(), riverSchemaLookupTimeout)
	defer cancel()
	schema, err := currentRiverSchema(schemaCtx, dbPool)
	if err != nil {
		return nil, err
	}
	return riverqueue.NewClient(riverpgxv5.New(dbPool), &riverqueue.Config{Schema: schema})
}

func NewRiverClient(dbPool *pgxpool.Pool, opts RiverServerOptions) (*riverqueue.Client[pgx.Tx], error) {
	if dbPool == nil {
		return nil, errors.New("river db pool is nil")
	}
	schemaCtx, cancel := context.WithTimeout(context.Background(), riverSchemaLookupTimeout)
	defer cancel()
	schema, err := currentRiverSchema(schemaCtx, dbPool)
	if err != nil {
		return nil, err
	}
	queueName, concurrency, maxRetry, jobTimeout := normalizeRiverServerOptions(opts)
	workers := riverqueue.NewWorkers()
	if opts.RegisterWorkers != nil {
		opts.RegisterWorkers(workers)
	}
	queues := buildRiverQueues(queueName, concurrency, opts.AdditionalQueues)

	client, err := riverqueue.NewClient(riverpgxv5.New(dbPool), &riverqueue.Config{
		Queues:       queues,
		Workers:      workers,
		MaxAttempts:  maxRetry,
		JobTimeout:   jobTimeout,
		Schema:       schema,
		PeriodicJobs: opts.PeriodicJobs,
	})
	if err != nil {
		return nil, err
	}
	return client, nil
}

func normalizeRiverServerOptions(opts RiverServerOptions) (string, int, int, time.Duration) {
	queueName := opts.QueueName
	if queueName == "" {
		queueName = riverpolicy.RiverQueueDefault
	}
	concurrency := opts.Concurrency
	if concurrency <= 0 {
		concurrency = 5
	}
	maxRetry := opts.MaxRetry
	if maxRetry <= 0 {
		maxRetry = riverpolicy.RiverMaxAttemptsDefault
	}
	jobTimeout := opts.JobTimeout
	if jobTimeout <= 0 {
		jobTimeout = defaultRiverJobTimeout
	}
	return queueName, concurrency, maxRetry, jobTimeout
}

func buildRiverQueues(queueName string, concurrency int, additional map[string]riverqueue.QueueConfig) map[string]riverqueue.QueueConfig {
	queues := map[string]riverqueue.QueueConfig{queueName: {MaxWorkers: concurrency}}
	for name, queue := range additional {
		if strings.TrimSpace(name) != "" {
			queues[name] = queue
		}
	}
	return queues
}

func MigrateRiver(ctx context.Context, dbPool *pgxpool.Pool) error {
	if dbPool == nil {
		return errors.New("river db pool is nil")
	}
	schema, err := currentRiverSchema(ctx, dbPool)
	if err != nil {
		return err
	}
	migrator, err := rivermigrate.New(riverpgxv5.New(dbPool), &rivermigrate.Config{Schema: schema})
	if err != nil {
		return err
	}
	_, err = migrator.Migrate(ctx, rivermigrate.DirectionUp, nil)
	return err
}

var Migrate = MigrateRiver

func currentRiverSchema(ctx context.Context, dbPool *pgxpool.Pool) (string, error) {
	var schema string
	if err := dbPool.QueryRow(ctx, `SELECT current_schema()`).Scan(&schema); err != nil {
		return "", fmt.Errorf("read current River schema: %w", err)
	}
	if strings.TrimSpace(schema) == "" {
		return "", errors.New("current River schema is empty")
	}
	return schema, nil
}

// NewRiverPgxPoolWithConfig opens the River pgxpool with deploy-level sizing
// from PoolConfig (env-tunable). A zero PoolConfig keeps the historical
// budget (10 max / 1 min) through the Effective* accessors: combined with the
// runtime pgxpool, a single API instance holds 35 conns against the
// PostgreSQL default max_connections 100, with headroom for migrations, CLIs
// and test fixtures. Idle recycling matches the runtime pool so both
// age out together.
func NewRiverPgxPoolWithConfig(ctx context.Context, dsn string, pool config.PoolConfig) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, err
	}
	cfg.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeExec
	applyRiverPoolLimits(cfg, pool)
	cfg.HealthCheckPeriod = time.Minute
	return pgxpool.NewWithConfig(ctx, cfg)
}

// applyRiverPoolLimits pins sizing and recycling on a parsed pgxpool config
// without opening connections, so pool tuning stays unit-testable without a
// live database.
func applyRiverPoolLimits(cfg *pgxpool.Config, pool config.PoolConfig) {
	cfg.MaxConns = int32(pool.EffectiveRiverMaxConns())
	cfg.MinConns = int32(pool.EffectiveRiverMinConns())
	cfg.MaxConnLifetime = time.Duration(pool.EffectiveMaxLifetimeMin()) * time.Minute
	cfg.MaxConnIdleTime = time.Duration(pool.EffectiveMaxIdleTimeMin()) * time.Minute
}

type RiverTx = pgx.Tx
