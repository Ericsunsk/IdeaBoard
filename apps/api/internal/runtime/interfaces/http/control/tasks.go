package control

// Control tasks family: scoped task read/ops behind
// control:tasks — get/list/cancel.
//
// There is deliberately NO task creation path here: submission stays on
// the Public Data Plane. Cancel reuses the task command seam with the
// operator as actor.

import (
	runtimehttp "ideaboard/api/internal/runtime/interfaces/http"
	"net/http"
	"strings"

	"ideaboard/api/internal/platform/contract"
	"ideaboard/api/internal/platform/errorrender"
	taskapp "ideaboard/api/internal/tasking/application/task"
	tasksdomain "ideaboard/api/internal/tasking/domain"

	"github.com/gin-gonic/gin"
)

// controlTaskListCursor is the opaque cursor over page-number pagination.
type controlTaskListCursor struct {
	Page int `json:"page"`
}

// controlTaskListCountsMeta extends the cursor page meta with the
// global status breakdown: the monitor pills need per-status
// totals that cursor pagination cannot derive from one page. Counts
// honor the user/keyword filters but never the status filter (see
// CountControlTasks), so pills keep their global breakdown while a
// status is active. Read-only projection; task state untouched.
type controlTaskListCountsMeta struct {
	NextCursor string                     `json:"next_cursor,omitempty"`
	HasMore    bool                       `json:"has_more"`
	Counts     tasksdomain.TaskListCounts `json:"counts"`
}

func (d *ContractControlDeps) listControlTasks(c *gin.Context, _ controlContext) {
	if d == nil || d.Tasks == nil {
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable25)
		return
	}
	cursor, ok := parseControlPageCursor(c, func(cursor *controlTaskListCursor) bool { return cursor.Page >= 1 })
	if !ok {
		return
	}
	page := 1
	if cursor != nil {
		page = cursor.Page
	}
	pageSize, ok := parseControlPageSize(c, contract.DefaultPageSize)
	if !ok {
		return
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
			if errorrender.IsValidationError(err) {
				contract.RenderError(c, http.StatusBadRequest, contract.CodeBadRequest, runtimehttp.MsgBadRequest24)
			} else {
				contract.RenderError(c, http.StatusInternalServerError, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable26)
			}
			return
		}
		req.UserID = userID
	}
	items, counts, err := d.Tasks.ListTasks(c.Request.Context(), &req)
	if err != nil {
		if errorrender.IsValidationError(err) {
			contract.RenderError(c, http.StatusBadRequest, contract.CodeBadRequest, runtimehttp.MsgBadRequest4)
			return
		}
		contract.RenderError(c, http.StatusInternalServerError, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable27)
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
			contract.RenderError(c, http.StatusInternalServerError, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable27)
			return
		}
		nextCursor = encoded
	}
	// The counts ride the list meta so monitor pills render
	// without a second round trip. Shape stays the unified
	// {data, meta} envelope; only this endpoint's meta carries counts.
	contract.RenderPaginatedWithMeta(c, items, controlTaskListCountsMeta{
		NextCursor: nextCursor,
		HasMore:    hasMore,
		Counts:     counts,
	})
}

func (d *ContractControlDeps) getControlTask(c *gin.Context, _ controlContext) {
	if d == nil || d.Tasks == nil {
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable25)
		return
	}
	taskID := strings.TrimSpace(c.Param("task_id"))
	if taskID == "" || !contract.IsTaskID(taskID) {
		contract.RenderError(c, http.StatusNotFound, contract.CodeControlNotFound, runtimehttp.MsgControlNotFound8)
		return
	}
	task, err := d.Tasks.GetTask(c.Request.Context(), taskID)
	if err != nil {
		contract.RenderError(c, http.StatusNotFound, contract.CodeControlNotFound, runtimehttp.MsgControlNotFound8)
		return
	}
	contract.RenderSuccess(c, struct {
		Task any `json:"task"`
	}{Task: task})
}

func (d *ContractControlDeps) cancelControlTask(c *gin.Context, ctx controlContext) {
	if d == nil || d.Tasks == nil {
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable25)
		return
	}
	taskID := strings.TrimSpace(c.Param("task_id"))
	if taskID == "" || !contract.IsTaskID(taskID) {
		contract.RenderError(c, http.StatusNotFound, contract.CodeControlNotFound, runtimehttp.MsgControlNotFound8)
		return
	}
	if err := d.Tasks.CancelAdminTask(c.Request.Context(), taskID, ctx.Operator.UserID()); err != nil {
		if isControlTaskNotFound(err) {
			contract.RenderError(c, http.StatusNotFound, contract.CodeControlNotFound, runtimehttp.MsgControlNotFound8)
			return
		}
		if isControlTaskConflict(err) {
			contract.RenderError(c, http.StatusConflict, contract.CodeConflict, runtimehttp.MsgConflict4)
			return
		}
		contract.RenderError(c, http.StatusInternalServerError, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable28)
		return
	}
	contract.RenderSuccess(c, struct {
		Cancelled bool `json:"cancelled"`
	}{Cancelled: true})
}
