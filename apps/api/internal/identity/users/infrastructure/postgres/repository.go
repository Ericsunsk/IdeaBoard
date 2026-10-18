package postgres

import (
	"context"
	"errors"
	"ideaboard/api/internal/platform/errorrender"
	"strings"

	"ideaboard/api/internal/db/dbutil"
	dbsqlc "ideaboard/api/internal/db/sqlc"
	"ideaboard/api/internal/identity/users/domain"
	"ideaboard/api/internal/identity/users/ports"
	"ideaboard/api/internal/platform/database"

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
	queries := r.q
	if tx, ok := dbutil.AmbientTxFromContext(ctx); ok {
		queries = dbsqlc.New(tx)
	}
	return repositoryQueryer{ctx: ctx, db: r.db, q: queries}
}

func (db repositoryQueryer) withTx(run func(*dbsqlc.Queries) error) error {
	return dbutil.WithTx(db.ctx, db.db, run)
}

func (q repositoryQueryer) getUsersByIDs(ids []int64) ([]dbsqlc.User, error) {
	return q.q.GetUsersByIDs(q.ctx, ids)
}

// Create 创建用户
func (r *Repository) Create(ctx context.Context, user *domain.User) error {
	if user == nil {
		return errors.New(errorrender.MsgCreate)
	}
	return r.dbWithContext(ctx).withTx(func(q *dbsqlc.Queries) error {
		uid, err := allocateStorageUID(ctx, q, strings.TrimSpace(user.StorageUID), 0)
		if err != nil {
			return err
		}
		user.StorageUID = uid
		role := strings.ToLower(strings.TrimSpace(user.Role))
		if role == "" {
			role = domain.RoleUser
		}
		displayName := strings.TrimSpace(user.DisplayName)
		if displayName == "" {
			displayName = "IdeaBoard User"
		}
		created, err := q.CreateUser(ctx, dbsqlc.CreateUserParams{
			DisplayName: displayName,
			AuthUserID:  dbutil.NullString(user.AuthUserID),
			StorageUid:  user.StorageUID,
			Avatar:      user.Avatar,
			Role:        role,
			Status:      user.Status,
		})
		if err != nil {
			return err
		}
		user.ID = created.ID
		user.PublicID = uuid.UUID(created.PublicID.Bytes)
		user.DisplayName = created.DisplayName
		return nil
	})
}

// GetByID 根据ID获取用户
func (r *Repository) GetByID(ctx context.Context, id int64) (*domain.User, error) {
	row, err := r.dbWithContext(ctx).q.GetUserByID(ctx, id)
	if err != nil {
		return nil, err
	}
	user := userFromSQLC(row)
	return &user, nil
}

// GetByIDs resolves a bounded set of users with one database query.
func (r *Repository) GetByIDs(ctx context.Context, ids []int64) (map[int64]*domain.User, error) {
	usersByID := make(map[int64]*domain.User, len(ids))
	if len(ids) == 0 {
		return usersByID, nil
	}
	rows, err := r.dbWithContext(ctx).getUsersByIDs(ids)
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		user := userFromSQLC(row)
		usersByID[user.ID] = &user
	}
	return usersByID, nil
}

// GetByPublicID resolves the immutable identifier accepted at public API boundaries.
func (r *Repository) GetByPublicID(ctx context.Context, rawPublicID string) (*domain.User, error) {
	publicID, err := uuid.Parse(strings.TrimSpace(rawPublicID))
	if err != nil {
		return nil, database.ErrNotFound
	}
	row, err := r.dbWithContext(ctx).q.GetUserByPublicID(ctx, pgtype.UUID{Bytes: publicID, Valid: true})
	if err != nil {
		return nil, err
	}
	user := userFromSQLC(row)
	return &user, nil
}

// GetByAuthUserID 根据 Better Auth user id 获取用户
func (r *Repository) GetByAuthUserID(ctx context.Context, authUserID string) (*domain.User, error) {
	trimmedAuthID := strings.TrimSpace(authUserID)
	row, err := r.dbWithContext(ctx).q.GetUserByAuthUserID(ctx, pgtype.Text{String: trimmedAuthID, Valid: trimmedAuthID != ""})
	if err != nil {
		return nil, err
	}
	user := userFromSQLC(row)
	return &user, nil
}

// GetByStorageUID 根据 storage uid 获取用户
func (r *Repository) GetByStorageUID(ctx context.Context, storageUID string) (*domain.User, error) {
	normalized := strings.TrimSpace(storageUID)
	if !domain.IsValidStorageUID(normalized) {
		return nil, database.ErrNotFound
	}
	row, err := r.dbWithContext(ctx).q.GetUserByStorageUID(ctx, normalized)
	if err != nil {
		return nil, err
	}
	user := userFromSQLC(row)
	return &user, nil
}

