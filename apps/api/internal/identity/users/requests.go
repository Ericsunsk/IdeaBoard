package users

import (
	"time"

	"github.com/google/uuid"
)

type UserListRequest struct {
	Keyword  string `form:"keyword"`
	Role     string `form:"role"`
	Status   string `form:"status"`
	Page     int    `form:"page,default=1"`
	PageSize int    `form:"page_size,default=10"`
}

type UserListCounts struct {
	Total       int64 `json:"total"`
	ActiveCount int64 `json:"active_count"`
	BannedCount int64 `json:"banned_count"`
	AdminCount  int64 `json:"admin_count"`
}

type UserListResponse struct {
	UserListCounts
	List []*AdminUserListItem `json:"list"`
}

type AdminUserListItem struct {
	PublicID         uuid.UUID `json:"public_id"`
	Email            string    `json:"email"`
	Providers        []string  `json:"providers"`
	DisplayName      string    `json:"display_name"`
	Avatar           string    `json:"avatar,omitempty"`
	Role             string    `json:"role"`
	Status           int16     `json:"status"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
	AvailableCredits int64     `json:"available_credits"`
	LastLoginIP      string    `json:"last_login_ip,omitempty"`
	LastLoginCountry string    `json:"last_login_country,omitempty"`
}

type AdminBanUserRequest struct {
	Reason        string `json:"reason"`
	ExpiresInDays int    `json:"expires_in_days"`
}
