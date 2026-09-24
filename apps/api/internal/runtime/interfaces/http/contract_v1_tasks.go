package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"ideaboard/api/internal/events"
	"ideaboard/api/internal/identity/principal"
	"ideaboard/api/internal/platform/contract"
	"ideaboard/api/internal/tasking/workspacetasks"
	"ideaboard/api/internal/tenancy"

	"github.com/gin-gonic/gin"
)

// Workspace task store seams (aliases so ContractV1Deps stays in one
// package without import cycles in dependents).
type (
	WorkspaceTaskStore       = workspacetasks.WorkspaceTaskStore
	WorkspaceTaskQuoter      = workspacetasks.Quoter
	WorkspaceTaskHolder      = workspacetasks.Holder
	WorkspaceTaskEnqueuer    = workspacetasks.Enqueuer
	WorkspaceTaskAssetReader = workspacetasks.AssetReader
)

// Workspace task lifecycle (issue #805). Additive only: legacy
// /api/user task routes are untouched (removal is #809).
//
// Agreed surface (ContractV1GroupPath + PublicContractChain):
//
//	POST /api/v1/workspaces/{workspace_id}/tasks (202 + Location)
//	GET  /api/v1/workspaces/{workspace_id}/tasks (cursor page_size)
//	GET  /api/v1/workspaces/{workspace_id}/tasks/{task_id}
//	POST /api/v1/workspaces/{workspace_id}/tasks/{task_id}/cancel
//	     (Gin-safe /cancel sub-resource; the colon form
//	     :cancel is documented as a deviation)
//	GET  /api/v1/workspaces/{workspace_id}/tasks/{task_id}/outputs
//	POST /api/v1/workspaces/{workspace_id}/task-quotes (preflight, no persistence)
//
// Auth: workspace membership gate; hidden workspaces/tasks converge to
// 404 task_not_found (Baseline 4 anti-enumeration), visible-but-forbidden
// to 403. Outputs inherit task visibility.
//
// Atomicity: settlement runs Quote -> Hold -> Insert -> Enqueue with
// compensating rollback (the PG transaction in #809); terminal settle
// (Capture/Release) is atomic with the state flip; events stage and
// flush only after commit (NATS fanout is #807).
//
// Production wiring leaves ContractV1Deps.Tasks nil until the postgres
// workspace-task mapping lands (#809); handlers fail closed with 501.
// Tests inject workspacetasks.NewInMemoryStoreForTests() plus fakes.

func (h *contractV1Handler) registerTaskRoutes(group *gin.RouterGroup) {
	group.POST("/workspaces/:workspace_id/tasks", h.createWorkspaceTask)
	group.GET("/workspaces/:workspace_id/tasks", h.listWorkspaceTasks)
	group.GET("/workspaces/:workspace_id/tasks/:task_id", h.getWorkspaceTask)
	group.POST("/workspaces/:workspace_id/tasks/:task_id/cancel", h.cancelWorkspaceTask)
	group.GET("/workspaces/:workspace_id/tasks/:task_id/outputs", h.listWorkspaceTaskOutputs)
	group.POST("/workspaces/:workspace_id/task-quotes", h.quoteWorkspaceTask)
}

func (h *contractV1Handler) requireTaskStores(c *gin.Context) bool {
	if h.deps.Tasks == nil {
		contract.RenderError(c, http.StatusNotImplemented,
			contract.CodeContractNotImplemented, "The v1 tasks surface is not implemented yet")
		c.Abort()
		return false
	}
	return true
}

// taskWorkspaceContext resolves the explicit workspace context for task
// routes. Hidden workspaces (malformed ids, unknown ids, or caller holds
// no membership) converge to 404 task_not_found (not workspace_not_found)
// so the task surface never leaks workspace existence.
func (h *contractV1Handler) taskWorkspaceContext(c *gin.Context, identityPrincipal principal.Principal) (*tenancy.Workspace, bool) {
	raw := strings.TrimSpace(c.Param("workspace_id"))
	if !contract.IsWorkspaceID(raw) {
		contract.RenderError(c, http.StatusNotFound, contract.CodeTaskNotFound, "任务不存在")
		c.Abort()
		return nil, false
	}
	workspace, err := h.deps.Store.GetWorkspace(c.Request.Context(), raw)
	if err != nil {
		contract.RenderError(c, http.StatusNotFound, contract.CodeTaskNotFound, "任务不存在")
		c.Abort()
		return nil, false
	}
	_, member, err := h.deps.Store.GetWorkspaceRole(c.Request.Context(), raw, identityPrincipal.UserID())
	if err != nil || !member {
		contract.RenderError(c, http.StatusNotFound, contract.CodeTaskNotFound, "任务不存在")
		c.Abort()
		return nil, false
	}
	return workspace, true
}

