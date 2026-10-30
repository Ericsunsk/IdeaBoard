package workspacemedia

import (
	"context"
	"errors"
	"fmt"
	"ideaboard/api/internal/db/dbutil"
	dbsqlc "ideaboard/api/internal/db/sqlc"
	"ideaboard/api/internal/tasking/workspacetasks"
	"ideaboard/api/internal/tenancy/workspacefacts"
	"strings"

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
	// Closed Organizations suspend new spending (#955): the settlement
	// transaction fails closed even if the HTTP prelude was bypassed.
	// Terminal Capture/Release on row-pinned accounts still converge
	// (they never pass this gate), so history survives closure.
	if closed, err := workspacefacts.IsOrganizationClosed(ctx, q, workspaceID); err != nil {
		return binding, err
	} else if closed {
		return binding, workspacetasks.ErrForbidden
	}
	if boardID := strings.TrimSpace(input.BoardID); boardID != "" {
		if a.TaskBoards == nil {
			return binding, workspacetasks.ErrSettlementUnavailable
		}
		access, err := a.TaskBoards.ResolveBoardAccess(ctx, workspaceID, boardID, input.OwnerUserID, role)
		if err != nil {
			return binding, err
		}
		// The submit gate is the shared board policy (P0-2): the same
		// submit/read/cancel Check tasking uses. Nil postures deny as
		// not-found; the decision maps onto the tasking submit sentinels.
		var facts workspacetasks.BoardTaskFacts
		if access != nil {
			facts = workspacetasks.BoardTaskFacts{
				CanRead: access.CanRead, CanGenerate: access.CanGenerate,
				CanWrite: access.CanWrite, Archived: access.Archived,
			}
		}
		switch workspacetasks.BoardPolicy.Check(facts, workspacetasks.BoardOpSubmit) {
		case workspacetasks.BoardDecisionAllow:
			binding = workspacetasks.TaskBoardBinding{BoardID: boardID, AccessVersion: access.AccessVersion}
		case workspacetasks.BoardDecisionArchived:
			return binding, workspacetasks.ErrBoardArchived
		case workspacetasks.BoardDecisionForbidden:
			return binding, workspacetasks.ErrForbidden
		default:
			return binding, workspacetasks.ErrBoardNotFound
		}
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
		// Single readiness gate (P0-4): GateAsset owns the
		// archived/ready comparison; its sentinels cross the seam as-is.
		if err := GateAsset(asset); err != nil {
			return binding, err
		}
	}
	return binding, nil
}

// TaskAssetAdapter supplies readiness facts for worker execution of accepted
// immutable inputs. Submission authorization belongs to TaskSubmissionAuthorizer.
type TaskAssetAdapter struct {
	Store AssetStore
}

// Compile-time convergence with the task seam.
var _ workspacetasks.AssetReader = (*TaskAssetAdapter)(nil)

// GetAsset projects one workspace-scoped asset to the minimal
// ready-gate facts tasks need. Unknown, malformed, and
// cross-workspace ids converge to ErrMediaNotFound (invisible, never
// a distinct status).
func (a *TaskAssetAdapter) GetAsset(ctx context.Context, workspaceID, assetID string) (*workspacetasks.AssetInfo, error) {
	if a == nil || a.Store == nil {
		return nil, fmt.Errorf("workspacemedia: task asset store is not configured: %w", workspacetasks.ErrMediaNotFound)
	}
	asset, err := a.Store.GetAsset(ctx, strings.TrimSpace(workspaceID), strings.TrimSpace(assetID))
	if err != nil {
		return nil, fmt.Errorf("workspacemedia: task asset lookup: %w", workspacetasks.ErrMediaNotFound)
	}
	// Single readiness gate (P0-4): archived and non-ready deny through
	// GateAsset; the sentinel (not the status string) crosses the seam.
	if err := GateAsset(asset); err != nil {
		return nil, fmt.Errorf("workspacemedia: task asset %s gate: %w", asset.ID, err)
	}
	return &workspacetasks.AssetInfo{
		ID:          asset.ID,
		WorkspaceID: asset.WorkspaceID,
		Status:      asset.Status,
		Archived:    false,
		Width:       asset.Width,
		Height:      asset.Height,
		MIMEType:    asset.MIMEType,
	}, nil
}
