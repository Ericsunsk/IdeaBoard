package boards

import (
	"context"
	"fmt"
	"strings"
)

// Board access requires current Workspace membership, including for the creator.
// The creator is the implicit Owner; selected members receive the common role. Workspace Owner/Admin have no implicit Board role.
// ResolveRole and CheckCapability are the shared policy for storage adapters;
// other contexts consume the narrow readers below.

// Capability is one board operation class. Reads need any visible role,
// writes need owner|editor, and sharing management needs owner.
type Capability string

const (
	// CapRead covers list/catalog (including the embedded Preview Scene),
	// metadata, document, immutable revisions, member listing, and access
	// reads.
	CapRead Capability = "read"
	// CapWrite covers rename, archive/restore, and document replacement.
	CapWrite Capability = "write"
	// CapManage covers selected-member sharing replacement and access-resource reads.
	CapManage Capability = "manage"
)

// RoleReader is the narrow port for Media / Tasking / Events: resolve one
// caller's effective board role without touching Boards private storage or
// HTTP handlers. Both WorkspaceBoardStore implementations satisfy it
// (asserted below).
type RoleReader interface {
	GetBoardRole(ctx context.Context, workspacePublicID, boardPublicID string, userID int64, workspaceRole string) (AccessRole, bool, error)
}

// Compile-time port convergence: both stores serve the narrow readers.
var (
	_ RoleReader          = (*InMemoryWorkspaceBoardStore)(nil)
	_ RoleReader          = (*PostgresWorkspaceBoardStore)(nil)
	_ BoardMediaRefReader = (*InMemoryWorkspaceBoardStore)(nil)
	_ BoardMediaRefReader = (*PostgresWorkspaceBoardStore)(nil)
)

// BoardMediaRefReader is the narrow port for Media authorization (#936):
// list the workspace-scoped boards whose accepted document references one
// media asset. Public ids only — no snapshots, no storage internals, no
// HTTP. Media resolves each board through RoleReader and only boards the
// caller can read authorize the asset. Both WorkspaceBoardStore
// implementations satisfy it (asserted below).
type BoardMediaRefReader interface {
	ListBoardsReferencingAsset(ctx context.Context, workspacePublicID, assetPublicID string) ([]string, error)
}

// Capabilities is the caller-visible operation set for one board: derived
// from the effective role plus archive state, never stored. can_generate
// additionally requires the existing Task/billing authorization at submit
// time and never expands it; archived boards report no write, generate, or
// management affordances (restore stays on the CapWrite gate).
type Capabilities struct {
	CanRead     bool
	CanWrite    bool
	CanManage   bool
	CanGenerate bool
}

// IsWorkspaceMemberRole reports whether a Workspace role permits Board access.
// Unknown or empty roles fail closed, including for the Board creator.
func IsWorkspaceMemberRole(workspaceRole string) bool {
	switch strings.TrimSpace(workspaceRole) {
	case "owner", "admin", "member":
		return true
	default:
		return false
	}
}

// ValidateWorkspaceAccess reports whether a workspace_access fact is known.
// Unknown values never authorize.
func ValidateWorkspaceAccess(access WorkspaceAccess) bool {
	switch access {
	case WorkspaceAccessNone, WorkspaceAccessViewer, WorkspaceAccessEditor:
		return true
	default:
		return false
	}
}

// ResolveRole applies the common role only to selected current Workspace members.
// The creator retains the implicit Owner role while their membership is current.
func ResolveRole(ownerUserID, callerUserID int64, selected bool, access WorkspaceAccess, workspaceMember bool) (AccessRole, bool) {
	if ownerUserID <= 0 || callerUserID <= 0 || !workspaceMember {
		return "", false
	}
	if callerUserID == ownerUserID {
		return RoleOwner, true
	}
	if !selected {
		return "", false
	}
	switch access {
	case WorkspaceAccessViewer:
		return RoleViewer, true
	case WorkspaceAccessEditor:
		return RoleEditor, true
	default:
		return "", false
	}
}

// CheckCapability maps an (role, ok) visibility fact to the operation
// outcome: invisible callers read as ErrNotFound (handlers render 404 and
// never leak existence), visible callers lacking the capability read as
// ErrPermissionDenied (handlers render 403). Pure function.
func CheckCapability(role AccessRole, ok bool, cap Capability) error {
	if !ok {
		return ErrNotFound
	}
	switch cap {
	case CapRead:
		return nil
	case CapWrite:
		if role == RoleOwner || role == RoleEditor {
			return nil
		}
		return ErrPermissionDenied
	case CapManage:
		if role == RoleOwner {
			return nil
		}
		return ErrPermissionDenied
	default:
		return ErrPermissionDenied
	}
}

// CapabilitiesFor derives the caller-visible operation set from the
// effective role fact plus archive state. Invisible callers get no
// capabilities; archived boards keep read only (restore itself stays on
// the CapWrite gate so owners/editors can still revert). Only the creator
// manages sharing. Pure function.
func CapabilitiesFor(role AccessRole, ok, archived bool) Capabilities {
	if !ok {
		return Capabilities{}
	}
	caps := Capabilities{CanRead: true}
	if archived {
		return caps
	}
	switch role {
	case RoleOwner:
		caps.CanWrite = true
		caps.CanManage = true
		caps.CanGenerate = true
	case RoleEditor:
		caps.CanWrite = true
		caps.CanGenerate = true
	default:
		// Viewers (and any future restricted role) stay read-only.
	}
	return caps
}

// DeriveAccessState reports actual sharing with at least one selected member.
func DeriveAccessState(access WorkspaceAccess, hasSelectedMembers bool) AccessState {
	if hasSelectedMembers && (access == WorkspaceAccessViewer || access == WorkspaceAccessEditor) {
		return AccessStateWorkspace
	}
	return AccessStatePrivate
}

// FormatAccessETag renders the access version as the access-resource ETag
// validator value (access-<version>), disjoint from the document revision
// ETag namespace.
func FormatAccessETag(version int64) string {
	return fmt.Sprintf("access-%d", version)
}

// ParseAccessIfMatch parses an If-Match header carrying an access ETag
// (access-<version>). Weak validators (W/), quotes, and surrounding spaces
// are tolerated; "*", empty, non-access, and non-positive values report
// ErrInvalidInput so callers render 400.
func ParseAccessIfMatch(raw string) (int64, error) {
	trimmed := strings.TrimSpace(raw)
	trimmed = strings.TrimPrefix(trimmed, "W/")
	trimmed = strings.Trim(trimmed, `"`)
	trimmed = strings.TrimSpace(trimmed)
	if trimmed == "" || trimmed == "*" {
		return 0, ErrInvalidInput
	}
	versionText, found := strings.CutPrefix(trimmed, "access-")
	if !found {
		return 0, ErrInvalidInput
	}
	var version int64
	if _, err := fmt.Sscanf(strings.TrimSpace(versionText), "%d", &version); err != nil || version <= 0 {
		return 0, ErrInvalidInput
	}
	return version, nil
}
