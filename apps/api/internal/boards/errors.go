package boards

import (
	"ideaboard/api/internal/platform/contract"
	"ideaboard/api/internal/platform/errorrender"

	"github.com/gin-gonic/gin"
)

// Board error codes are wire-stable. Do not rename a code without
// coordinating the frontend caller that branches on it. Messages stay
// intentionally safe for public display; the permission-denied rows keep
// endpoint-specific copy (the old behavior) while sharing one stable code.
//
// Classification lives here at the HTTP edge so boards domain sentinels stay
// dependency-free. The row variables below are the single source of truth
// for boards HTTP error rendering; handlers compose per-endpoint row lists
// instead of repeating errors.Is ladders. Every boards failure renders
// through response.MachineError, never through the numeric response.Error
// envelope. 5xx outcomes are logged server-side (InternalError semantics)
// so they stay attributable; 4xx outcomes render without logging.
//
// Idempotency outcomes are intentionally absent here: board POST writes use
// the platform idempotency codes (idempotency_key_required /
// idempotency_conflict), never a boards-owned code.
// The values below alias the kernel registry (platform/contract): the
// registry is the single source of truth, these names keep the edge
// diffs trivial. codeBoardNotFound, codeBoardRevisionConflict,
// codeBoardMemberNotFound, codeBoardShareTokenNotFound, and
// codeBoardShareTokenInvalid alias the #802 kernel codes shared with the
// v1 workspace board surface; the rest are canvas-edge-only outcomes
// promoted into the same registry. Wire values are unchanged.
const (
	codeBoardInvalidInput       = string(contract.CodeBoardInvalidInput)
	codeBoardNotFound           = string(contract.CodeBoardNotFound)
	codeBoardNameConflict       = string(contract.CodeBoardNameConflict)
	codeBoardRevisionConflict   = string(contract.CodeBoardRevisionConflict)
	codeBoardIDConflict         = string(contract.CodeBoardIDConflict)
	codeBoardPermissionDenied   = string(contract.CodeBoardPermissionDenied)
	codeBoardMemberNotFound     = string(contract.CodeBoardMemberNotFound)
	codeBoardMemberRoleInvalid  = string(contract.CodeBoardMemberRoleInvalid)
	codeBoardMemberSelfInvalid  = string(contract.CodeBoardMemberSelfInvalid)
	codeBoardShareTokenNotFound = string(contract.CodeBoardShareTokenNotFound)
	codeBoardShareTokenRevoked  = string(contract.CodeBoardShareTokenRevoked)
	codeBoardShareTokenExpired  = string(contract.CodeBoardShareTokenExpired)
	codeBoardShareTokenInvalid  = string(contract.CodeBoardShareTokenInvalid)
	codeBoardOperationFailed    = string(contract.CodeBoardOperationFailed)
)

// boardErrorMapping binds one stable outcome to its wire rendering. The row
// variables below are the single source of truth for boards HTTP error
// rendering; handlers compose per-endpoint row lists instead of repeating
// errors.Is ladders. Messages stay endpoint-specific where the old code
// distinguished them (e.g. permission denied).
type boardErrorMapping struct {
	target   error
	status   int
	code     string
	message  string
	internal bool
}

var (
	boardErrInvalidInput     = boardErrorMapping{ErrInvalidInput, 400, codeBoardInvalidInput, msgBoardInvalidInput, false}
	boardErrNotFound         = boardErrorMapping{ErrNotFound, 404, codeBoardNotFound, msgBoardNotFound, false}
	boardErrNameConflict     = boardErrorMapping{ErrNameConflict, 409, codeBoardNameConflict, msgBoardNameConflict, false}
	boardErrRevisionConflict = boardErrorMapping{ErrRevisionConflict, 409, codeBoardRevisionConflict, msgBoardRevisionConflict, false}
	boardErrIDConflict       = boardErrorMapping{ErrIDConflict, 409, codeBoardIDConflict, msgBoardIDConflict, false}

	boardErrModifyForbidden = boardErrorMapping{ErrPermissionDenied, 403, codeBoardPermissionDenied, msgBoardPermissionDenied, false}
	boardErrDeleteForbidden = boardErrorMapping{ErrPermissionDenied, 403, codeBoardPermissionDenied, msgBoardPermissionDenied2, false}
	boardErrMemberForbidden = boardErrorMapping{ErrPermissionDenied, 403, codeBoardPermissionDenied, msgBoardPermissionDenied3, false}
	boardErrShareForbidden  = boardErrorMapping{ErrPermissionDenied, 403, codeBoardPermissionDenied, msgBoardPermissionDenied4, false}

	boardErrCanvasCleanupFailed = boardErrorMapping{ErrCanvasCleanupFailed, 500, codeBoardOperationFailed, msgBoardOperationFailed, true}

	boardErrMemberNotFound    = boardErrorMapping{ErrMemberNotFound, 404, codeBoardMemberNotFound, msgBoardMemberNotFound, false}
	boardErrMemberInvalidRole = boardErrorMapping{ErrMemberInvalidRole, 400, codeBoardMemberRoleInvalid, msgBoardMemberRoleInvalid, false}
	boardErrMemberInviteSelf  = boardErrorMapping{ErrMemberInviteSelf, 400, codeBoardMemberSelfInvalid, msgBoardMemberSelfInvalid, false}

	boardErrShareTokenNotFound = boardErrorMapping{ErrShareTokenNotFound, 404, codeBoardShareTokenNotFound, msgBoardShareTokenNotFound, false}
	boardErrShareTokenRevoked  = boardErrorMapping{ErrShareTokenRevoked, 400, codeBoardShareTokenRevoked, msgBoardShareTokenRevoked, false}
	boardErrShareTokenExpired  = boardErrorMapping{ErrShareTokenExpired, 400, codeBoardShareTokenExpired, msgBoardShareTokenExpired, false}
	boardErrShareTokenInvalid  = boardErrorMapping{ErrShareTokenInvalid, 400, codeBoardShareTokenInvalid, msgBoardShareTokenInvalid, false}
)

