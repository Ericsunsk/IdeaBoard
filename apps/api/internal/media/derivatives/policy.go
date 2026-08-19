package derivatives

import (
	"context"
	"fmt"
	"strings"

	dbsqlc "ideaboard/api/internal/db/sqlc"
)

type TierPolicy struct {
	Tier   string
	Format string
}

var defaultTierPolicies = []TierPolicy{
	{Tier: TierThumb, Format: FormatWebP},
	{Tier: TierPreview, Format: FormatWebP},
}

var onDemandTierPolicies = map[string]TierPolicy{
	TierLargePreview: {Tier: TierLargePreview, Format: FormatWebP},
}

func ScheduleDefault(ctx context.Context, q *dbsqlc.Queries, assetID, ownerUserID int64) ([]int64, error) {
	if q == nil || assetID <= 0 || ownerUserID <= 0 {
		return nil, fmt.Errorf("invalid default derivative scheduling request")
	}
	return schedulePolicies(ctx, q, assetID, ownerUserID, defaultTierPolicies)
}

func EnsureOnDemand(ctx context.Context, q *dbsqlc.Queries, assetID, ownerUserID int64, tier string) (int64, error) {
	if q == nil || assetID <= 0 || ownerUserID <= 0 {
		return 0, fmt.Errorf("invalid on-demand derivative request")
	}
	policy, ok := onDemandTierPolicies[strings.TrimSpace(tier)]
	if !ok {
		return 0, fmt.Errorf("unsupported on-demand derivative tier %q", tier)
	}
	derivative, err := q.UpsertMediaDerivative(ctx, dbsqlc.UpsertMediaDerivativeParams{
		MediaAssetID: assetID, OwnerUserID: ownerUserID, Tier: policy.Tier, Format: policy.Format,
	})
	if err != nil {
		return 0, fmt.Errorf("upsert %s derivative: %w", policy.Tier, err)
	}
	if derivative.Status == "ready" {
		return derivative.ID, nil
	}
	if _, err := q.EnqueueMediaDerivativeOutbox(ctx, derivative.ID); err != nil {
		return 0, fmt.Errorf("enqueue %s derivative: %w", policy.Tier, err)
	}
	return derivative.ID, nil
}

func schedulePolicies(ctx context.Context, q *dbsqlc.Queries, assetID, ownerUserID int64, policies []TierPolicy) ([]int64, error) {
	ids := make([]int64, 0, len(policies))
	for _, policy := range policies {
		derivative, err := q.UpsertMediaDerivative(ctx, dbsqlc.UpsertMediaDerivativeParams{
			MediaAssetID: assetID, OwnerUserID: ownerUserID, Tier: policy.Tier, Format: policy.Format,
		})
		if err != nil {
			return nil, fmt.Errorf("upsert %s derivative: %w", policy.Tier, err)
		}
		if derivative.Status == "ready" {
			continue
		}
		if _, err := q.EnqueueMediaDerivativeOutbox(ctx, derivative.ID); err != nil {
			return nil, fmt.Errorf("enqueue %s derivative: %w", policy.Tier, err)
		}
		ids = append(ids, derivative.ID)
	}
	return ids, nil
}
