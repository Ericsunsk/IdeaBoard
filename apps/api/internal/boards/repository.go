package boards

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"ideaboard/api/internal/db/dbutil"
	dbsqlc "ideaboard/api/internal/db/sqlc"
	"ideaboard/api/internal/media/reclamation"
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
		if row.OwnerUserID != userID {
			role = normalizeBoardAccessRole(row.MemberRole.String)
		}
		if role == "" {
			continue
		}
		result = append(result, BoardWithRole{
			Board: UserBoard{
				ID:                 row.ID,
				BoardID:            row.BoardID,
				OwnerUserID:        row.OwnerUserID,
				CreatedBy:          row.CreatedBy,
				Name:               row.Name,
				State:              boardStateString(row.State),
				Revision:           row.Revision,
				StateSchemaVersion: row.StateSchemaVersion,
				LastSavedBy:        dbutil.Int64Ptr(row.LastSavedBy),
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
			OwnerUserID:        row.OwnerUserID,
			CreatedBy:          row.CreatedBy,
			Name:               row.Name,
			State:              boardStateString(row.State),
			Revision:           row.Revision,
			StateSchemaVersion: row.StateSchemaVersion,
			LastSavedBy:        dbutil.Int64Ptr(row.LastSavedBy),
			CreatedAt:          row.CreatedAt,
			UpdatedAt:          row.UpdatedAt,
		},
		Role: BoardAccessRoleOwner,
	}
	if row.OwnerUserID != userID {
		board.Role = normalizeBoardAccessRole(row.MemberRole.String)
	}
	return &board, nil
}

func (r *BoardRepository) GetByBoardID(ctx context.Context, boardID string) (*UserBoard, error) {
	row, err := r.q.GetBoardByBoardID(ctx, boardID)
	if err != nil {
		return nil, err
	}
	board := userBoardFromSQLC(row)
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
	if board == nil {
		return ErrInvalidBoardInput
	}
	mediaRefs, shouldReconcileMediaRefs := parseBoardMediaRefsForReconcile(board.State)
	return dbutil.WithTx(ctx, r.db, func(q *dbsqlc.Queries) error {
		rows, err := q.UpsertBoard(ctx, upsertBoardParams(board))
		if err != nil {
			return err
		}
		if rows == 0 {
			return ErrBoardIDConflict
		}
		if !shouldReconcileMediaRefs {
			return nil
		}
		return reconcileBoardMediaRefsTx(ctx, q, board, mediaRefs)
	})
}

func upsertBoardParams(board *UserBoard) dbsqlc.UpsertBoardParams {
	return dbsqlc.UpsertBoardParams{
		BoardID:     board.BoardID,
		OwnerUserID: board.OwnerUserID,
		CreatedBy:   board.CreatedBy,
		Name:        board.Name,
		State:       boardStateJSON(board.State),
		Revision: sql.NullInt64{
			Int64: board.Revision,
			Valid: board.Revision > 0,
		},
		StateSchemaVersion: sql.NullInt32{
			Int32: board.StateSchemaVersion,
			Valid: board.StateSchemaVersion > 0,
		},
		LastSavedBy: dbutil.NullInt64(board.LastSavedBy),
		CreatedAt:   board.CreatedAt,
		UpdatedAt:   board.UpdatedAt,
	}
}

