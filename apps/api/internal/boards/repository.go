package boards

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"

	"ideaboard/api/internal/db/dbutil"
	dbsqlc "ideaboard/api/internal/db/sqlc"
	"ideaboard/api/internal/platform/database"
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
				ID:                 row.ID,
				BoardID:            row.BoardID,
				UserID:             row.UserID,
				Name:               row.Name,
				State:              boardStateString(row.State),
				Revision:           row.Revision,
				StateSchemaVersion: row.StateSchemaVersion,
				LastSavedBy:        int64Ptr(row.LastSavedBy),
				CreatedAt:          row.CreatedAt,
				UpdatedAt:          row.UpdatedAt,
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
			ID:                 row.ID,
			BoardID:            row.BoardID,
			UserID:             row.UserID,
			Name:               row.Name,
			State:              boardStateString(row.State),
			Revision:           row.Revision,
			StateSchemaVersion: row.StateSchemaVersion,
			LastSavedBy:        int64Ptr(row.LastSavedBy),
			CreatedAt:          row.CreatedAt,
			UpdatedAt:          row.UpdatedAt,
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
		BoardID: board.BoardID,
		UserID:  board.UserID,
		Name:    board.Name,
		State:   boardStateJSON(board.State),
		Revision: sql.NullInt64{
			Int64: board.Revision,
			Valid: board.Revision > 0,
		},
		StateSchemaVersion: sql.NullInt32{
			Int32: board.StateSchemaVersion,
			Valid: board.StateSchemaVersion > 0,
		},
		LastSavedBy: nullInt64(board.LastSavedBy),
		CreatedAt:   board.CreatedAt,
		UpdatedAt:   board.UpdatedAt,
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
	rows, err := r.q.DeleteBoardByOwnerAndBoardID(ctx, dbsqlc.DeleteBoardByOwnerAndBoardIDParams{OwnerUserID: ownerUserID, BoardID: boardID})
	if err != nil {
		return err
	}
	if rows == 0 {
		return database.ErrNotFound
	}
	return nil
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
		rows, err := q.DeleteBoardByOwnerAndBoardID(ctx, dbsqlc.DeleteBoardByOwnerAndBoardIDParams{OwnerUserID: ownerUserID, BoardID: boardID})
		if err != nil {
			return err
		}
		if rows == 0 {
			return database.ErrNotFound
		}
		return nil
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
	rows, err := r.q.DeleteBoardMember(ctx, dbsqlc.DeleteBoardMemberParams{OwnerUserID: ownerUserID, BoardID: boardID, UserID: userID})
	if err != nil {
		return err
	}
	if rows == 0 {
		return database.ErrNotFound
	}
	return nil
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
		BoardID:     token.BoardID,
		TokenHash:   token.TokenHash,
		TokenPrefix: token.TokenPrefix,
		Role:        token.Role,
		CreatedBy:   token.CreatedBy,
		Revoked:     token.Revoked,
		ExpiresAt:   dbutil.NullTime(token.ExpiresAt),
		MaxUses:     nullInt32(token.MaxUses),
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

func (r *BoardRepository) GetShareTokenByHash(ctx context.Context, tokenHash string) (*BoardShareToken, error) {
	row, err := r.q.GetBoardShareTokenByHash(ctx, tokenHash)
	if err != nil {
		return nil, err
	}
	shareToken := boardShareTokenFromSQLC(row)
	return &shareToken, nil
}

func (r *BoardRepository) RevokeShareTokenByID(ctx context.Context, ownerUserID int64, boardID string, tokenID int64) error {
	rows, err := r.q.RevokeBoardShareTokenByID(ctx, dbsqlc.RevokeBoardShareTokenByIDParams{
		OwnerUserID: ownerUserID,
		BoardID:     boardID,
		TokenID:     tokenID,
		RevokedBy:   sql.NullInt64{Int64: ownerUserID, Valid: true},
	})
	if err != nil {
		return err
	}
	if rows == 0 {
		return database.ErrNotFound
	}
	return nil
}

