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

func (r *BoardRepository) ListAccessibleByUserID(ctx context.Context, userID uint64) ([]BoardWithRole, error) {
	rows, err := r.q.ListAccessibleBoardsByUserID(ctx, dbsqlc.ListAccessibleBoardsByUserIDParams{UserID: userID})
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

func (r *BoardRepository) GetByBoardID(ctx context.Context, boardID string) (*UserBoard, error) {
	row, err := r.q.GetBoardByBoardID(ctx, boardID)
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

func (r *BoardRepository) HasNameConflict(ctx context.Context, userID uint64, boardID, name string) (bool, error) {
	hasBoardID := int64(0)
	if strings.TrimSpace(boardID) != "" {
		hasBoardID = 1
	}
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

func (r *BoardRepository) GetMemberRole(ctx context.Context, boardID string, userID uint64) (BoardAccessRole, error) {
	raw, err := r.q.GetBoardMemberRole(ctx, dbsqlc.GetBoardMemberRoleParams{BoardID: boardID, UserID: userID})
	if err != nil {
		return "", err
	}
	role := normalizeBoardAccessRole(raw)
	if role == "" || role == BoardAccessRoleOwner {
		return "", database.ErrNotFound
	}
	return role, nil
}

func (r *BoardRepository) DeleteByBoardID(ctx context.Context, boardID string) error {
	return r.q.DeleteBoardByBoardID(ctx, boardID)
}

func (r *BoardRepository) DeleteMembersByBoardID(ctx context.Context, boardID string) error {
	return r.q.DeleteBoardMembersByBoardID(ctx, boardID)
}

func (r *BoardRepository) DeleteShareTokensByBoardID(ctx context.Context, boardID string) error {
	return r.q.DeleteBoardShareTokensByBoardID(ctx, boardID)
}

func (r *BoardRepository) DeleteBoardCascade(ctx context.Context, boardID string) error {
	return dbutil.WithTx(ctx, r.db, func(q *dbsqlc.Queries) error {
		if err := q.DeleteBoardShareTokensByBoardID(ctx, boardID); err != nil {
			return err
		}
		if err := q.DeleteBoardMembersByBoardID(ctx, boardID); err != nil {
			return err
		}
		return q.DeleteBoardByBoardID(ctx, boardID)
	})
}

func (r *BoardRepository) ListMembers(ctx context.Context, boardID string) ([]BoardMemberWithUser, error) {
	rows, err := r.q.ListBoardMembers(ctx, boardID)
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

func (r *BoardRepository) UpsertMember(ctx context.Context, boardID string, userID uint64, role BoardAccessRole) error {
	return r.q.UpsertBoardMember(ctx, dbsqlc.UpsertBoardMemberParams{
		BoardID: boardID,
		UserID:  userID,
		Role:    string(role),
	})
}

func (r *BoardRepository) DeleteMember(ctx context.Context, boardID string, userID uint64) error {
	return r.q.DeleteBoardMember(ctx, dbsqlc.DeleteBoardMemberParams{BoardID: boardID, UserID: userID})
}

func (r *BoardRepository) GetMemberByUserID(ctx context.Context, boardID string, userID uint64) (*BoardMember, error) {
	row, err := r.q.GetBoardMemberByUserID(ctx, dbsqlc.GetBoardMemberByUserIDParams{BoardID: boardID, UserID: userID})
	if err != nil {
		return nil, err
	}
	member := BoardMember{
		ID:        row.ID,
		BoardID:   row.BoardID,
		UserID:    row.UserID,
		Role:      row.Role,
		CreatedAt: row.CreatedAt,
		UpdatedAt: row.UpdatedAt,
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
	token.ID = uint64(id)
	return nil
}

func (r *BoardRepository) ListShareTokens(ctx context.Context, boardID string) ([]BoardShareToken, error) {
	rows, err := r.q.ListBoardShareTokens(ctx, boardID)
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

func (r *BoardRepository) RevokeShareToken(ctx context.Context, boardID, token string) error {
	return r.q.RevokeBoardShareToken(ctx, dbsqlc.RevokeBoardShareTokenParams{BoardID: boardID, Token: token})
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
