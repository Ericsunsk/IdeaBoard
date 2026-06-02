package boards

import "time"

type BoardAccessRole string

const (
	BoardAccessRoleOwner  BoardAccessRole = "owner"
	BoardAccessRoleEditor BoardAccessRole = "editor"
	BoardAccessRoleViewer BoardAccessRole = "viewer"
)

type UserBoard struct {
	ID        uint64    `json:"id"`
	BoardID   string    `json:"board_id"`
	UserID    uint64    `json:"user_id"`
	Name      string    `json:"name"`
	State     string    `json:"state"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type BoardMember struct {
	ID        uint64    `json:"id"`
	BoardID   string    `json:"board_id"`
	UserID    uint64    `json:"user_id"`
	Role      string    `json:"role"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type BoardShareToken struct {
	ID        uint64     `json:"id"`
	BoardID   string     `json:"board_id"`
	Token     string     `json:"token"`
	Role      string     `json:"role"`
	CreatedBy uint64     `json:"created_by"`
	Revoked   bool       `json:"revoked"`
	ExpiresAt *time.Time `json:"expires_at"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
}

type BoardWithRole struct {
	Board UserBoard
	Role  BoardAccessRole
}

type BoardMemberWithUser struct {
	UserID    uint64          `json:"user_id"`
	MemberUID string          `json:"member_uid"`
	Username  string          `json:"username"`
	Role      BoardAccessRole `json:"role"`
}
