package health

import (
	"context"
	"fmt"
	"time"

	"ideaboard/api/internal/credits/domain"
	"ideaboard/api/internal/credits/ports"
	"ideaboard/api/internal/platform/database"

	"github.com/jackc/pgx/v5/pgxpool"
)

const CreditHealthFreshnessWindow = 48 * time.Hour

type CreditHealthStatus string

const (
	CreditHealthUnchecked CreditHealthStatus = "unchecked"
	CreditHealthHealthy   CreditHealthStatus = "healthy"
	CreditHealthAttention CreditHealthStatus = "attention"
	CreditHealthCritical  CreditHealthStatus = "critical"
	CreditHealthStale     CreditHealthStatus = "stale"
	CreditHealthFailed    CreditHealthStatus = "failed"
)

type CreditHealthFindingSummary = ports.CreditHealthFindingSummary

type CreditHealth struct {
	Status                CreditHealthStatus         `json:"status"`
	ActiveRun             *ports.ReconciliationRun   `json:"active_run,omitempty"`
	LatestSuccessfulRun   *ports.ReconciliationRun   `json:"latest_successful_run,omitempty"`
	LatestFailedRun       *ports.ReconciliationRun   `json:"latest_failed_run,omitempty"`
	OpenFindings          CreditHealthFindingSummary `json:"open_findings"`
	RestrictedWalletCount int64                      `json:"restricted_wallet_count"`
}

type Service struct {
	pool    *pgxpool.Pool
	backend Backend
}

type Backend struct {
	GetSnapshot func(context.Context, *pgxpool.Pool) (ports.CreditHealthSnapshot, error)
}

func NewService(pool *pgxpool.Pool, backend Backend) (*Service, error) {
	if err := database.RequirePool(pool); err != nil {
		return nil, fmt.Errorf("credits health: %w", err)
	}
	if backend.GetSnapshot == nil {
		return nil, fmt.Errorf("credits health: persistence backend not configured")
	}
	return &Service{pool: pool, backend: backend}, nil
}

func (s *Service) GetCreditHealth(ctx context.Context) (CreditHealth, error) {
	if s == nil || s.pool == nil {
		return CreditHealth{}, domain.ErrReconciliationUnavailable
	}
	snapshot, err := s.backend.GetSnapshot(ctx, s.pool)
	if err != nil {
		return CreditHealth{}, err
	}
	health := CreditHealth{
		ActiveRun:             snapshot.ActiveRun,
		LatestSuccessfulRun:   snapshot.LatestSuccessfulRun,
		LatestFailedRun:       snapshot.LatestFailedRun,
		OpenFindings:          snapshot.OpenFindings,
		RestrictedWalletCount: snapshot.RestrictedWalletCount,
	}
	health.Status = DeriveCreditHealthStatus(snapshot.SnapshotTime, snapshot.LatestSuccessfulRun, snapshot.LatestFailedRun, snapshot.OpenFindings, snapshot.RestrictedWalletCount)
	return health, nil
}

func DeriveCreditHealthStatus(
	now time.Time,
	latestSuccessfulRun *ports.ReconciliationRun,
	latestFailedRun *ports.ReconciliationRun,
	openFindings CreditHealthFindingSummary,
	restrictedWalletCount int64,
) CreditHealthStatus {
	if openFindings.HighRisk > 0 || restrictedWalletCount > 0 {
		return CreditHealthCritical
	}
	if latestFailedRun != nil && latestFailedRun.CompletedAt != nil &&
		(latestSuccessfulRun == nil || latestSuccessfulRun.CompletedAt == nil ||
			latestFailedRun.CompletedAt.After(*latestSuccessfulRun.CompletedAt)) {
		return CreditHealthFailed
	}
	if latestSuccessfulRun == nil || latestSuccessfulRun.CompletedAt == nil {
		return CreditHealthUnchecked
	}
	if now.After(latestSuccessfulRun.CompletedAt.Add(CreditHealthFreshnessWindow)) {
		return CreditHealthStale
	}
	if openFindings.Total > 0 {
		return CreditHealthAttention
	}
	return CreditHealthHealthy
}
