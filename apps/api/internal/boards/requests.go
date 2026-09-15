package boards

type UpsertRequest struct {
	Name  string      `json:"name" binding:"required"`
	State interface{} `json:"state"`
	// BaseRevision is the revision observed by the client before writing. New
	// boards use zero; existing boards must match the persisted revision.
	BaseRevision int64  `json:"base_revision"`
	CreatedAt    *int64 `json:"created_at"`
	UpdatedAt    *int64 `json:"updated_at"`
}

type InviteMemberRequest struct {
	MemberPublicID string `json:"member_public_id" binding:"required,uuid"`
	Role           string `json:"role" binding:"required"`
}

type UpdateMemberRoleRequest struct {
	Role string `json:"role" binding:"required"`
}

type CreateShareTokenRequest struct {
	Role      string `json:"role" binding:"required"`
	ExpiresAt *int64 `json:"expires_at"`
}

type JoinByShareTokenRequest struct {
	Token string `json:"token" binding:"required"`
}
