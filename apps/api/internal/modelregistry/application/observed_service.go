package application

import (
	"context"
	"errors"
	"fmt"
	"time"

	"ideaboard/api/internal/modelregistry/domain"
)

// ObservedService composes attempt facts and the billing SKU catalog into
// the admin models-observed read model.
type ObservedService struct {
	attempts domain.ObservedAttemptStore
	billing  domain.SKUCatalogReader
}

// NewObservedService builds the service over its two narrow seams.
func NewObservedService(attempts domain.ObservedAttemptStore, billing domain.SKUCatalogReader) *ObservedService {
	return &ObservedService{attempts: attempts, billing: billing}
}

// ListModelsObserved returns per-model 7-day first-party observations with
// SKU margin rows. Models absent from the result have no attempts in the
// window; callers render them as 暂无数据.
func (s *ObservedService) ListModelsObserved(ctx context.Context) ([]domain.ModelObservedRow, error) {
	if s == nil || s.attempts == nil || s.billing == nil {
		return nil, errors.New("model observation service is unavailable")
	}
	since := time.Now().UTC().Add(-domain.ObservedWindowDays * 24 * time.Hour)
	obs, err := s.attempts.ListAttemptObservations(ctx, since)
	if err != nil {
		return nil, fmt.Errorf("list model attempt observations: %w", err)
	}
	skuCards, err := s.billing.ListSKUCatalog(ctx)
	if err != nil {
		return nil, fmt.Errorf("list billing SKU catalog: %w", err)
	}
	return domain.AggregateModelObservations(obs, skuCards, since), nil
}

// Compile-time seam check: the service satisfies the ports reader consumed
// by the handler.
var _ = (*ObservedService)(nil)
