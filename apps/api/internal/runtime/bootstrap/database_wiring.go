package bootstrap

import (
	"context"
	"database/sql"
	"fmt"

	"ideaboard/api/internal/accounts"
	"ideaboard/api/internal/assets"
	"ideaboard/api/internal/boards"
	"ideaboard/api/internal/channels"
	"ideaboard/api/internal/db/migrations"
	"ideaboard/api/internal/platform/config"
	"ideaboard/api/internal/platform/database"
	"ideaboard/api/internal/platform/settings"
	"ideaboard/api/internal/prompts"
	"ideaboard/api/internal/runtime/worker"
	"ideaboard/api/internal/storage"
	"ideaboard/api/internal/taskstore"
	"ideaboard/api/internal/users"

	"github.com/jackc/pgx/v5/pgxpool"
)

type appDatabases struct {
	db        *sql.DB
	riverPool *pgxpool.Pool
}

type appRepositories struct {
	userRepo           *users.UserRepository
	accountRepo        *accounts.AccountRepository
	taskRepo           *taskstore.Repository
	channelRepo        *channels.ChannelRepository
	systemConfigRepo   *settings.SystemConfigRepository
	featurePromptRepo  *prompts.FeaturePromptRepository
	storageProfileRepo *storage.StorageProfileRepository
	lineDraftModeRepo  *assets.LineDraftModeRepository
	boardRepo          *boards.BoardRepository
}

func openAppDatabases(ctx context.Context, cfg *config.Config) (appDatabases, error) {
	db, err := database.NewPostgres(cfg.Database)
	if err != nil {
		return appDatabases{}, fmt.Errorf("failed to connect to database: %w", err)
	}
	if err := migrations.Up(ctx, db); err != nil {
		_ = db.Close()
		return appDatabases{}, fmt.Errorf("failed to migrate database schema: %w", err)
	}
	riverPool, err := worker.NewRiverPgxPool(ctx, database.PostgresDSN(cfg.Database))
	if err != nil {
		_ = db.Close()
		return appDatabases{}, fmt.Errorf("failed to connect River database pool: %w", err)
	}
	if err := worker.MigrateRiver(ctx, riverPool); err != nil {
		riverPool.Close()
		_ = db.Close()
		return appDatabases{}, fmt.Errorf("failed to migrate River schema: %w", err)
	}
	return appDatabases{db: db, riverPool: riverPool}, nil
}

func newAppRepositories(ctx context.Context, db *sql.DB) (appRepositories, error) {
	repos := appRepositories{
		userRepo:           users.NewUserRepository(db),
		accountRepo:        accounts.NewAccountRepository(db),
		taskRepo:           taskstore.NewRepository(db),
		channelRepo:        channels.NewChannelRepository(db),
		systemConfigRepo:   settings.NewSystemConfigRepository(db),
		featurePromptRepo:  prompts.NewFeaturePromptRepository(db),
		storageProfileRepo: storage.NewStorageProfileRepository(db),
		lineDraftModeRepo:  assets.NewLineDraftModeRepository(db),
		boardRepo:          boards.NewBoardRepository(db),
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
