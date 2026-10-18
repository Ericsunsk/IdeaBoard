package workspacemedia

import (
	"context"
	"errors"
	"strings"

	dbsqlc "ideaboard/api/internal/db/sqlc"
	"ideaboard/api/internal/platform/database"

	"github.com/jackc/pgx/v5/pgtype"
)

// authorizeLoadedAsset resolves independent read authorization sources: membership precondition, owner, library fact, task
// fact, upload-target board (live role via the narrow port), then
// referencing boards (live role each via the narrow port). Every denial —
// including non-members, malformed ids, and cross-workspace ids — reports
// ErrNotFound so existence never leaks.
func authorizeLoadedAsset(
	ctx context.Context,
	q *dbsqlc.Queries,
	workspacePublicID string,
	asset *Asset,
	callerUserID int64,
	workspaceRole string,
	access BoardAccess,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if asset == nil {
		return ErrNotFound
	}
	if err := checkMemberPrecondition(workspaceRole); err != nil {
		return err
	}
	trimmedWorkspace := strings.TrimSpace(workspacePublicID)
	if callerUserID > 0 && asset.OwnerUserID == callerUserID {
		return nil
	}
	library, err := q.MediaAssetHasLibraryLink(ctx, dbsqlc.MediaAssetHasLibraryLinkParams{
		PublicID:          asset.ID,
		WorkspacePublicID: trimmedWorkspace,
	})
	if err != nil {
		return err
	}
	if library {
		return nil
	}
	tasks, err := q.ListMediaAssetTaskOrigins(ctx, dbsqlc.ListMediaAssetTaskOriginsParams{
		PublicID:          asset.ID,
		WorkspacePublicID: trimmedWorkspace,
	})
	if err != nil {
		return err
	}
	for _, task := range tasks {
		if task.SubmitterUserID == callerUserID || authorizeBoardRead(ctx, access, trimmedWorkspace, task.BoardPublicID, callerUserID, workspaceRole) {
			return nil
		}
	}
	target, err := q.GetMediaAssetUploadTargetBoard(ctx, dbsqlc.GetMediaAssetUploadTargetBoardParams{
		PublicID:          asset.ID,
		WorkspacePublicID: trimmedWorkspace,
	})
	if err != nil {
		if database.IsNotFound(err) {
			return ErrNotFound
		}
		return err
	}
	if target.Valid && strings.TrimSpace(target.String) != "" {
		if authorizeBoardRead(ctx, access, trimmedWorkspace, target.String, callerUserID, workspaceRole) {
			return nil
		}
	}
	// The document-reference source fans out over the boards that accepted
	// the asset. Reference facts arrive through the narrow port (never a
	// second copy of the table read) and each board resolves live, so
	// losing one board's access never revokes an independent source.
	if access != nil {
		referencing, err := access.ListBoardsReferencingAsset(ctx, trimmedWorkspace, asset.ID)
		if err != nil {
			return err
		}
		for _, boardPublicID := range referencing {
			if authorizeBoardRead(ctx, access, trimmedWorkspace, boardPublicID, callerUserID, workspaceRole) {
				return nil
			}
		}
	}
	return ErrNotFound
}

// AuthorizeAsset resolves one workspace-scoped asset for a caller and
// reports ErrNotFound unless an independent source authorizes the read
// (see access.go for the source order). The returned asset is
// metadata-readable; binary endpoints apply their archived/readiness gates
// after authorization passes.
func (s *Service) AuthorizeAsset(ctx context.Context, workspaceID, assetID string, callerUserID int64, workspaceRole string, access BoardAccess) (*Asset, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := checkMemberPrecondition(workspaceRole); err != nil {
		return nil, err
	}
	db := s.dbWithContext(ctx)
	q, err := db.queries()
	if err != nil {
		return nil, err
	}
	return loadAuthorizedAsset(ctx, q, workspaceID, assetID, callerUserID, workspaceRole, access)
}

