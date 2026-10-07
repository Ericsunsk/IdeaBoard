package httpapi

// Control tasks family (issue #808): scoped task read/ops behind
// control:tasks — get/list/cancel with operator reason.
//
// There is deliberately NO task creation path here: submission stays on
// the Public Data Plane. Cancel reuses the task command seam with the
// operator as actor.

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

// controlTaskListCountsMeta extends the cursor page meta with the
// global status breakdown (#852): the monitor pills need per-status
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
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, msgServiceUnavailable25)
		return
	}
	page := 1
	if raw := strings.TrimSpace(c.Query("cursor")); raw != "" {
		decoded, err := contract.DecodeCursor[controlTaskListCursor](raw)
		if err != nil || decoded == nil || decoded.Page < 1 {
			contract.RenderError(c, http.StatusBadRequest, contract.CodePaginationCursorInvalid, msgPaginationCursorInvalid)
			return
		}
		page = decoded.Page
	}
	pageSize := contract.DefaultPageSize
	if raw := strings.TrimSpace(c.Query("page_size")); raw != "" {
		parsed, err := contract.ParsePageSize(raw)
		if err != nil {
			contract.RenderError(c, http.StatusBadRequest, contract.CodeBadRequest, msgBadRequest13)
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
				contract.RenderError(c, http.StatusBadRequest, contract.CodeBadRequest, msgBadRequest24)
			} else {
				contract.RenderError(c, http.StatusInternalServerError, contract.CodeServiceUnavailable, msgServiceUnavailable26)
			}
			return
		}
		req.UserID = userID
	}
	items, counts, err := d.Tasks.ListTasks(c.Request.Context(), &req)
	if err != nil {
		if validation.Is(err) {
			contract.RenderError(c, http.StatusBadRequest, contract.CodeBadRequest, msgBadRequest4)
			return
		}
		contract.RenderError(c, http.StatusInternalServerError, contract.CodeServiceUnavailable, msgServiceUnavailable27)
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
			contract.RenderError(c, http.StatusInternalServerError, contract.CodeServiceUnavailable, msgServiceUnavailable27)
			return
		}
		nextCursor = encoded
	}
	// The counts ride the list meta (#852) so monitor pills render
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
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, msgServiceUnavailable25)
		return
	}
	taskID := strings.TrimSpace(c.Param("task_id"))
	if taskID == "" || !contract.IsTaskID(taskID) {
		contract.RenderError(c, http.StatusNotFound, contract.CodeControlNotFound, msgControlNotFound8)
		return
	}
	task, err := d.Tasks.GetTask(c.Request.Context(), taskID)
	if err != nil {
		contract.RenderError(c, http.StatusNotFound, contract.CodeControlNotFound, msgControlNotFound8)
		return
	}
	contract.RenderSuccess(c, struct {
		Task any `json:"task"`
	}{Task: task})
}

func (d *ContractControlDeps) cancelControlTask(c *gin.Context, ctx controlContext) {
	if d == nil || d.Tasks == nil {
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, msgServiceUnavailable25)
		return
	}
	taskID := strings.TrimSpace(c.Param("task_id"))
	if taskID == "" || !contract.IsTaskID(taskID) {
		contract.RenderError(c, http.StatusNotFound, contract.CodeControlNotFound, msgControlNotFound8)
		return
	}
	if err := d.Tasks.CancelAdminTask(c.Request.Context(), taskID, ctx.Operator.UserID(), ctx.Reason); err != nil {
		if isControlTaskNotFound(err) {
			contract.RenderError(c, http.StatusNotFound, contract.CodeControlNotFound, msgControlNotFound8)
			return
		}
		if isControlTaskConflict(err) {
			contract.RenderError(c, http.StatusConflict, contract.CodeConflict, msgConflict4)
			return
		}
		contract.RenderError(c, http.StatusInternalServerError, contract.CodeServiceUnavailable, msgServiceUnavailable28)
		return
	}
	contract.RenderSuccess(c, struct {
		Cancelled bool `json:"cancelled"`
	}{Cancelled: true})
}