// Update 更新用户
func (r *Repository) Update(ctx context.Context, user *domain.User) error {
	updated, err := r.dbWithContext(ctx).q.UpdateUser(ctx, dbsqlc.UpdateUserParams{
		DisplayName: strings.TrimSpace(user.DisplayName),
		AuthUserID:  dbutil.NullString(user.AuthUserID),
		StorageUid:  user.StorageUID,
		Avatar:      user.Avatar,
		Role:        user.Role,
		Status:      user.Status,
		ID:          user.ID,
	})
	if err != nil {
		return err
	}
	return requireOneUserRow(updated)
}

// UpdateLastLoginMetadata records the latest successful auth session.
func (r *Repository) UpdateLastLoginMetadata(ctx context.Context, authUserID, ip, country string) error {
	updated, err := r.dbWithContext(ctx).q.UpdateUserLastLoginMetadata(ctx, dbsqlc.UpdateUserLastLoginMetadataParams{
		AuthUserID:       pgtype.Text{String: authUserID, Valid: authUserID != ""},
		LastLoginIp:      pgtype.Text{String: ip, Valid: ip != ""},
		LastLoginCountry: pgtype.Text{String: country, Valid: country != ""},
	})
	if err != nil {
		return err
	}
	return requireOneUserRow(updated)
}

// List 获取用户列表
func (r *Repository) List(
	ctx context.Context,
	keyword string,
	role string,
	status int16,
	hasStatus bool,
	page int,
	pageSize int,
) ([]*domain.User, ports.UserListCounts, error) {
	keyword = strings.TrimSpace(keyword)
	role = strings.TrimSpace(role)
	hasKeyword := keyword != ""
	hasRole := role != ""
	keywordLike := "%" + keyword + "%"
	queries := r.dbWithContext(ctx).q
	summary, err := queries.SummarizeUsers(ctx, dbsqlc.SummarizeUsersParams{
		HasKeyword:  hasKeyword,
		KeywordLike: keywordLike,
		HasRole:     hasRole,
		Role:        role,
		HasStatus:   hasStatus,
		Status:      status,
	})
	if err != nil {
		return nil, ports.UserListCounts{}, err
	}
	offset := (page - 1) * pageSize
	rows, err := queries.ListUsers(ctx, dbsqlc.ListUsersParams{
		HasKeyword:  hasKeyword,
		KeywordLike: keywordLike,
		HasRole:     hasRole,
		Role:        role,
		HasStatus:   hasStatus,
		Status:      status,
		PageLimit:   int32(pageSize),
		PageOffset:  int32(offset),
	})
	if err != nil {
		return nil, ports.UserListCounts{}, err
	}
	result := make([]*domain.User, 0, len(rows))
	for _, row := range rows {
		user := userFromSQLC(row)
		result = append(result, &user)
	}
	return result, ports.UserListCounts{
		Total:       summary.Total,
		ActiveCount: summary.ActiveCount,
		BannedCount: summary.BannedCount,
		AdminCount:  summary.AdminCount,
	}, nil
}

// UpdateStatus 更新用户状态
func (r *Repository) UpdateStatus(ctx context.Context, id int64, status int16) error {
	updated, err := r.dbWithContext(ctx).q.UpdateUserStatus(ctx, dbsqlc.UpdateUserStatusParams{Status: status, ID: id})
	if err != nil {
		return err
	}
	return requireOneUserRow(updated)
}

func (r *Repository) UpdateRoleByID(ctx context.Context, id int64, role string) error {
	updated, err := r.dbWithContext(ctx).q.UpdateUserRole(ctx, dbsqlc.UpdateUserRoleParams{Role: role, ID: id})
	if err != nil {
		return err
	}
	return requireOneUserRow(updated)
}

func (r *Repository) UpdateAvatarByID(ctx context.Context, id int64, avatar string) error {
	updated, err := r.dbWithContext(ctx).q.UpdateUserAvatar(ctx, dbsqlc.UpdateUserAvatarParams{Avatar: strings.TrimSpace(avatar), ID: id})
	if err != nil {
		return err
	}
	return requireOneUserRow(updated)
}

