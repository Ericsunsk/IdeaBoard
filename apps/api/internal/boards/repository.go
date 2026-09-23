package boards

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"ideaboard/api/internal/db/dbutil"
	dbsqlc "ideaboard/api/internal/db/sqlc"
	"ideaboard/api/internal/media/reclamation"
	"ideaboard/api/internal/platform/database"
	"ideaboard/api/internal/platform/idempotency"
	"ideaboard/api/internal/platform/secrets"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	db *pgxpool.Pool
	q  *dbsqlc.Queries
}

type repositoryQueryer struct {
	ctx context.Context
	db  *pgxpool.Pool
	q   *dbsqlc.Queries
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db, q: dbsqlc.New(db)}
}

func (r *Repository) dbWithContext(ctx context.Context) repositoryQueryer {
	return repositoryQueryer{ctx: ctx, db: r.db, q: r.q}
}

func (db repositoryQueryer) withTx(run func(*dbsqlc.Queries) error) error {
	return dbutil.WithTx(db.ctx, db.db, run)
}

// Bounded list caps: every list query carries a LIMIT so no caller can
// trigger an unbounded scan. Values cover the largest realistic shape
// (board media refs on dense canvases) with headroom.
const (
	maxAccessibleBoards = int32(500)
	maxBoardMembers     = int32(500)
	maxBoardShareTokens = int32(200)
	maxBoardMediaRefs   = int32(5000)
)

func (r *Repository) ListAccessibleByUserID(ctx context.Context, userID int64) ([]WithRole, error) {
	rows, err := r.dbWithContext(ctx).q.ListAccessibleBoardsByUserID(ctx, dbsqlc.ListAccessibleBoardsByUserIDParams{
		UserID: userID, RowLimit: maxAccessibleBoards,
	})
	if err != nil {
		return nil, err
	}

	result := make([]WithRole, 0, len(rows))
	for _, row := range rows {
		role := RoleOwner
		if row.OwnerUserID != userID {
			role = normalizeAccessRole(pgTextString(row.MemberRole))
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
				CreatedAt:          dbutil.TimeValue(row.CreatedAt),
				UpdatedAt:          dbutil.TimeValue(row.UpdatedAt),
			},
			Role: role,
		})
	}
	return result, nil
}

func (r *Repository) GetAccessibleByBoardID(ctx context.Context, userID int64, boardID string) (*WithRole, error) {
	row, err := r.dbWithContext(ctx).q.GetAccessibleBoardByBoardID(ctx, dbsqlc.GetAccessibleBoardByBoardIDParams{UserID: userID, BoardID: boardID})
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
			CreatedAt:          dbutil.TimeValue(row.CreatedAt),
			UpdatedAt:          dbutil.TimeValue(row.UpdatedAt),
		},
		Role: RoleOwner,
	}
	if row.OwnerUserID != userID {
		board.Role = normalizeAccessRole(pgTextString(row.MemberRole))
	}
	return &board, nil
}

func (r *Repository) GetByBoardID(ctx context.Context, boardID string) (*Board, error) {
	row, err := r.dbWithContext(ctx).q.GetBoardByBoardID(ctx, boardID)
	if err != nil {
		return nil, err
	}
	board := boardFromSQLC(row)
	return &board, nil
}

