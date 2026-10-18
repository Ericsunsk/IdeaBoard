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
	ErrRevisionConflict   = errors.New("board revision conflict")
	ErrAccessConflict     = errors.New("board access conflict")
	ErrPreconditionNeeded = errors.New("board access precondition required")
)

// WorkspaceAccess is the common role for a Board’s selected Workspace members.
// none disables sharing and has no selected members.
type WorkspaceAccess string

const (
	WorkspaceAccessNone   WorkspaceAccess = "none"
	WorkspaceAccessViewer WorkspaceAccess = "viewer"
	WorkspaceAccessEditor WorkspaceAccess = "editor"
)

// AccessState is the Board's actual sharing posture.
type AccessState string

const (
	AccessStatePrivate   AccessState = "private"
	AccessStateWorkspace AccessState = "workspace"
)
