package boards

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestBoardErrorRowsRenderLockedMapping(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name     string
		rows     []boardErrorMapping
		fallback string
		err      error
		status   int
		code     string
		msg      string
	}{
		{name: "upsert invalid", rows: upsertBoardErrorRows, fallback: "更新画布失败", err: ErrInvalidInput, status: 400, code: "board_invalid_input", msg: "参数错误"},
		{name: "upsert forbidden", rows: upsertBoardErrorRows, fallback: "更新画布失败", err: ErrPermissionDenied, status: 403, code: "board_permission_denied", msg: "无权修改该画布"},
		{name: "upsert name conflict", rows: upsertBoardErrorRows, fallback: "更新画布失败", err: ErrNameConflict, status: 409, code: "board_name_conflict", msg: "画布名称已存在，请使用其他名称"},
		{name: "upsert revision conflict", rows: upsertBoardErrorRows, fallback: "更新画布失败", err: ErrRevisionConflict, status: 409, code: "board_revision_conflict", msg: "画布版本已变更，请刷新后重试"},
		{name: "upsert id conflict", rows: upsertBoardErrorRows, fallback: "更新画布失败", err: ErrIDConflict, status: 409, code: "board_id_conflict", msg: "画布ID已存在，请刷新后重试"},
		{name: "upsert unknown", rows: upsertBoardErrorRows, fallback: "更新画布失败", err: errors.New("boom"), status: 500, code: "board_operation_failed", msg: "更新画布失败"},
		{name: "delete not found", rows: deleteBoardErrorRows, fallback: "删除画布失败", err: ErrNotFound, status: 404, code: "board_not_found", msg: "画布不存在"},
		{name: "delete forbidden", rows: deleteBoardErrorRows, fallback: "删除画布失败", err: ErrPermissionDenied, status: 403, code: "board_permission_denied", msg: "无权删除该画布"},
		{name: "delete cleanup failed", rows: deleteBoardErrorRows, fallback: "删除画布失败", err: ErrCanvasCleanupFailed, status: 500, code: "board_operation_failed", msg: "删除画布失败：文件清理失败"},
		{name: "delete unknown", rows: deleteBoardErrorRows, fallback: "删除画布失败", err: errors.New("boom"), status: 500, code: "board_operation_failed", msg: "删除画布失败"},
		{name: "member forbidden", rows: boardMemberErrorRows, fallback: "获取成员列表失败", err: ErrPermissionDenied, status: 403, code: "board_permission_denied", msg: "仅 owner 可管理成员"},
		{name: "member not found", rows: boardMemberErrorRows, fallback: "获取成员列表失败", err: ErrMemberNotFound, status: 404, code: "board_member_not_found", msg: "成员不存在"},
		{name: "member invalid role", rows: boardMemberErrorRows, fallback: "获取成员列表失败", err: ErrMemberInvalidRole, status: 400, code: "board_member_role_invalid", msg: "角色无效，仅支持 editor/viewer"},
		{name: "member invite self", rows: boardMemberErrorRows, fallback: "获取成员列表失败", err: ErrMemberInviteSelf, status: 400, code: "board_member_self_invalid", msg: "不能邀请或移除 owner 自己"},
		{name: "member unknown", rows: boardMemberErrorRows, fallback: "获取成员列表失败", err: errors.New("boom"), status: 500, code: "board_operation_failed", msg: "获取成员列表失败"},
		{name: "share forbidden", rows: shareTokenErrorRows, fallback: "加入画布失败", err: ErrPermissionDenied, status: 403, code: "board_permission_denied", msg: "仅 owner 可管理分享链接"},
		{name: "share token not found", rows: shareTokenErrorRows, fallback: "加入画布失败", err: ErrShareTokenNotFound, status: 404, code: "board_share_token_not_found", msg: "分享链接不存在"},
		{name: "share token revoked", rows: shareTokenErrorRows, fallback: "加入画布失败", err: ErrShareTokenRevoked, status: 400, code: "board_share_token_revoked", msg: "分享链接已撤销"},
		{name: "share token expired", rows: shareTokenErrorRows, fallback: "加入画布失败", err: ErrShareTokenExpired, status: 400, code: "board_share_token_expired", msg: "分享链接已过期"},
		{name: "share token invalid", rows: shareTokenErrorRows, fallback: "加入画布失败", err: ErrShareTokenInvalid, status: 400, code: "board_share_token_invalid", msg: "分享链接无效"},
		{name: "share unknown", rows: shareTokenErrorRows, fallback: "加入画布失败", err: errors.New("boom"), status: 500, code: "board_operation_failed", msg: "加入画布失败"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			router := gin.New()
			router.GET("/err", func(c *gin.Context) { respondBoardError(c, test.err, test.rows, test.fallback) })
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/err", nil))

			if recorder.Code != test.status {
				t.Fatalf("status = %d, want %d", recorder.Code, test.status)
			}
			var body struct {
				Code string `json:"code"`
				Msg  string `json:"msg"`
			}
			if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if body.Code != test.code {
				t.Fatalf("code = %q, want %q", body.Code, test.code)
			}
			if body.Msg != test.msg {
				t.Fatalf("msg = %q, want %q", body.Msg, test.msg)
			}
		})
	}
}

func TestWriteBoardInputErrorRendersMachineCode(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/err", func(c *gin.Context) { writeBoardInputError(c, "缺少画布ID") })
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/err", nil))

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", recorder.Code)
	}
	var body struct {
		Code string `json:"code"`
		Msg  string `json:"msg"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Code != "board_invalid_input" || body.Msg != "缺少画布ID" {
		t.Fatalf("body = %+v, want code=board_invalid_input msg=缺少画布ID", body)
	}
}
