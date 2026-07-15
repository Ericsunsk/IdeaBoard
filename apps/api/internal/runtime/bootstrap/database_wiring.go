package bootstrap

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"ideaboard/api/internal/accountpool/accounts"
	"ideaboard/api/internal/accountpool/providerendpoints"
	"ideaboard/api/internal/billing"
	"ideaboard/api/internal/boards"
	"ideaboard/api/internal/credits"
	"ideaboard/api/internal/db/migrations"
	"ideaboard/api/internal/identity/users"
	"ideaboard/api/internal/media/assets"
	"ideaboard/api/internal/media/storage"
	"ideaboard/api/internal/platform/config"
	"ideaboard/api/internal/platform/database"
	dbprivileges "ideaboard/api/internal/platform/database/privileges"
	"ideaboard/api/internal/platform/settings"
	"ideaboard/api/internal/prompts"
	"ideaboard/api/internal/runtime/worker"
	"ideaboard/api/internal/tasking/taskbilling"
	"ideaboard/api/internal/tasking/taskstore"

	"github.com/jackc/pgx/v5/pgxpool"
)

type appDatabases struct {
	db        *sql.DB
	riverPool *pgxpool.Pool
}

type appDatabaseOpeners struct {
	postgres func(config.DatabaseConfig) (*sql.DB, error)
	river    func(context.Context, string) (*pgxpool.Pool, error)
}

type appRepositories struct {
	userRepo             *users.UserRepository
	accountRepo          *accounts.AccountRepository
	taskRepo             *taskstore.Repository
	taskBillingUsage     *taskbilling.UsageStore
	providerEndpointRepo *providerendpoints.ProviderEndpointRepository
	systemConfigRepo     *settings.SystemConfigRepository
	featurePromptRepo    *prompts.FeaturePromptRepository
	storageProfileRepo   *storage.StorageProfileRepository
	lineDraftModeRepo    *assets.LineDraftModeRepository
	boardRepo            *boards.BoardRepository
}

func openAppDatabases(ctx context.Context, cfg *config.Config) (appDatabases, error) {
	return openAppDatabasesWithOpeners(ctx, cfg, appDatabaseOpeners{
		postgres: database.NewPostgres,
		river:    worker.NewRiverPgxPool,
	})
}

func openAppDatabasesWithOpeners(
	ctx context.Context,
	cfg *config.Config,
	openers appDatabaseOpeners,
) (appDatabases, error) {
	if cfg == nil {
		return appDatabases{}, fmt.Errorf("database config is nil")
	}
	if openers.postgres == nil || openers.river == nil {
		return appDatabases{}, fmt.Errorf("database openers are incomplete")
	}
	db, err := openers.postgres(cfg.Database)
	if err != nil {
		return appDatabases{}, fmt.Errorf("failed to connect to database: %w", err)
	}
	if err := verifyRuntimePrivileges(ctx, db); err != nil {
		return appDatabases{}, closeStartupDatabase(db, err)
	}
	riverPool, err := openers.river(ctx, database.PostgresDSN(cfg.Database))
	if err != nil {
		return appDatabases{}, closeStartupDatabase(db, fmt.Errorf("failed to connect River database pool: %w", err))
	}
	return appDatabases{db: db, riverPool: riverPool}, nil
}

func closeStartupDatabase(db *sql.DB, cause error) error {
	if closeErr := db.Close(); closeErr != nil {
		return errors.Join(cause, fmt.Errorf("close startup database: %w", closeErr))
	}
	return cause
}

