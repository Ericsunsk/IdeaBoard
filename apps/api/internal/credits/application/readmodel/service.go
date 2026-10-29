package readmodel

import (
	"context"
	"fmt"

	"ideaboard/api/internal/credits/domain"
	"ideaboard/api/internal/platform/database"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Service struct {
	pool    *pgxpool.Pool
	backend Backend
}

type Backend struct {
	GetAvailableBalances func(context.Context, *pgxpool.Pool, []int64) (map[int64]int64, error)
	GetWallet            func(context.Context, *pgxpool.Pool, int64) (domain.WalletDetails, error)
	ListLedger           func(context.Context, *pgxpool.Pool, domain.ListLedgerInput) (domain.LedgerPage, error)
}

func NewService(pool *pgxpool.Pool, backend Backend) (*Service, error) {
	if err := database.RequirePool(pool); err != nil {
		return nil, fmt.Errorf("credits readmodel: %w", err)
	}
	if backend.GetAvailableBalances == nil || backend.GetWallet == nil || backend.ListLedger == nil {
		return nil, fmt.Errorf("credits readmodel: persistence backend not configured")
	}
	return &Service{pool: pool, backend: backend}, nil
}

func (s *Service) GetAvailableBalances(
	ctx context.Context,
	billingAccountIDs []int64,
) (map[int64]int64, error) {
	if s == nil || s.pool == nil {
		return nil, fmt.Errorf("credits service: %w", database.ErrPoolNotConfigured)
	}
	return s.backend.GetAvailableBalances(ctx, s.pool, billingAccountIDs)
}

func (s *Service) GetWallet(ctx context.Context, billingAccountID int64) (domain.WalletDetails, error) {
	if s == nil || s.pool == nil {
		return domain.WalletDetails{}, fmt.Errorf("credits service: %w", database.ErrPoolNotConfigured)
	}
	return s.backend.GetWallet(ctx, s.pool, billingAccountID)
}

func (s *Service) ListLedger(ctx context.Context, input domain.ListLedgerInput) (domain.LedgerPage, error) {
	if s == nil || s.pool == nil {
		return domain.LedgerPage{}, fmt.Errorf("credits service: %w", database.ErrPoolNotConfigured)
	}
	return s.backend.ListLedger(ctx, s.pool, input)
}
