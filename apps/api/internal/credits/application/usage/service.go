package usage

import (
	"context"
	"fmt"
	"time"

	"ideaboard/api/internal/credits/domain"
	"ideaboard/api/internal/platform/database"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Service struct {
	pool    *pgxpool.Pool
	backend Backend
}

type Backend struct {
	GetCaptureUsage      func(context.Context, *pgxpool.Pool, int64, domain.UsageFilter, *time.Location) (int64, int64, time.Time, error)
	GetDailyCaptureUsage func(context.Context, *pgxpool.Pool, int64, domain.UsageFilter, *time.Location, time.Time, time.Time) (map[string]int64, error)
}

func NewService(pool *pgxpool.Pool, backend Backend) (*Service, error) {
	if err := database.RequirePool(pool); err != nil {
		return nil, fmt.Errorf("credits usage: %w", err)
	}
	if backend.GetCaptureUsage == nil || backend.GetDailyCaptureUsage == nil {
		return nil, fmt.Errorf("credits usage: persistence backend not configured")
	}
	return &Service{pool: pool, backend: backend}, nil
}

func (s *Service) GetCaptureUsage(
	ctx context.Context,
	billingAccountID int64,
	filter domain.UsageFilter,
	location *time.Location,
) (totalCredits int64, peakDailyCredits int64, firstCaptureDay time.Time, err error) {
	if s == nil || s.pool == nil {
		return 0, 0, time.Time{}, fmt.Errorf("credits service: %w", database.ErrPoolNotConfigured)
	}
	if billingAccountID <= 0 || !filter.Valid() {
		return 0, 0, time.Time{}, domain.ErrInvalidCommand
	}
	return s.backend.GetCaptureUsage(ctx, s.pool, billingAccountID, filter, location)
}

func (s *Service) GetDailyCaptureUsage(
	ctx context.Context,
	billingAccountID int64,
	filter domain.UsageFilter,
	location *time.Location,
	start time.Time,
	endExclusive time.Time,
) (map[string]int64, error) {
	if s == nil || s.pool == nil {
		return nil, fmt.Errorf("credits service: %w", database.ErrPoolNotConfigured)
	}
	return s.backend.GetDailyCaptureUsage(ctx, s.pool, billingAccountID, filter, location, start, endExclusive)
}
