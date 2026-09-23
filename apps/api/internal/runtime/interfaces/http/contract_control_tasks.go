package httpapi

// Control tasks family (issue #808): scoped task read/ops behind
// control:tasks — get/list/cancel with operator reason.
//
// There is deliberately NO task creation path here: the submit surface
// stays on the Public Data Plane (#805) and legacy admin submit stays
// untouched until #809. Cancel reuses the task command seam with the
// operator as actor. Legacy /api/admin/tasks/* routes are untouched.

import (
	"net/http"
	"strings"

	"ideaboard/api/internal/platform/contract"
	"ideaboard/api/internal/platform/validation"
	taskapp "ideaboard/api/internal/tasking/application/task"
	tasksdomain "ideaboard/api/internal/tasking/domain"

	"github.com/gin-gonic/gin"
)

// controlTaskListCursor is the opaque cursor over the legacy page dialect.
type controlTaskListCursor struct {
	Page int `json:"page"`
}

func (d *ContractControlDeps) listControlTasks(c *gin.Context, _ controlContext) {
	if d == nil || d.Tasks == nil {
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, "任务服务暂不可用")
		return
	}
	page := 1
	if raw := strings.TrimSpace(c.Query("cursor")); raw != "" {
		decoded, err := contract.DecodeCursor[controlTaskListCursor](raw)
		if err != nil || decoded == nil || decoded.Page < 1 {
			contract.RenderError(c, http.StatusBadRequest, contract.CodePaginationCursorInvalid, "分页游标无效")
			return
		}
		page = decoded.Page
	}
	pageSize := contract.DefaultPageSize
	if raw := strings.TrimSpace(c.Query("page_size")); raw != "" {
		parsed, err := contract.ParsePageSize(raw)
		if err != nil {
			contract.RenderError(c, http.StatusBadRequest, contract.CodeBadRequest, "page_size 无效")
			return
		}
		pageSize = parsed
	}
	req := tasksdomain.TaskListRequest{
		Status:   strings.TrimSpace(c.Query("status")),
		Keyword:  strings.TrimSpace(c.Query("keyword")),
		Page:     page,
		PageSize: pageSize,
	}
	if raw := strings.TrimSpace(c.Query("user_id")); raw != "" {
		userID, err := d.Tasks.ResolveAdminTaskUserID(c.Request.Context(), raw)
		if err != nil {
			if validation.Is(err) {
				contract.RenderError(c, http.StatusBadRequest, contract.CodeBadRequest, "user_id 无效")
			} else {
				contract.RenderError(c, http.StatusInternalServerError, contract.CodeServiceUnavailable, "解析用户失败")
			}
			return
		}
		req.UserID = userID
	}
	items, counts, err := d.Tasks.ListTasks(c.Request.Context(), &req)
	if err != nil {
		if validation.Is(err) {
			contract.RenderError(c, http.StatusBadRequest, contract.CodeBadRequest, "参数错误")
			return
		}
		contract.RenderError(c, http.StatusInternalServerError, contract.CodeServiceUnavailable, "获取任务列表失败")
		return
	}
	if items == nil {
		items = []taskapp.AdminMonitorListItem{}
	}
	// counts.Total is the list fact source for the cursor meta; the
	// per-status breakdown stays inside the items payload owner (the
	// tasking read model) and is not re-projected here.
	hasMore := int64(page*pageSize) < counts.Total
	var nextCursor string
	if hasMore {
		encoded, err := contract.EncodeCursor(controlTaskListCursor{Page: page + 1})
		if err != nil {
			contract.RenderError(c, http.StatusInternalServerError, contract.CodeServiceUnavailable, "获取任务列表失败")
			return
		}
		nextCursor = encoded
	}
	contract.RenderPaginated(c, items, contract.NewPageMeta(nextCursor, hasMore))
}

func (d *ContractControlDeps) getControlTask(c *gin.Context, _ controlContext) {
	if d == nil || d.Tasks == nil {
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, "任务服务暂不可用")
		return
	}
	taskID := strings.TrimSpace(c.Param("task_id"))
	if taskID == "" || !contract.IsTaskID(taskID) {
		contract.RenderError(c, http.StatusNotFound, contract.CodeControlNotFound, "任务不存在")
		return
	}
	task, err := d.Tasks.GetTask(c.Request.Context(), taskID)
	if err != nil {
		contract.RenderError(c, http.StatusNotFound, contract.CodeControlNotFound, "任务不存在")
		return
	}
	contract.RenderSuccess(c, struct {
		Task any `json:"task"`
	}{Task: task})
}

func (d *ContractControlDeps) cancelControlTask(c *gin.Context, ctx controlContext) {
	if d == nil || d.Tasks == nil {
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, "任务服务暂不可用")
		return
	}
	taskID := strings.TrimSpace(c.Param("task_id"))
	if taskID == "" || !contract.IsTaskID(taskID) {
		contract.RenderError(c, http.StatusNotFound, contract.CodeControlNotFound, "任务不存在")
		return
	}
	if err := d.Tasks.CancelAdminTask(c.Request.Context(), taskID, ctx.Operator.UserID()); err != nil {
		contract.RenderError(c, http.StatusInternalServerError, contract.CodeServiceUnavailable, "取消失败")
		return
	}
	contract.RenderSuccess(c, struct {
		Cancelled bool `json:"cancelled"`
	}{Cancelled: true})
}
