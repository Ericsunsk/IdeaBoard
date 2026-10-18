package workspacemedia

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"ideaboard/api/internal/db/dbutil"
	dbsqlc "ideaboard/api/internal/db/sqlc"
	librarydomain "ideaboard/api/internal/library/domain"
	libraryports "ideaboard/api/internal/library/ports"
	"ideaboard/api/internal/platform/database"
	"ideaboard/api/internal/tasking/workspacetasks"

	"github.com/jackc/pgx/v5"
)

// LibraryPublicationAuthorizer owns Media usage and explicit publication rights.
type LibraryPublicationAuthorizer struct {
	Boards     BoardAccess
	TaskBoards workspacetasks.BoardAuthorizer
}

var _ libraryports.PublicationAuthorizer = (*LibraryPublicationAuthorizer)(nil)

func (a *LibraryPublicationAuthorizer) AuthorizeManual(ctx context.Context, tx pgx.Tx, workspaceID string, input librarydomain.V1SaveManagedAssetInput) error {
	if a == nil || tx == nil || a.Boards == nil || a.TaskBoards == nil {
		return errNotConfigured
	}
	ctx = dbutil.ContextWithAmbientTx(ctx, tx)
	q := dbsqlc.New(tx)
	role, err := lockWorkspaceMemberRole(ctx, q, workspaceID, input.OperatorUserID)
	if errors.Is(err, ErrNotFound) {
		return librarydomain.ErrV1AssetNotFound
	}
	if err != nil {
		return err
	}
	asset, err := loadAuthorizedAsset(ctx, q, workspaceID, input.MediaAssetID, input.OperatorUserID, role, a.Boards)
	if errors.Is(err, ErrNotFound) {
		return librarydomain.ErrV1AssetNotFound
	}
	if err != nil {
		return err
	}
	// Single readiness gate (P0-4): any gate denial (archived or non-ready)
	// converges to not-found for publication, as before.
	if err := GateAsset(asset); err != nil {
		return librarydomain.ErrV1AssetNotFound
	}
	provenances, err := q.ListMediaAssetTaskOrigins(ctx, dbsqlc.ListMediaAssetTaskOriginsParams{WorkspacePublicID: workspaceID, PublicID: asset.ID})
	if err != nil {
		return err
	}
	// Board attribution controls publication even when an Editor submitted the Task
	// or owns its generated Media row.
	if len(provenances) > 0 {
		for _, provenance := range provenances {
			if boardID := strings.TrimSpace(provenance.BoardPublicID); boardID != "" {
				if !a.isBoardCreator(ctx, workspaceID, boardID, input.OperatorUserID, role) {
					return workspacetasks.ErrForbidden
				}
			} else if provenance.SubmitterUserID != input.OperatorUserID {
				return workspacetasks.ErrForbidden
			}
		}
		return nil
	}
	if asset.OwnerUserID == input.OperatorUserID {
		return nil
	}
	published, err := q.MediaAssetHasLibraryLink(ctx, dbsqlc.MediaAssetHasLibraryLinkParams{WorkspacePublicID: workspaceID, PublicID: asset.ID})
	if err != nil {
		return err
	}
	if published {
		return nil
	}
	target, err := q.GetMediaAssetUploadTargetBoard(ctx, dbsqlc.GetMediaAssetUploadTargetBoardParams{WorkspacePublicID: workspaceID, PublicID: asset.ID})
	if err != nil && !database.IsNotFound(err) {
		return err
	}
	if target.Valid && a.isBoardCreator(ctx, workspaceID, target.String, input.OperatorUserID, role) {
		return nil
	}
	references, err := a.Boards.ListBoardsReferencingAsset(ctx, workspaceID, asset.ID)
	if err != nil {
		return err
	}
	for _, boardID := range references {
		if a.isBoardCreator(ctx, workspaceID, boardID, input.OperatorUserID, role) {
			return nil
		}
	}
	return workspacetasks.ErrForbidden
}

func (a *LibraryPublicationAuthorizer) isBoardCreator(ctx context.Context, workspaceID, boardID string, userID int64, role string) bool {
	if strings.TrimSpace(boardID) == "" {
		return false
	}
	access, err := a.TaskBoards.ResolveBoardAccess(ctx, workspaceID, boardID, userID, role)
	return err == nil && access != nil && access.CanRead && access.IsOwner && !access.Archived
}

// AuthorizeAutomatic uses persisted Task identity; a completion callback cannot
// publish Board-scoped output into the Workspace-wide Library.
func (a *LibraryPublicationAuthorizer) AuthorizeAutomatic(ctx context.Context, tx pgx.Tx, workspaceID string, input librarydomain.V1SaveManagedAssetInput) (bool, error) {
	if a == nil || tx == nil {
		return false, errNotConfigured
	}
	if input.Lineage == nil || input.OperatorUserID <= 0 {
		return false, librarydomain.ErrV1AssetNotFound
	}
	ctx = dbutil.ContextWithAmbientTx(ctx, tx)
	q := dbsqlc.New(tx)
	role, err := lockWorkspaceMemberRole(ctx, q, workspaceID, input.OperatorUserID)
	if errors.Is(err, ErrNotFound) {
		return false, librarydomain.ErrV1AssetNotFound
	}
	if err != nil {
		return false, err
	}
	task, err := q.GetWorkspaceTaskByPublicID(ctx, dbsqlc.GetWorkspaceTaskByPublicIDParams{WorkspacePublicID: workspaceID, PublicID: input.Lineage.TaskID})
	if database.IsNotFound(err) {
		return false, librarydomain.ErrV1AssetNotFound
	}
	if err != nil {
		return false, err
	}
	if task.SubmitterUserID != input.OperatorUserID || task.Status != "completed" {
		return false, librarydomain.ErrV1AssetNotFound
	}
	var binding struct {
		BoardID string `json:"board_id"`
	}
	if err := json.Unmarshal(task.WorkflowInput, &binding); err != nil {
		return false, err
	}
	if strings.TrimSpace(binding.BoardID) != "" {
		return false, nil
	}
	outputs, err := q.ListWorkspaceTaskOutputs(ctx, dbsqlc.ListWorkspaceTaskOutputsParams{WorkspacePublicID: workspaceID, PublicID: input.Lineage.TaskID})
	if err != nil {
		return false, err
	}
	asset, err := loadAuthorizedAsset(ctx, q, workspaceID, input.MediaAssetID, input.OperatorUserID, role, a.Boards)
	if errors.Is(err, ErrNotFound) {
		return false, librarydomain.ErrV1AssetNotFound
	}
	if err != nil {
		return false, err
	}
	// Single readiness gate (P0-4): any gate denial (archived or non-ready)
	// converges to not-found for publication, as before.
	if err := GateAsset(asset); err != nil {
		return false, librarydomain.ErrV1AssetNotFound
	}
	for _, output := range outputs {
		if output.OutputIndex == input.Lineage.ResultIndex && output.OutputKind == "media_asset" && output.MediaAssetID.Valid && output.MediaAssetID.Int64 == asset.InternalID {
			return true, nil
		}
	}
	return false, librarydomain.ErrV1AssetNotFound
}
