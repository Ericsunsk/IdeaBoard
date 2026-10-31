package previewscene

import (
	"context"
	"fmt"

	dbsqlc "ideaboard/api/internal/db/sqlc"

	"github.com/jackc/pgx/v5/pgtype"
)

// BackfillStats reports one BackfillBoardPreviews run.
type BackfillStats struct {
	Scanned int
	Updated int
	Scenes  int
}

// BackfillBoardPreviews assigns missing content identities and projections in
// internal-id ordered pages of batchSize (clamped to 1..1000). At most
// maxBoards rows are processed when maxBoards > 0. With dryRun no writes
// are issued and only the first page is reported (scanned rows never
// leave the predicate without writes, so paging further would loop).
func BackfillBoardPreviews(ctx context.Context, q *dbsqlc.Queries, batchSize, maxBoards int, dryRun bool) (BackfillStats, error) {
	var stats BackfillStats
	if q == nil {
		return stats, fmt.Errorf("boards: backfill requires queries")
	}
	if batchSize < 1 {
		batchSize = 1
	}
	if batchSize > 1000 {
		batchSize = 1000
	}
	for {
		if err := ctx.Err(); err != nil {
			return stats, err
		}
		if maxBoards > 0 && stats.Scanned >= maxBoards {
			return stats, nil
		}
		rows, err := q.ScanBoardDocumentsMissingPreview(ctx, int32(batchSize))
		if err != nil {
			return stats, err
		}
		if len(rows) == 0 {
			return stats, nil
		}
		for _, row := range rows {
			if maxBoards > 0 && stats.Scanned >= maxBoards {
				return stats, nil
			}
			stats.Scanned++
			version, scene, _ := DeriveCatalog(row.SnapshotData)
			if version == "" {
				// Empty stored bytes: nothing to identify; leave the
				// row for a future document accept (monotonic only if
				// every scanned row leaves the predicate — an empty
				// snapshot cannot occur from the store paths, which
				// reject empty documents, so fail closed here).
				return stats, fmt.Errorf("boards: backfill found empty snapshot for board %d", row.BoardID)
			}
			if !dryRun {
				affected, err := q.UpdateBoardDocumentPreview(ctx, dbsqlc.UpdateBoardDocumentPreviewParams{
					ContentVersion: pgtype.Text{String: version, Valid: true},
					PreviewScene:   []byte(scene),
					BoardID:        row.BoardID,
				})
				if err != nil {
					return stats, err
				}
				if affected == 0 {
					// Won by a concurrent run: already healed.
					continue
				}
			}
			stats.Updated++
			if len(scene) > 0 {
				stats.Scenes++
			}
		}
		if dryRun {
			// A dry run writes nothing, so scanned rows never leave
			// the predicate: report the first page only instead of
			// looping over it forever.
			return stats, nil
		}
	}
}
