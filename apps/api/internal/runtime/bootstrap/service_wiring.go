package bootstrap

import (
	"context"
	"encoding/base64"
	"fmt"

	"ideaboard/api/internal/billing"
	"ideaboard/api/internal/boards"
	"ideaboard/api/internal/credits"
	"ideaboard/api/internal/identity/auth"
	"ideaboard/api/internal/identity/users"
	"ideaboard/api/internal/media/storage"
	"ideaboard/api/internal/platform/config"
	"ideaboard/api/internal/platform/settings"
	"ideaboard/api/internal/runtime/billingcoverage"

	"github.com/jackc/pgx/v5/pgxpool"
)

type appDomainServices struct {
	authServiceClient *auth.ServiceClient
	runtimeSettings   *settings.RuntimeSettingsService
	storageService    *storage.StorageService
	userService       *users.UserService
	boardService      *boards.BoardService
	billingService    *billing.Service
	billingCoverage   *billingcoverage.Service
	creditService     *credits.Service
	userLifecycle     *users.LifecycleService
}

func newAppDomainServices(ctx context.Context, cfg *config.Config, repos appRepositories, appPool *pgxpool.Pool) (appDomainServices, error) {
	runtimeSettings := settings.NewRuntimeSettingsService(repos.systemConfigRepo)
	storageService := storage.NewStorageService(repos.storageProfileRepo, runtimeSettings)
	if err := storageService.EnsureDefaultProfiles(ctx); err != nil {
		return appDomainServices{}, fmt.Errorf("failed to ensure default storage profiles: %w", err)
	}

	welcomeIdentityKey, err := base64.StdEncoding.DecodeString(cfg.Credits.WelcomeIdentityKey)
	if err != nil || len(welcomeIdentityKey) != 32 {
		return appDomainServices{}, fmt.Errorf("CREDITS_WELCOME_IDENTITY_KEY must be base64 encoded 32 bytes")
	}
	creditService := credits.NewService(appPool)
	billingService := billing.NewService(appPool)
	billingCoverage := billingcoverage.NewService(repos.accountRepo, billingService)
	userLifecycle := users.NewLifecycleService(appPool, creditService, welcomeIdentityKey)
	if err := userLifecycle.VerifyWelcomeIdentityKey(ctx); err != nil {
		return appDomainServices{}, err
	}
	userService := users.NewUserService(repos.userRepo, userLifecycle, storageService)
	return appDomainServices{
		authServiceClient: auth.NewServiceClient(cfg.Auth),
		runtimeSettings:   runtimeSettings,
		storageService:    storageService,
		userService:       userService,
		boardService:      boards.NewBoardService(repos.boardRepo, repos.userRepo, storageService),
		billingService:    billingService,
		billingCoverage:   billingCoverage,
		creditService:     creditService,
		userLifecycle:     userLifecycle,
	}, nil
}
