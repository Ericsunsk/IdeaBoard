package boards

import (
	"errors"

	"ideaboard/api/internal/platform/response"

	"github.com/gin-gonic/gin"
)

// boardErrorMapping binds one stable outcome to its wire rendering. The row
// variables below are the single source of truth for boards HTTP error
// rendering; handlers compose per-endpoint row lists instead of repeating
// errors.Is ladders. Messages stay endpoint-specific where the old code
// distinguished them (e.g. permission denied).
type boardErrorMapping struct {
	target   error
	status   int
	message  string
	internal bool
}

var (
	boardErrInvalidInput     = boardErrorMapping{ErrInvalidInput, 400, "参数错误", false}
	boardErrNotFound         = boardErrorMapping{ErrNotFound, 404, "画布不存在", false}
	boardErrNameConflict     = boardErrorMapping{ErrNameConflict, 409, "画布名称已存在，请使用其他名称", false}
	boardErrRevisionConflict = boardErrorMapping{ErrRevisionConflict, 409, "画布版本已变更，请刷新后重试", false}
	boardErrIDConflict       = boardErrorMapping{ErrIDConflict, 409, "画布ID已存在，请刷新后重试", false}

	boardErrModifyForbidden = boardErrorMapping{ErrPermissionDenied, 403, "无权修改该画布", false}
	boardErrDeleteForbidden = boardErrorMapping{ErrPermissionDenied, 403, "无权删除该画布", false}
	boardErrMemberForbidden = boardErrorMapping{ErrPermissionDenied, 403, "仅 owner 可管理成员", false}
	boardErrShareForbidden  = boardErrorMapping{ErrPermissionDenied, 403, "仅 owner 可管理分享链接", false}

	boardErrCanvasCleanupFailed = boardErrorMapping{ErrCanvasCleanupFailed, 500, "删除画布失败：文件清理失败", true}

	boardErrMemberNotFound    = boardErrorMapping{ErrMemberNotFound, 404, "成员不存在", false}
	boardErrMemberInvalidRole = boardErrorMapping{ErrMemberInvalidRole, 400, "角色无效，仅支持 editor/viewer", false}
	boardErrMemberInviteSelf  = boardErrorMapping{ErrMemberInviteSelf, 400, "不能邀请或移除 owner 自己", false}

	boardErrShareTokenNotFound = boardErrorMapping{ErrShareTokenNotFound, 404, "分享链接不存在", false}
	boardErrShareTokenRevoked  = boardErrorMapping{ErrShareTokenRevoked, 400, "分享链接已撤销", false}
	boardErrShareTokenExpired  = boardErrorMapping{ErrShareTokenExpired, 400, "分享链接已过期", false}
	boardErrShareTokenInvalid  = boardErrorMapping{ErrShareTokenInvalid, 400, "分享链接无效", false}
)

func respondBoardError(c *gin.Context, err error, rows []boardErrorMapping, internalMessage string) {
	for _, m := range rows {
		if errors.Is(err, m.target) {
			if m.internal {
				response.InternalError(c, m.status, m.message, err)
				return
			}
			response.Error(c, m.status, m.message)
			return
		}
	}
	response.InternalError(c, 500, internalMessage, err)
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
