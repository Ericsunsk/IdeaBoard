package bootstrap

import (
	"context"
	"fmt"

	"ideaboard/api/internal/auth"
	"ideaboard/api/internal/boards"
	"ideaboard/api/internal/platform/config"
	"ideaboard/api/internal/platform/settings"
	"ideaboard/api/internal/storage"
	"ideaboard/api/internal/users"
)

type appDomainServices struct {
	authServiceClient *auth.ServiceClient
	runtimeSettings   *settings.RuntimeSettingsService
	storageService    *storage.StorageService
	userService       *users.UserService
	boardService      *boards.BoardService
}

func newAppDomainServices(ctx context.Context, cfg *config.Config, repos appRepositories) (appDomainServices, error) {
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
	}, nil
}