func (h *contractV1Handler) taskService() *workspacetasks.Service {
	return workspacetasks.NewService(workspacetasks.ServiceDeps{
		Tasks: h.deps.Tasks, Quoter: h.deps.TaskQuoter,
		Holder: h.deps.TaskHolder, Queue: h.deps.TaskQueue, Assets: h.deps.TaskAssets,
	})
}

func renderTaskError(c *gin.Context, err error) {
	var validationErr *workspacetasks.ValidationError
	if errors.As(err, &validationErr) {
		fields := make(map[string]string, len(validationErr.Fields))
		for field, message := range validationErr.Fields {
			fields[field] = message
		}
		contract.RenderErrorWithDetails(c, contract.StatusOf(contract.CodeValidationFailed),
			contract.CodeValidationFailed, "任务参数非法",
			contract.FieldErrors{Fields: fields})
		c.Abort()
		return
	}
	switch {
	case errors.Is(err, workspacetasks.ErrNotFound):
		contract.RenderError(c, http.StatusNotFound, contract.CodeTaskNotFound, "任务不存在")
	case errors.Is(err, workspacetasks.ErrForbidden):
		contract.RenderError(c, http.StatusForbidden, contract.CodeTaskForbidden, "权限不足")
	case errors.Is(err, workspacetasks.ErrStateConflict):
		contract.RenderError(c, http.StatusConflict, contract.CodeTaskStateConflict, "任务状态不允许此操作")
	case errors.Is(err, workspacetasks.ErrInsufficientCredits):
		contract.RenderError(c, contract.StatusOf(contract.CodeTaskInsufficientCredits),
			contract.CodeTaskInsufficientCredits, "Idea不足")
	case errors.Is(err, workspacetasks.ErrQuoteUnavailable):
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeBillingQuoteUnavailable, "价格服务暂不可用")
	case errors.Is(err, workspacetasks.ErrWalletRestricted):
		contract.RenderError(c, http.StatusForbidden, contract.CodeBillingWalletRestricted, "当前工作区钱包受限，暂不可用")
	case errors.Is(err, workspacetasks.ErrMediaNotFound):
		contract.RenderError(c, http.StatusNotFound, contract.CodeMediaNotFound, "媒体资源不存在")
	case errors.Is(err, workspacetasks.ErrMediaNotReady):
		contract.RenderError(c, http.StatusConflict, contract.CodeMediaNotReady, "媒体资源尚未就绪")
	case errors.Is(err, workspacetasks.ErrMediaArchived):
		contract.RenderError(c, contract.StatusOf(contract.CodeMediaArchived), contract.CodeMediaArchived, "媒体资源已归档")
	default:
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, "任务服务暂不可用")
	}
	c.Abort()
}

func taskLocation(workspaceID, taskID string) string {
	return ContractV1GroupPath + "/workspaces/" + workspaceID + "/tasks/" + taskID
}

// v1CreateTaskBody is the discriminated input union:
// {workflow_kind, inputs:{media_asset_ids}, params}. DecodeStrict rejects
// unknown top-level fields; ValidateParams rejects unknown params keys and
// raw storage URLs.
type v1CreateTaskBody struct {
	Data struct {
		WorkflowKind string `json:"workflow_kind"`
		Inputs       struct {
			MediaAssetIDs []string `json:"media_asset_ids"`
		} `json:"inputs"`
		Params     map[string]any `json:"params"`
		Quantity   *int64         `json:"quantity"`
		Resolution *string        `json:"resolution"`
	} `json:"data"`
}

