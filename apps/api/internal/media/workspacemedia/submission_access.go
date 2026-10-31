package workspacemedia

import (
	"context"
	"errors"
	"fmt"
	"ideaboard/api/internal/db/dbutil"
	dbsqlc "ideaboard/api/internal/db/sqlc"
	taskdomain "ideaboard/api/internal/tasking/domain"
	taskports "ideaboard/api/internal/tasking/ports/tasks"
	"ideaboard/api/internal/tenancy/workspacefacts"
	"strings"

	"github.com/jackc/pgx/v5"
)

// TaskSubmissionAuthorizer checks live authorization on the accepting settlement
// transaction. The Workspace lock serializes every source Board and membership.
type TaskSubmissionAuthorizer struct {
	Boards     BoardAccess
	TaskBoards taskports.BoardAuthorizer
}

func (a *TaskSubmissionAuthorizer) AuthorizeSubmission(ctx context.Context, tx pgx.Tx, input taskdomain.CreateInput) (taskdomain.TaskBoardBinding, error) {
	var binding taskdomain.TaskBoardBinding
	if a == nil || tx == nil || a.Boards == nil {
		return binding, taskdomain.ErrSettlementUnavailable
	}
	ctx = dbutil.ContextWithAmbientTx(ctx, tx)
	q := dbsqlc.New(tx)
	workspaceID := strings.TrimSpace(input.WorkspaceID)
	role, err := lockWorkspaceMemberRole(ctx, q, workspaceID, input.OwnerUserID)
	if errors.Is(err, ErrNotFound) {
		return binding, taskdomain.ErrNotFound
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
		return binding, taskdomain.ErrForbidden
	}
	if boardID := strings.TrimSpace(input.BoardID); boardID != "" {
		if a.TaskBoards == nil {
			return binding, taskdomain.ErrSettlementUnavailable
		}
		access, err := a.TaskBoards.ResolveBoardAccess(ctx, workspaceID, boardID, input.OwnerUserID, role)
		if err != nil {
			return binding, err
		}
		// The submit gate is the shared board policy (P0-2): the same
		// submit/read/cancel Check tasking uses. Nil postures deny as
		// not-found; the decision maps onto the tasking submit sentinels.
		var facts taskdomain.BoardTaskFacts
		if access != nil {
			facts = taskdomain.BoardTaskFacts{
				CanRead: access.CanRead, CanGenerate: access.CanGenerate,
				CanWrite: access.CanWrite, Archived: access.Archived,
			}
		}
		switch taskdomain.BoardPolicy.Check(facts, taskdomain.BoardOpSubmit) {
		case taskdomain.BoardDecisionAllow:
			binding = taskdomain.TaskBoardBinding{BoardID: boardID, AccessVersion: access.AccessVersion}
		case taskdomain.BoardDecisionArchived:
			return binding, taskdomain.ErrBoardArchived
		case taskdomain.BoardDecisionForbidden:
			return binding, taskdomain.ErrForbidden
		default:
			return binding, taskdomain.ErrBoardNotFound
		}
	}
	ids := append([]string(nil), input.MediaAssetIDs...)
	if maskID, ok := taskdomain.MaskAssetIDFromParams(input.Params); ok {
		ids = append(ids, maskID)
	}
	for _, id := range ids {
		asset, err := loadAuthorizedAsset(ctx, q, workspaceID, id, input.OwnerUserID, role, a.Boards)
		if errors.Is(err, ErrNotFound) {
			return binding, taskdomain.ErrMediaNotFound
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
var _ taskports.AssetReader = (*TaskAssetAdapter)(nil)

// GetAsset projects one workspace-scoped asset to the minimal
// ready-gate facts tasks need. Unknown, malformed, and
// cross-workspace ids converge to ErrMediaNotFound (invisible, never
// a distinct status).
func (a *TaskAssetAdapter) GetAsset(ctx context.Context, workspaceID, assetID string) (*taskdomain.AssetInfo, error) {
	if a == nil || a.Store == nil {
		return nil, fmt.Errorf("workspacemedia: task asset store is not configured: %w", taskdomain.ErrMediaNotFound)
	}
	asset, err := a.Store.GetAsset(ctx, strings.TrimSpace(workspaceID), strings.TrimSpace(assetID))
	if err != nil {
		return nil, fmt.Errorf("workspacemedia: task asset lookup: %w", taskdomain.ErrMediaNotFound)
	}
	// Single readiness gate (P0-4): archived and non-ready deny through
	// GateAsset; the sentinel (not the status string) crosses the seam.
	if err := GateAsset(asset); err != nil {
		return nil, fmt.Errorf("workspacemedia: task asset %s gate: %w", asset.ID, err)
	}
	return &taskdomain.AssetInfo{
		ID:          asset.ID,
		WorkspaceID: asset.WorkspaceID,
		Status:      asset.Status,
		Archived:    false,
		Width:       asset.Width,
		Height:      asset.Height,
		MIMEType:    asset.MIMEType,
	}, nil
}
