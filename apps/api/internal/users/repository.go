package users

import (
	"errors"
	"fmt"
	"strings"

	mysqlDriver "github.com/go-sql-driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type UserRepository struct {
	db *gorm.DB
}

type cleanupUserBoard struct {
	BoardID string `gorm:"column:board_id"`
	UserID  uint64 `gorm:"column:user_id"`
}

func (cleanupUserBoard) TableName() string {
	return "user_boards"
}

type cleanupBoardMember struct {
	BoardID string `gorm:"column:board_id"`
	UserID  uint64 `gorm:"column:user_id"`
}

func (cleanupBoardMember) TableName() string {
	return "board_members"
}

type cleanupBoardShareToken struct {
	BoardID   string `gorm:"column:board_id"`
	CreatedBy uint64 `gorm:"column:created_by"`
}

func (cleanupBoardShareToken) TableName() string {
	return "board_share_tokens"
}

type cleanupTask struct {
	UserID uint64 `gorm:"column:user_id"`
}

func (cleanupTask) TableName() string {
	return "tasks"
}

func NewUserRepository(db *gorm.DB) *UserRepository {
	return &UserRepository{db: db}
}

// Create 创建用户
func (r *UserRepository) Create(user *User) error {
	if user == nil {
		return fmt.Errorf("用户不能为空")
	}
	return r.db.Transaction(func(tx *gorm.DB) error {
		uid, err := allocateStorageUID(tx, strings.TrimSpace(user.StorageUID), 0)
		if err != nil {
			return err
		}
		user.StorageUID = uid
		return tx.Create(user).Error
	})
}

// GetByID 根据ID获取用户
func (r *UserRepository) GetByID(id uint64) (*User, error) {
	var user User
	err := r.db.Select("id, username, auth_user_id, storage_uid, role, status, created_at, updated_at").Where("id = ?", id).First(&user).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// GetByUsername 根据用户名获取用户
func (r *UserRepository) GetByUsername(username string) (*User, error) {
	var user User
	err := r.db.Select("id, username, auth_user_id, storage_uid, role, status, created_at, updated_at").Where("username = ?", username).First(&user).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// GetByAuthUserID 根据 Better Auth user id 获取用户
func (r *UserRepository) GetByAuthUserID(authUserID string) (*User, error) {
	var user User
	err := r.db.Select("id, username, auth_user_id, storage_uid, role, status, created_at, updated_at").
		Where("auth_user_id = ?", strings.TrimSpace(authUserID)).
		First(&user).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// GetByStorageUID 根据 storage uid 获取用户
func (r *UserRepository) GetByStorageUID(storageUID string) (*User, error) {
	normalized := strings.TrimSpace(storageUID)
	if !IsValidStorageUID(normalized) {
		return nil, gorm.ErrRecordNotFound
	}
	var user User
	err := r.db.Select("id, username, auth_user_id, storage_uid, role, status, created_at, updated_at").Where("storage_uid = ?", normalized).First(&user).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// Update 更新用户
func (r *UserRepository) Update(user *User) error {
	return r.db.Save(user).Error
}

// Delete 删除用户及关联资产
func (r *UserRepository) Delete(id uint64) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		var user User
		if err := tx.Select("id").Where("id = ?", id).First(&user).Error; err != nil {
			return err
		}

		var ownedBoardIDs []string
		if err := tx.Model(&cleanupUserBoard{}).Where("user_id = ?", id).Pluck("board_id", &ownedBoardIDs).Error; err != nil {
			return err
		}
		if len(ownedBoardIDs) > 0 {
			if err := tx.Where("board_id IN ?", ownedBoardIDs).Delete(&cleanupBoardShareToken{}).Error; err != nil {
				return err
			}
			if err := tx.Where("board_id IN ?", ownedBoardIDs).Delete(&cleanupBoardMember{}).Error; err != nil {
				return err
			}
			if err := tx.Where("board_id IN ?", ownedBoardIDs).Delete(&cleanupUserBoard{}).Error; err != nil {
				return err
			}
		}

		if err := tx.Where("created_by = ?", id).Delete(&cleanupBoardShareToken{}).Error; err != nil {
			return err
		}
		if err := tx.Where("user_id = ?", id).Delete(&cleanupBoardMember{}).Error; err != nil {
			return err
		}
		if err := tx.Where("user_id = ?", id).Delete(&cleanupTask{}).Error; err != nil {
			return err
		}

		return tx.Delete(&User{}, id).Error
	})
}

// List 获取用户列表
func (r *UserRepository) List(keyword string, page, pageSize int) ([]*User, int64, error) {
	var users []*User
	var total int64

	query := r.db.Model(&User{})

	if keyword != "" {
		query = query.Where("username LIKE ?", "%"+keyword+"%")
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * pageSize
	if err := query.Offset(offset).Limit(pageSize).Order("id DESC").Find(&users).Error; err != nil {
		return nil, 0, err
	}

	return users, total, nil
}

// UpdateStatus 更新用户状态
func (r *UserRepository) UpdateStatus(id uint64, status int8) error {
	return r.db.Model(&User{}).Where("id = ?", id).Update("status", status).Error
}

func (r *UserRepository) UpdateRoleByID(id uint64, role string) error {
	return r.db.Model(&User{}).Where("id = ?", id).Update("role", role).Error
}

func (r *UserRepository) UpdateAuthUserID(id uint64, authUserID string) error {
	normalized := strings.TrimSpace(authUserID)
	if normalized == "" {
		return fmt.Errorf("auth_user_id 不能为空")
	}
	return r.db.Model(&User{}).Where("id = ?", id).Update("auth_user_id", normalized).Error
}

func (r *UserRepository) EnsureStorageUIDs() error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		var users []User
		if err := tx.Select("id, storage_uid").Order("id ASC").Find(&users).Error; err != nil {
			return err
		}

		seen := make(map[string]uint64, len(users))
		for _, user := range users {
			if user.ID == 0 {
				continue
			}

			uid := strings.TrimSpace(user.StorageUID)
			if IsValidStorageUID(uid) {
				if firstUserID, duplicated := seen[uid]; !duplicated || firstUserID == user.ID {
					if err := ensureUsedUIDRecorded(tx, uid); err != nil {
						return err
					}
					seen[uid] = user.ID
					continue
				}
			}

			uniqueUID, err := allocateStorageUID(tx, "", user.ID)
			if err != nil {
				return err
			}
			if err := tx.Model(&User{}).
				Where("id = ?", user.ID).
				Update("storage_uid", uniqueUID).Error; err != nil {
				return err
			}
			seen[uniqueUID] = user.ID
		}
		return nil
	})
}