type v1TaskDTO struct {
	ID              string                   `json:"id"`
	WorkspaceID     string                   `json:"workspace_id"`
	WorkflowKind    string                   `json:"workflow_kind"`
	MediaAssetIDs   []string                 `json:"media_asset_ids,omitempty"`
	Status          string                   `json:"status"`
	RetryCount      int                      `json:"retry_count"`
	ExecutionToken  string                   `json:"execution_token,omitempty"`
	OutputRefs      []v1TaskOutputRefDTO     `json:"output_refs,omitempty"`
	BillingSnapshot v1TaskBillingSnapshotDTO `json:"billing_snapshot"`
	ErrorMsg        string                   `json:"error_msg,omitempty"`
	CancelReason    string                   `json:"cancel_reason,omitempty"`
	CreatedAt       string                   `json:"created_at"`
	UpdatedAt       string                   `json:"updated_at"`
	CompletedAt     *string                  `json:"completed_at,omitempty"`
}

type v1TaskBillingSnapshotDTO struct {
	SKU               string `json:"sku"`
	DisplayName       string `json:"display_name"`
	PriceVersionID    int64  `json:"price_version_id"`
	PriceVersion      int64  `json:"price_version"`
	OutputResolution  string `json:"output_resolution"`
	UnitCredits       int64  `json:"unit_credits"`
	Quantity          int64  `json:"quantity"`
	TotalCredits      int64  `json:"total_credits"`
	QuotedAt          string `json:"quoted_at"`
	HoldTransactionID string `json:"hold_transaction_id,omitempty"`
	SettlementState   string `json:"settlement_state"`
}

type v1TaskOutputRefDTO struct {
	Kind           string `json:"kind"`
	AssetID        string `json:"asset_id,omitempty"`
	URL            string `json:"url,omitempty"`
	ChecksumSHA256 string `json:"checksum_sha256,omitempty"`
	Role           string `json:"role,omitempty"`
	ResultIndex    int    `json:"result_index"`
}

func toTaskDTO(task *workspacetasks.WorkspaceTask) v1TaskDTO {
	dto := v1TaskDTO{
		ID: task.ID, WorkspaceID: task.WorkspaceID, WorkflowKind: task.WorkflowKind,
		MediaAssetIDs: append([]string(nil), task.MediaAssetIDs...),
		Status:        task.Status, RetryCount: task.RetryCount, ExecutionToken: task.ExecutionToken,
		ErrorMsg: task.ErrorMsg, CancelReason: task.CancelReason,
		CreatedAt: v1FormatTime(task.CreatedAt), UpdatedAt: v1FormatTime(task.UpdatedAt),
		BillingSnapshot: v1TaskBillingSnapshotDTO{
			SKU: task.BillingSnapshot.SKU, DisplayName: task.BillingSnapshot.DisplayName,
			PriceVersionID: task.BillingSnapshot.PriceVersionID, PriceVersion: task.BillingSnapshot.PriceVersion,
			OutputResolution: task.BillingSnapshot.OutputResolution, UnitCredits: task.BillingSnapshot.UnitCredits,
			Quantity: task.BillingSnapshot.Quantity, TotalCredits: task.BillingSnapshot.TotalCredits,
			QuotedAt:          v1FormatTime(task.BillingSnapshot.QuotedAt),
			HoldTransactionID: task.BillingSnapshot.HoldTransactionID,
			SettlementState:   task.BillingSnapshot.SettlementState,
		},
	}
	if task.CompletedAt != nil {
		completed := v1FormatTime(*task.CompletedAt)
		dto.CompletedAt = &completed
	}
	for _, ref := range task.OutputRefs {
		dto.OutputRefs = append(dto.OutputRefs, v1TaskOutputRefDTO{
			Kind: ref.Kind, AssetID: ref.AssetID, URL: ref.URL,
			ChecksumSHA256: ref.ChecksumSHA256, Role: ref.Role, ResultIndex: ref.ResultIndex,
		})
	}
	return dto
}

func requireTaskIdempotencyKey(c *gin.Context) (string, bool) {
	key, err := contract.IdempotencyKeyFromHeader(c)
	if err != nil {
		contract.RenderError(c, http.StatusBadRequest, contract.CodeIdempotencyKeyRequired, "缺少幂等键，请重试此次操作")
		c.Abort()
		return "", false
	}
	return key, true
}

