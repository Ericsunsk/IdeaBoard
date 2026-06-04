package boards

import (
	"context"
	"database/sql"
	"strings"

	"api/internal/db/dbutil"
	dbsqlc "api/internal/db/sqlc"
	"api/internal/platform/database"
)

type BoardRepository struct {
	db *sql.DB
	q  *dbsqlc.Queries
}

func NewBoardRepository(db *sql.DB) *BoardRepository {
	return &BoardRepository{db: db, q: dbsqlc.New(db)}
}

func (r *BoardRepository) ListAccessibleByUserID(ctx context.Context, userID int64) ([]BoardWithRole, error) {
	rows, err := r.q.ListAccessibleBoardsByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}

	result := make([]BoardWithRole, 0, len(rows))
	for _, row := range rows {
		role := BoardAccessRoleOwner
		if row.UserID != userID {
			role = normalizeBoardAccessRole(row.MemberRole.String)
		}
		if role == "" {
			continue
		}
		result = append(result, BoardWithRole{
			Board: UserBoard{
				ID:        row.ID,
				BoardID:   row.BoardID,
				UserID:    row.UserID,
				Name:      row.Name,
				State:     row.State,
				CreatedAt: row.CreatedAt,
				UpdatedAt: row.UpdatedAt,
			},
			Role: role,
		})
	}
	return result, nil
}

func (r *BoardRepository) GetAccessibleByBoardID(ctx context.Context, userID int64, boardID string) (*BoardWithRole, error) {
	row, err := r.q.GetAccessibleBoardByBoardID(ctx, dbsqlc.GetAccessibleBoardByBoardIDParams{UserID: userID, BoardID: boardID})
	if err != nil {
		return nil, err
	}
	board := BoardWithRole{
		Board: UserBoard{
			ID:        row.ID,
			BoardID:   row.BoardID,
			UserID:    row.UserID,
			Name:      row.Name,
			State:     row.State,
			CreatedAt: row.CreatedAt,
			UpdatedAt: row.UpdatedAt,
		},
		Role: BoardAccessRoleOwner,
	}
	if row.UserID != userID {
		board.Role = normalizeBoardAccessRole(row.MemberRole.String)
	}
	return &board, nil
}

func (r *BoardRepository) GetByOwnerAndBoardID(ctx context.Context, ownerUserID int64, boardID string) (*UserBoard, error) {
	row, err := r.q.GetBoardByOwnerAndBoardID(ctx, dbsqlc.GetBoardByOwnerAndBoardIDParams{OwnerUserID: ownerUserID, BoardID: boardID})
	if err != nil {
		return nil, err
	}
	board := userBoardFromSQLC(row)
	return &board, nil
}

func (r *BoardRepository) Upsert(ctx context.Context, board *UserBoard) error {
	return r.q.UpsertBoard(ctx, dbsqlc.UpsertBoardParams{
		BoardID:   board.BoardID,
		UserID:    board.UserID,
		Name:      board.Name,
		State:     board.State,
		CreatedAt: board.CreatedAt,
		UpdatedAt: board.UpdatedAt,
	})
}