func allocateStorageUID(tx *gorm.DB, preferredUID string, excludeID uint64) (string, error) {
	normalizedPreferred := strings.TrimSpace(preferredUID)
	if IsValidStorageUID(normalizedPreferred) {
		reserved, err := reserveStorageUID(tx, normalizedPreferred, excludeID)
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
		reserved, err := reserveStorageUID(tx, storageUID, excludeID)
		if err != nil {
			return "", err
		}
		if reserved {
			return storageUID, nil
		}
	}
	return "", fmt.Errorf("生成唯一用户存储标识失败")
}

func reserveStorageUID(tx *gorm.DB, storageUID string, excludeID uint64) (bool, error) {
	taken, err := storageUIDTaken(tx, storageUID, excludeID)
	if err != nil {
		return false, err
	}
	if taken {
		return false, nil
	}

	recorded, err := recordUsedUID(tx, storageUID)
	if err != nil {
		return false, err
	}
	return recorded, nil
}

func storageUIDTaken(tx *gorm.DB, storageUID string, excludeID uint64) (bool, error) {
	exists, err := storageUIDExists(tx, storageUID, excludeID)
	if err != nil {
		return false, err
	}
	if exists {
		return true, nil
	}
	return usedUIDExists(tx, storageUID)
}

func storageUIDExists(tx *gorm.DB, storageUID string, excludeID uint64) (bool, error) {
	query := tx.Model(&User{}).Where("storage_uid = ?", storageUID)
	if excludeID > 0 {
		query = query.Where("id <> ?", excludeID)
	}
	var count int64
	err := withOptionalUpdateLock(query).Count(&count).Error
	return count > 0, err
}

func usedUIDExists(tx *gorm.DB, storageUID string) (bool, error) {
	var count int64
	err := withOptionalUpdateLock(tx.Model(&UsedUID{}).Where("storage_uid = ?", storageUID)).Count(&count).Error
	return count > 0, err
}

func recordUsedUID(tx *gorm.DB, storageUID string) (bool, error) {
	result := tx.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "storage_uid"}},
		DoNothing: true,
	}).Create(&UsedUID{StorageUID: storageUID})
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected > 0, nil
}

func ensureUsedUIDRecorded(tx *gorm.DB, storageUID string) error {
	_, err := recordUsedUID(tx, storageUID)
	return err
}

func withOptionalUpdateLock(query *gorm.DB) *gorm.DB {
	switch strings.ToLower(strings.TrimSpace(query.Dialector.Name())) {
	case "mysql", "postgres", "sqlserver":
		return query.Clauses(clause.Locking{Strength: "UPDATE"})
	default:
		return query
	}
}

func isUniqueConstraintError(err error) bool {
	if err == nil {
		return false
	}

	var mysqlErr *mysqlDriver.MySQLError
	if errors.As(err, &mysqlErr) {
		return mysqlErr.Number == 1062
	}

	msg := strings.ToLower(strings.TrimSpace(err.Error()))
	return strings.Contains(msg, "duplicate") ||
		strings.Contains(msg, "unique constraint") ||
		strings.Contains(msg, "unique failed")
}

func IsDuplicateConstraintError(err error) bool {
	return isUniqueConstraintError(err)
}