func (h *contractV1Handler) createWorkspaceTask(c *gin.Context) {
	identityPrincipal, ok := v1RequestPrincipal(c)
	if !ok || !h.requireStore(c) || !h.requireTaskStores(c) {
		return
	}
	workspace, _ := h.taskWorkspaceContext(c, identityPrincipal)
	if workspace == nil {
		return
	}
	key, ok := requireTaskIdempotencyKey(c)
	if !ok {
		return
	}
	var body v1CreateTaskBody
	if bindErr := contract.BindStrictJSON(c, &body); bindErr != nil {
		contract.RenderErrorWithDetails(c, http.StatusBadRequest, contract.CodeBadRequest,
			"请求体非法", contract.FieldErrors{Fields: map[string]string{"data": "invalid"}})
		c.Abort()
		return
	}
	kind := strings.TrimSpace(body.Data.WorkflowKind)
	assetIDs := append([]string(nil), body.Data.Inputs.MediaAssetIDs...)
	for i := range assetIDs {
		assetIDs[i] = strings.TrimSpace(assetIDs[i])
	}
	var quantity int64 = 1
	if body.Data.Quantity != nil {
		quantity = *body.Data.Quantity
	}
	resolution := ""
	if body.Data.Resolution != nil {
		resolution = strings.TrimSpace(*body.Data.Resolution)
	}
	// Canonical request hash for idempotent replay (same key+hash ->
	// same task_id; conflict hash -> 409).
	paramsJSON, _ := json.Marshal(body.Data.Params)
	hash := contract.HashIdempotencyRequest("task:create", workspace.PublicID, kind,
		strings.Join(assetIDs, ","), string(paramsJSON),
		strconv.FormatInt(quantity, 10), resolution)
	externalUser := tenancy.FormatUserPublicID(identityPrincipal.PublicID())
	scope := contract.IdempotencyScope(externalUser, "task:create")
	service := h.taskService()
	result, _, doErr := h.deps.Idempotency.Do(c.Request.Context(), scope, identityPrincipal.UserID(), key, hash, func() (any, error) {
		created, err := service.Create(c.Request.Context(), workspacetasks.CreateInput{
			WorkspaceID: workspace.PublicID, OwnerUserID: identityPrincipal.UserID(),
			WorkflowKind: kind, MediaAssetIDs: assetIDs, Params: body.Data.Params,
			Quantity: quantity, Resolution: resolution,
		}, key)
		if err != nil {
			return nil, err
		}
		// #807: emit AFTER the settlement commit, inside the once-fn so
		// idempotent replays never re-emit. Staged pending maps to
		// task.created; the hold/free ref maps to billing.settled.
		for _, staged := range service.StagedEvents() {
			typ, ok := events.TaskStatusV1Type(staged.Status)
			if !ok {
				continue
			}
			h.emitWorkspaceEvent(c.Request.Context(), identityPrincipal, staged.WorkspaceID, typ,
				events.V1Resource{Type: events.V1ResourceTask, ID: staged.TaskID},
				events.V1TaskCreatedData{TaskID: staged.TaskID, WorkflowKind: kind})
		}
		h.emitTaskBillingSettled(c.Request.Context(), identityPrincipal,
			created.WorkspaceID, created.ID,
			created.BillingSnapshot.SettlementState, created.BillingSnapshot.HoldTransactionID)
		dto := toTaskDTO(created)
		return &dto, nil
	})
	if doErr != nil {
		if contract.IsIdempotencyConflict(doErr) {
			contract.RenderError(c, http.StatusConflict, contract.CodeTaskIdempotencyConflict, "幂等请求与原请求不一致")
			c.Abort()
			return
		}
		renderTaskError(c, doErr)
		return
	}
	dto, ok := result.(*v1TaskDTO)
	if !ok || dto == nil {
		renderTaskError(c, errors.New("tasks: create type mismatch"))
		return
	}
	c.Header("Location", taskLocation(workspace.PublicID, dto.ID))
	contract.RenderAccepted(c, *dto)
}

