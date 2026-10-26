package boards

// Postgres board access: workspace locks, role resolution, access versions,
// member grants, and media-reference cleanup participation.

import (
	"context"
	"strings"
	"time"

	"ideaboard/api/internal/db/dbutil"
	dbsqlc "ideaboard/api/internal/db/sqlc"
	eventoutbox "ideaboard/api/internal/events/outbox"
	"ideaboard/api/internal/platform/contract"
	"ideaboard/api/internal/platform/database"
	"ideaboard/api/internal/tenancy"
	"ideaboard/api/internal/tenancy/workspacefacts"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// boardRoleTx resolves current membership and selected members through ResolveRole.
// Organization Owner/Admin (#951) count as workspace members via their org
// role even without an explicit workspace row; org Members and personal
// scopes use the direct row only.
func organizationWorkspaceOpenTx(ctx context.Context, q *dbsqlc.Queries, workspace dbsqlc.Workspace) (bool, error) {
	if !workspace.OrganizationID.Valid {
		return true, nil
	}
	closed, err := workspacefacts.IsOrganizationClosed(ctx, q, workspace.PublicID)
	if err != nil {
		return false, err
	}
	return !closed, nil
}

func boardRoleTx(ctx context.Context, q *dbsqlc.Queries, board *V1Board, userID int64) (AccessRole, bool, error) {
	workspace, err := q.GetWorkspaceByPublicID(ctx, board.WorkspacePublicID)
	if err != nil {
		return "", false, err
	}
	open, err := organizationWorkspaceOpenTx(ctx, q, workspace)
	if err != nil {
		return "", false, err
	}
	if !open {
		return "", false, nil
	}
	membership, err := q.GetWorkspaceMembership(ctx, dbsqlc.GetWorkspaceMembershipParams{WorkspaceID: workspace.InternalID, UserID: userID})
	if err == nil {
		if !IsWorkspaceMemberRole(membership.Role) {
			return "", false, nil
		}
	} else if database.IsNotFound(err) {
		if !workspace.OrganizationID.Valid {
			return "", false, nil
		}
		orgMember, orgErr := q.GetOrganizationMembership(ctx, dbsqlc.GetOrganizationMembershipParams{
			OrganizationID: workspace.OrganizationID.Int64, UserID: userID,
		})
		if database.IsNotFound(orgErr) {
			return "", false, nil
		}
		if orgErr != nil {
			return "", false, orgErr
		}
		if orgMember.Role != tenancy.RoleOwner && orgMember.Role != tenancy.RoleAdmin {
			return "", false, nil
		}
	} else {
		return "", false, err
	}
	_, err = q.GetWorkspaceBoardMembership(ctx, dbsqlc.GetWorkspaceBoardMembershipParams{WorkspacePublicID: board.WorkspacePublicID, BoardPublicID: board.PublicID, UserID: userID})
	if err != nil && !database.IsNotFound(err) {
		return "", false, err
	}
	role, visible := ResolveRole(board.OwnerUserID, userID, err == nil, board.WorkspaceAccess, true)
	return role, visible, nil
}

func requireBoardWriteTx(ctx context.Context, q *dbsqlc.Queries, board *V1Board, actorUserID int64) error {
	role, visible, err := boardRoleTx(ctx, q, board, actorUserID)
	if err != nil {
		return err
	}
	return CheckCapability(role, visible, CapWrite)
}

func lockWorkspaceMembersTx(ctx context.Context, q *dbsqlc.Queries, workspacePublicID string) error {
	_, err := q.LockWorkspaceMemberships(ctx, workspacePublicID)
	if database.IsNotFound(err) {
		return ErrNotFound
	}
	return err
}

func lockWritableBoardTx(ctx context.Context, q *dbsqlc.Queries, workspacePublicID, boardPublicID string, actorUserID int64) (*V1Board, error) {
	if err := lockWorkspaceMembersTx(ctx, q, workspacePublicID); err != nil {
		return nil, err
	}
	if _, err := q.LockWorkspaceBoardAccess(ctx, dbsqlc.LockWorkspaceBoardAccessParams{
		WorkspacePublicID: workspacePublicID,
		BoardPublicID:     boardPublicID,
	}); err != nil {
		if database.IsNotFound(err) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	board, err := getPostgresBoard(ctx, q, workspacePublicID, boardPublicID)
	if err != nil {
		return nil, err
	}
	if err := requireBoardWriteTx(ctx, q, board, actorUserID); err != nil {
		return nil, err
	}
	return board, nil
}

// GetBoardRole reports the effective board role through the unified policy
// (creator counts as owner; every non-creator grant needs valid workspace
// membership; selected members receive the common role; Workspace Owner / Admin hold no implicit access).
func (s *PostgresWorkspaceBoardStore) GetBoardRole(ctx context.Context, workspacePublicID, boardPublicID string, userID int64, workspaceRole string) (AccessRole, bool, error) {
	if !IsWorkspaceMemberRole(workspaceRole) {
		return "", false, nil
	}
	if err := ctx.Err(); err != nil {
		return "", false, err
	}
	board, err := s.getBoard(ctx, workspacePublicID, boardPublicID)
	if err != nil {
		return "", false, err
	}
	return boardRoleTx(ctx, s.dbWithContext(ctx).q, board, userID)
}

// ListBoardsReferencingAsset serves the narrow BoardMediaRefReader port
// (#936): the workspace-scoped boards whose accepted document references
// one media asset. Public ids only; unknown or cross-workspace assets list
// empty (fail-closed, never a distinct status).
func (s *PostgresWorkspaceBoardStore) ListBoardsReferencingAsset(ctx context.Context, workspacePublicID, assetPublicID string) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	trimmedAsset := strings.TrimSpace(assetPublicID)
	if !contract.IsAssetID(trimmedAsset) {
		return nil, nil
	}
	rows, err := s.dbWithContext(ctx).q.ListBoardsReferencingMediaAsset(ctx, dbsqlc.ListBoardsReferencingMediaAssetParams{
		WorkspacePublicID: strings.TrimSpace(workspacePublicID),
		PublicID:          trimmedAsset,
	})
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		if trimmed := strings.TrimSpace(row); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out, nil
}

// ListBoardMembers lists selected members ordered by user id.
// The Creator has implicit access and does not need a selection row.
func (s *PostgresWorkspaceBoardStore) ListBoardMembers(ctx context.Context, workspacePublicID, boardPublicID string) ([]*V1BoardMember, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	workspacePublicID = strings.TrimSpace(workspacePublicID)
	trimmedID := strings.TrimSpace(boardPublicID)
	if _, err := s.getBoard(ctx, workspacePublicID, trimmedID); err != nil {
		return nil, err
	}
	rows, err := s.dbWithContext(ctx).q.ListWorkspaceBoardMemberships(ctx, dbsqlc.ListWorkspaceBoardMembershipsParams{
		WorkspacePublicID: workspacePublicID,
		BoardPublicID:     trimmedID,
		RowLimit:          maxWorkspaceBoardRows,
	})
	if err != nil {
		return nil, err
	}
	out := make([]*V1BoardMember, 0, len(rows))
	for _, row := range rows {
		out = append(out, &V1BoardMember{
			BoardPublicID: trimmedID,
			UserID:        row.UserID,
			CreatedAt:     dbutil.TimeValue(row.CreatedAt),
		})
	}
	return out, nil
}

// readBoardAccessTx reads the common role and selected identities together.
func readBoardAccessTx(ctx context.Context, q *dbsqlc.Queries, workspacePublicID, boardPublicID string) (*V1BoardAccess, error) {
	row, err := q.GetWorkspaceBoardAccess(ctx, dbsqlc.GetWorkspaceBoardAccessParams{WorkspacePublicID: workspacePublicID, BoardPublicID: boardPublicID})
	if database.IsNotFound(err) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if !row.OwnerUserID.Valid {
		return nil, ErrNotFound
	}
	members, err := q.ListWorkspaceBoardMemberships(ctx, dbsqlc.ListWorkspaceBoardMembershipsParams{WorkspacePublicID: workspacePublicID, BoardPublicID: boardPublicID, RowLimit: maxWorkspaceBoardRows})
	if err != nil {
		return nil, err
	}
	ids := make([]int64, 0, len(members))
	for _, member := range members {
		ids = append(ids, member.UserID)
	}
	return &V1BoardAccess{WorkspacePublicID: workspacePublicID, BoardPublicID: boardPublicID, WorkspaceAccess: WorkspaceAccess(row.WorkspaceAccess), AccessVersion: row.AccessVersion, AccessState: DeriveAccessState(WorkspaceAccess(row.WorkspaceAccess), len(ids) > 0), MemberUserIDs: ids, ArchivedAt: dbutil.TimePtr(row.ArchivedAt)}, nil
}

func (s *PostgresWorkspaceBoardStore) GetBoardAccess(ctx context.Context, workspacePublicID, boardPublicID string) (*V1BoardAccess, error) {
	workspacePublicID = strings.TrimSpace(workspacePublicID)
	boardPublicID = strings.TrimSpace(boardPublicID)
	var out *V1BoardAccess
	err := s.dbWithContext(ctx).withTx(func(q *dbsqlc.Queries) error {
		if err := lockWorkspaceMembersTx(ctx, q, workspacePublicID); err != nil {
			return err
		}
		var err error
		out, err = readBoardAccessTx(ctx, q, workspacePublicID, boardPublicID)
		return err
	})
	return out, err
}

// ReplaceBoardAccess accepts the complete selected-member policy in one transaction.
func (s *PostgresWorkspaceBoardStore) ReplaceBoardAccess(ctx context.Context, workspacePublicID, boardPublicID string, actorUserID, expectedVersion int64, access WorkspaceAccess, selectedUserIDs []int64) (*V1BoardAccess, error) {
	if !ValidateWorkspaceAccess(access) || actorUserID <= 0 || expectedVersion <= 0 {
		return nil, ErrInvalidInput
	}
	workspacePublicID = strings.TrimSpace(workspacePublicID)
	boardPublicID = strings.TrimSpace(boardPublicID)
	var out *V1BoardAccess
	err := dbutil.WithTxDBTX(ctx, s.db, func(q *dbsqlc.Queries, tx pgx.Tx) error {
		if err := lockWorkspaceMembersTx(ctx, q, workspacePublicID); err != nil {
			return err
		}
		locked, err := q.LockWorkspaceBoardAccess(ctx, dbsqlc.LockWorkspaceBoardAccessParams{WorkspacePublicID: workspacePublicID, BoardPublicID: boardPublicID})
		if database.IsNotFound(err) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if !locked.OwnerUserID.Valid {
			return ErrNotFound
		}
		board := &V1Board{WorkspacePublicID: workspacePublicID, PublicID: boardPublicID, OwnerUserID: locked.OwnerUserID.Int64, WorkspaceAccess: WorkspaceAccess(locked.WorkspaceAccess), ArchivedAt: dbutil.TimePtr(locked.ArchivedAt)}
		role, visible, err := boardRoleTx(ctx, q, board, actorUserID)
		if err != nil {
			return err
		}
		if err := CheckCapability(role, visible, CapManage); err != nil {
			return err
		}
		if board.ArchivedAt != nil {
			return ErrV1Archived
		}
		if locked.AccessVersion != expectedVersion {
			return ErrAccessConflict
		}
		ids, err := selectedBoardMembers(access, selectedUserIDs, board.OwnerUserID)
		if err != nil {
			return err
		}
		workspace, err := q.GetWorkspaceByPublicID(ctx, workspacePublicID)
		if err != nil {
			return err
		}
		for _, id := range ids {
			member, err := q.GetWorkspaceMembership(ctx, dbsqlc.GetWorkspaceMembershipParams{WorkspaceID: workspace.InternalID, UserID: id})
			if database.IsNotFound(err) {
				return ErrMemberNotFound
			}
			if err != nil {
				return err
			}
			if !IsWorkspaceMemberRole(member.Role) {
				return ErrMemberNotFound
			}
		}
		before, err := readBoardAccessTx(ctx, q, workspacePublicID, boardPublicID)
		if err != nil {
			return err
		}
		if before.WorkspaceAccess == access && equalMemberIDs(before.MemberUserIDs, ids) {
			out = before
			return nil
		}
		if _, err := q.ClearBoardMemberships(ctx, dbsqlc.ClearBoardMembershipsParams{WorkspacePublicID: workspacePublicID, BoardPublicID: boardPublicID}); err != nil {
			return err
		}
		for _, id := range ids {
			if _, err := q.InsertWorkspaceBoardMembershipIgnore(ctx, dbsqlc.InsertWorkspaceBoardMembershipIgnoreParams{WorkspacePublicID: workspacePublicID, BoardPublicID: boardPublicID, UserID: id}); err != nil {
				return err
			}
		}
		affected, err := q.SetBoardWorkspaceAccess(ctx, dbsqlc.SetBoardWorkspaceAccessParams{WorkspacePublicID: workspacePublicID, BoardPublicID: boardPublicID, WorkspaceAccess: string(access), ExpectedVersion: expectedVersion})
		if err != nil {
			return err
		}
		if affected != 1 {
			return ErrAccessConflict
		}
		out, err = readBoardAccessTx(ctx, q, workspacePublicID, boardPublicID)
		if err != nil {
			return err
		}
		ordered := changedBoardMembers(before.MemberUserIDs, ids, board.OwnerUserID)
		occurredAt := time.Now().UTC()
		for _, id := range ordered {
			user, err := q.GetUserByID(ctx, id)
			if err != nil {
				return err
			}
			record := eventoutbox.NewBoardAccessOutboxRecord(workspacePublicID, boardPublicID, tenancy.FormatUserPublicID(uuid.UUID(user.PublicID.Bytes)), out.AccessVersion, occurredAt)
			if err := eventoutbox.NewBoardAccessOutboxStore(s.db).InsertTx(ctx, tx, record); err != nil {
				return err
			}
			out.AccessSignals = append(out.AccessSignals, record)
		}
		return nil
	})
	return out, err
}

// RemoveWorkspaceMemberGrants participates in the caller's membership transaction.
// Workspace membership is locked before Board rows, which lock in public_id order.
func (s *PostgresWorkspaceBoardStore) RemoveWorkspaceMemberGrants(ctx context.Context, workspacePublicID string, userID int64) ([]tenancy.MemberGrantCleanup, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if userID <= 0 {
		return nil, ErrInvalidInput
	}
	workspacePublicID = strings.TrimSpace(workspacePublicID)
	if workspacePublicID == "" {
		return nil, ErrInvalidInput
	}
	tx, ok := dbutil.AmbientTxFromContext(ctx)
	if !ok {
		return nil, ErrPermissionDenied
	}

	var out []tenancy.MemberGrantCleanup
	err := s.dbWithContext(ctx).withTx(func(q *dbsqlc.Queries) error {
		grants, err := q.ListWorkspaceMemberBoardGrants(ctx, dbsqlc.ListWorkspaceMemberBoardGrantsParams{
			WorkspacePublicID: workspacePublicID,
			UserID:            userID,
		})
		if err != nil {
			return err
		}
		if len(grants) == 0 {
			return nil
		}
		ids := make([]string, 0, len(grants))
		for _, grant := range grants {
			ids = append(ids, grant.PublicID)
		}
		locked, err := q.LockWorkspaceMemberBoards(ctx, dbsqlc.LockWorkspaceMemberBoardsParams{
			WorkspacePublicID: workspacePublicID,
			BoardPublicIds:    ids,
		})
		if err != nil {
			return err
		}
		if _, err := q.DeleteWorkspaceMemberBoardGrants(ctx, dbsqlc.DeleteWorkspaceMemberBoardGrantsParams{
			WorkspacePublicID: workspacePublicID,
			UserID:            userID,
		}); err != nil {
			return err
		}
		bumped, err := q.BumpWorkspaceMemberBoardsAccess(ctx, dbsqlc.BumpWorkspaceMemberBoardsAccessParams{
			WorkspacePublicID: workspacePublicID,
			BoardPublicIds:    ids,
		})
		if err != nil {
			return err
		}
		bumpedByID := make(map[string]int64, len(bumped))
		for _, row := range bumped {
			bumpedByID[row.PublicID] = row.AccessVersion
		}
		out = make([]tenancy.MemberGrantCleanup, 0, len(locked))
		occurredAt := time.Now().UTC()
		for _, row := range locked {
			cleanup := tenancy.MemberGrantCleanup{BoardPublicID: row.PublicID, AccessVersion: bumpedByID[row.PublicID]}
			for _, recipient := range changedBoardMembers(nil, []int64{userID}, row.OwnerUserID.Int64) {
				user, err := q.GetUserByID(ctx, recipient)
				if err != nil {
					return err
				}
				publicID := tenancy.FormatUserPublicID(uuid.UUID(user.PublicID.Bytes))
				record := eventoutbox.NewBoardAccessOutboxRecord(workspacePublicID, row.PublicID, publicID, cleanup.AccessVersion, occurredAt)
				if err := eventoutbox.NewBoardAccessOutboxStore(s.db).InsertTx(ctx, tx, record); err != nil {
					return err
				}
				cleanup.AccessSignals = append(cleanup.AccessSignals, record)
			}
			out = append(out, cleanup)
		}

		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}
