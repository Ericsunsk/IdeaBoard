package users

type UserListRequest struct {
	Keyword  string `form:"keyword"`
	Page     int    `form:"page,default=1"`
	PageSize int    `form:"page_size,default=10"`
}

type UserListResponse struct {
	Total int64   `json:"total"`
	List  []*User `json:"list"`
}