func (h *contractV1Handler) listWorkspaceTasks(c *gin.Context) {
	identityPrincipal, ok := v1RequestPrincipal(c)
	if !ok || !h.requireStore(c) || !h.requireTaskStores(c) {
		return
	}
	workspace, _ := h.taskWorkspaceContext(c, identityPrincipal)
	if workspace == nil {
		return
	}
	pageSize, err := contract.ParsePageSize(c.Query(contract.PageSizeParam))
	if err != nil {
		contract.RenderError(c, http.StatusBadRequest, contract.CodeBadRequest, "page_size 非法")
		c.Abort()
		return
	}
	offset := 0
	if raw := strings.TrimSpace(c.Query(contract.CursorParam)); raw != "" {
		cursor, decodeErr := contract.DecodeCursor[v1ListCursor](raw)
		if decodeErr != nil || cursor == nil || cursor.Offset < 0 {
			contract.RenderError(c, http.StatusBadRequest, contract.CodePaginationCursorInvalid, "分页游标非法")
			c.Abort()
			return
		}
		offset = cursor.Offset
	}
	tasks, err := h.deps.Tasks.ListTasks(c.Request.Context(), workspace.PublicID, pageSize+1, offset)
	if err != nil {
		renderTaskError(c, err)
		return
	}
	hasMore := len(tasks) > pageSize
	if hasMore {
		tasks = tasks[:pageSize]
	}
	page := make([]v1TaskDTO, 0, len(tasks))
	for _, task := range tasks {
		page = append(page, toTaskDTO(task))
	}
	meta := contract.NewPageMeta("", false)
	if hasMore {
		next, encodeErr := contract.EncodeCursor(v1ListCursor{Offset: offset + pageSize})
		if encodeErr != nil {
			renderTaskError(c, encodeErr)
			return
		}
		meta = contract.NewPageMeta(next, true)
	}
	contract.RenderPaginated(c, page, meta)
}

func (h *contractV1Handler) getWorkspaceTask(c *gin.Context) {
	identityPrincipal, ok := v1RequestPrincipal(c)
	if !ok || !h.requireStore(c) || !h.requireTaskStores(c) {
		return
	}
	workspace, _ := h.taskWorkspaceContext(c, identityPrincipal)
	if workspace == nil {
		return
	}
	raw := strings.TrimSpace(c.Param("task_id"))
	if !contract.IsTaskID(raw) {
		contract.RenderError(c, http.StatusNotFound, contract.CodeTaskNotFound, "任务不存在")
		c.Abort()
		return
	}
	task, err := h.deps.Tasks.GetTask(c.Request.Context(), workspace.PublicID, raw)
	if err != nil {
		renderTaskError(c, err)
		return
	}
	contract.RenderSuccess(c, toTaskDTO(task))
}

type v1CancelTaskBody struct {
	Data *struct {
		Reason *string `json:"reason"`
	} `json:"data"`
}

func (h *contractV1Handler) cancelWorkspaceTask(c *gin.Context) {
	identityPrincipal, ok := v1RequestPrincipal(c)
	if !ok || !h.requireStore(c) || !h.requireTaskStores(c) {
		return
	}
	workspace, _ := h.taskWorkspaceContext(c, identityPrincipal)
	if workspace == nil {
		return
	}
	raw := strings.TrimSpace(c.Param("task_id"))
	if !contract.IsTaskID(raw) {
		contract.RenderError(c, http.StatusNotFound, contract.CodeTaskNotFound, "任务不存在")
		c.Abort()
		return
	}
	key, ok := requireTaskIdempotencyKey(c)
	if !ok {
		return
	}
	reason := "user cancel"
	if c.Request.ContentLength != 0 {
		var body v1CancelTaskBody
		if bindErr := contract.BindStrictJSON(c, &body); bindErr != nil {
			contract.RenderErrorWithDetails(c, http.StatusBadRequest, contract.CodeBadRequest,
				"请求体非法", contract.FieldErrors{Fields: map[string]string{"data": "invalid"}})
			c.Abort()
			return
		}
		if body.Data != nil && body.Data.Reason != nil && strings.TrimSpace(*body.Data.Reason) != "" {
			reason = strings.TrimSpace(*body.Data.Reason)
		}
	}
	externalUser := tenancy.FormatUserPublicID(identityPrincipal.PublicID())
	scope := contract.IdempotencyScope(externalUser, "task:cancel")
	hash := contract.HashIdempotencyRequest("task:cancel", workspace.PublicID, raw, reason)
	service := h.taskService()
	result, _, doErr := h.deps.Idempotency.Do(c.Request.Context(), scope, identityPrincipal.UserID(), key, hash, func() (any, error) {
		cancelled, already, err := service.Cancel(c.Request.Context(), workspace.PublicID, raw, identityPrincipal.UserID(), reason)
		if err != nil {
			return nil, err
		}
		// #807: fresh flips emit task.cancelled + billing.settled
		// (released) AFTER the atomic flip; terminal replays (already)
		// never re-emit.
		if !already {
			for _, staged := range service.StagedEvents() {
				typ, ok := events.TaskStatusV1Type(staged.Status)
				if !ok {
					continue
				}
				h.emitWorkspaceEvent(c.Request.Context(), identityPrincipal, staged.WorkspaceID, typ,
					events.V1Resource{Type: events.V1ResourceTask, ID: staged.TaskID},
					events.V1TaskCancelledData{TaskID: staged.TaskID, CancelReason: reason})
			}
			h.emitTaskBillingSettled(c.Request.Context(), identityPrincipal,
				cancelled.WorkspaceID, cancelled.ID,
				cancelled.BillingSnapshot.SettlementState, cancelled.BillingSnapshot.HoldTransactionID)
		}
		dto := toTaskDTO(cancelled)
		return &dto, nil
	})
	if doErr != nil {
		if contract.IsIdempotencyConflict(doErr) {
			contract.RenderError(c, http.StatusConflict, contract.CodeTaskIdempotencyConflict, "幂等请求与原请求不一致")
			c.Abort()
			return
		}
		renderTaskError(c, doErr)
		return
	}
	dto, ok := result.(*v1TaskDTO)
	if !ok || dto == nil {
		renderTaskError(c, errors.New("tasks: cancel type mismatch"))
		return
	}
	contract.RenderSuccess(c, *dto)
}

