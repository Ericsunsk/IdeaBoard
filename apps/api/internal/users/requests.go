package users

type RegisterRequest struct {
	Username string `json:"username" binding:"required,min=3,max=50"`
	Password string `json:"password" binding:"required,min=6,max=50"`
}

type LoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

type UpdatePasswordRequest struct {
	OldPassword string `json:"old_password" binding:"required"`
	NewPassword string `json:"new_password" binding:"required,min=6,max=50"`
}

type CreateUserRequest struct {
	Username string `json:"username" binding:"required,min=3,max=50"`
	Password string `json:"password" binding:"required,min=6,max=50"`
}

type UserListRequest struct {
	Keyword  string `form:"keyword"`
	Page     int    `form:"page,default=1"`
	PageSize int    `form:"page_size,default=10"`
}

type UserListResponse struct {
	Total int64  `json:"total"`
	List  []*User `json:"list"`
}

type UpdateStatusRequest struct {
	Status *int8 `json:"status" binding:"required"`
}

type UpdateUserPasswordRequest struct {
	NewPassword string `json:"new_password" binding:"required,min=6,max=50"`
}
