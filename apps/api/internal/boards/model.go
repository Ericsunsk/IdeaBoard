package boards

import "errors"

type AccessRole string

const (
	RoleOwner  AccessRole = "owner"
	RoleEditor AccessRole = "editor"
	RoleViewer AccessRole = "viewer"
)

var (
	ErrNotFound           = errors.New("board not found")
	ErrInvalidInput       = errors.New("invalid board input")
	ErrIDConflict         = errors.New("board id conflict")
	ErrPermissionDenied   = errors.New("board permission denied")
	ErrMemberNotFound     = errors.New("board member not found")
	ErrMemberInvalidRole  = errors.New("board member invalid role")
	ErrMemberInviteSelf   = errors.New("board member invite self")
	ErrRevisionConflict   = errors.New("board revision conflict")
	ErrShareTokenNotFound = errors.New("share token not found")
	ErrShareTokenInvalid  = errors.New("share token invalid")
	ErrShareTokenExpired  = errors.New("share token expired")
	ErrShareTokenRevoked  = errors.New("share token revoked")
)