func (r *Repository) GetByOwnerAndBoardID(ctx context.Context, ownerUserID int64, boardID string) (*Board, error) {
	row, err := r.dbWithContext(ctx).q.GetBoardByOwnerAndBoardID(ctx, dbsqlc.GetBoardByOwnerAndBoardIDParams{OwnerUserID: ownerUserID, BoardID: boardID})
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
	return r.dbWithContext(ctx).withTx(func(q *dbsqlc.Queries) error {
		rows, err := q.UpsertBoard(ctx, upsertBoardParams(board))
		if err != nil {
			return err
		}
		if rows == 0 {
			count, countErr := q.CountBoardsByBoardID(ctx, board.BoardID)
			if countErr != nil {
				return countErr
			}
			if count > 0 && board.ExpectedRevision != nil {
				return ErrRevisionConflict
			}
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
		Revision: pgtype.Int8{
			Int64: board.Revision,
			Valid: board.Revision > 0,
		},
		StateSchemaVersion: pgtype.Int4{
			Int32: board.StateSchemaVersion,
			Valid: board.StateSchemaVersion > 0,
		},
		LastSavedBy:         dbutil.NullInt64(board.LastSavedBy),
		CreatedAt:           pgtype.Timestamptz{Time: board.CreatedAt, Valid: true},
		UpdatedAt:           pgtype.Timestamptz{Time: board.UpdatedAt, Valid: true},
		HasExpectedRevision: board.ExpectedRevision != nil,
		ExpectedRevision: func() int64 {
			if board.ExpectedRevision == nil {
				return 0
			}
			return *board.ExpectedRevision
		}(),
	}
}

func (r *Repository) HasNameConflict(ctx context.Context, userID int64, boardID, name string) (bool, error) {
	hasBoardID := strings.TrimSpace(boardID) != ""
	count, err := r.dbWithContext(ctx).q.CountBoardNameConflicts(ctx, dbsqlc.CountBoardNameConflictsParams{
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
	count, err := r.dbWithContext(ctx).q.CountBoardsByBoardID(ctx, boardID)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (r *Repository) GetMemberRole(ctx context.Context, boardID string, userID int64) (AccessRole, error) {
	raw, err := r.dbWithContext(ctx).q.GetBoardMemberRole(ctx, dbsqlc.GetBoardMemberRoleParams{BoardID: boardID, UserID: userID})
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
	return r.dbWithContext(ctx).withTx(func(q *dbsqlc.Queries) error {
		previous, err := q.ListMediaRefsByBoardID(ctx, dbsqlc.ListMediaRefsByBoardIDParams{
			OwnerUserID: ownerUserID,
			BoardID:     boardID,
			RowLimit:    maxBoardMediaRefs,
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
				ReclaimAfter:  gracePeriodTimestamptz(),
				MediaAssetIds: uniqueMediaAssetIDs(assetIDs),
			}); err != nil {
				return err
			}
		}
		return nil
	})
}

func (r *Repository) ListMembers(ctx context.Context, ownerUserID int64, boardID string) ([]MemberWithUser, error) {
	rows, err := r.dbWithContext(ctx).q.ListBoardMembers(ctx, dbsqlc.ListBoardMembersParams{
		BoardID: boardID, RowLimit: maxBoardMembers,
	})
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
			PublicID:    uuidFromPgType(row.PublicID),
			DisplayName: row.DisplayName,
			Role:        role,
		})
	}
	return result, nil
}

func (r *Repository) UpsertMember(ctx context.Context, ownerUserID int64, boardID string, userID int64, role AccessRole) error {
	return r.dbWithContext(ctx).q.UpsertBoardMember(ctx, dbsqlc.UpsertBoardMemberParams{
		BoardID: boardID,
		UserID:  userID,
		Role:    string(role),
	})
}

func (r *Repository) DeleteMember(ctx context.Context, ownerUserID int64, boardID string, userID int64) error {
	rows, err := r.dbWithContext(ctx).q.DeleteBoardMember(ctx, dbsqlc.DeleteBoardMemberParams{BoardID: boardID, UserID: userID})
	if err != nil {
		return err
	}
	if rows == 0 {
		return database.ErrNotFound
	}
	return nil
}

func (r *Repository) GetMemberByUserID(ctx context.Context, ownerUserID int64, boardID string, userID int64) (*Member, error) {
	row, err := r.dbWithContext(ctx).q.GetBoardMemberByUserID(ctx, dbsqlc.GetBoardMemberByUserIDParams{BoardID: boardID, UserID: userID})
	if err != nil {
		return nil, err
	}
	member := Member{
		ID:        row.ID,
		BoardID:   row.BoardID,
		UserID:    row.UserID,
		Role:      row.Role,
		CreatedAt: dbutil.TimeValue(row.CreatedAt),
		UpdatedAt: dbutil.TimeValue(row.UpdatedAt),
	}
	return &member, nil
}

func (r *Repository) CreateShareToken(ctx context.Context, token *ShareToken) error {
	id, err := r.dbWithContext(ctx).q.CreateBoardShareToken(ctx, dbsqlc.CreateBoardShareTokenParams{
		BoardID:        token.BoardID,
		TokenHash:      token.TokenHash,
		TokenPrefix:    token.TokenPrefix,
		Role:           token.Role,
		CreatedBy:      token.CreatedBy,
		Revoked:        token.Revoked,
		ExpiresAt:      dbutil.NullTime(token.ExpiresAt),
		MaxUses:        dbutil.NullInt32(token.MaxUses),
		IdempotencyKey: "", RequestHash: "", TokenSecret: "",
	})
	if err != nil {
		return err
	}
	token.ID = int64(id)
	return nil
}

// CreateShareTokenIdempotent durably creates and replays a share token. The
// bearer token is encrypted at rest because it is otherwise unrecoverable
// after a process restart.
func (r *Repository) CreateShareTokenIdempotent(ctx context.Context, token *ShareToken, key, requestHash string) (*ShareToken, bool, error) {
	if r == nil || r.db == nil || r.q == nil {
		return nil, false, errors.New("board repository is unavailable")
	}
	ciphertext, err := secrets.EncryptString(token.Token)
	if err != nil {
		return nil, false, fmt.Errorf("encrypt share token for replay: %w", err)
	}
	var result *ShareToken
	var replayed bool
	err = dbutil.WithTx(ctx, r.db, func(q *dbsqlc.Queries) error {
		if err := q.LockBoardShareTokenIdempotency(ctx, fmt.Sprintf("%d:%s", token.CreatedBy, key)); err != nil {
			return fmt.Errorf("lock share token idempotency: %w", err)
		}
		existing, getErr := q.GetBoardShareTokenByIdempotency(ctx, dbsqlc.GetBoardShareTokenByIdempotencyParams{CreatedBy: token.CreatedBy, IdempotencyKey: key})
		if getErr == nil {
			if existing.RequestHash != requestHash {
				return idempotency.ConflictError()
			}
			plain, decryptErr := secrets.DecryptString(existing.TokenSecret)
			if decryptErr != nil {
				return fmt.Errorf("decrypt replayed share token: %w", decryptErr)
			}
			mapped := shareTokenFromSQLC(dbsqlc.BoardShareToken{
				ID: existing.ID, BoardID: existing.BoardID, TokenHash: existing.TokenHash,
				TokenPrefix: existing.TokenPrefix, Role: existing.Role, CreatedBy: existing.CreatedBy,
				Revoked: existing.Revoked, UsageCount: existing.UsageCount, MaxUses: existing.MaxUses,
				LastUsedAt: existing.LastUsedAt, RevokedAt: existing.RevokedAt, RevokedBy: existing.RevokedBy,
				ExpiresAt: existing.ExpiresAt, CreatedAt: existing.CreatedAt, UpdatedAt: existing.UpdatedAt,
			})
			mapped.Token = plain
			result = &mapped
			replayed = true
			return nil
		}
		if !database.IsNotFound(getErr) {
			return getErr
		}
		id, insertErr := q.CreateBoardShareToken(ctx, dbsqlc.CreateBoardShareTokenParams{
			BoardID: token.BoardID, TokenHash: token.TokenHash, TokenPrefix: token.TokenPrefix,
			Role: token.Role, CreatedBy: token.CreatedBy, Revoked: token.Revoked,
			ExpiresAt: dbutil.NullTime(token.ExpiresAt), MaxUses: dbutil.NullInt32(token.MaxUses),
			IdempotencyKey: key, RequestHash: requestHash, TokenSecret: ciphertext,
		})
		if insertErr != nil {
			return insertErr
		}
		copy := *token
		copy.ID = int64(id)
		result = &copy
		return nil
	})
	return result, replayed, err
}

func (r *Repository) ListShareTokens(ctx context.Context, ownerUserID int64, boardID string) ([]ShareToken, error) {
	rows, err := r.dbWithContext(ctx).q.ListBoardShareTokens(ctx, dbsqlc.ListBoardShareTokensParams{
		BoardID: boardID, RowLimit: maxBoardShareTokens,
	})
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
	row, err := r.dbWithContext(ctx).q.GetBoardShareTokenByHash(ctx, tokenHash)
	if err != nil {
		return nil, err
	}
	shareToken := shareTokenFromSQLC(row)
	return &shareToken, nil
}

func (r *Repository) RevokeShareTokenByID(ctx context.Context, ownerUserID int64, boardID string, tokenID int64) error {
	rows, err := r.dbWithContext(ctx).q.RevokeBoardShareTokenByID(ctx, dbsqlc.RevokeBoardShareTokenByIDParams{
		BoardID:   boardID,
		TokenID:   tokenID,
		RevokedBy: pgtype.Int8{Int64: ownerUserID, Valid: true},
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
	err := r.dbWithContext(ctx).withTx(func(q *dbsqlc.Queries) error {
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
	count, err := r.dbWithContext(ctx).q.CountBoardShareTokenByHash(ctx, tokenHash)
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
		RowLimit:    maxBoardMediaRefs,
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
			StaleClaimBefore: claimLeaseTimestamptz(),
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
			ReclaimAfter:  gracePeriodTimestamptz(),
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
		CreatedAt:          dbutil.TimeValue(row.CreatedAt),
		UpdatedAt:          dbutil.TimeValue(row.UpdatedAt),
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

func shareTokenFromSQLC(row any) ShareToken {
	// sqlc emits a distinct row type for each query even when the selected
	// columns match. JSON tags provide a stable adapter without editing the
	// generated package whenever the schema gains replay-only columns.
	encoded, _ := json.Marshal(row)
	var base dbsqlc.BoardShareToken
	_ = json.Unmarshal(encoded, &base)
	return ShareToken{
		ID: base.ID, BoardID: base.BoardID, TokenHash: base.TokenHash,
		TokenPrefix: base.TokenPrefix, Role: base.Role, CreatedBy: base.CreatedBy,
		Revoked: base.Revoked, UsageCount: base.UsageCount,
		MaxUses: dbutil.Int32Ptr(base.MaxUses), LastUsedAt: dbutil.TimePtr(base.LastUsedAt),
		RevokedAt: dbutil.TimePtr(base.RevokedAt), RevokedBy: dbutil.Int64Ptr(base.RevokedBy),
		ExpiresAt: dbutil.TimePtr(base.ExpiresAt), CreatedAt: dbutil.TimeValue(base.CreatedAt),
		UpdatedAt: dbutil.TimeValue(base.UpdatedAt),
	}
}

func pgTextString(v pgtype.Text) string {
	if !v.Valid {
		return ""
	}
	return v.String
}

func gracePeriodTimestamptz() pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: time.Now().Add(reclamation.GracePeriod), Valid: true}
}

func claimLeaseTimestamptz() pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: time.Now().Add(-reclamation.ClaimLeaseDuration), Valid: true}
}

func uuidFromPgType(v pgtype.UUID) uuid.UUID {
	return uuid.UUID(v.Bytes)
}