func lockWorkspaceMemberRole(ctx context.Context, q *dbsqlc.Queries, workspaceID string, userID int64) (string, error) {
	workspace, err := q.LockWorkspaceMemberships(ctx, strings.TrimSpace(workspaceID))
	if database.IsNotFound(err) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", err
	}
	member, err := q.GetWorkspaceMembership(ctx, dbsqlc.GetWorkspaceMembershipParams{WorkspaceID: workspace, UserID: userID})
	if database.IsNotFound(err) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", err
	}
	if checkMemberPrecondition(member.Role) != nil {
		return "", ErrNotFound
	}
	return member.Role, nil
}

func loadAuthorizedAsset(ctx context.Context, q *dbsqlc.Queries, workspaceID, assetID string, userID int64, role string, access BoardAccess) (*Asset, error) {
	workspaceID = strings.TrimSpace(workspaceID)
	assetID = strings.TrimSpace(assetID)
	if !IsAssetID(assetID) {
		return nil, ErrNotFound
	}
	row, err := q.GetMediaAssetByPublicID(ctx, dbsqlc.GetMediaAssetByPublicIDParams{WorkspacePublicID: workspaceID, PublicID: assetID})
	if database.IsNotFound(err) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	asset := assetFromRow(workspaceID, row)
	if err := authorizeLoadedAsset(ctx, q, workspaceID, asset, userID, role, access); err != nil {
		return nil, err
	}
	return asset, nil
}

// authorizedPageChunk bounds one authorized-list scan chunk. Authorization
// is evaluated per asset in Go (the board role rules must not be copied
// into SQL per #934), so listing scans newest-first chunks and filters
// before paginating.
const authorizedPageChunk = int32(100)

// ListAuthorizedAssets pages the caller's readable assets newest-first.
// Offset counts AUTHORIZED items: filtering happens before pagination so
// page contents and has_more never leak unauthorized rows.
func (s *Service) ListAuthorizedAssets(ctx context.Context, workspaceID string, callerUserID int64, workspaceRole string, access BoardAccess, limit, offset int) ([]*Asset, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := checkMemberPrecondition(workspaceRole); err != nil {
		return nil, err
	}
	if limit < 1 {
		limit = 1
	}
	if limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	db := s.dbWithContext(ctx)
	q, err := db.queries()
	if err != nil {
		return nil, err
	}
	trimmedWorkspace := strings.TrimSpace(workspaceID)
	out := make([]*Asset, 0, limit)
	skipped := 0
	rawOffset := int32(0)
	for {
		rows, err := q.ListMediaAssets(ctx, dbsqlc.ListMediaAssetsParams{
			WorkspacePublicID: trimmedWorkspace,
			RowOffset:         rawOffset,
			RowLimit:          authorizedPageChunk,
		})
		if err != nil {
			return nil, err
		}
		if len(rows) == 0 {
			break
		}
		rawOffset += int32(len(rows))
		for _, row := range rows {
			asset := assetFromRow(trimmedWorkspace, row)
			if err := authorizeLoadedAsset(ctx, q, trimmedWorkspace, asset, callerUserID, workspaceRole, access); err != nil {
				if errors.Is(err, ErrNotFound) {
					continue
				}
				return nil, err
			}
			if skipped < offset {
				skipped++
				continue
			}
			out = append(out, asset)
			if len(out) >= limit {
				return out, nil
			}
		}
		if int32(len(rows)) < authorizedPageChunk {
			break
		}
	}
	return out, nil
}

// resolveTargetBoardInternal maps an optional board binding to its internal
// id inside the calling workspace. Empty binds nothing; misses report
// ErrNotFound so forged or foreign boards never persist.
func resolveTargetBoardInternal(ctx context.Context, q *dbsqlc.Queries, workspacePublicID, boardPublicID string) (pgtype.Int8, error) {
	trimmed := strings.TrimSpace(boardPublicID)
	if trimmed == "" {
		return pgtype.Int8{}, nil
	}
	internalID, err := q.ResolveBoardInternalID(ctx, dbsqlc.ResolveBoardInternalIDParams{
		PublicID:          trimmed,
		WorkspacePublicID: strings.TrimSpace(workspacePublicID),
	})
	if err != nil {
		if database.IsNotFound(err) {
			return pgtype.Int8{}, ErrNotFound
		}
		return pgtype.Int8{}, err
	}
	return pgtype.Int8{Int64: internalID, Valid: true}, nil
}