func (h *contractV1Handler) listWorkspaceTaskOutputs(c *gin.Context) {
	identityPrincipal, ok := v1RequestPrincipal(c)
	if !ok || !h.requireStore(c) || !h.requireTaskStores(c) {
		return
	}
	workspace, _ := h.taskWorkspaceContext(c, identityPrincipal)
	if workspace == nil {
		return
	}
	raw := strings.TrimSpace(c.Param("task_id"))
	if !contract.IsTaskID(raw) {
		contract.RenderError(c, http.StatusNotFound, contract.CodeTaskNotFound, "任务不存在")
		c.Abort()
		return
	}
	task, err := h.deps.Tasks.GetTask(c.Request.Context(), workspace.PublicID, raw)
	if err != nil {
		renderTaskError(c, err)
		return
	}
	refs := make([]v1TaskOutputRefDTO, 0, len(task.OutputRefs))
	for _, ref := range task.OutputRefs {
		refs = append(refs, v1TaskOutputRefDTO{
			Kind: ref.Kind, AssetID: ref.AssetID, URL: ref.URL,
			ChecksumSHA256: ref.ChecksumSHA256, Role: ref.Role, ResultIndex: ref.ResultIndex,
		})
	}
	contract.RenderSuccess(c, map[string]any{"task_id": task.ID, "outputs": refs})
}

type v1QuoteTaskBody struct {
	Data struct {
		WorkflowKind string  `json:"workflow_kind"`
		Quantity     *int64  `json:"quantity"`
		Resolution   *string `json:"resolution"`
	} `json:"data"`
}

// quoteWorkspaceTask is the preflight seam: it returns an immutable price
// snapshot with no persistence (separate from authoritative create).
func (h *contractV1Handler) quoteWorkspaceTask(c *gin.Context) {
	identityPrincipal, ok := v1RequestPrincipal(c)
	if !ok || !h.requireStore(c) || !h.requireTaskStores(c) {
		return
	}
	workspace, _ := h.taskWorkspaceContext(c, identityPrincipal)
	if workspace == nil {
		return
	}
	var body v1QuoteTaskBody
	if bindErr := contract.BindStrictJSON(c, &body); bindErr != nil {
		contract.RenderErrorWithDetails(c, http.StatusBadRequest, contract.CodeBadRequest,
			"请求体非法", contract.FieldErrors{Fields: map[string]string{"data": "invalid"}})
		c.Abort()
		return
	}
	quantity := int64(1)
	if body.Data.Quantity != nil {
		quantity = *body.Data.Quantity
	}
	resolution := "default"
	if body.Data.Resolution != nil && strings.TrimSpace(*body.Data.Resolution) != "" {
		resolution = strings.TrimSpace(*body.Data.Resolution)
	}
	service := h.taskService()
	snapshot, err := service.Quote(c.Request.Context(), workspacetasks.QuoteParams{
		WorkflowKind: strings.TrimSpace(body.Data.WorkflowKind), Quantity: quantity, Resolution: resolution,
	})
	if err != nil {
		renderTaskError(c, err)
		return
	}
	contract.RenderSuccess(c, v1TaskBillingSnapshotDTO{
		SKU: snapshot.SKU, DisplayName: snapshot.DisplayName,
		PriceVersionID: snapshot.PriceVersionID, PriceVersion: snapshot.PriceVersion,
		OutputResolution: snapshot.OutputResolution, UnitCredits: snapshot.UnitCredits,
		Quantity: snapshot.Quantity, TotalCredits: snapshot.TotalCredits,
		QuotedAt: v1FormatTime(snapshot.QuotedAt), SettlementState: "quoted",
	})
}

