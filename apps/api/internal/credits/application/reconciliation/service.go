package reconciliation

import (
	"context"
	"fmt"
	"time"

	"ideaboard/api/internal/credits/domain"
	"ideaboard/api/internal/credits/ports"
	"ideaboard/api/internal/platform/adminaction"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Service struct {
	pool       *pgxpool.Pool
	dispatcher ports.ReconciliationRunDispatcher
	backend    Backend
}

type Backend struct {
	CreateReconciliationRun          func(context.Context, *pgxpool.Pool, ports.ReconciliationRunDispatcher, ports.ReconciliationTrigger, *int64, *string, *time.Time, bool) (*int64, bool, error)
	ExecuteReconciliation            func(context.Context, *pgxpool.Pool, int64) error
	FailReconciliation               func(context.Context, *pgxpool.Pool, int64, error) error
	GetReconciliationRun             func(context.Context, *pgxpool.Pool, int64) (ports.ReconciliationRun, error)
	ListReconciliationRuns           func(context.Context, *pgxpool.Pool, ports.ListReconciliationInput) (ports.ReconciliationRunPage, error)
	ListGlobalReconciliationFindings func(context.Context, *pgxpool.Pool, ports.ReconciliationFindingStatus, ports.ListReconciliationInput) (ports.ReconciliationFindingPage, error)
}

func NewService(pool *pgxpool.Pool, dispatcher ports.ReconciliationRunDispatcher, backend Backend) (*Service, error) {
	if pool == nil {
		return nil, fmt.Errorf("credits reconciliation: pool is nil")
	}
	if dispatcher == nil {
		return nil, fmt.Errorf("credits reconciliation: dispatcher is nil")
	}
	if !backendComplete(backend) {
		return nil, fmt.Errorf("credits reconciliation: persistence backend not configured")
	}
	return &Service{pool: pool, dispatcher: dispatcher, backend: backend}, nil
}

func NewServiceWithoutDispatcher(pool *pgxpool.Pool, backend Backend) (*Service, error) {
	if pool == nil {
		return nil, fmt.Errorf("credits reconciliation: pool is nil")
	}
	if !backendComplete(backend) {
		return nil, fmt.Errorf("credits reconciliation: persistence backend not configured")
	}
	return &Service{pool: pool, backend: backend}, nil
}

func backendComplete(backend Backend) bool {
	return backend.CreateReconciliationRun != nil && backend.ExecuteReconciliation != nil && backend.FailReconciliation != nil &&
		backend.GetReconciliationRun != nil && backend.ListReconciliationRuns != nil && backend.ListGlobalReconciliationFindings != nil
}

type startReconciliationInput struct {
	trigger       ports.ReconciliationTrigger
	actorID       int64
	reason        string
	scheduledFor  *time.Time
	onlyIfOverdue bool
}

type normalizedReconciliationInput struct {
	trigger       ports.ReconciliationTrigger
	requestedBy   *int64
	reason        *string
	scheduledFor  *time.Time
	onlyIfOverdue bool
}

func (s *Service) StartReconciliation(
	ctx context.Context,
	input ports.StartManualReconciliationInput,
) (ports.ReconciliationRun, error) {
	run, err := s.startReconciliation(ctx, startReconciliationInput{
		trigger: ports.ReconciliationTriggerManual,
		actorID: input.ActorID,
		reason:  input.Reason,
	})
	if err != nil {
		return ports.ReconciliationRun{}, err
	}
	if run == nil {
		return ports.ReconciliationRun{}, domain.ErrInvalidCommand
	}
	return *run, nil
}

func (s *Service) ScheduleReconciliation(
	ctx context.Context,
	scheduledFor time.Time,
	onlyIfOverdue bool,
) (*ports.ReconciliationRun, error) {
	return s.startReconciliation(ctx, startReconciliationInput{
		trigger:       ports.ReconciliationTriggerScheduled,
		scheduledFor:  &scheduledFor,
		onlyIfOverdue: onlyIfOverdue,
	})
}

func (s *Service) startReconciliation(
	ctx context.Context,
	input startReconciliationInput,
) (*ports.ReconciliationRun, error) {
	if s == nil || s.pool == nil || s.dispatcher == nil {
		return nil, domain.ErrReconciliationUnavailable
	}
	normalized, err := normalizeReconciliationInput(input)
	if err != nil {
		return nil, err
	}
	runID, enqueueRequired, err := s.backend.CreateReconciliationRun(ctx, s.pool, s.dispatcher, normalized.trigger, normalized.requestedBy, normalized.reason, normalized.scheduledFor, normalized.onlyIfOverdue)
	if err != nil {
		return nil, err
	}
	if runID == nil || (input.trigger == ports.ReconciliationTriggerScheduled && !enqueueRequired) {
		return nil, nil
	}
	run, err := s.GetReconciliationRun(ctx, *runID)
	if err != nil {
		return nil, err
	}
	return &run, nil
}

func normalizeReconciliationInput(input startReconciliationInput) (normalizedReconciliationInput, error) {
	normalized := normalizedReconciliationInput{
		trigger:       input.trigger,
		onlyIfOverdue: input.onlyIfOverdue,
	}
	switch input.trigger {
	case ports.ReconciliationTriggerManual:
		action, _ := adminaction.New(input.actorID, input.reason)
		if !action.IsValid() {
			return normalizedReconciliationInput{}, domain.ErrInvalidCommand
		}
		actorID := action.ActorID()
		reason := action.Reason()
		normalized.requestedBy = &actorID
		normalized.reason = &reason
	case ports.ReconciliationTriggerScheduled:
		if input.scheduledFor == nil || input.scheduledFor.IsZero() {
			return normalizedReconciliationInput{}, domain.ErrInvalidCommand
		}
		scheduledFor := input.scheduledFor.UTC().Truncate(time.Second)
		normalized.scheduledFor = &scheduledFor
	default:
		return normalizedReconciliationInput{}, domain.ErrInvalidCommand
	}
	return normalized, nil
}

func (s *Service) ExecuteReconciliation(ctx context.Context, runID int64) error {
	if s == nil || s.pool == nil {
		return domain.ErrInvalidCommand
	}
	return s.backend.ExecuteReconciliation(ctx, s.pool, runID)
}

func (s *Service) FailReconciliation(ctx context.Context, runID int64, cause error) error {
	if s == nil || s.pool == nil {
		return domain.ErrInvalidCommand
	}
	return s.backend.FailReconciliation(ctx, s.pool, runID, cause)
}

func (s *Service) GetReconciliationRun(ctx context.Context, runID int64) (ports.ReconciliationRun, error) {
	if s == nil || s.pool == nil {
		return ports.ReconciliationRun{}, domain.ErrInvalidCommand
	}
	return s.backend.GetReconciliationRun(ctx, s.pool, runID)
}

func (s *Service) ListReconciliationRuns(ctx context.Context, input ports.ListReconciliationInput) (ports.ReconciliationRunPage, error) {
	if s == nil || s.pool == nil {
		return ports.ReconciliationRunPage{}, domain.ErrInvalidCommand
	}
	return s.backend.ListReconciliationRuns(ctx, s.pool, input)
}

func (s *Service) ListGlobalReconciliationFindings(ctx context.Context, status ports.ReconciliationFindingStatus, input ports.ListReconciliationInput) (ports.ReconciliationFindingPage, error) {
	if s == nil || s.pool == nil {
		return ports.ReconciliationFindingPage{}, domain.ErrInvalidCommand
	}
	return s.backend.ListGlobalReconciliationFindings(ctx, s.pool, status, input)
}