// Matches reports errors.Is membership for this row's sentinel so the
// shared first-win core can order rows without repeating Is ladders.
func (m boardErrorMapping) Matches(err error) bool {
	return errorrender.SingleMatch(m.target, err)
}

func respondBoardError(c *gin.Context, err error, rows []boardErrorMapping, internalMessage string) {
	if row, ok := errorrender.FirstMatch(rows, err); ok {
		if row.internal {
			errorrender.RenderDefault(c, "board operation failed", row.status, row.code, row.message, err)
			return
		}
		errorrender.Render(c, row.status, row.code, row.message)
		return
	}
	errorrender.RenderDefault(c, "board operation failed", 500, codeBoardOperationFailed, internalMessage, err)
}

// writeBoardInputError renders request-shape failures (missing IDs,
// malformed JSON) through the machine envelope so callers branch on a stable
// code instead of copy.
func writeBoardInputError(c *gin.Context, message string) {
	if message == "" {
		message = msgBoardInvalidInput
	}
	errorrender.Render(c, 400, codeBoardInvalidInput, message)
}

// Per-endpoint row lists. Handlers reference these instead of inlining
// errors.Is ladders; errors_test.go renders every row to lock behavior.
var (
	upsertBoardErrorRows = []boardErrorMapping{
		boardErrInvalidInput,
		boardErrModifyForbidden,
		boardErrNameConflict,
		boardErrRevisionConflict,
		boardErrIDConflict,
	}
	deleteBoardErrorRows = []boardErrorMapping{
		boardErrInvalidInput,
		boardErrNotFound,
		boardErrDeleteForbidden,
		boardErrCanvasCleanupFailed,
	}
	boardMemberErrorRows = []boardErrorMapping{
		boardErrInvalidInput,
		boardErrNotFound,
		boardErrMemberForbidden,
		boardErrMemberNotFound,
		boardErrMemberInvalidRole,
		boardErrMemberInviteSelf,
	}
	shareTokenErrorRows = []boardErrorMapping{
		boardErrInvalidInput,
		boardErrNotFound,
		boardErrShareForbidden,
		boardErrMemberInvalidRole,
		boardErrShareTokenNotFound,
		boardErrShareTokenRevoked,
		boardErrShareTokenExpired,
		boardErrShareTokenInvalid,
	}
)

// Backend-owned user-facing copy for this package. Each constant
// is the single source of truth for one rendered message; call
// sites reference these names so copy changes stay in one place.
// Values are byte-identical to the previously inline literals.
const (
	msgBoardInvalidInput        = "参数错误"                   // codeBoardInvalidInput
	msgBoardNotFound            = "画布不存在"                  // codeBoardNotFound
	msgBoardNameConflict        = "画布名称已存在，请使用其他名称"        // codeBoardNameConflict
	msgBoardRevisionConflict    = "画布版本已变更，请刷新后重试"         // codeBoardRevisionConflict
	msgBoardIDConflict          = "画布ID已存在，请刷新后重试"         // codeBoardIDConflict
	msgBoardPermissionDenied    = "无权修改该画布"                // codeBoardPermissionDenied
	msgBoardPermissionDenied2   = "无权删除该画布"                // codeBoardPermissionDenied
	msgBoardPermissionDenied3   = "仅 owner 可管理成员"          // codeBoardPermissionDenied
	msgBoardPermissionDenied4   = "仅 owner 可管理分享链接"        // codeBoardPermissionDenied
	msgBoardOperationFailed     = "删除画布失败：文件清理失败"          // codeBoardOperationFailed
	msgBoardMemberNotFound      = "成员不存在"                  // codeBoardMemberNotFound
	msgBoardMemberRoleInvalid   = "角色无效，仅支持 editor/viewer" // codeBoardMemberRoleInvalid
	msgBoardMemberSelfInvalid   = "不能邀请或移除 owner 自己"       // codeBoardMemberSelfInvalid
	msgBoardShareTokenNotFound  = "分享链接不存在"                // codeBoardShareTokenNotFound
	msgBoardShareTokenRevoked   = "分享链接已撤销"                // codeBoardShareTokenRevoked
	msgBoardShareTokenExpired   = "分享链接已过期"                // codeBoardShareTokenExpired
	msgBoardShareTokenInvalid   = "分享链接无效"                 // codeBoardShareTokenInvalid
	msgGenerateBoardShareToken  = "生成分享令牌失败: %w"           // GenerateBoardShareToken @ service.go
	msgGenerateBoardShareToken2 = "生成分享令牌失败"               // GenerateBoardShareToken @ service.go
)
