package boards

import (
	"time"

	"github.com/google/uuid"
)

type AccessRole string

const (
	RoleOwner  AccessRole = "owner"
	RoleEditor AccessRole = "editor"
	RoleViewer AccessRole = "viewer"
)

type Board struct {
	ID                 int64     `json:"id"`
	BoardID            string    `json:"board_id"`
	OwnerUserID        int64     `json:"owner_user_id"`
	CreatedBy          int64     `json:"created_by"`
	Name               string    `json:"name"`
	State              string    `json:"state"`
	Revision           int64     `json:"revision"`
	StateSchemaVersion int32     `json:"state_schema_version"`
	LastSavedBy        *int64    `json:"last_saved_by,omitempty"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
	ExpectedRevision   *int64    `json:"-"`
}

type Member struct {
	ID        int64     `json:"id"`
	BoardID   string    `json:"board_id"`
	UserID    int64     `json:"user_id"`
	Role      string    `json:"role"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type ShareToken struct {
	ID          int64      `json:"id"`
	BoardID     string     `json:"board_id"`
	Token       string     `json:"token,omitempty"`
	TokenHash   string     `json:"-"`
	TokenPrefix string     `json:"token_prefix"`
	Role        string     `json:"role"`
	CreatedBy   int64      `json:"created_by"`
	Revoked     bool       `json:"revoked"`
	UsageCount  int64      `json:"usage_count"`
	MaxUses     *int32     `json:"max_uses,omitempty"`
	LastUsedAt  *time.Time `json:"last_used_at"`
	RevokedAt   *time.Time `json:"revoked_at"`
	RevokedBy   *int64     `json:"revoked_by,omitempty"`
	ExpiresAt   *time.Time `json:"expires_at"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

type WithRole struct {
	Board Board
	Role  AccessRole
}

func (b WithRole) CanWrite() bool {
	return b.Role == RoleOwner || b.Role == RoleEditor
}

type MemberWithUser struct {
	UserID      int64      `json:"-"`
	PublicID    uuid.UUID  `json:"public_id"`
	DisplayName string     `json:"display_name"`
	Role        AccessRole `json:"role"`
}
