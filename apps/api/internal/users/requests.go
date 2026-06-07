package users

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
	List []*User `json:"list"`
}

type AdminBanUserRequest struct {
	Reason        string `json:"reason"`
	ExpiresInDays int    `json:"expires_in_days"`
}
