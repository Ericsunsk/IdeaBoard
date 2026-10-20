package domain

import (
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

var (
	ErrUserNotFound                      = errors.New("user not found")
	ErrUserDeletionInProgress            = errors.New("user deletion is already in progress")
	ErrUserDeletionBlockedByPaymentState = errors.New("user deletion is blocked by unsettled payment state")
	ErrUserDeletionAuthUnavailable       = errors.New("user deletion Auth deleter is not configured")
	ErrUserDeletionStorageUnavailable    = errors.New("user deletion storage cleaner is not configured")
	ErrInvalidStorageUID                 = errors.New("invalid storage UID")
	ErrInvalidDisplayName                = errors.New("invalid display name")
)

// MaxDisplayNameLength mirrors the users.display_name column width
// (VARCHAR(100)) and its CHECK (length(btrim(display_name)) BETWEEN 1
// AND 100): validation counts characters, never bytes.
const MaxDisplayNameLength = 100

// ValidateDisplayName trims surrounding whitespace, rejects empty names,
// and caps length at the column width. It mirrors the database CHECK so
// callers fail fast with 422 instead of surfacing a constraint violation.
func ValidateDisplayName(raw string) error {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || len([]rune(trimmed)) > MaxDisplayNameLength {
		return ErrInvalidDisplayName
	}
	return nil
}

const (
	RoleUser  = "user"
	RoleAdmin = "admin"

	StatusActive int16 = 1
	StatusBanned int16 = 0
)

type User struct {
	ID          int64     `json:"id"`
	PublicID    uuid.UUID `json:"public_id"`
	DisplayName string    `json:"display_name"`
	AuthUserID  *string   `json:"auth_user_id"`
	StorageUID  string    `json:"uid"`
	Avatar      string    `json:"avatar,omitempty"`
	Role        string    `json:"role"`
	Status      int16     `json:"status"`
	// AuthSyncVersion is the durable Auth sync fencing counter (issue
	// #888). Lane mutations that change role/status bump it in the same
	// transaction that persists the Auth sync intent; the sync worker
	// only pushes intents whose base version still matches.
	AuthSyncVersion int64 `json:"auth_sync_version"`
	// BanExpiresAt is the business ban deadline fact source (issue
	// #895). Nil means a permanent ban that never expires.
	BanExpiresAt     *time.Time `json:"ban_expires_at,omitempty"`
	LastLoginIP      string     `json:"-"`
	LastLoginCountry string     `json:"-"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

func (u *User) IsActive() bool {
	return u != nil && u.Status == StatusActive
}

func (u *User) IsAdmin() bool {
	return u != nil && u.Role == RoleAdmin
}

type UsedUID struct {
	ID         int64     `json:"id"`
	StorageUID string    `json:"uid"`
	CreatedAt  time.Time `json:"created_at"`
}
