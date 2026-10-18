package workspacetasks

import (
	"context"
	"fmt"
	"strings"

	"ideaboard/api/internal/platform/contract"
)

// Board binding and task visibility (issue #937).
//
// A Canvas Task carries the Board it was submitted from as a durable fact
// (BoardID + the Board access_version observed at submit). The binding
// persists inside the task row (workflow_input envelope) in the SAME
// settlement transaction as the task insert, Credit Hold, and River
// enqueue, so it can never diverge from the settled task. Non-Canvas tasks
// omit the binding and keep their own workspace-level permission: omitting
// the Board ID never degrades into workspace-wide visibility.
//
// Visibility rules:
//   - the submitter always sees their own tasks;
//   - otherwise the caller must currently hold Board read capability on the
//     linked Board (resolved live, so revocation takes effect on read);
//   - Board-less tasks are submitter-only.
//
// Prompts and params stay write-only (they never enter the v1 DTO), so the
// visibility gate above plus the envelope split is what keeps restricted
// content out of workspace-global lists, search, statistics, and billing
// detail: those surfaces only ever see billing facts.

// TaskBoardBinding is the durable Board linkage snapshot persisted with a
// Task. AccessVersion records the Board authorization version accepted at submit.
type TaskBoardBinding struct {
	BoardID       string `json:"board_id"`
	AccessVersion int64  `json:"access_version"`
}

// ValidateBoardID reports whether a Board binding value is a well-formed
// opaque board_ identifier. Empty means "no binding" (non-Canvas task) and
// is valid; a non-empty malformed value is a caller bug.
func ValidateBoardID(boardID string) error {
	trimmed := strings.TrimSpace(boardID)
	if trimmed == "" {
		return nil
	}
	if !contract.IsValidPublicID(contract.PrefixBoard, trimmed) {
		return newValidationError(FieldError{Field: "board_id", Message: "invalid"})
	}
	return nil
}

// BoardAccess is the resolved Board posture for one caller at one moment.
// It is a point-in-time fact: submit snapshots the version, completion and
// reads re-resolve.
type BoardAccess struct {
	BoardID string
	// IsOwner reports Board creator standing. Only owners may publish restricted
	// material to the Workspace Library.
	IsOwner bool
	// CanRead gates Task content reads; CanGenerate gates Task submission;
	// CanWrite gates Task cancellation by non-submitters (owner/editor on
	// a live Board).
	CanRead     bool
	CanGenerate bool
	CanWrite    bool
	// Archived boards keep reads but reject generation and publishing.
	Archived bool
	// AccessVersion is the monotonic Board access version backing the
	// access-<version> ETag.
	AccessVersion int64
}

// BoardAuthorizer resolves one caller's Board posture without touching
// Boards storage internals or HTTP handlers. Production adapts the Boards
// WorkspaceBoardStore; tests stub this interface. A nil authorizer fails
// closed: Board-bound submits are rejected and Board-bound reads are
// invisible.
type BoardAuthorizer interface {
	ResolveBoardAccess(ctx context.Context, workspacePublicID, boardPublicID string, userID int64, workspaceRole string) (*BoardAccess, error)
}

// AuthorizeTaskRead reports whether a caller may read one Task (detail,
// outputs, or list membership). The submitter always passes; otherwise the
// linked Board must currently grant read. Board-less tasks are
// submitter-only, so omitting the Board ID never widens visibility.
func AuthorizeTaskRead(task *WorkspaceTask, callerUserID int64, access *BoardAccess) bool {
	if task == nil || callerUserID <= 0 {
		return false
	}
	if task.OwnerUserID == callerUserID {
		return true
	}
	if strings.TrimSpace(task.BoardID) == "" {
		return false
	}
	if access == nil {
		return false
	}
	if strings.TrimSpace(access.BoardID) != strings.TrimSpace(task.BoardID) {
		return false
	}
	return access.CanRead
}

// resolveSubmitBoardAccess resolves the caller's live Board posture for a
// Board-bound submit. Invisible Boards (unknown id or no effective role)
// read as ErrBoardNotFound; archived Boards read as ErrBoardArchived;
// visible Boards without generate capability read as ErrForbidden. A nil
// authorizer fails closed: bound submits are rejected rather than
// degrading into unbound tasks.
func (s *Service) resolveSubmitBoardAccess(ctx context.Context, workspaceID, boardID string, userID int64, workspaceRole string) (*BoardAccess, error) {
	if s.boards == nil {
		return nil, fmt.Errorf("workspacetasks: board authorizer unavailable: %w", ErrSettlementUnavailable)
	}
	access, err := s.boards.ResolveBoardAccess(ctx, strings.TrimSpace(workspaceID), strings.TrimSpace(boardID), userID, workspaceRole)
	if err != nil {
		return nil, err
	}
	if access == nil || !access.CanRead {
		return nil, ErrBoardNotFound
	}
	if access.Archived {
		return nil, ErrBoardArchived
	}
	if !access.CanGenerate {
		return nil, ErrForbidden
	}
	return access, nil
}

// AuthorizeTaskCancel reports whether a caller may cancel one Task: the
// submitter, or a Board writer on the linked Board (owner/editor on a live
// Board). Readers and outsiders cannot cancel.
func AuthorizeTaskCancel(task *WorkspaceTask, callerUserID int64, access *BoardAccess) bool {
	if task == nil || callerUserID <= 0 {
		return false
	}
	if task.OwnerUserID == callerUserID {
		return true
	}
	if strings.TrimSpace(task.BoardID) == "" {
		return false
	}
	if access == nil || access.Archived {
		return false
	}
	return access.CanWrite
}
