package users

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"ideaboard/api/internal/db/dbutil"
	dbsqlc "ideaboard/api/internal/db/sqlc"
	"ideaboard/api/internal/platform/database"
)

type UserRepository struct {
	db *sql.DB
	q  *dbsqlc.Queries
}

func NewUserRepository(db *sql.DB) *UserRepository {
	return &UserRepository{db: db, q: dbsqlc.New(db)}
}

// Create 创建用户
func (r *UserRepository) Create(ctx context.Context, user *User) error {
	if user == nil {
		return fmt.Errorf("用户不能为空")
	}
	return dbutil.WithTx(ctx, r.db, func(q *dbsqlc.Queries) error {
		uid, err := allocateStorageUID(ctx, q, strings.TrimSpace(user.StorageUID), 0)
		if err != nil {
			return err
		}
		user.StorageUID = uid
		role := strings.ToLower(strings.TrimSpace(user.Role))
		if role == "" {
			role = "user"
		}
		id, err := q.CreateUser(ctx, dbsqlc.CreateUserParams{
			Username:   user.Username,
			AuthUserID: dbutil.NullString(user.AuthUserID),
			StorageUid: user.StorageUID,
			Avatar:     user.Avatar,
			Role:       role,
			Status:     user.Status,
		})
		if err != nil {
			return err
		}
		user.ID = int64(id)
		return nil
	})
}

// GetByID 根据ID获取用户
func (r *UserRepository) GetByID(ctx context.Context, id int64) (*User, error) {
	row, err := r.q.GetUserByID(ctx, id)
	if err != nil {
		return nil, err
	}
	user := userFromSQLC(row)
	return &user, nil
}

// GetByUsername 根据用户名获取用户
func (r *UserRepository) GetByUsername(ctx context.Context, username string) (*User, error) {
	row, err := r.q.GetUserByUsername(ctx, username)
	if err != nil {
		return nil, err
	}
	user := userFromSQLC(row)
	return &user, nil
}

// GetByAuthUserID 根据 Better Auth user id 获取用户
func (r *UserRepository) GetByAuthUserID(ctx context.Context, authUserID string) (*User, error) {
	row, err := r.q.GetUserByAuthUserID(ctx, sql.NullString{String: strings.TrimSpace(authUserID), Valid: strings.TrimSpace(authUserID) != ""})
	if err != nil {
		return nil, err
	}
	user := userFromSQLC(row)
	return &user, nil
}

// GetByStorageUID 根据 storage uid 获取用户
func (r *UserRepository) GetByStorageUID(ctx context.Context, storageUID string) (*User, error) {
	normalized := strings.TrimSpace(storageUID)
	if !IsValidStorageUID(normalized) {
		return nil, database.ErrNotFound
	}
	row, err := r.q.GetUserByStorageUID(ctx, normalized)
	if err != nil {
		return nil, err
	}
	user := userFromSQLC(row)
	return &user, nil
}

// Update 更新用户
func (r *UserRepository) Update(ctx context.Context, user *User) error {
	updated, err := r.q.UpdateUser(ctx, dbsqlc.UpdateUserParams{
		Username:   user.Username,
		AuthUserID: dbutil.NullString(user.AuthUserID),
		StorageUid: user.StorageUID,
		Avatar:     user.Avatar,
		Role:       user.Role,
		Status:     user.Status,
		ID:         user.ID,
	})
	if err != nil {
		return err
	}
	return requireOneUserRow(updated)
}

// List 获取用户列表
func (r *UserRepository) List(
	ctx context.Context,
	keyword string,
	role string,
	status int16,
	hasStatus bool,
	page int,
	pageSize int,
) ([]*User, UserListCounts, error) {
	keyword = strings.TrimSpace(keyword)
	role = strings.TrimSpace(role)
	hasKeyword := keyword != ""
	hasRole := role != ""
	keywordLike := "%" + keyword + "%"
	summary, err := r.q.SummarizeUsers(ctx, dbsqlc.SummarizeUsersParams{
		HasKeyword:  hasKeyword,
		KeywordLike: keywordLike,
		HasRole:     hasRole,
		Role:        role,
		HasStatus:   hasStatus,
		Status:      status,
	})
	if err != nil {
		return nil, UserListCounts{}, err
	}
	offset := (page - 1) * pageSize
	rows, err := r.q.ListUsers(ctx, dbsqlc.ListUsersParams{
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
		return nil, UserListCounts{}, err
	}
	result := make([]*User, 0, len(rows))
	for _, row := range rows {
		user := userFromSQLC(row)
		result = append(result, &user)
	}
	return result, UserListCounts{
		Total:       summary.Total,
		ActiveCount: summary.ActiveCount,
		BannedCount: summary.BannedCount,
		AdminCount:  summary.AdminCount,
	}, nil
}

// UpdateStatus 更新用户状态
func (r *UserRepository) UpdateStatus(ctx context.Context, id int64, status int16) error {
	updated, err := r.q.UpdateUserStatus(ctx, dbsqlc.UpdateUserStatusParams{Status: status, ID: id})
	if err != nil {
		return err
	}
	return requireOneUserRow(updated)
}

func (r *UserRepository) UpdateRoleByID(ctx context.Context, id int64, role string) error {
	updated, err := r.q.UpdateUserRole(ctx, dbsqlc.UpdateUserRoleParams{Role: role, ID: id})
	if err != nil {
		return err
	}
	return requireOneUserRow(updated)
}

func (r *UserRepository) UpdateAvatarByID(ctx context.Context, id int64, avatar string) error {
	updated, err := r.q.UpdateUserAvatar(ctx, dbsqlc.UpdateUserAvatarParams{Avatar: strings.TrimSpace(avatar), ID: id})
	if err != nil {
		return err
	}
	return requireOneUserRow(updated)
}

func (r *UserRepository) EnsureStorageUIDs(ctx context.Context) error {
	return dbutil.WithTx(ctx, r.db, func(q *dbsqlc.Queries) error {
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
			if IsValidStorageUID(uid) {
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
	if IsValidStorageUID(normalizedPreferred) {
		reserved, err := reserveStorageUID(ctx, q, normalizedPreferred, excludeID)
		if err != nil {
			return "", err
		}
		if reserved {
			return normalizedPreferred, nil
		}
	}

	for i := 0; i < 128; i++ {
		storageUID, err := NewStorageUID()
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
	return "", fmt.Errorf("生成唯一用户存储标识失败")
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

func userFromSQLC(row dbsqlc.User) User {
	return User{
		ID:         row.ID,
		Username:   row.Username,
		AuthUserID: dbutil.StringPtr(row.AuthUserID),
		StorageUID: row.StorageUid,
		Avatar:     row.Avatar,
		Role:       row.Role,
		Status:     row.Status,
		CreatedAt:  row.CreatedAt,
		UpdatedAt:  row.UpdatedAt,
	}
}
