package workspacemedia

import (
	"context"
	"fmt"
	"strings"

	"ideaboard/api/internal/db/dbutil"
	dbsqlc "ideaboard/api/internal/db/sqlc"
)

func derivativeFromRow(row dbsqlc.MediaAssetDerivative) *Derivative {
	return &Derivative{
		VariantName: row.VariantName,
		MIMEType:    row.MimeType,
		Width:       int4Value(row.Width),
		Height:      int4Value(row.Height),
		SizeBytes:   row.SizeBytes,
		StorageKey:  row.StorageKey,
		CreatedAt:   dbutil.TimeValue(row.CreatedAt),
	}
}

// RecordDerivative upserts one materialized derivative variant for a
// workspace-scoped asset. Regeneration converges on
// (asset_id, variant_name).
func (s *Service) RecordDerivative(ctx context.Context, workspaceID, assetID string, derivative Derivative) (*Derivative, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	db := s.dbWithContext(ctx)
	q, err := db.queries()
	if err != nil {
		return nil, err
	}
	trimmedWorkspace := strings.TrimSpace(workspaceID)
	asset, err := getSessionRowAsset(ctx, q, trimmedWorkspace, assetID)
	if err != nil {
		return nil, err
	}
	variant := strings.TrimSpace(derivative.VariantName)
	if variant == "" {
		return nil, NewFieldValidationError("variant")
	}
	slot := strings.TrimSpace(derivative.StorageKey)
	if slot == "" {
		return nil, NewFieldValidationError("storage_key")
	}
	row, err := q.UpsertMediaAssetDerivative(ctx, dbsqlc.UpsertMediaAssetDerivativeParams{
		AssetID:     asset.InternalID,
		VariantName: variant,
		StorageKey:  slot,
		MimeType:    strings.TrimSpace(derivative.MIMEType),
		SizeBytes:   derivative.SizeBytes,
		Width:       nullInt4(derivative.Width, derivative.Width > 0),
		Height:      nullInt4(derivative.Height, derivative.Height > 0),
	})
	if err != nil {
		return nil, fmt.Errorf("workspacemedia: record derivative: %w", err)
	}
	return derivativeFromRow(row), nil
}

// ListDerivatives returns stored variants for a workspace-scoped
// asset. Unknown assets read as ErrNotFound.
func (s *Service) ListDerivatives(ctx context.Context, workspaceID, assetID string) ([]*Derivative, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	db := s.dbWithContext(ctx)
	q, err := db.queries()
	if err != nil {
		return nil, err
	}
	asset, err := getSessionRowAsset(ctx, q, strings.TrimSpace(workspaceID), assetID)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListMediaAssetDerivatives(ctx, asset.InternalID)
	if err != nil {
		return nil, fmt.Errorf("workspacemedia: list derivatives: %w", err)
	}
	out := make([]*Derivative, 0, len(rows))
	for _, row := range rows {
		out = append(out, derivativeFromRow(row))
	}
	return out, nil
}
