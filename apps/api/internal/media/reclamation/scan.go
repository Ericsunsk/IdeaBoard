package reclamation

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"ideaboard/api/internal/media/maintenance"
)

const StaleUploadingAfter = maintenance.StaleUploadingAfter

type ScanReport struct {
	StaleUploadingAssets       int64
	StaleProcessingDerivatives int64
	OrphanAssets               int64
	PendingDeletionAssets      int64
}

// Scan reports media lifecycle states that need operator attention. It is
// intentionally read-only; the workers remain the only mutation path.
func Scan(ctx context.Context, db *sql.DB, staleAfter time.Duration) (ScanReport, error) {
	if db == nil {
		return ScanReport{}, fmt.Errorf("media reclamation scan database is nil")
	}
	if staleAfter <= 0 {
		staleAfter = StaleUploadingAfter
	}
	var report ScanReport
	queries := []struct {
		name  string
		dest  *int64
		query string
		args  []any
	}{
		{
			name:  "stale uploading assets",
			dest:  &report.StaleUploadingAssets,
			query: `SELECT COUNT(*) FROM media_assets WHERE status = 'uploading' AND updated_at < $1`,
			args:  []any{time.Now().Add(-staleAfter)},
		},
		{
			name: "orphan assets",
			dest: &report.OrphanAssets,
			query: `SELECT COUNT(*)
FROM media_assets AS asset
WHERE asset.deleted_at IS NULL
  AND asset.status <> 'deleted'
  AND asset.reclaim_after IS NULL
  AND NOT EXISTS (SELECT 1 FROM media_refs AS ref WHERE ref.media_asset_id = asset.id)`,
			args: nil,
		},
		{
			name:  "stale processing derivatives",
			dest:  &report.StaleProcessingDerivatives,
			query: `SELECT COUNT(*) FROM media_derivatives WHERE status = 'processing' AND updated_at < $1`,
			args:  []any{time.Now().Add(-maintenance.ProcessingLease)},
		},
		{
			name:  "pending deletion assets",
			dest:  &report.PendingDeletionAssets,
			query: `SELECT COUNT(*) FROM media_assets WHERE deleted_at IS NULL AND reclaim_after IS NOT NULL`,
			args:  nil,
		},
	}
	for _, item := range queries {
		if err := db.QueryRowContext(ctx, item.query, item.args...).Scan(item.dest); err != nil {
			return ScanReport{}, fmt.Errorf("scan %s: %w", item.name, err)
		}
	}
	return report, nil
}
