package boards

import (
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
const (
	codeBoardInvalidInput       = "board_invalid_input"
	codeBoardNotFound           = "board_not_found"
	codeBoardNameConflict       = "board_name_conflict"
	codeBoardRevisionConflict   = "board_revision_conflict"
	codeBoardIDConflict         = "board_id_conflict"
	codeBoardPermissionDenied   = "board_permission_denied"
	codeBoardMemberNotFound     = "board_member_not_found"
	codeBoardMemberRoleInvalid  = "board_member_role_invalid"
	codeBoardMemberSelfInvalid  = "board_member_self_invalid"
	codeBoardShareTokenNotFound = "board_share_token_not_found"
	codeBoardShareTokenRevoked  = "board_share_token_revoked"
	codeBoardShareTokenExpired  = "board_share_token_expired"
	codeBoardShareTokenInvalid  = "board_share_token_invalid"
	codeBoardOperationFailed    = "board_operation_failed"
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
	boardErrInvalidInput     = boardErrorMapping{ErrInvalidInput, 400, codeBoardInvalidInput, "参数错误", false}
	boardErrNotFound         = boardErrorMapping{ErrNotFound, 404, codeBoardNotFound, "画布不存在", false}
	boardErrNameConflict     = boardErrorMapping{ErrNameConflict, 409, codeBoardNameConflict, "画布名称已存在，请使用其他名称", false}
	boardErrRevisionConflict = boardErrorMapping{ErrRevisionConflict, 409, codeBoardRevisionConflict, "画布版本已变更，请刷新后重试", false}
	boardErrIDConflict       = boardErrorMapping{ErrIDConflict, 409, codeBoardIDConflict, "画布ID已存在，请刷新后重试", false}

	boardErrModifyForbidden = boardErrorMapping{ErrPermissionDenied, 403, codeBoardPermissionDenied, "无权修改该画布", false}
	boardErrDeleteForbidden = boardErrorMapping{ErrPermissionDenied, 403, codeBoardPermissionDenied, "无权删除该画布", false}
	boardErrMemberForbidden = boardErrorMapping{ErrPermissionDenied, 403, codeBoardPermissionDenied, "仅 owner 可管理成员", false}
	boardErrShareForbidden  = boardErrorMapping{ErrPermissionDenied, 403, codeBoardPermissionDenied, "仅 owner 可管理分享链接", false}

	boardErrCanvasCleanupFailed = boardErrorMapping{ErrCanvasCleanupFailed, 500, codeBoardOperationFailed, "删除画布失败：文件清理失败", true}

	boardErrMemberNotFound    = boardErrorMapping{ErrMemberNotFound, 404, codeBoardMemberNotFound, "成员不存在", false}
	boardErrMemberInvalidRole = boardErrorMapping{ErrMemberInvalidRole, 400, codeBoardMemberRoleInvalid, "角色无效，仅支持 editor/viewer", false}
	boardErrMemberInviteSelf  = boardErrorMapping{ErrMemberInviteSelf, 400, codeBoardMemberSelfInvalid, "不能邀请或移除 owner 自己", false}

	boardErrShareTokenNotFound = boardErrorMapping{ErrShareTokenNotFound, 404, codeBoardShareTokenNotFound, "分享链接不存在", false}
	boardErrShareTokenRevoked  = boardErrorMapping{ErrShareTokenRevoked, 400, codeBoardShareTokenRevoked, "分享链接已撤销", false}
	boardErrShareTokenExpired  = boardErrorMapping{ErrShareTokenExpired, 400, codeBoardShareTokenExpired, "分享链接已过期", false}
	boardErrShareTokenInvalid  = boardErrorMapping{ErrShareTokenInvalid, 400, codeBoardShareTokenInvalid, "分享链接无效", false}
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
		message = "参数错误"
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
