package domain

import (
	"strings"
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
	if !isBoardID(trimmed) {
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

// BoardOperation is the task-facing board operation unified across the
// tasking submit/read/cancel gates and the media submit gate. One operation
// vocabulary keeps every call site on the same fail-closed semantics.
//
// The gate operates on resolved capability booleans only, never on board
// role strings: the role mapping lives exactly once in Boards (ResolveRole)
// and reaches here as facts through the ResolveBoardAccess seam, so this
// file copies no role rules (see TestBoardRoleRulesStayUncopiedOutsideBoards).
type BoardOperation string

const (
	// BoardOpSubmit gates task submission (generate capability on a live board).
	BoardOpSubmit BoardOperation = "submit"
	// BoardOpRead gates task content reads (any visible role, archived included).
	BoardOpRead BoardOperation = "read"
	// BoardOpCancel gates task cancellation by non-submitters (writer on a live board).
	BoardOpCancel BoardOperation = "cancel"
)

// BoardTaskFacts is the minimal capability snapshot for task-facing gates.
// The zero value denies every operation.
type BoardTaskFacts struct {
	CanRead     bool
	CanGenerate bool
	CanWrite    bool
	Archived    bool
}

// BoardDecision is the deny reason for task-facing gates. Submit maps each
// reason to its own sentinel (not-found / archived / forbidden); read and
// cancel converge every denial to invisible. Pure value, no new error
// registry: callers map to their existing domain sentinels.
type BoardDecision string

const (
	// BoardDecisionAllow authorizes the operation.
	BoardDecisionAllow BoardDecision = "allow"
	// BoardDecisionNotFound denies an invisible caller or a caller without
	// read (submit/read). Handlers render 404 and never leak existence.
	BoardDecisionNotFound BoardDecision = "not_found"
	// BoardDecisionArchived denies generation and writer cancellation on an
	// archived board (submit/cancel). Reads stay allowed.
	BoardDecisionArchived BoardDecision = "archived"
	// BoardDecisionForbidden denies a visible live caller lacking the
	// operation capability (submit without generate, cancel without write).
	BoardDecisionForbidden BoardDecision = "forbidden"
)

type boardPolicy struct{}

// BoardPolicy is the single task-facing board gate (P0-2). Tasking
// (submit/read/cancel) and media (submit) delegate to BoardPolicy.Check;
// the per-call-site error/bool mapping preserves each gate's exact
// fail-closed semantics. Pure function.
var BoardPolicy = boardPolicy{}

// Check applies the unified gate: submit needs read on a live board plus
// generate; read needs read (archived keeps reads); cancel needs write on
// a live board. Unknown operations deny as not-found. Pure function.
func (boardPolicy) Check(facts BoardTaskFacts, op BoardOperation) BoardDecision {
	switch op {
	case BoardOpSubmit:
		if !facts.CanRead {
			return BoardDecisionNotFound
		}
		if facts.Archived {
			return BoardDecisionArchived
		}
		if !facts.CanGenerate {
			return BoardDecisionForbidden
		}
		return BoardDecisionAllow
	case BoardOpRead:
		if !facts.CanRead {
			return BoardDecisionNotFound
		}
		return BoardDecisionAllow
	case BoardOpCancel:
		if facts.Archived {
			return BoardDecisionArchived
		}
		if !facts.CanWrite {
			return BoardDecisionForbidden
		}
		return BoardDecisionAllow
	default:
		return BoardDecisionNotFound
	}
}

// BoardTaskFactsFromAccess projects one resolved posture onto the shared board gate.
// A nil posture projects to the zero facts, which deny every operation.
func BoardTaskFactsFromAccess(access *BoardAccess) BoardTaskFacts {
	if access == nil {
		return BoardTaskFacts{}
	}
	return BoardTaskFacts{
		CanRead: access.CanRead, CanGenerate: access.CanGenerate,
		CanWrite: access.CanWrite, Archived: access.Archived,
	}
}

// CheckBoardSubmit maps the shared submit decision onto the tasking submit
// sentinels: invisible Boards read as ErrBoardNotFound; archived Boards
// read as ErrBoardArchived; visible Boards without generate capability read
// as ErrForbidden.
func CheckBoardSubmit(access *BoardAccess) error {
	switch BoardPolicy.Check(BoardTaskFactsFromAccess(access), BoardOpSubmit) {
	case BoardDecisionAllow:
		return nil
	case BoardDecisionArchived:
		return ErrBoardArchived
	case BoardDecisionForbidden:
		return ErrForbidden
	default:
		return ErrBoardNotFound
	}
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
	return BoardPolicy.Check(BoardTaskFactsFromAccess(access), BoardOpRead) == BoardDecisionAllow
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
	return BoardPolicy.Check(BoardTaskFactsFromAccess(access), BoardOpCancel) == BoardDecisionAllow
}
