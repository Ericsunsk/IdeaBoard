package boards

// Postgres board catalog cards: Gallery listing without full documents.

import (
	"context"
	"encoding/json"
	"strings"

	"ideaboard/api/internal/db/dbutil"
	dbsqlc "ideaboard/api/internal/db/sqlc"
)

func mapPostgresBoardCatalogRow(row dbsqlc.ListWorkspaceBoardCatalogRow) (*V1BoardCatalogEntry, error) {
	if !row.OwnerUserID.Valid {
		// Owner-less rows predate the owner column: fail closed.
		return nil, ErrNotFound
	}
	role, ok := mapPostgresBoardRole(row.EffectiveRole.String)
	if !ok {
		return nil, ErrNotFound
	}
	hasDirectGrant, _ := row.HasDirectGrant.(bool)
	entry := &V1BoardCatalogEntry{
		WorkspacePublicID: row.WorkspacePublicID.String,
		PublicID:          row.PublicID.String,
		Name:              row.Title.String,
		Revision:          int64(row.DocumentVersion.Int32),
		SchemaVersion:     row.SchemaVersion.Int32,
		ArchivedAt:        dbutil.TimePtr(row.ArchivedAt),
		OwnerUserID:       row.OwnerUserID.Int64,
		CreatedBy:         row.OwnerUserID.Int64,
		WorkspaceAccess:   WorkspaceAccess(strings.TrimSpace(row.WorkspaceAccess.String)),
		AccessVersion:     row.AccessVersion.Int64,
		EffectiveRole:     role,
		HasDirectGrant:    hasDirectGrant,
		AccessState:       DeriveAccessState(WorkspaceAccess(strings.TrimSpace(row.WorkspaceAccess.String)), row.HasAnyGrants),
		CreatedAt:         dbutil.TimeValue(row.CreatedAt),
		UpdatedAt:         dbutil.TimeValue(row.UpdatedAt),
	}
	if row.ContentVersion.Valid {
		entry.ContentVersion = row.ContentVersion.String
	}
	if len(row.PreviewScene) > 0 {
		scene := make(json.RawMessage, len(row.PreviewScene))
		copy(scene, row.PreviewScene)
		entry.PreviewScene = scene
	}
	return entry, nil
}

// ListBoardCatalog reads only the requested authorized cards and a bounded
// total count, without loading full documents or out-of-page scene bodies.
func (s *PostgresWorkspaceBoardStore) ListBoardCatalog(ctx context.Context, workspacePublicID string, includeArchived bool, userID int64, workspaceRole string, page V1BoardCatalogPageRequest) (V1BoardCatalogPage, error) {
	if err := ctx.Err(); err != nil {
		return V1BoardCatalogPage{}, err
	}
	if err := page.validate(); err != nil {
		return V1BoardCatalogPage{}, err
	}
	rows, err := s.dbWithContext(ctx).q.ListWorkspaceBoardCatalog(ctx, dbsqlc.ListWorkspaceBoardCatalogParams{
		WorkspacePublicID: strings.TrimSpace(workspacePublicID),
		IncludeArchived:   includeArchived,
		IsWorkspaceMember: IsWorkspaceMemberRole(workspaceRole),
		CatalogLimit:      maxBoardCatalogRows,
		RowOffset:         int32(page.Offset),
		RowLimit:          int32(page.Limit),
		UserID:            dbutil.NullInt64(&userID),
	})
	if err != nil {
		return V1BoardCatalogPage{}, err
	}
	out := V1BoardCatalogPage{Entries: make([]*V1BoardCatalogEntry, 0, len(rows))}
	for _, row := range rows {
		if int64(page.Offset) > row.TotalCount {
			return V1BoardCatalogPage{}, ErrV1CatalogOffset
		}
		out.HasMore = int64(page.Offset+page.Limit) < row.TotalCount
		if !row.InternalID.Valid {
			continue
		}
		entry, err := mapPostgresBoardCatalogRow(row)
		if err != nil {
			return V1BoardCatalogPage{}, err
		}
		out.Entries = append(out.Entries, entry)
	}
	return out, nil
}
