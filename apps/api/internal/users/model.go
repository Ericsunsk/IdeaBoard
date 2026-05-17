package users

import "time"

type User struct {
	ID         uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	Username   string    `gorm:"type:varchar(50);not null;uniqueIndex" json:"username"`
	AuthUserID *string   `gorm:"column:auth_user_id;type:varchar(128);uniqueIndex" json:"auth_user_id"`
	StorageUID string    `gorm:"type:char(6);not null;uniqueIndex" json:"uid"`
	Avatar     string    `gorm:"type:varchar(1024)" json:"avatar,omitempty"`
	Role       string    `gorm:"type:varchar(20);default:'user'" json:"role"`
	Status     int8      `gorm:"type:tinyint;default:1" json:"status"`
	CreatedAt  time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt  time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

type UsedUID struct {
	ID         uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	StorageUID string    `gorm:"column:storage_uid;type:char(6);not null;uniqueIndex" json:"uid"`
	CreatedAt  time.Time `gorm:"autoCreateTime" json:"created_at"`
}

func (UsedUID) TableName() string {
	return "users_used_uids"
}