func (r *BoardRepository) HasNameConflict(ctx context.Context, userID int64, boardID, name string) (bool, error) {
	hasBoardID := strings.TrimSpace(boardID) != ""
	count, err := r.q.CountBoardNameConflicts(ctx, dbsqlc.CountBoardNameConflictsParams{
		OwnerUserID: userID,
		Name:        name,
		HasBoardID:  hasBoardID,
		BoardID:     boardID,
	})
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (r *BoardRepository) BoardIDExists(ctx context.Context, boardID string) (bool, error) {
	count, err := r.q.CountBoardsByBoardID(ctx, boardID)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (r *BoardRepository) GetMemberRole(ctx context.Context, boardID string, userID int64) (BoardAccessRole, error) {
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

func (r *BoardRepository) DeleteBoardCascade(ctx context.Context, ownerUserID int64, boardID string) error {
	return dbutil.WithTx(ctx, r.db, func(q *dbsqlc.Queries) error {
		previous, err := q.ListMediaRefsByBoardID(ctx, dbsqlc.ListMediaRefsByBoardIDParams{
			OwnerUserID: ownerUserID,
			BoardID:     boardID,
		})
		if err != nil {
			return err
		}
		if err := q.DeleteBoardMediaRefsByOwnerAndBoardID(ctx, dbsqlc.DeleteBoardMediaRefsByOwnerAndBoardIDParams{OwnerUserID: ownerUserID, BoardID: boardID}); err != nil {
			return err
		}
		if err := q.DeleteBoardShareTokensByBoardID(ctx, boardID); err != nil {
			return err
		}
		if err := q.DeleteBoardMembersByBoardID(ctx, boardID); err != nil {
			return err
		}
		rows, err := q.DeleteBoardByOwnerAndBoardID(ctx, dbsqlc.DeleteBoardByOwnerAndBoardIDParams{OwnerUserID: ownerUserID, BoardID: boardID})
		if err != nil {
			return err
		}
		if rows == 0 {
			return database.ErrNotFound
		}
		assetIDs := make([]int64, 0, len(previous))
		for _, ref := range previous {
			assetIDs = append(assetIDs, ref.MediaAssetID)
		}
		if len(assetIDs) > 0 {
			if err := q.ScheduleUnreferencedMediaAssetsForReclamation(ctx, dbsqlc.ScheduleUnreferencedMediaAssetsForReclamationParams{
				ReclaimAfter:  sql.NullTime{Time: time.Now().Add(reclamation.GracePeriod), Valid: true},
				MediaAssetIds: uniqueMediaAssetIDs(assetIDs),
			}); err != nil {
				return err
			}
		}
		return nil
	})
}

func (r *BoardRepository) ListMembers(ctx context.Context, ownerUserID int64, boardID string) ([]BoardMemberWithUser, error) {
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

func (r *BoardRepository) UpsertMember(ctx context.Context, ownerUserID int64, boardID string, userID int64, role BoardAccessRole) error {
	return r.q.UpsertBoardMember(ctx, dbsqlc.UpsertBoardMemberParams{
		BoardID: boardID,
		UserID:  userID,
		Role:    string(role),
	})
}

func (r *BoardRepository) DeleteMember(ctx context.Context, ownerUserID int64, boardID string, userID int64) error {
	rows, err := r.q.DeleteBoardMember(ctx, dbsqlc.DeleteBoardMemberParams{BoardID: boardID, UserID: userID})
	if err != nil {
		return err
	}
	if rows == 0 {
		return database.ErrNotFound
	}
	return nil
}

func (r *BoardRepository) GetMemberByUserID(ctx context.Context, ownerUserID int64, boardID string, userID int64) (*BoardMember, error) {
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
		BoardID:     token.BoardID,
		TokenHash:   token.TokenHash,
		TokenPrefix: token.TokenPrefix,
		Role:        token.Role,
		CreatedBy:   token.CreatedBy,
		Revoked:     token.Revoked,
		ExpiresAt:   dbutil.NullTime(token.ExpiresAt),
		MaxUses:     dbutil.NullInt32(token.MaxUses),
	})
	if err != nil {
		return err
	}
	token.ID = int64(id)
	return nil
}

func (r *BoardRepository) ListShareTokens(ctx context.Context, ownerUserID int64, boardID string) ([]BoardShareToken, error) {
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
		BoardID:   boardID,
		TokenID:   tokenID,
		RevokedBy: sql.NullInt64{Int64: ownerUserID, Valid: true},
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
			BoardID: shareToken.BoardID,
			UserID:  userID,
			Role:    string(role),
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

type boardMediaRefKey struct {
	mediaAssetID int64
	refType      string
	refID        string
	role         string
}

func parseBoardMediaRefsForReconcile(state string) ([]BoardStateMediaRef, bool) {
	if strings.TrimSpace(state) == "" {
		return nil, false
	}
	envelope, err := validateBoardStateEnvelopeJSON([]byte(state))
	if err != nil {
		return nil, false
	}
	return reconcileableBoardMediaRefs(envelope.MediaRefs), true
}

func reconcileableBoardMediaRefs(refs []BoardStateMediaRef) []BoardStateMediaRef {
	if len(refs) == 0 {
		return nil
	}
	seen := make(map[boardMediaRefKey]struct{}, len(refs))
	result := make([]BoardStateMediaRef, 0, len(refs))
	for _, ref := range refs {
		key := boardMediaRefKey{
			mediaAssetID: ref.MediaAssetID,
			refType:      strings.TrimSpace(ref.RefType),
			refID:        strings.TrimSpace(ref.RefID),
			role:         strings.TrimSpace(ref.Role),
		}
		if key.mediaAssetID <= 0 || key.refType == "" || key.refID == "" {
			continue
		}
		if key.refType != "board" && key.refType != "board_node" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, BoardStateMediaRef{
			MediaAssetID: key.mediaAssetID,
			RefType:      key.refType,
			RefID:        key.refID,
			Role:         key.role,
		})
	}
	return result
}

func reconcileBoardMediaRefsTx(ctx context.Context, q *dbsqlc.Queries, board *UserBoard, refs []BoardStateMediaRef) error {
	if board == nil {
		return ErrInvalidBoardInput
	}
	previous, err := q.ListMediaRefsByBoardID(ctx, dbsqlc.ListMediaRefsByBoardIDParams{
		OwnerUserID: board.OwnerUserID,
		BoardID:     board.BoardID,
	})
	if err != nil {
		return err
	}
	if err := q.DeleteBoardMediaRefsByOwnerAndBoardID(ctx, dbsqlc.DeleteBoardMediaRefsByOwnerAndBoardIDParams{OwnerUserID: board.OwnerUserID, BoardID: board.BoardID}); err != nil {
		return err
	}
	currentAssetIDs := make([]int64, 0, len(refs))
	for _, ref := range refs {
		if _, err := q.UpsertMediaRef(ctx, dbsqlc.UpsertMediaRefParams{
			MediaAssetID: ref.MediaAssetID,
			OwnerUserID:  board.OwnerUserID,
			RefType:      ref.RefType,
			RefID:        ref.RefID,
			BoardID:      board.BoardID,
			TaskID:       "",
			Role:         ref.Role,
			Metadata:     json.RawMessage(`{"source":"board_state"}`),
		}); err != nil {
			return err
		}
		currentAssetIDs = append(currentAssetIDs, ref.MediaAssetID)
	}
	if len(currentAssetIDs) > 0 {
		assetIDs := uniqueMediaAssetIDs(currentAssetIDs)
		cancelledAssetIDs, err := q.CancelMediaAssetReclamation(ctx, dbsqlc.CancelMediaAssetReclamationParams{
			MediaAssetIds:    assetIDs,
			StaleClaimBefore: sql.NullTime{Time: time.Now().Add(-reclamation.ClaimLeaseDuration), Valid: true},
		})
		if err != nil {
			return err
		}
		if len(cancelledAssetIDs) != len(assetIDs) {
			return ErrMediaAssetNoLongerAvailable
		}
	}
	previousAssetIDs := make([]int64, 0, len(previous))
	for _, ref := range previous {
		previousAssetIDs = append(previousAssetIDs, ref.MediaAssetID)
	}
	if len(previousAssetIDs) > 0 {
		if err := q.ScheduleUnreferencedMediaAssetsForReclamation(ctx, dbsqlc.ScheduleUnreferencedMediaAssetsForReclamationParams{
			ReclaimAfter:  sql.NullTime{Time: time.Now().Add(reclamation.GracePeriod), Valid: true},
			MediaAssetIds: uniqueMediaAssetIDs(previousAssetIDs),
		}); err != nil {
			return err
		}
	}
	return nil
}

var ErrMediaAssetNoLongerAvailable = errors.New("media asset is no longer available")

func uniqueMediaAssetIDs(ids []int64) []int64 {
	seen := make(map[int64]struct{}, len(ids))
	result := make([]int64, 0, len(ids))
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		result = append(result, id)
	}
	return result
}

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

func userBoardFromSQLC(row dbsqlc.Board) UserBoard {
	return UserBoard{
		ID:                 row.ID,
		BoardID:            row.BoardID,
		OwnerUserID:        row.OwnerUserID,
		CreatedBy:          row.CreatedBy,
		Name:               row.Name,
		State:              boardStateString(row.State),
		Revision:           row.Revision,
		StateSchemaVersion: row.StateSchemaVersion,
		LastSavedBy:        dbutil.Int64Ptr(row.LastSavedBy),
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
		MaxUses:     dbutil.Int32Ptr(row.MaxUses),
		LastUsedAt:  dbutil.TimePtr(row.LastUsedAt),
		RevokedAt:   dbutil.TimePtr(row.RevokedAt),
		RevokedBy:   dbutil.Int64Ptr(row.RevokedBy),
		ExpiresAt:   dbutil.TimePtr(row.ExpiresAt),
		CreatedAt:   row.CreatedAt,
		UpdatedAt:   row.UpdatedAt,
	}
}
