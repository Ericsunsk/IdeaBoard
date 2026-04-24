package boards

type UpsertBoardRequest struct {
	Name      string      `json:"name" binding:"required"`
	State     interface{} `json:"state"`
	CreatedAt *int64      `json:"created_at"`
	UpdatedAt *int64      `json:"updated_at"`
}

type InviteBoardMemberRequest struct {
	MemberUID string `json:"member_uid" binding:"required,uppercase,alphanum,len=6"`
	Role      string `json:"role" binding:"required"`
}

type UpdateBoardMemberRoleRequest struct {
	Role string `json:"role" binding:"required"`
}

type CreateBoardShareTokenRequest struct {
	Role      string `json:"role" binding:"required"`
	ExpiresAt *int64 `json:"expires_at"`
}

type JoinBoardByShareTokenRequest struct {
	Token string `json:"token" binding:"required"`
}
