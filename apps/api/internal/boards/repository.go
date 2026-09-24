package boards

import (
	"context"
	"errors"
	"strings"

	dbsqlc "ideaboard/api/internal/db/sqlc"
	"ideaboard/api/internal/platform/database"

	"github.com/jackc/pgx/v5/pgxpool"
)

// errLegacyBoardsRemoved is the #817 (Phase 3b-08) fail-closed stand-in for
// the deleted legacy user-scoped boards postgres adapter.
//
// All board sqlc queries (ListAccessibleBoardsByUserID,
// GetAccessibleBoardByBoardID, GetBoardByBoardID, GetBoardByOwnerAndBoardID,
// UpsertBoard, CountBoardsByBoardID, CountBoardNameConflicts,
// GetBoardMemberRole, ListMediaRefsByBoardID, DeleteBoardMediaRefsByOwnerAndBoardID,
// DeleteBoardShareTokensByBoardID, DeleteBoardMembersByBoardID,
// DeleteBoardByOwnerAndBoardID, ScheduleUnreferencedMediaAssetsForReclamation,
// CancelMediaAssetReclamation, ListBoardMembers, UpsertBoardMember,
// DeleteBoardMember, GetBoardMemberByUserID, CreateBoardShareToken,
// LockBoardShareTokenIdempotency, GetBoardShareTokenByIdempotency,
// ListBoardShareTokens, GetBoardShareTokenByHash, RevokeBoardShareTokenByID,
// ConsumeBoardShareToken, CountBoardShareTokenByHash, UpsertMediaRef) were
// removed with the boards table rename to the workspace-scoped model
// (boards with public_id board_%), so every method below fails closed.
// The workspace-scoped rebuild (WorkspaceBoardStore in v1_workspace.go,
// postgres mapping + immutable board_revisions table) lands in Phase 4;
// until then legacy /api/user/boards routes and board-gated upload paths
// fail closed through these stubs.
var errLegacyBoardsRemoved = errors.New("legacy boards store removed (#817 Phase 3b-08): workspace-scoped store lands in Phase 4")

type Repository struct {
	db *pgxpool.Pool
	q  *dbsqlc.Queries
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db, q: dbsqlc.New(db)}
}

func (r *Repository) ListAccessibleByUserID(_ context.Context, _ int64) ([]WithRole, error) {
	return nil, errLegacyBoardsRemoved
}

func (r *Repository) GetAccessibleByBoardID(_ context.Context, _ int64, _ string) (*WithRole, error) {
	return nil, errLegacyBoardsRemoved
}

func (r *Repository) GetByBoardID(_ context.Context, _ string) (*Board, error) {
	return nil, errLegacyBoardsRemoved
}

func (r *Repository) GetByOwnerAndBoardID(_ context.Context, _ int64, _ string) (*Board, error) {
	return nil, errLegacyBoardsRemoved
}

func (r *Repository) Upsert(_ context.Context, _ *Board) error {
	return errLegacyBoardsRemoved
}

func (r *Repository) HasNameConflict(_ context.Context, _ int64, _, _ string) (bool, error) {
	return false, errLegacyBoardsRemoved
}

func (r *Repository) BoardIDExists(_ context.Context, _ string) (bool, error) {
	return false, errLegacyBoardsRemoved
}

func (r *Repository) GetMemberRole(_ context.Context, _ string, _ int64) (AccessRole, error) {
	return "", errLegacyBoardsRemoved
}

func (r *Repository) DeleteBoardCascade(_ context.Context, _ int64, _ string) error {
	return errLegacyBoardsRemoved
}

func (r *Repository) ListMembers(_ context.Context, _ int64, _ string) ([]MemberWithUser, error) {
	return nil, errLegacyBoardsRemoved
}

func (r *Repository) UpsertMember(_ context.Context, _ int64, _ string, _ int64, _ AccessRole) error {
	return errLegacyBoardsRemoved
}

func (r *Repository) DeleteMember(_ context.Context, _ int64, _ string, _ int64) error {
	return errLegacyBoardsRemoved
}

func (r *Repository) GetMemberByUserID(_ context.Context, _ int64, _ string, _ int64) (*Member, error) {
	return nil, errLegacyBoardsRemoved
}

func (r *Repository) CreateShareToken(_ context.Context, _ *ShareToken) error {
	return errLegacyBoardsRemoved
}

func (r *Repository) CreateShareTokenIdempotent(_ context.Context, _ *ShareToken, _, _ string) (*ShareToken, bool, error) {
	return nil, false, errLegacyBoardsRemoved
}

func (r *Repository) ListShareTokens(_ context.Context, _ int64, _ string) ([]ShareToken, error) {
	return nil, errLegacyBoardsRemoved
}

func (r *Repository) GetShareTokenByHash(_ context.Context, _ string) (*ShareToken, error) {
	return nil, errLegacyBoardsRemoved
}

func (r *Repository) RevokeShareTokenByID(_ context.Context, _ int64, _ string, _ int64) error {
	return errLegacyBoardsRemoved
}

func (r *Repository) ConsumeShareTokenAndUpsertMember(_ context.Context, _ string, _ int64) (*ShareToken, error) {
	return nil, errLegacyBoardsRemoved
}

func (r *Repository) ShareTokenHashExists(_ context.Context, _ string) (bool, error) {
	return false, errLegacyBoardsRemoved
}

var ErrMediaAssetNoLongerAvailable = errors.New("media asset is no longer available")

func IsBoardNameUniqueConstraintError(err error) bool {
	if !database.IsDuplicateConstraintError(err) {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "uq_boards_owner_name")
}

func IsBoardIDUniqueConstraintError(err error) bool {
	if !database.IsDuplicateConstraintError(err) {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "uq_boards_board_id")
}
