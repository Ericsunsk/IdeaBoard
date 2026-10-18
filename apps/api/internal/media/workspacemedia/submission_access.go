package workspacemedia

import (
	"context"
	"errors"
	"strings"

	"ideaboard/api/internal/db/dbutil"
	dbsqlc "ideaboard/api/internal/db/sqlc"
	"ideaboard/api/internal/tasking/workspacetasks"

	"github.com/jackc/pgx/v5"
)

// TaskSubmissionAuthorizer checks live authorization on the accepting settlement
// transaction. The Workspace lock serializes every source Board and membership.
type TaskSubmissionAuthorizer struct {
	Boards     BoardAccess
	TaskBoards workspacetasks.BoardAuthorizer
}

func (a *TaskSubmissionAuthorizer) AuthorizeSubmission(ctx context.Context, tx pgx.Tx, input workspacetasks.CreateInput) (workspacetasks.TaskBoardBinding, error) {
	var binding workspacetasks.TaskBoardBinding
	if a == nil || tx == nil || a.Boards == nil {
		return binding, workspacetasks.ErrSettlementUnavailable
	}
	ctx = dbutil.ContextWithAmbientTx(ctx, tx)
	q := dbsqlc.New(tx)
	workspaceID := strings.TrimSpace(input.WorkspaceID)
	role, err := lockWorkspaceMemberRole(ctx, q, workspaceID, input.OwnerUserID)
	if errors.Is(err, ErrNotFound) {
		return binding, workspacetasks.ErrNotFound
	}
	if err != nil {
		return binding, err
	}
	if boardID := strings.TrimSpace(input.BoardID); boardID != "" {
		if a.TaskBoards == nil {
			return binding, workspacetasks.ErrBoardNotFound
		}
		access, err := a.TaskBoards.ResolveBoardAccess(ctx, workspaceID, boardID, input.OwnerUserID, role)
		if err != nil {
			return binding, err
		}
		if access == nil || !access.CanRead {
			return binding, workspacetasks.ErrBoardNotFound
		}
		if access.Archived {
			return binding, workspacetasks.ErrBoardArchived
		}
		if !access.CanGenerate {
			return binding, workspacetasks.ErrForbidden
		}
		binding = workspacetasks.TaskBoardBinding{BoardID: boardID, AccessVersion: access.AccessVersion}
	}
	ids := append([]string(nil), input.MediaAssetIDs...)
	if maskID, ok := workspacetasks.MaskAssetIDFromParams(input.Params); ok {
		ids = append(ids, maskID)
	}
	for _, id := range ids {
		asset, err := loadAuthorizedAsset(ctx, q, workspaceID, id, input.OwnerUserID, role, a.Boards)
		if errors.Is(err, ErrNotFound) {
			return binding, workspacetasks.ErrMediaNotFound
		}
		if err != nil {
			return binding, err
		}
		if asset.IsArchived() {
			return binding, workspacetasks.ErrMediaArchived
		}
		if asset.Status != AssetStatusReady {
			return binding, workspacetasks.ErrMediaNotReady
		}
	}
	return binding, nil
}
