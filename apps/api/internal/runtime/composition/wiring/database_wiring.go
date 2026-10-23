// Wiring segment 1/4: pools and repositories (assembly only).
//
// This file only annotates and composes: it opens pools, verifies runtime
// privileges, and builds repository adapters. Business logic lives in the
// owning contexts (credits, billing, identity, media, payments, prompts,
// rewards, search, tasking); nothing here writes business facts.
package wiring

import (
	"context"
	"fmt"
	creditpostgres "ideaboard/api/internal/credits/infrastructure/postgres"
	"strings"

	billingpostgres "ideaboard/api/internal/billing/infrastructure/postgres"
	generationcatalog "ideaboard/api/internal/generation/catalog"
	identitypostgres "ideaboard/api/internal/identity/users/infrastructure/postgres"
	storagepostgres "ideaboard/api/internal/media/storage/infrastructure/postgres"
	paymentpostgres "ideaboard/api/internal/payments/infrastructure/postgres"
	"ideaboard/api/internal/payments/infrastructure/postgres/adminreads"
	"ideaboard/api/internal/payments/infrastructure/postgres/disputes"
	"ideaboard/api/internal/payments/infrastructure/postgres/refunds"
	"ideaboard/api/internal/platform/adminbranding"
	"ideaboard/api/internal/platform/config"
	"ideaboard/api/internal/platform/database"
	dbprivileges "ideaboard/api/internal/platform/database/privileges"
	"ideaboard/api/internal/platform/settings"
	"ideaboard/api/internal/prompts"
	rewardspostgres "ideaboard/api/internal/rewards/infrastructure/postgres"
	workerriver "ideaboard/api/internal/runtime/interfaces/worker/river"
	searchinfra "ideaboard/api/internal/searchcatalog/infrastructure"
	taskbilling "ideaboard/api/internal/tasking/application/billing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type AppDatabases struct {
	DB        *pgxpool.Pool
	RiverPool *pgxpool.Pool
}

type appDatabaseOpeners struct {
	postgres func(context.Context, config.DatabaseConfig) (*pgxpool.Pool, error)
	river    func(context.Context, string) (*pgxpool.Pool, error)
}

type AppRepositories struct {
	db                 *pgxpool.Pool
	userRepo           *identitypostgres.Repository
	taskBillingUsage   *taskbilling.UsageStore
	systemConfigRepo   *settings.SystemConfigRepository
	featurePromptRepo  *prompts.FeaturePromptRepository
	storageProfileRepo *storagepostgres.Repository
	paymentRepo        *paymentStores
	adminBrandLogoRepo *adminbranding.Repository
}

// Local aliases keep each aggregate's name distinct when embedding its store.
type paymentAdminReads = adminreads.Store
type paymentRefunds = refunds.Store
type paymentDisputes = disputes.Store

// paymentStores shares one root repository across all aggregate stores.
// The direct embedding takes precedence over the aggregates' promoted root methods.
type paymentStores struct {
	*paymentpostgres.Repository
	*paymentAdminReads
	*paymentRefunds
	*paymentDisputes
}

func newPaymentStores(repo *paymentpostgres.Repository) *paymentStores {
	return &paymentStores{
		Repository:        repo,
		paymentAdminReads: adminreads.NewStore(repo),
		paymentRefunds:    refunds.NewStore(repo),
		paymentDisputes:   disputes.NewStore(repo),
	}
}

func OpenAppDatabases(ctx context.Context, cfg *config.Config) (AppDatabases, error) {
	return openAppDatabasesWithOpeners(ctx, cfg, appDatabaseOpeners{
		postgres: database.NewPostgres,
		river: func(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
			return workerriver.NewRiverPgxPool(ctx, dsn)
		},
	})
}

func openAppDatabasesWithOpeners(
	ctx context.Context,
	cfg *config.Config,
	openers appDatabaseOpeners,
) (AppDatabases, error) {
	if cfg == nil {
		return AppDatabases{}, fmt.Errorf("database config is nil")
	}
	if openers.postgres == nil || openers.river == nil {
		return AppDatabases{}, fmt.Errorf("database openers are incomplete")
	}
	db, err := openers.postgres(ctx, cfg.Database)
	if err != nil {
		return AppDatabases{}, fmt.Errorf("failed to connect to database: %w", err)
	}
	if err := verifyRuntimePrivileges(ctx, db); err != nil {
		closeStartupDatabase(db)
		return AppDatabases{}, err
	}
	riverPool, err := openers.river(ctx, database.PostgresDSN(cfg.Database))
	if err != nil {
		closeStartupDatabase(db)
		return AppDatabases{}, fmt.Errorf("failed to connect River database pool: %w", err)
	}
	return AppDatabases{DB: db, RiverPool: riverPool}, nil
}

