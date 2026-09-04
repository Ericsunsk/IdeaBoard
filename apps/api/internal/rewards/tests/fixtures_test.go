package tests

import (
	"bytes"
	"context"
	"testing"
	"time"

	"ideaboard/api/internal/credits"
	"ideaboard/api/internal/rewards"
	rewardlocks "ideaboard/api/internal/rewards/application/locks"
	creditsadapter "ideaboard/api/internal/rewards/infrastructure/credits"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type (
	CreateRewardCodeCommand     = rewards.CreateRewardCodeCommand
	CreatedRewardCode           = rewards.CreatedRewardCode
	UpdateRewardCodeCommand     = rewards.UpdateRewardCodeCommand
	UpdatedRewardCode           = rewards.UpdatedRewardCode
	SetRewardCodeEnabledCommand = rewards.SetRewardCodeEnabledCommand
	DeleteRewardCodeCommand     = rewards.DeleteRewardCodeCommand
	DeletedRewardCode           = rewards.DeletedRewardCode
	SearchRewardCodesInput      = rewards.SearchRewardCodesInput
	RewardCodePage              = rewards.RewardCodePage
	ListAdminRedemptionsInput   = rewards.ListAdminRedemptionsInput
	AdminRedemptionPage         = rewards.AdminRedemptionPage
	AdminRedemptionSummary      = rewards.AdminRedemptionSummary
	GetAdminRedemptionInput     = rewards.GetAdminRedemptionInput
	AdminRedemptionDetail       = rewards.AdminRedemptionDetail
	RedeemCommand               = rewards.RedeemCommand
	Redemption                  = rewards.Redemption
	RedeemResult                = rewards.RedeemResult
	RedemptionHistoryItem       = rewards.RedemptionHistoryItem
	ListRedemptionsInput        = rewards.ListRedemptionsInput
	RedemptionHistoryPage       = rewards.RedemptionHistoryPage
	RewardCode                  = rewards.RewardCode
	RewardCodeStatus            = rewards.RewardCodeStatus
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
	redemptionMetricsRecorder   = rewards.RedemptionMetricsRecorder
	redemptionOutcome           = rewards.RedemptionOutcome
)

var (
	ErrInvalidCommand      = rewards.ErrInvalidCommand
	ErrCodeCollision       = rewards.ErrCodeCollision
	ErrRewardCodeConflict  = rewards.ErrRewardCodeConflict
	ErrCodeUnavailable     = rewards.ErrCodeUnavailable
	ErrRewardCodeNotFound  = rewards.ErrRewardCodeNotFound
	ErrCapBelowRedemptions = rewards.ErrCapBelowRedemptions
	ErrRewardCodeRedeemed  = rewards.ErrRewardCodeRedeemed
	ErrRedemptionNotFound  = rewards.ErrRedemptionNotFound
)

const (
	RewardCodeStatusExpired   = rewards.RewardCodeStatusExpired
	RewardCodeStatusExhausted = rewards.RewardCodeStatusExhausted
	RewardCodeStatusDisabled  = rewards.RewardCodeStatusDisabled
	RewardCodeStatusEnabled   = rewards.RewardCodeStatusEnabled

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
	return rewardlocks.LockRewardCodeID(ctx, tx, rewardCodeID)
}
