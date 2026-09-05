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

type Repository struct {
	db *sql.DB
	q  *dbsqlc.Queries
}

func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db, q: dbsqlc.New(db)}
}

func (r *Repository) ListAccessibleByUserID(ctx context.Context, userID int64) ([]WithRole, error) {
	rows, err := r.q.ListAccessibleBoardsByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}

	result := make([]WithRole, 0, len(rows))
	for _, row := range rows {
		role := RoleOwner
		if row.OwnerUserID != userID {
			role = normalizeAccessRole(row.MemberRole.String)
		}
		if role == "" {
			continue
		}
		result = append(result, WithRole{
			Board: Board{
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

func (r *Repository) GetAccessibleByBoardID(ctx context.Context, userID int64, boardID string) (*WithRole, error) {
	row, err := r.q.GetAccessibleBoardByBoardID(ctx, dbsqlc.GetAccessibleBoardByBoardIDParams{UserID: userID, BoardID: boardID})
	if err != nil {
		return nil, err
	}
	board := WithRole{
		Board: Board{
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
		Role: RoleOwner,
	}
	if row.OwnerUserID != userID {
		board.Role = normalizeAccessRole(row.MemberRole.String)
	}
	return &board, nil
}

func (r *Repository) GetByBoardID(ctx context.Context, boardID string) (*Board, error) {
	row, err := r.q.GetBoardByBoardID(ctx, boardID)
	if err != nil {
		return nil, err
	}
	board := boardFromSQLC(row)
	return &board, nil
}

func (r *Repository) GetByOwnerAndBoardID(ctx context.Context, ownerUserID int64, boardID string) (*Board, error) {
	row, err := r.q.GetBoardByOwnerAndBoardID(ctx, dbsqlc.GetBoardByOwnerAndBoardIDParams{OwnerUserID: ownerUserID, BoardID: boardID})
	if err != nil {
		return nil, err
	}
	board := boardFromSQLC(row)
	return &board, nil
}

func (r *Repository) Upsert(ctx context.Context, board *Board) error {
	if board == nil {
		return ErrInvalidInput
	}
	mediaRefs, shouldReconcileMediaRefs := parseBoardMediaRefsForReconcile(board.State)
	return dbutil.WithTx(ctx, r.db, func(q *dbsqlc.Queries) error {
		rows, err := q.UpsertBoard(ctx, upsertBoardParams(board))
		if err != nil {
			return err
		}
		if rows == 0 {
			return ErrIDConflict
		}
		if !shouldReconcileMediaRefs {
			return nil
		}
		return reconcileBoardMediaRefsTx(ctx, q, board, mediaRefs)
	})
}

func upsertBoardParams(board *Board) dbsqlc.UpsertBoardParams {
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

func (r *Repository) HasNameConflict(ctx context.Context, userID int64, boardID, name string) (bool, error) {
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

func (r *Repository) BoardIDExists(ctx context.Context, boardID string) (bool, error) {
	count, err := r.q.CountBoardsByBoardID(ctx, boardID)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (r *Repository) GetMemberRole(ctx context.Context, boardID string, userID int64) (AccessRole, error) {
	raw, err := r.q.GetBoardMemberRole(ctx, dbsqlc.GetBoardMemberRoleParams{BoardID: boardID, UserID: userID})
	if err != nil {
		return "", err
	}
	role := normalizeAccessRole(raw)
	if role == "" || role == RoleOwner {
		return "", database.ErrNotFound
	}
	return role, nil
}

func (r *Repository) DeleteBoardCascade(ctx context.Context, ownerUserID int64, boardID string) error {
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

func (r *Repository) ListMembers(ctx context.Context, ownerUserID int64, boardID string) ([]MemberWithUser, error) {
	rows, err := r.q.ListBoardMembers(ctx, boardID)
	if err != nil {
		return nil, err
	}
	result := make([]MemberWithUser, 0, len(rows))
	for _, row := range rows {
		role := normalizeAccessRole(row.Role)
		if role == "" {
			continue
		}
		result = append(result, MemberWithUser{
			UserID:      row.UserID,
			PublicID:    row.PublicID,
			DisplayName: row.DisplayName,
			Role:        role,
		})
	}
	return result, nil
}

func (r *Repository) UpsertMember(ctx context.Context, ownerUserID int64, boardID string, userID int64, role AccessRole) error {
	return r.q.UpsertBoardMember(ctx, dbsqlc.UpsertBoardMemberParams{
		BoardID: boardID,
		UserID:  userID,
		Role:    string(role),
	})
}

func (r *Repository) DeleteMember(ctx context.Context, ownerUserID int64, boardID string, userID int64) error {
	rows, err := r.q.DeleteBoardMember(ctx, dbsqlc.DeleteBoardMemberParams{BoardID: boardID, UserID: userID})
	if err != nil {
		return err
	}
	if rows == 0 {
		return database.ErrNotFound
	}
	return nil
}

func (r *Repository) GetMemberByUserID(ctx context.Context, ownerUserID int64, boardID string, userID int64) (*Member, error) {
	row, err := r.q.GetBoardMemberByUserID(ctx, dbsqlc.GetBoardMemberByUserIDParams{BoardID: boardID, UserID: userID})
	if err != nil {
		return nil, err
	}
	member := Member{
		ID:        row.ID,
		BoardID:   row.BoardID,
		UserID:    row.UserID,
		Role:      row.Role,
		CreatedAt: row.CreatedAt,
		UpdatedAt: row.UpdatedAt,
	}
	return &member, nil
}

func (r *Repository) CreateShareToken(ctx context.Context, token *ShareToken) error {
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

func (r *Repository) ListShareTokens(ctx context.Context, ownerUserID int64, boardID string) ([]ShareToken, error) {
	rows, err := r.q.ListBoardShareTokens(ctx, boardID)
	if err != nil {
		return nil, err
	}
	result := make([]ShareToken, 0, len(rows))
	for _, row := range rows {
		result = append(result, shareTokenFromSQLC(row))
	}
	return result, nil
}

func (r *Repository) GetShareTokenByHash(ctx context.Context, tokenHash string) (*ShareToken, error) {
	row, err := r.q.GetBoardShareTokenByHash(ctx, tokenHash)
	if err != nil {
		return nil, err
	}
	shareToken := shareTokenFromSQLC(row)
	return &shareToken, nil
}

func (r *Repository) RevokeShareTokenByID(ctx context.Context, ownerUserID int64, boardID string, tokenID int64) error {
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

func (r *Repository) ConsumeShareTokenAndUpsertMember(ctx context.Context, tokenHash string, userID int64) (*ShareToken, error) {
	var shareToken ShareToken
	err := dbutil.WithTx(ctx, r.db, func(q *dbsqlc.Queries) error {
		row, err := q.ConsumeBoardShareToken(ctx, tokenHash)
		if err != nil {
			return err
		}
		shareToken = shareTokenFromSQLC(row)
		role := normalizeMemberRole(shareToken.Role)
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

func (r *Repository) ShareTokenHashExists(ctx context.Context, tokenHash string) (bool, error) {
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

func parseBoardMediaRefsForReconcile(state string) ([]StateMediaRef, bool) {
	if strings.TrimSpace(state) == "" {
		return nil, false
	}
	envelope, err := validateBoardStateEnvelopeJSON([]byte(state))
	if err != nil {
		return nil, false
	}
	return reconcileableBoardMediaRefs(envelope.MediaRefs), true
}

func reconcileableBoardMediaRefs(refs []StateMediaRef) []StateMediaRef {
	if len(refs) == 0 {
		return nil
	}
	seen := make(map[boardMediaRefKey]struct{}, len(refs))
	result := make([]StateMediaRef, 0, len(refs))
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
		result = append(result, StateMediaRef{
			MediaAssetID: key.mediaAssetID,
			RefType:      key.refType,
			RefID:        key.refID,
			Role:         key.role,
		})
	}
	return result
}

func reconcileBoardMediaRefsTx(ctx context.Context, q *dbsqlc.Queries, board *Board, refs []StateMediaRef) error {
	if board == nil {
		return ErrInvalidInput
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

func boardFromSQLC(row dbsqlc.Board) Board {
	return Board{
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

func shareTokenFromSQLC(row dbsqlc.BoardShareToken) ShareToken {
	return ShareToken{
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