// OpenAPI deltas for the merger (DO NOT edit apps/api/openapi*.yaml here).
// Add under paths (tag: WorkspaceTasks), reusing components parameters
// Cursor/PageSize/WorkspaceId/TaskId/IdempotencyKey/RequestId and responses
// BadRequest/Unauthorized/Forbidden/NotFound/Conflict/UnprocessableEntity:
//
//	/workspaces/{workspace_id}/tasks:
//	  post: {operationId: createWorkspaceTask, parameters: [WorkspaceId, IdempotencyKey],
//	         responses: {'202': {headers: {Location, Request-Id}, content: {TaskEnvelope}}}}
//	  get: {operationId: listWorkspaceTasks, parameters: [WorkspaceId, Cursor, PageSize]}
//	/workspaces/{workspace_id}/tasks/{task_id}:
//	  get: {operationId: getWorkspaceTask, parameters: [WorkspaceId, TaskId]}
//	/workspaces/{workspace_id}/tasks/{task_id}/cancel:
//	  post: {operationId: cancelWorkspaceTask, parameters: [WorkspaceId, TaskId, IdempotencyKey]
//	         (Gin-safe /cancel sub-resource; the `:cancel` colon form is a documented deviation)}
//	/workspaces/{workspace_id}/tasks/{task_id}/outputs:
//	  get: {operationId: listWorkspaceTaskOutputs, parameters: [WorkspaceId, TaskId]}
//	/workspaces/{workspace_id}/task-quotes:
//	  post: {operationId: quoteWorkspaceTask, parameters: [WorkspaceId]}
//
// Schemas:
//
//	WorkspaceTask: {id: TaskId (task_...), workspace_id: WorkspaceId, workflow_kind: {enum: [render,
//	  variations, inpaint, cutout, fusion, upscale, outpaint]}, media_asset_ids?: asset_...[],
//	  params?: object (no storage URLs, unknown fields rejected), status: {enum: [pending, running,
//	  completed, failed, cancelled]}, retry_count: int>=0, execution_token?: string,
//	  output_refs?: TaskOutputRef[], billing_snapshot: TaskBillingSnapshot,
//	  error_msg?, cancel_reason?, created_at: date-time, updated_at: date-time, completed_at?: date-time}
//	TaskOutputRef: {kind: {enum: [asset, remote]}, asset_id?: asset_... (kind=asset, ready only),
//	  url?: uri (kind=remote, no storage internals), checksum_sha256?: 64-hex (kind=remote),
//	  role?: string, result_index: int}
//	TaskBillingSnapshot: {sku: string (ai-image:<feature>), display_name: string,
//	  price_version_id: int64, price_version: int64, output_resolution: string,
//	  unit_credits: int64>=0, quantity: int64>=1, total_credits: int64>=0,
//	  quoted_at: date-time, hold_transaction_id?: string, settlement_state: {enum: [quoted, free, held, captured, released]}}
//	CreateWorkspaceTaskRequest: {data: {workflow_kind: string, inputs: {media_asset_ids?: asset_...[]},
//	  params?: object, quantity?: int64>=1, resolution?: string}}
//	QuoteWorkspaceTaskRequest: {data: {workflow_kind: string, quantity?: int64>=1, resolution?: string}}
//
// Error codes frozen: task_not_found (404 hidden), task_forbidden (403),
// task_state_conflict (409), task_insufficient_credits (402),
// task_idempotency_conflict (409, kernel); reused kernel codes
// workspace_not_found/workspace_forbidden/pagination_cursor_invalid/bad_request/
// validation_failed/idempotency_key_required/service_unavailable plus
// billing_quote_unavailable (503), billing_wallet_restricted (403),
// media_not_found (404), media_not_ready (409), media_archived (410).
