package boards

import "time"

type BoardAccessRole string

const (
	BoardAccessRoleOwner  BoardAccessRole = "owner"
	BoardAccessRoleEditor BoardAccessRole = "editor"
	BoardAccessRoleViewer BoardAccessRole = "viewer"
)

type UserBoard struct {
	ID        uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	BoardID   string    `gorm:"type:varchar(100);not null;uniqueIndex:uk_user_boards_board_id" json:"board_id"`
	UserID    uint64    `gorm:"not null;uniqueIndex:idx_user_board_name,priority:1;index:idx_user_updated,priority:1" json:"user_id"`
	Name      string    `gorm:"type:varchar(60);not null;uniqueIndex:idx_user_board_name,priority:2" json:"name"`
	State     string    `gorm:"type:longtext;not null" json:"state"`
	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time `gorm:"autoUpdateTime;index:idx_user_updated,priority:2,sort:desc" json:"updated_at"`
}

type BoardMember struct {
	ID        uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	BoardID   string    `gorm:"type:varchar(100);not null;uniqueIndex:idx_board_member,priority:1;index:idx_board_member_user,priority:2" json:"board_id"`
	UserID    uint64    `gorm:"not null;uniqueIndex:idx_board_member,priority:2;index:idx_board_member_user,priority:1" json:"user_id"`
	Role      string    `gorm:"type:varchar(20);not null" json:"role"`
	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

type BoardShareToken struct {
	ID        uint64     `gorm:"primaryKey;autoIncrement" json:"id"`
	BoardID   string     `gorm:"type:varchar(100);not null;index:idx_board_share_token_board,priority:1" json:"board_id"`
	Token     string     `gorm:"type:varchar(128);not null;uniqueIndex" json:"token"`
	Role      string     `gorm:"type:varchar(20);not null" json:"role"`
	CreatedBy uint64     `gorm:"not null;index:idx_board_share_token_board,priority:2" json:"created_by"`
	Revoked   bool       `gorm:"not null;default:false;index:idx_board_share_token_board,priority:3" json:"revoked"`
	ExpiresAt *time.Time `gorm:"index:idx_board_share_token_board,priority:4" json:"expires_at"`
	CreatedAt time.Time  `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time  `gorm:"autoUpdateTime" json:"updated_at"`
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