func migrateAppDatabases(ctx context.Context, cfg *config.Config) error {
	if cfg == nil {
		return fmt.Errorf("database config is nil")
	}
	migrationConfig, err := separatedMigrationDatabaseConfig(cfg.Database)
	if err != nil {
		return err
	}
	migrationDB, err := database.NewPostgres(migrationConfig)
	if err != nil {
		return fmt.Errorf("failed to connect migration database: %w", err)
	}
	defer migrationDB.Close()
	if err := migrations.Up(ctx, migrationDB); err != nil {
		return fmt.Errorf("failed to migrate database schema: %w", err)
	}
	migrationPool, err := worker.NewRiverPgxPool(ctx, database.PostgresDSN(migrationConfig))
	if err != nil {
		return fmt.Errorf("failed to connect River migration pool: %w", err)
	}
	defer migrationPool.Close()
	if err := worker.MigrateRiver(ctx, migrationPool); err != nil {
		return fmt.Errorf("failed to migrate River schema: %w", err)
	}
	privilegeTx, err := migrationDB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin runtime privilege transaction: %w", err)
	}
	if err := dbprivileges.Configure(ctx, privilegeTx, cfg.Database.User, runtimePrivilegePolicies()...); err != nil {
		return rollbackPrivilegeTransaction(privilegeTx, err)
	}
	if err := privilegeTx.Commit(); err != nil {
		return fmt.Errorf("commit runtime privileges: %w", err)
	}
	return nil
}

func runtimePrivilegePolicies() []dbprivileges.Policy {
	return []dbprivileges.Policy{
		database.RuntimePrivilegePolicy(),
		credits.RuntimePrivilegePolicy(),
		billing.RuntimePrivilegePolicy(),
		taskbilling.RuntimePrivilegePolicy(),
		worker.RuntimePrivilegePolicy(),
	}
}

func verifyRuntimePrivileges(ctx context.Context, db dbprivileges.DBTX) error {
	return dbprivileges.Verify(ctx, db, runtimePrivilegePolicies()...)
}

func rollbackPrivilegeTransaction(tx *sql.Tx, cause error) error {
	rollbackErr := tx.Rollback()
	if rollbackErr == nil || errors.Is(rollbackErr, sql.ErrTxDone) {
		return cause
	}
	return errors.Join(cause, fmt.Errorf("rollback runtime privileges: %w", rollbackErr))
}

func separatedMigrationDatabaseConfig(runtimeConfig config.DatabaseConfig) (config.DatabaseConfig, error) {
	migrationUser := strings.TrimSpace(runtimeConfig.MigrationUser)
	if migrationUser == "" || runtimeConfig.MigrationPassword == "" {
		return config.DatabaseConfig{}, fmt.Errorf("DB_MIGRATION_USER and DB_MIGRATION_PASSWORD are required for migrations")
	}
	if migrationUser == strings.TrimSpace(runtimeConfig.User) {
		return config.DatabaseConfig{}, fmt.Errorf("DB_MIGRATION_USER must differ from DB_USER so runtime cannot own schema objects")
	}
	migrationConfig := runtimeConfig
	migrationConfig.User = migrationUser
	migrationConfig.Password = runtimeConfig.MigrationPassword
	return migrationConfig, nil
}

func newAppRepositories(ctx context.Context, db *sql.DB) (appRepositories, error) {
	taskRepo := taskstore.NewRepository(db)
	repos := appRepositories{
		userRepo:             users.NewUserRepository(db),
		accountRepo:          accounts.NewAccountRepositoryWithRunningTaskFinalizer(db, taskRepo),
		taskRepo:             taskRepo,
		taskBillingUsage:     taskbilling.NewUsageStore(db),
		providerEndpointRepo: providerendpoints.NewProviderEndpointRepository(db),
		systemConfigRepo:     settings.NewSystemConfigRepository(db),
		featurePromptRepo:    prompts.NewFeaturePromptRepository(db),
		storageProfileRepo:   storage.NewStorageProfileRepository(db),
		lineDraftModeRepo:    assets.NewLineDraftModeRepository(db),
		boardRepo:            boards.NewBoardRepository(db),
	}
	if err := repos.userRepo.EnsureStorageUIDs(ctx); err != nil {
		return appRepositories{}, err
	}
	return repos, nil
}

func closeStartupResources(db *sql.DB, riverPool *pgxpool.Pool) {
	if riverPool != nil {
		riverPool.Close()
	}
	if db != nil {
		_ = db.Close()
	}
}
