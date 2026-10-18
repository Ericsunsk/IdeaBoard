// Package rewards is the composition root and public facade for the Rewards bounded context.
package rewards

import (
	"context"
	"fmt"
	"time"

	"ideaboard/api/internal/rewards/application/redemptions"
	"ideaboard/api/internal/rewards/application/rewardcodes"
	"ideaboard/api/internal/rewards/domain"
	"ideaboard/api/internal/rewards/infrastructure/postgres"
	rewardhttp "ideaboard/api/internal/rewards/interfaces/http"
	"ideaboard/api/internal/rewards/ports"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Service is the unified rewards domain service facade.
type Service struct {
	pool        *pgxpool.Pool
	keyring     domain.CodeKeyring
	rewardCodes *rewardcodes.Service
	redemptions *redemptions.Service
	now         func() time.Time
}

// ServiceOptions configures the composition root.
type ServiceOptions struct {
	Now func() time.Time
}

// NewService constructs a new Rewards composition root.
func NewService(pool *pgxpool.Pool, credits redemptions.CreditGateway, keyring domain.CodeKeyring) *Service {
	return NewServiceWithOptions(pool, credits, keyring, ServiceOptions{})
}

// NewServiceWithOptions constructs a new Rewards composition root with custom options.

func NewServiceWithOptions(pool *pgxpool.Pool, credits redemptions.CreditGateway, keyring domain.CodeKeyring, opts ServiceOptions) *Service {
	locker := postgres.NewLocker()
	return &Service{
		pool:        pool,
		keyring:     keyring,
		rewardCodes: rewardcodes.NewServiceWithOptions(pool, keyring, rewardcodes.ServiceOptions{Now: opts.Now, Locker: locker}),
		redemptions: redemptions.NewServiceWithOptions(pool, credits, keyring, redemptions.ServiceOptions{Now: opts.Now, Locker: locker}),
		now:         opts.Now,
	}
}

type CodeKey = domain.CodeKey

func NewCodeKeyring(current CodeKey, previous *CodeKey) (CodeKeyring, error) {
	return domain.NewCodeKeyring(current, previous)
}

// ValidateActiveCodeKeyVersions validates that all persisted codes can be verified with the active keyring.
func (s *Service) ValidateActiveCodeKeyVersions(ctx context.Context) error {
	if s == nil || s.pool == nil {
		return fmt.Errorf("validate reward Code key versions: service unavailable")
	}
	return rewardcodes.ValidateActiveCodeKeyVersions(ctx, s.pool, s.keyring)
}

// --- AdminCommands implementation ---

func (s *Service) CreateRewardCode(ctx context.Context, cmd ports.CreateRewardCodeCommand) (ports.CreatedRewardCode, error) {
	return s.rewardCodes.CreateRewardCode(ctx, cmd)
}

func (s *Service) UpdateRewardCode(ctx context.Context, cmd ports.UpdateRewardCodeCommand) (ports.UpdatedRewardCode, error) {
	return s.rewardCodes.UpdateRewardCode(ctx, cmd)
}

func (s *Service) SetRewardCodeEnabled(ctx context.Context, cmd ports.SetRewardCodeEnabledCommand) (ports.RewardCode, error) {
	return s.rewardCodes.SetRewardCodeEnabled(ctx, cmd)
}

func (s *Service) DeleteRewardCode(ctx context.Context, cmd ports.DeleteRewardCodeCommand) (ports.DeletedRewardCode, error) {
	return s.rewardCodes.DeleteRewardCode(ctx, cmd)
}

// --- AdminReads implementation ---

func (s *Service) SearchRewardCodes(ctx context.Context, input ports.SearchRewardCodesInput) (ports.RewardCodePage, error) {
	return s.rewardCodes.SearchRewardCodes(ctx, input)
}

func (s *Service) ListAdminRedemptions(ctx context.Context, input ports.ListAdminRedemptionsInput) (ports.AdminRedemptionPage, error) {
	return s.rewardCodes.ListAdminRedemptions(ctx, input)
}

func (s *Service) GetAdminRedemption(ctx context.Context, input ports.GetAdminRedemptionInput) (ports.AdminRedemptionDetail, error) {
	return s.rewardCodes.GetAdminRedemption(ctx, input)
}

// --- UserCommands implementation ---

func (s *Service) Redeem(ctx context.Context, cmd ports.RedeemCommand) (ports.RedeemResult, error) {
	return s.redemptions.Redeem(ctx, cmd)
}

// --- UserReads implementation ---

func (s *Service) ListRedemptions(ctx context.Context, input ports.ListRedemptionsInput) (ports.RedemptionHistoryPage, error) {
	return s.redemptions.ListRedemptions(ctx, input)
}

// --- Re-exports for consumers and bootstrap ---

type UserHandler = rewardhttp.UserHandler
type AdminCommandHandler = rewardhttp.AdminCommandHandler
type AdminReadHandler = rewardhttp.AdminReadHandler

type IdentityProjection = ports.IdentityProjection
type IdentityProjectionReader = ports.IdentityProjectionReader
type CodeKeyring = domain.CodeKeyring
type RedemptionHandlerOptions = rewardhttp.RedemptionHandlerOptions

const DefaultRedemptionAttemptLimit = rewardhttp.DefaultRedemptionAttemptLimit

func ParseCodeKeyring(raw string) (CodeKeyring, error) {
	return domain.ParseCodeKeyring(raw)
}

// Compile-time interface assertion checks.
var (
	_ ports.AdminCommands       = (*Service)(nil)
	_ ports.AdminReads          = (*Service)(nil)
	_ ports.UserCommands        = (*Service)(nil)
	_ ports.UserReads           = (*Service)(nil)
	_ ports.UserService         = (*Service)(nil)
	_ ports.AdminCommandService = (*Service)(nil)
	_ ports.AdminReadService    = (*Service)(nil)
)