func (r *BoardRepository) ConsumeShareTokenAndUpsertMember(ctx context.Context, tokenHash string, userID int64) (*BoardShareToken, error) {
	var shareToken BoardShareToken
	err := dbutil.WithTx(ctx, r.db, func(q *dbsqlc.Queries) error {
		row, err := q.ConsumeBoardShareToken(ctx, tokenHash)
		if err != nil {
			return err
		}
		shareToken = boardShareTokenFromSQLC(row)
		role := normalizeBoardMemberRole(shareToken.Role)
		if role == "" {
			return ErrShareTokenInvalid
		}
		return q.UpsertBoardMember(ctx, dbsqlc.UpsertBoardMemberParams{
			OwnerUserID: shareToken.CreatedBy,
			BoardID:     shareToken.BoardID,
			UserID:      userID,
			Role:        string(role),
		})
	})
	if err != nil {
		return nil, err
	}
	return &shareToken, nil
}

func (r *BoardRepository) ShareTokenHashExists(ctx context.Context, tokenHash string) (bool, error) {
	count, err := r.q.CountBoardShareTokenByHash(ctx, tokenHash)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func IsBoardNameUniqueConstraintError(err error) bool {
	if !database.IsDuplicateConstraintError(err) {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "uq_user_board_name")
}

func IsBoardIDUniqueConstraintError(err error) bool {
	if !database.IsDuplicateConstraintError(err) {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "uq_user_boards_board_id")
}

func userBoardFromSQLC(row dbsqlc.UserBoard) UserBoard {
	return UserBoard{
		ID:                 row.ID,
		BoardID:            row.BoardID,
		UserID:             row.UserID,
		Name:               row.Name,
		State:              boardStateString(row.State),
		Revision:           row.Revision,
		StateSchemaVersion: row.StateSchemaVersion,
		LastSavedBy:        int64Ptr(row.LastSavedBy),
		CreatedAt:          row.CreatedAt,
		UpdatedAt:          row.UpdatedAt,
	}
}

func boardStateJSON(state string) json.RawMessage {
	state = strings.TrimSpace(state)
	if state == "" {
		return json.RawMessage("null")
	}
	return json.RawMessage(state)
}

func boardStateString(state json.RawMessage) string {
	if len(state) == 0 {
		return "null"
	}
	return string(state)
}

func boardShareTokenFromSQLC(row dbsqlc.BoardShareToken) BoardShareToken {
	return BoardShareToken{
		ID:          row.ID,
		BoardID:     row.BoardID,
		TokenHash:   row.TokenHash,
		TokenPrefix: row.TokenPrefix,
		Role:        row.Role,
		CreatedBy:   row.CreatedBy,
		Revoked:     row.Revoked,
		UsageCount:  row.UsageCount,
		MaxUses:     int32Ptr(row.MaxUses),
		LastUsedAt:  dbutil.TimePtr(row.LastUsedAt),
		RevokedAt:   dbutil.TimePtr(row.RevokedAt),
		RevokedBy:   int64Ptr(row.RevokedBy),
		ExpiresAt:   dbutil.TimePtr(row.ExpiresAt),
		CreatedAt:   row.CreatedAt,
		UpdatedAt:   row.UpdatedAt,
	}
}

func nullInt64(value *int64) sql.NullInt64 {
	if value == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: *value, Valid: true}
}

func int64Ptr(value sql.NullInt64) *int64 {
	if !value.Valid {
		return nil
	}
	v := value.Int64
	return &v
}

func nullInt32(value *int32) sql.NullInt32 {
	if value == nil {
		return sql.NullInt32{}
	}
	return sql.NullInt32{Int32: *value, Valid: true}
}

func int32Ptr(value sql.NullInt32) *int32 {
	if !value.Valid {
		return nil
	}
	v := value.Int32
	return &v
}
