package users

import "time"

type User struct {
	ID         int64     `json:"id"`
	Username   string    `json:"username"`
	AuthUserID *string   `json:"auth_user_id"`
	StorageUID string    `json:"uid"`
	Avatar     string    `json:"avatar,omitempty"`
	Role       string    `json:"role"`
	Status     int16     `json:"status"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type UsedUID struct {
	ID         int64     `json:"id"`
	StorageUID string    `json:"uid"`
	CreatedAt  time.Time `json:"created_at"`
}