func (r *BoardRepository) HasNameConflict(ctx context.Context, userID int64, boardID, name string) (bool, error) {
	hasBoardID := strings.TrimSpace(boardID) != ""
	count, err := r.q.CountBoardNameConflicts(ctx, dbsqlc.CountBoardNameConflictsParams{
		UserID:     userID,
		Name:       name,
		HasBoardID: hasBoardID,
		BoardID:    boardID,
	})
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (r *BoardRepository) BoardIDExistsForOtherOwner(ctx context.Context, userID int64, boardID string) (bool, error) {
	var count int64
	err := r.db.QueryRowContext(ctx, `
SELECT COUNT(*)
FROM user_boards
WHERE board_id = $1 AND user_id <> $2`, boardID, userID).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (r *BoardRepository) GetMemberRole(ctx context.Context, ownerUserID int64, boardID string, userID int64) (BoardAccessRole, error) {
	raw, err := r.q.GetBoardMemberRole(ctx, dbsqlc.GetBoardMemberRoleParams{OwnerUserID: ownerUserID, BoardID: boardID, UserID: userID})
	if err != nil {
		return "", err
	}
	role := normalizeBoardAccessRole(raw)
	if role == "" || role == BoardAccessRoleOwner {
		return "", database.ErrNotFound
	}
	return role, nil
}

func (r *BoardRepository) DeleteByBoardID(ctx context.Context, ownerUserID int64, boardID string) error {
	return r.q.DeleteBoardByOwnerAndBoardID(ctx, dbsqlc.DeleteBoardByOwnerAndBoardIDParams{OwnerUserID: ownerUserID, BoardID: boardID})
}

func (r *BoardRepository) DeleteMembersByBoardID(ctx context.Context, ownerUserID int64, boardID string) error {
	return r.q.DeleteBoardMembersByOwnerAndBoardID(ctx, dbsqlc.DeleteBoardMembersByOwnerAndBoardIDParams{OwnerUserID: ownerUserID, BoardID: boardID})
}

func (r *BoardRepository) DeleteShareTokensByBoardID(ctx context.Context, ownerUserID int64, boardID string) error {
	return r.q.DeleteBoardShareTokensByOwnerAndBoardID(ctx, dbsqlc.DeleteBoardShareTokensByOwnerAndBoardIDParams{OwnerUserID: ownerUserID, BoardID: boardID})
}

func (r *BoardRepository) DeleteBoardCascade(ctx context.Context, ownerUserID int64, boardID string) error {
	return dbutil.WithTx(ctx, r.db, func(q *dbsqlc.Queries) error {
		if err := q.DeleteBoardShareTokensByOwnerAndBoardID(ctx, dbsqlc.DeleteBoardShareTokensByOwnerAndBoardIDParams{OwnerUserID: ownerUserID, BoardID: boardID}); err != nil {
			return err
		}
		if err := q.DeleteBoardMembersByOwnerAndBoardID(ctx, dbsqlc.DeleteBoardMembersByOwnerAndBoardIDParams{OwnerUserID: ownerUserID, BoardID: boardID}); err != nil {
			return err
		}
		return q.DeleteBoardByOwnerAndBoardID(ctx, dbsqlc.DeleteBoardByOwnerAndBoardIDParams{OwnerUserID: ownerUserID, BoardID: boardID})
	})
}

func (r *BoardRepository) ListMembers(ctx context.Context, ownerUserID int64, boardID string) ([]BoardMemberWithUser, error) {
	rows, err := r.q.ListBoardMembers(ctx, dbsqlc.ListBoardMembersParams{OwnerUserID: ownerUserID, BoardID: boardID})
	if err != nil {
		return nil, err
	}
	result := make([]BoardMemberWithUser, 0, len(rows))
	for _, row := range rows {
		role := normalizeBoardAccessRole(row.Role)
		if role == "" {
			continue
		}
		result = append(result, BoardMemberWithUser{
			UserID:    row.UserID,
			MemberUID: row.MemberUid,
			Username:  row.Username,
			Role:      role,
		})
	}
	return result, nil
}

func (r *BoardRepository) UpsertMember(ctx context.Context, ownerUserID int64, boardID string, userID int64, role BoardAccessRole) error {
	return r.q.UpsertBoardMember(ctx, dbsqlc.UpsertBoardMemberParams{
		OwnerUserID: ownerUserID,
		BoardID:     boardID,
		UserID:      userID,
		Role:        string(role),
	})
}

func (r *BoardRepository) DeleteMember(ctx context.Context, ownerUserID int64, boardID string, userID int64) error {
	return r.q.DeleteBoardMember(ctx, dbsqlc.DeleteBoardMemberParams{OwnerUserID: ownerUserID, BoardID: boardID, UserID: userID})
}

func (r *BoardRepository) GetMemberByUserID(ctx context.Context, ownerUserID int64, boardID string, userID int64) (*BoardMember, error) {
	row, err := r.q.GetBoardMemberByUserID(ctx, dbsqlc.GetBoardMemberByUserIDParams{OwnerUserID: ownerUserID, BoardID: boardID, UserID: userID})
	if err != nil {
		return nil, err
	}
	member := BoardMember{
		ID:          row.ID,
		OwnerUserID: row.OwnerUserID,
		BoardID:     row.BoardID,
		UserID:      row.UserID,
		Role:        row.Role,
		CreatedAt:   row.CreatedAt,
		UpdatedAt:   row.UpdatedAt,
	}
	return &member, nil
}

func (r *BoardRepository) CreateShareToken(ctx context.Context, token *BoardShareToken) error {
	id, err := r.q.CreateBoardShareToken(ctx, dbsqlc.CreateBoardShareTokenParams{
		BoardID:   token.BoardID,
		Token:     token.Token,
		Role:      token.Role,
		CreatedBy: token.CreatedBy,
		Revoked:   token.Revoked,
		ExpiresAt: dbutil.NullTime(token.ExpiresAt),
	})
	if err != nil {
		return err
	}
	token.ID = int64(id)
	return nil
}

func (r *BoardRepository) ListShareTokens(ctx context.Context, ownerUserID int64, boardID string) ([]BoardShareToken, error) {
	rows, err := r.q.ListBoardShareTokens(ctx, dbsqlc.ListBoardShareTokensParams{OwnerUserID: ownerUserID, BoardID: boardID})
	if err != nil {
		return nil, err
	}
	result := make([]BoardShareToken, 0, len(rows))
	for _, row := range rows {
		result = append(result, boardShareTokenFromSQLC(row))
	}
	return result, nil
}

func (r *BoardRepository) GetShareTokenByToken(ctx context.Context, token string) (*BoardShareToken, error) {
	row, err := r.q.GetBoardShareTokenByToken(ctx, token)
	if err != nil {
		return nil, err
	}
	shareToken := boardShareTokenFromSQLC(row)
	return &shareToken, nil
}

func (r *BoardRepository) RevokeShareToken(ctx context.Context, ownerUserID int64, boardID, token string) error {
	result, err := r.db.ExecContext(ctx, `
UPDATE board_share_tokens
SET revoked = TRUE
WHERE created_by = $1 AND board_id = $2 AND token = $3`, ownerUserID, boardID, token)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return database.ErrNotFound
	}
	return nil
}

func (r *BoardRepository) ShareTokenExists(ctx context.Context, token string) (bool, error) {
	count, err := r.q.CountBoardShareTokenByToken(ctx, token)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func IsBoardNameUniqueConstraintError(err error) bool {
	if !database.IsDuplicateConstraintError(err) {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "idx_user_board_name")
}

func IsBoardIDUniqueConstraintError(err error) bool {
	if !database.IsDuplicateConstraintError(err) {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "uk_user_boards_board_id")
}

func userBoardFromSQLC(row dbsqlc.UserBoard) UserBoard {
	return UserBoard{
		ID:        row.ID,
		BoardID:   row.BoardID,
		UserID:    row.UserID,
		Name:      row.Name,
		State:     row.State,
		CreatedAt: row.CreatedAt,
		UpdatedAt: row.UpdatedAt,
	}
}

func boardShareTokenFromSQLC(row dbsqlc.BoardShareToken) BoardShareToken {
	return BoardShareToken{
		ID:        row.ID,
		BoardID:   row.BoardID,
		Token:     row.Token,
		Role:      row.Role,
		CreatedBy: row.CreatedBy,
		Revoked:   row.Revoked,
		ExpiresAt: dbutil.TimePtr(row.ExpiresAt),
		CreatedAt: row.CreatedAt,
		UpdatedAt: row.UpdatedAt,
	}
}
