package tests

import (
	"bytes"
	"context"
	"testing"
	"time"

	"ideaboard/api/internal/credits"
	"ideaboard/api/internal/rewards"
	"ideaboard/api/internal/rewards/domain"
	creditsadapter "ideaboard/api/internal/rewards/infrastructure/credits"
	"ideaboard/api/internal/rewards/infrastructure/postgres"
	"ideaboard/api/internal/rewards/ports"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type (
	CreateRewardCodeCommand     = ports.CreateRewardCodeCommand
	CreatedRewardCode           = ports.CreatedRewardCode
	UpdateRewardCodeCommand     = ports.UpdateRewardCodeCommand
	UpdatedRewardCode           = ports.UpdatedRewardCode
	SetRewardCodeEnabledCommand = ports.SetRewardCodeEnabledCommand
	DeleteRewardCodeCommand     = ports.DeleteRewardCodeCommand
	DeletedRewardCode           = ports.DeletedRewardCode
	SearchRewardCodesInput      = ports.SearchRewardCodesInput
	RewardCodePage              = ports.RewardCodePage
	ListAdminRedemptionsInput   = ports.ListAdminRedemptionsInput
	AdminRedemptionPage         = ports.AdminRedemptionPage
	AdminRedemptionSummary      = ports.AdminRedemptionSummary
	GetAdminRedemptionInput     = ports.GetAdminRedemptionInput
	AdminRedemptionDetail       = ports.AdminRedemptionDetail
	RedeemCommand               = ports.RedeemCommand
	Redemption                  = ports.Redemption
	RedeemResult                = ports.RedeemResult
	RedemptionHistoryItem       = ports.RedemptionHistoryItem
	ListRedemptionsInput        = ports.ListRedemptionsInput
	RedemptionHistoryPage       = ports.RedemptionHistoryPage
	RewardCode                  = ports.RewardCode
	RewardCodeStatus            = ports.RewardCodeStatus
	Service                     = rewards.Service
	UserHandler                 = rewards.UserHandler
	AdminCommandHandler         = rewards.AdminCommandHandler
	AdminReadHandler            = rewards.AdminReadHandler
	IdentityProjection          = rewards.IdentityProjection
	ServiceOptions              = rewards.ServiceOptions
	CodeKey                     = rewards.CodeKey
	CodeKeyring                 = rewards.CodeKeyring

	redemptionHandlerOptions    = rewards.RedemptionHandlerOptions
	redemptionMetricObservation = rewards.RedemptionMetricObservation
	redemptionOutcome           = rewards.RedemptionOutcome
)

var (
	ErrInvalidCommand      = domain.ErrInvalidCode
	ErrCodeCollision       = domain.ErrCodeCollision
	ErrRewardCodeConflict  = domain.ErrRewardCodeConflict
	ErrCodeUnavailable     = domain.ErrCodeUnavailable
	ErrRewardCodeNotFound  = domain.ErrRewardCodeNotFound
	ErrCapBelowRedemptions = domain.ErrCapBelowRedemptions
	ErrRewardCodeRedeemed  = domain.ErrRewardCodeRedeemed
	ErrRedemptionNotFound  = domain.ErrRedemptionNotFound
)

const (
	RewardCodeStatusExpired   = ports.RewardCodeStatusExpired
	RewardCodeStatusExhausted = ports.RewardCodeStatusExhausted
	RewardCodeStatusDisabled  = ports.RewardCodeStatusDisabled
	RewardCodeStatusEnabled   = ports.RewardCodeStatusEnabled

	defaultRedemptionAttemptLimit  = rewards.DefaultRedemptionAttemptLimit
	redemptionOutcomeRateLimited   = rewards.RedemptionOutcomeRateLimited
	redemptionOutcomeSuccess       = rewards.RedemptionOutcomeSuccess
	redemptionOutcomeReplayed      = rewards.RedemptionOutcomeReplayed
	redemptionOutcomeInvalid       = rewards.RedemptionOutcomeInvalid
	redemptionOutcomeInternalError = rewards.RedemptionOutcomeInternalError
)

var (
	NewUserHandler            = rewards.NewUserHandler
	NewAdminCommandHandler    = rewards.NewAdminCommandHandler
	NewAdminReadHandler       = rewards.NewAdminReadHandler
	RuntimePrivilegePolicy    = rewards.RuntimePrivilegePolicy
	NewService                = newRewardsService
	NewServiceWithOptions     = newRewardsServiceWithOptions
	NewCodeKeyring            = rewards.NewCodeKeyring
	newUserHandlerWithOptions = rewards.NewUserHandlerWithOptions
)

func newTestRewardService(t *testing.T, pool *pgxpool.Pool, creditService *credits.Service) *rewards.Service {
	t.Helper()
	return newTestRewardServiceAt(t, pool, creditService, time.Now())
}

func newRewardsService(pool *pgxpool.Pool, creditService *credits.Service, keyring rewards.CodeKeyring) *rewards.Service {
	return rewards.NewService(pool, creditsadapter.New(creditService), keyring)
}

func newRewardsServiceWithOptions(pool *pgxpool.Pool, creditService *credits.Service, keyring rewards.CodeKeyring, opts rewards.ServiceOptions) *rewards.Service {
	return rewards.NewServiceWithOptions(pool, creditsadapter.New(creditService), keyring, opts)
}

func newTestRewardServiceAt(
	t *testing.T,
	pool *pgxpool.Pool,
	creditService *credits.Service,
	now time.Time,
) *rewards.Service {
	t.Helper()
	return rewards.NewServiceWithOptions(pool, creditsadapter.New(creditService), newTestRewardKeyring(t), rewards.ServiceOptions{
		Now: func() time.Time { return now },
	})
}

func newTestRewardKeyring(t *testing.T) rewards.CodeKeyring {
	t.Helper()
	keyring, err := rewards.NewCodeKeyring(rewards.CodeKey{
		Version: "test-v1",
		Secret:  bytes.Repeat([]byte{0x2a}, 32),
	}, nil)
	if err != nil {
		t.Fatalf("NewCodeKeyring() error = %v", err)
	}
	return keyring
}

func provisionRewardWallet(t *testing.T, pool *pgxpool.Pool, creditService *credits.Service, userID int64) {
	t.Helper()
	tx, err := pool.Begin(t.Context())
	if err != nil {
		t.Fatalf("begin wallet transaction: %v", err)
	}
	defer func() { _ = tx.Rollback(t.Context()) }()
	if err := creditService.LifecycleInTx(tx).ProvisionWallet(t.Context(), userID); err != nil {
		t.Fatalf("ProvisionWallet() error = %v", err)
	}
	if err := tx.Commit(t.Context()); err != nil {
		t.Fatalf("commit wallet: %v", err)
	}
}

func lockRewardCodeID(ctx context.Context, tx pgx.Tx, rewardCodeID int64) error {
	return postgres.LockRewardCodeID(ctx, tx, rewardCodeID)
}
