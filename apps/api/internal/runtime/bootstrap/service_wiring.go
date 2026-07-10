package bootstrap

import (
	"context"
	"fmt"

	"ideaboard/api/internal/boards"
	"ideaboard/api/internal/credits"
	"ideaboard/api/internal/identity/auth"
	"ideaboard/api/internal/identity/users"
	"ideaboard/api/internal/media/storage"
	"ideaboard/api/internal/platform/config"
	"ideaboard/api/internal/platform/settings"

	"github.com/jackc/pgx/v5/pgxpool"
)

type appDomainServices struct {
	authServiceClient *auth.ServiceClient
	runtimeSettings   *settings.RuntimeSettingsService
	storageService    *storage.StorageService
	userService       *users.UserService
	boardService      *boards.BoardService
	creditService     *credits.Service
}

func newAppDomainServices(ctx context.Context, cfg *config.Config, repos appRepositories, creditPool *pgxpool.Pool) (appDomainServices, error) {
	runtimeSettings := settings.NewRuntimeSettingsService(repos.systemConfigRepo)
	storageService := storage.NewStorageService(repos.storageProfileRepo, runtimeSettings)
	if err := storageService.EnsureDefaultProfiles(ctx); err != nil {
		return appDomainServices{}, fmt.Errorf("failed to ensure default storage profiles: %w", err)
	}

	return appDomainServices{
		authServiceClient: auth.NewServiceClient(cfg.Auth),
		runtimeSettings:   runtimeSettings,
		storageService:    storageService,
		userService:       users.NewUserService(repos.userRepo, storageService),
		boardService:      boards.NewBoardService(repos.boardRepo, repos.userRepo, storageService),
		creditService:     credits.NewService(creditPool),
	}, nil
}