func closeStartupDatabase(db *pgxpool.Pool) {
	if db != nil {
		db.Close()
	}
}

func FinalizeAppDatabases(ctx context.Context, cfg *config.Config) error {
	if cfg == nil {
		return fmt.Errorf("database config is nil")
	}
	migrationConfig, err := separatedMigrationDatabaseConfig(cfg.Database)
	if err != nil {
		return err
	}
	migrationDB, err := database.NewPostgres(ctx, migrationConfig)
	if err != nil {
		return fmt.Errorf("failed to connect migration database: %w", err)
	}
	defer migrationDB.Close()
	migrationPool, err := workerriver.NewRiverPgxPool(ctx, database.PostgresDSN(migrationConfig))
	if err != nil {
		return fmt.Errorf("failed to connect River migration pool: %w", err)
	}
	defer migrationPool.Close()
	if err := workerriver.MigrateRiver(ctx, migrationPool); err != nil {
		return fmt.Errorf("failed to migrate River schema: %w", err)
	}
	privilegeTx, err := migrationDB.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin runtime privilege transaction: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = privilegeTx.Rollback(ctx)
		}
	}()
	if err := dbprivileges.Configure(ctx, privilegeTx, cfg.Database.User, runtimePrivilegePolicies()...); err != nil {
		return err
	}
	if err := privilegeTx.Commit(ctx); err != nil {
		return fmt.Errorf("commit runtime privileges: %w", err)
	}
	committed = true
	return nil
}

func runtimePrivilegePolicies() []dbprivileges.Policy {
	return []dbprivileges.Policy{
		dbprivileges.RuntimePrivilegePolicy(),
		creditpostgres.RuntimePrivilegePolicy(),
		billingpostgres.RuntimePrivilegePolicy(),
		workerriver.RuntimePrivilegePolicy(),
		rewardspostgres.RuntimePrivilegePolicy(),
		paymentpostgres.RuntimePrivilegePolicy(),
		searchinfra.RuntimePrivilegePolicy(),
		generationcatalog.RuntimePrivilegePolicy(),
	}
}

func verifyRuntimePrivileges(ctx context.Context, db dbprivileges.DBTX) error {
	return dbprivileges.Verify(ctx, db, runtimePrivilegePolicies()...)
}

func separatedMigrationDatabaseConfig(runtimeConfig config.DatabaseConfig) (config.DatabaseConfig, error) {
	migrationUser := strings.TrimSpace(runtimeConfig.MigrationUser)
	if migrationUser == "" || runtimeConfig.MigrationPassword == "" {
		return config.DatabaseConfig{}, fmt.Errorf("PG_MIGRATION_USER and PG_MIGRATION_PASSWORD are required for migrations")
	}
	if migrationUser == strings.TrimSpace(runtimeConfig.User) {
		return config.DatabaseConfig{}, fmt.Errorf("PG_MIGRATION_USER must differ from API_PG_USER so runtime cannot own schema objects")
	}
	migrationConfig := runtimeConfig
	migrationConfig.User = migrationUser
	migrationConfig.Password = runtimeConfig.MigrationPassword
	return migrationConfig, nil
}

func NewAppRepositories(ctx context.Context, db *pgxpool.Pool) (AppRepositories, error) {
	repos := AppRepositories{
		db:                 db,
		userRepo:           identitypostgres.NewRepository(db),
		taskBillingUsage:   taskbilling.NewUsageStore(db),
		systemConfigRepo:   settings.NewSystemConfigRepository(db),
		featurePromptRepo:  prompts.NewFeaturePromptRepository(db),
		storageProfileRepo: storagepostgres.NewRepository(db),
		paymentRepo:        newPaymentStores(paymentpostgres.NewRepository(db)),
		adminBrandLogoRepo: adminbranding.NewRepository(db),
	}
	if err := repos.userRepo.EnsureStorageUIDs(ctx); err != nil {
		return AppRepositories{}, err
	}
	return repos, nil
}

func CloseStartupResources(db *pgxpool.Pool, riverPool *pgxpool.Pool) {
	if riverPool != nil {
		riverPool.Close()
	}
	if db != nil {
		db.Close()
	}
}
