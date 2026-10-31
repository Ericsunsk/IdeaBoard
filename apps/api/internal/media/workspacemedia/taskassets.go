package workspacemedia

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"ideaboard/api/internal/db/dbutil"
	dbsqlc "ideaboard/api/internal/db/sqlc"
	"ideaboard/api/internal/platform/contract"
)

// outputRole extracts the #819 output role from task_outputs.metadata.
func outputRole(raw []byte) string {
	var decoded struct {
		Role string `json:"role"`
	}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return ""
	}
	return strings.TrimSpace(decoded.Role)
}

// Ownership ruling (#841): ListTaskAssets stays media-side as a
// read-model over task_outputs. The output→asset mapping contract
// (#819: asset public-id in metadata, role projection) is owned
// tasking-side, but this projection joins media_assets rows (media
// tables) with the role envelope from the output metadata. Moving it
// tasking-side would either duplicate the media row projection or
// force tasking to depend on media row shapes; keeping the read here
// leaves tasking as the mapping owner and media as the read owner
// with no write path through this method.
func (s *Service) ListTaskAssets(ctx context.Context, workspaceID, taskID string) ([]*TaskAsset, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	db := s.dbWithContext(ctx)
	q, err := db.queries()
	if err != nil {
		return nil, err
	}
	trimmedWorkspace := strings.TrimSpace(workspaceID)
	if !contract.IsTaskID(strings.TrimSpace(taskID)) {
		return nil, ErrNotFound
	}
	rows, err := q.ListTaskMediaAssets(ctx, dbsqlc.ListTaskMediaAssetsParams{
		PublicID:          strings.TrimSpace(taskID),
		WorkspacePublicID: trimmedWorkspace,
	})
	if err != nil {
		return nil, fmt.Errorf("workspacemedia: list task media: %w", err)
	}
	out := make([]*TaskAsset, 0, len(rows))
	for _, row := range rows {
		out = append(out, &TaskAsset{
			Asset: &Asset{
				InternalID:     row.InternalID,
				ID:             row.PublicID,
				WorkspaceID:    trimmedWorkspace,
				OwnerUserID:    row.OwnerUserID,
				Status:         AssetStatusReady,
				MediaKind:      row.MediaKind,
				MIMEType:       row.MimeType,
				Width:          int4Value(row.Width),
				Height:         int4Value(row.Height),
				SizeBytes:      row.SizeBytes,
				ChecksumSHA256: row.Checksum,
				StorageKey:     row.StorageKey,
				ArchivedAt:     dbutil.TimePtr(row.ArchivedAt),
				CreatedAt:      dbutil.TimeValue(row.CreatedAt),
			},
			OutputIndex: row.OutputIndex,
			Role:        outputRole(row.OutputMetadata),
		})
	}
	return out, nil
}