func (r *Repository) EnsureStorageUIDs(ctx context.Context) error {
	return r.dbWithContext(ctx).withTx(func(q *dbsqlc.Queries) error {
		rows, err := q.ListUserStorageUIDs(ctx)
		if err != nil {
			return err
		}

		seen := make(map[string]int64, len(rows))
		for _, row := range rows {
			if row.ID == 0 {
				continue
			}

			uid := strings.TrimSpace(row.StorageUid)
			if domain.IsValidStorageUID(uid) {
				if firstUserID, duplicated := seen[uid]; !duplicated || firstUserID == row.ID {
					if err := ensureUsedUIDRecorded(ctx, q, uid); err != nil {
						return err
					}
					seen[uid] = row.ID
					continue
				}
			}

			uniqueUID, err := allocateStorageUID(ctx, q, "", row.ID)
			if err != nil {
				return err
			}
			updated, err := q.UpdateUserStorageUID(ctx, dbsqlc.UpdateUserStorageUIDParams{StorageUid: uniqueUID, ID: row.ID})
			if err != nil {
				return err
			}
			if err := requireOneUserRow(updated); err != nil {
				return err
			}
			seen[uniqueUID] = row.ID
		}
		return nil
	})
}

func allocateStorageUID(ctx context.Context, q *dbsqlc.Queries, preferredUID string, excludeID int64) (string, error) {
	normalizedPreferred := strings.TrimSpace(preferredUID)
	if domain.IsValidStorageUID(normalizedPreferred) {
		reserved, err := reserveStorageUID(ctx, q, normalizedPreferred, excludeID)
		if err != nil {
			return "", err
		}
		if reserved {
			return normalizedPreferred, nil
		}
	}

	for i := 0; i < 128; i++ {
		storageUID, err := domain.NewStorageUID()
		if err != nil {
			return "", err
		}
		reserved, err := reserveStorageUID(ctx, q, storageUID, excludeID)
		if err != nil {
			return "", err
		}
		if reserved {
			return storageUID, nil
		}
	}
	return "", errors.New(errorrender.MsgAllocateStorageUID)
}

func reserveStorageUID(ctx context.Context, q *dbsqlc.Queries, storageUID string, excludeID int64) (bool, error) {
	taken, err := storageUIDTaken(ctx, q, storageUID, excludeID)
	if err != nil {
		return false, err
	}
	if taken {
		return false, nil
	}

	recorded, err := recordUsedUID(ctx, q, storageUID)
	if err != nil {
		return false, err
	}
	return recorded, nil
}

func storageUIDTaken(ctx context.Context, q *dbsqlc.Queries, storageUID string, excludeID int64) (bool, error) {
	exists, err := storageUIDExists(ctx, q, storageUID, excludeID)
	if err != nil {
		return false, err
	}
	if exists {
		return true, nil
	}
	return usedUIDExists(ctx, q, storageUID)
}

func storageUIDExists(ctx context.Context, q *dbsqlc.Queries, storageUID string, excludeID int64) (bool, error) {
	_, err := q.GetUserIDByStorageUIDForUpdate(ctx, dbsqlc.GetUserIDByStorageUIDForUpdateParams{StorageUid: storageUID, ID: excludeID})
	if err == nil {
		return true, nil
	}
	if database.IsNotFound(err) {
		return false, nil
	}
	return false, err
}

func usedUIDExists(ctx context.Context, q *dbsqlc.Queries, storageUID string) (bool, error) {
	_, err := q.GetUsedUIDForUpdate(ctx, storageUID)
	if err == nil {
		return true, nil
	}
	if database.IsNotFound(err) {
		return false, nil
	}
	return false, err
}

func recordUsedUID(ctx context.Context, q *dbsqlc.Queries, storageUID string) (bool, error) {
	rows, err := q.InsertUsedUIDIgnore(ctx, storageUID)
	if err != nil {
		return false, err
	}
	return rows > 0, nil
}

func ensureUsedUIDRecorded(ctx context.Context, q *dbsqlc.Queries, storageUID string) error {
	_, err := recordUsedUID(ctx, q, storageUID)
	return err
}

func requireOneUserRow(rows int64) error {
	if rows == 0 {
		return database.ErrNotFound
	}
	return nil
}

func userFromSQLC(row dbsqlc.User) domain.User {
	return domain.User{
		ID:               row.ID,
		PublicID:         uuid.UUID(row.PublicID.Bytes),
		DisplayName:      row.DisplayName,
		AuthUserID:       dbutil.StringPtr(row.AuthUserID),
		StorageUID:       row.StorageUid,
		Avatar:           row.Avatar,
		Role:             row.Role,
		Status:           row.Status,
		AuthSyncVersion:  row.AuthSyncVersion,
		BanExpiresAt:     dbutil.TimePtr(row.BanExpiresAt),
		LastLoginIP:      pgTextToString(row.LastLoginIp),
		LastLoginCountry: pgTextToString(row.LastLoginCountry),
		CreatedAt:        dbutil.TimeValue(row.CreatedAt),
		UpdatedAt:        dbutil.TimeValue(row.UpdatedAt),
	}
}

func pgTextToString(v pgtype.Text) string {
	if !v.Valid {
		return ""
	}
	return v.String
}
