package workspacemedia

import (
	"context"
	"strings"

	"ideaboard/api/internal/boards"
	dbsqlc "ideaboard/api/internal/db/sqlc"
	"ideaboard/api/internal/platform/database"
)

// ValidateDocumentAssets checks every referenced asset before a document
// changes its durable references. Queries and board access must use the
// caller's accepting transaction so existing references are read before
// they are replaced, and newly supplied ids cannot authorize themselves.
func ValidateDocumentAssets(ctx context.Context, q *dbsqlc.Queries, workspaceID string, actorUserID int64, access boards.DocumentMediaAccess, assetIDs []string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	if q == nil {
		return errNotConfigured
	}
	workspace, err := q.GetWorkspaceByPublicID(ctx, strings.TrimSpace(workspaceID))
	if database.IsNotFound(err) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	membership, err := q.GetWorkspaceMembership(ctx, dbsqlc.GetWorkspaceMembershipParams{WorkspaceID: workspace.InternalID, UserID: actorUserID})
	if database.IsNotFound(err) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	workspaceRole := membership.Role
	if err := checkMemberPrecondition(workspaceRole); err != nil {
		return ErrNotFound
	}
	workspaceID = strings.TrimSpace(workspaceID)
	for _, assetID := range assetIDs {
		if _, err := loadAuthorizedAsset(ctx, q, workspaceID, assetID, actorUserID, workspaceRole, access); err != nil {
			return err
		}
	}
	return nil
}
