package v1

import (
	"context"
	"encoding/json"
	"errors"
	runtimehttp "ideaboard/api/internal/runtime/interfaces/http"
	"net/http"
	"strconv"
	"strings"
	"time"

	"ideaboard/api/internal/credits/domain"
	"ideaboard/api/internal/events"
	"ideaboard/api/internal/generation/routing"
	"ideaboard/api/internal/identity/principal"
	"ideaboard/api/internal/media/workspacemedia"
	"ideaboard/api/internal/platform/contract"
	"ideaboard/api/internal/platform/errorrender"
	taskservice "ideaboard/api/internal/tasking/application/task"
	"ideaboard/api/internal/tasking/workspacetasks"
	"ideaboard/api/internal/tenancy"
	"ideaboard/api/internal/tenancy/funds"
	"ideaboard/api/internal/workflow"
	"ideaboard/api/internal/workflow/paramkeys"

	"github.com/gin-gonic/gin"
)

// Workspace task store seams (aliases so ContractV1Deps stays in one
// package without import cycles in dependents).
type (
	WorkspaceTaskReadStore      = workspacetasks.TaskReadStore
	WorkspaceTaskLifecycleStore = workspacetasks.TaskSettlementStore
	WorkspaceTaskBillingStore   = workspacetasks.TaskBillingStore
	WorkspaceTaskQuoter         = workspacetasks.Quoter
	WorkspaceTaskHolder         = workspacetasks.TxHolder
	WorkspaceTaskEnqueuer       = workspacetasks.TxEnqueuer
	WorkspaceTaskAssetReader    = workspacetasks.AssetReader
	// WorkspaceTaskSnapshotSaver pins the submit-time RoutingSnapshot
	// (catalog.SaveRoutingSnapshot in production, capture fakes in tests).
	WorkspaceTaskSnapshotSaver = workspacetasks.RoutingSnapshotSaver
)

// taskQuoteBillingAccountReader resolves the stable funds identity for the
// quote/readiness preflight without a settlement transaction (#917: Quote,
// readiness, and submit spend the same account). Production adapts the
// funds seam (ws_ only); tests stub it. Nil fails the preflight closed.
type taskQuoteBillingAccountReader interface {
	ResolveFunds(ctx context.Context, workspacePublicID string) (funds.Identity, error)
}

// taskQuoteWalletReader reads the account Wallet for the preflight funds
// gate (#917: insufficient vs restricted surfaces here exactly as on
// submit). Production wires the Credits service; tests stub it. Nil fails
// the preflight closed.
type taskQuoteWalletReader interface {
	GetWallet(ctx context.Context, billingAccountID int64) (domain.WalletDetails, error)
}

// taskQuoteQuotaReader reads the member cross-workspace consumption quota
// for the quote/readiness preflight (#946: the same cap submit enforces,
// reported early as not-ready). Production wires the memberquotas Store;
// tests stub it. Nil skips the quota gate (unlimited) so existing
// fixtures stay green.
type taskQuoteQuotaReader interface {
	CheckQuotaForQuote(ctx context.Context, billingAccountID, userID int64, amount int64, reservedAt time.Time) error
}

// Workspace task lifecycle (issue #805). Additive only: legacy
// /api/user task routes were removed in #809.
//
// Agreed surface (runtimehttp.ContractV1GroupPath + runtimehttp.PublicContractChain):
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
// Atomicity: settlement commits Quote -> Hold -> Insert -> Enqueue in
// ONE PostgreSQL transaction; terminal settle (Capture/Release) is
// atomic with the state flip; events stage and flush only after commit
// (NATS fanout is #807).
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
		contract.RenderError(c, http.StatusNotFound, contract.CodeTaskNotFound, runtimehttp.MsgControlNotFound8)
		c.Abort()
		return nil, false
	}
	workspace, err := h.deps.Store.GetWorkspace(c.Request.Context(), raw)
	if err != nil {
		contract.RenderError(c, http.StatusNotFound, contract.CodeTaskNotFound, runtimehttp.MsgControlNotFound8)
		c.Abort()
		return nil, false
	}
	_, member, err := h.deps.Store.GetWorkspaceRole(c.Request.Context(), raw, identityPrincipal.UserID())
	if err != nil || !member {
		contract.RenderError(c, http.StatusNotFound, contract.CodeTaskNotFound, runtimehttp.MsgControlNotFound8)
		c.Abort()
		return nil, false
	}
	return workspace, true
}

func (h *contractV1Handler) taskService() *workspacetasks.Service {
	deps := workspacetasks.ServiceDeps{
		TaskRead: h.deps.Tasks, TaskLifecycle: h.deps.TaskLifecycle,
		TaskBilling: h.deps.TaskBilling, SubmissionClaims: h.deps.TaskSubmissionClaims,
		Quoter: h.deps.TaskQuoter,
		Holder: h.deps.TaskHolder, Queue: h.deps.TaskQueue, Assets: h.deps.TaskAssets,
		Resolver: h.deps.TaskRouter, SnapshotSaver: h.deps.TaskSnapshotSaver,
		Acceptance:      h.taskSubmissionAcceptance(),
		BillingAccounts: h.deps.TaskBillingAccounts,
		Quotas:          h.deps.TaskQuotas,
	}
	return workspacetasks.NewService(deps)
}

// taskSubmissionAcceptance returns the submit gate for the settlement
// transaction: the production authorizer, or the injected test double when
// the fixture supplies one (in-memory HTTP tests have no database for the
// workspace lock the production gate takes).
func (h *contractV1Handler) taskSubmissionAcceptance() workspacetasks.SubmissionAuthorizer {
	if h.deps.TaskAcceptance != nil {
		return h.deps.TaskAcceptance
	}
	return &workspacemedia.TaskSubmissionAuthorizer{Boards: h.deps.Boards, TaskBoards: h.taskBoardsAuthorizer()}
}

func renderTaskError(c *gin.Context, err error) {
	var validationErr *workspacetasks.ValidationError
	if errors.As(err, &validationErr) {
		fields := make(map[string]string, len(validationErr.Fields))
		for field, message := range validationErr.Fields {
			fields[field] = message
		}
		contract.RenderErrorWithDetails(c, contract.StatusOf(contract.CodeValidationFailed),
			contract.CodeValidationFailed, runtimehttp.MsgValidationFailed10,
			contract.FieldErrors{Fields: fields})
		c.Abort()
		return
	}
	// Canonical-compile validation failures (prompt bound/policy, mask
	// shape, plan contract) are client errors: they carry a safe Chinese
	// message (errorrender.ValidationError is safe to expose) and converge to 422
	// validation_failed, never 503.
	if errorrender.IsValidationError(err) {
		contract.RenderErrorWithDetails(c, contract.StatusOf(contract.CodeValidationFailed),
			contract.CodeValidationFailed, err.Error(),
			contract.FieldErrors{Fields: map[string]string{"data": "invalid"}})
		c.Abort()
		return
	}
	if renderBillingPricingUnavailable(c, err) {
		c.Abort()
		return
	}
	switch {
	case errors.Is(err, workspacetasks.ErrNotFound):
		contract.RenderError(c, contract.StatusOf(contract.CodeTaskNotFound), contract.CodeTaskNotFound, runtimehttp.MsgControlNotFound8)
	case errors.Is(err, workspacetasks.ErrBoardNotFound):
		contract.RenderError(c, contract.StatusOf(contract.CodeBoardNotFound), contract.CodeBoardNotFound, runtimehttp.MsgBoardNotFound)
	case errors.Is(err, workspacetasks.ErrBoardArchived):
		contract.RenderError(c, contract.StatusOf(contract.CodeBoardArchived), contract.CodeBoardArchived, runtimehttp.MsgBoardArchived)
	case errors.Is(err, workspacetasks.ErrForbidden):
		contract.RenderError(c, contract.StatusOf(contract.CodeTaskForbidden), contract.CodeTaskForbidden, runtimehttp.MsgForbidden)
	case errors.Is(err, workspacetasks.ErrIdempotencyConflict):
		// The durable submit claim already holds this (workspace, operation,
		// key) for a different request body. Reusing a key across different
		// bodies is a client bug, never a replay.
		contract.RenderError(c, contract.StatusOf(contract.CodeTaskIdempotencyConflict), contract.CodeTaskIdempotencyConflict, runtimehttp.MsgWorkspaceIdempotencyConflict)
	case errors.Is(err, workspacetasks.ErrStateConflict):
		contract.RenderError(c, contract.StatusOf(contract.CodeTaskStateConflict), contract.CodeTaskStateConflict, runtimehttp.MsgTaskStateConflict)
	case errors.Is(err, workspacetasks.ErrInsufficientCredits):
		contract.RenderError(c, contract.StatusOf(contract.CodeTaskInsufficientCredits),
			contract.CodeTaskInsufficientCredits, runtimehttp.MsgValidationFailed2)
	case errors.Is(err, workspacetasks.ErrMemberQuotaExhausted):
		contract.RenderError(c, contract.StatusOf(contract.CodeMemberQuotaExhausted),
			contract.CodeMemberQuotaExhausted, runtimehttp.MsgMemberQuotaExhausted)
	case errors.Is(err, workspacetasks.ErrQuoteUnavailable):
		contract.RenderError(c, contract.StatusOf(contract.CodeBillingQuoteUnavailable), contract.CodeBillingQuoteUnavailable, runtimehttp.MsgBillingQuoteUnavailable)
	// An empty eligible pool converges to 422
	// service_resolution_unavailable with the normalized request values at
	// details.fields.service/details.fields.resolution (ADR-0012 rule 8).
	case errors.Is(err, routing.ErrServiceResolutionUnavailable):
		var unavailable *routing.ResolutionUnavailableError
		fields := map[string]string{"service": "", "resolution": ""}
		if errors.As(err, &unavailable) && unavailable != nil {
			fields["service"] = unavailable.Service
			fields["resolution"] = string(unavailable.Resolution)
		}
		contract.RenderErrorWithDetails(c, contract.StatusOf(contract.CodeServiceResolutionUnavailable), contract.CodeServiceResolutionUnavailable, runtimehttp.MsgBillingPoolUnavailable,
			contract.FieldErrors{Fields: fields})
	case errors.Is(err, workspacetasks.ErrWalletRestricted):
		contract.RenderError(c, contract.StatusOf(contract.CodeBillingWalletRestricted), contract.CodeBillingWalletRestricted, runtimehttp.MsgBillingWalletRestricted2)
	case errors.Is(err, workspacetasks.ErrMediaNotFound):
		contract.RenderError(c, contract.StatusOf(contract.CodeMediaNotFound), contract.CodeMediaNotFound, runtimehttp.MsgMediaNotFound2)
	case errors.Is(err, workspacetasks.ErrMediaNotReady):
		contract.RenderError(c, contract.StatusOf(contract.CodeMediaNotReady), contract.CodeMediaNotReady, runtimehttp.MsgMediaNotReady)
	case errors.Is(err, workspacetasks.ErrMediaArchived):
		contract.RenderError(c, contract.StatusOf(contract.CodeMediaArchived), contract.CodeMediaArchived, runtimehttp.MsgMediaArchived)
	default:
		contract.RenderError(c, contract.StatusOf(contract.CodeServiceUnavailable), contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable25)
	}
	c.Abort()
}

func taskLocation(workspaceID, taskID string) string {
	return runtimehttp.ContractV1GroupPath + "/workspaces/" + workspaceID + "/tasks/" + taskID
}

// v1CreateTaskBody is the discriminated input union:
// {service, inputs:{media_asset_ids}, prompt, params}. DecodeStrict
// rejects unknown top-level fields; ValidateParams rejects unknown params
// keys and raw storage URLs.
//
// Breaking rename (#847): `service` replaces `workflow_kind` with no
// compatibility alias. A body carrying only the old `workflow_kind` field
// fails DecodeStrict (unknown field) and renders 400 bad_request; a body
// with neither field binds and then fails validation as 422
// validation_failed on details.fields.service.
//
// Prompt rides the explicit data.prompt field (bound:
// workspacetasks.MaxWorkspaceTaskPromptBytes = 8KB), never
// params.prompt (still 422 unknown_field): it threads into the canonical
// Generation snapshot (UserPrompt + CompiledPrompt) that the worker
// bridges to the provider. The v1 mask rides params.mask_asset_id
// (workspace public asset_... from the upload-session flow, inpaint only).
//
// The optional data.board_id binds a Canvas submit to its Board (#937):
// the board_ public id is validated against the caller's live Board
// posture (visible identity + generate capability) before any Quote/Hold,
// and snapshots the Board access_version into the durable task binding.
// Omitted Board IDs stay non-Canvas tasks with their own workspace-level
// permission (submitter-only reads, never auto-published); callers must
// never forge a Board attribution.
type v1CreateTaskBody struct {
	Data struct {
		Service string `json:"service"`
		Inputs  struct {
			MediaAssetIDs []string `json:"media_asset_ids"`
		} `json:"inputs"`
		Prompt     string         `json:"prompt,omitempty"`
		Params     map[string]any `json:"params"`
		Quantity   *int64         `json:"quantity"`
		Resolution *string        `json:"resolution"`
		BoardID    string         `json:"board_id,omitempty"`
	} `json:"data"`
}

type v1TaskDTO struct {
	ID                 string                   `json:"id"`
	WorkspaceID        string                   `json:"workspace_id"`
	Service            string                   `json:"service"`
	MediaAssetIDs      []string                 `json:"media_asset_ids,omitempty"`
	BoardID            string                   `json:"board_id,omitempty"`
	BoardAccessVersion int64                    `json:"board_access_version,omitempty"`
	Status             string                   `json:"status"`
	RetryCount         int                      `json:"retry_count"`
	ExecutionToken     string                   `json:"execution_token,omitempty"`
	OutputRefs         []v1TaskOutputRefDTO     `json:"output_refs,omitempty"`
	BillingSnapshot    v1TaskBillingSnapshotDTO `json:"billing_snapshot"`
	ErrorMsg           string                   `json:"error_msg,omitempty"`
	CancelReason       string                   `json:"cancel_reason,omitempty"`
	CreatedAt          string                   `json:"created_at"`
	UpdatedAt          string                   `json:"updated_at"`
	CompletedAt        *string                  `json:"completed_at,omitempty"`
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
		ID: task.ID, WorkspaceID: task.WorkspaceID, Service: task.Service,
		MediaAssetIDs: append([]string(nil), task.MediaAssetIDs...),
		BoardID:       task.BoardID, BoardAccessVersion: task.BoardAccessVersion,
		Status: string(task.Status), RetryCount: task.RetryCount, ExecutionToken: task.ExecutionToken,
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

func (h *contractV1Handler) createWorkspaceTask(c *gin.Context) {
	h.withWorkspace(c, v1WorkspaceOpts{Require: h.requireStoreAndTasks, Resolve: h.resolveTaskWorkspace}, func(wctx v1WorkspaceCtx) {
		identityPrincipal := wctx.Principal
		workspace := wctx.Workspace
		key, ok := requireV1IdempotencyKey(c)
		if !ok {
			return
		}
		var body v1CreateTaskBody
		if bindErr := contract.BindStrictJSON(c, &body); bindErr != nil {
			writeV1InputError(c)
			return
		}
		kind := strings.TrimSpace(body.Data.Service)
		assetIDs := append([]string(nil), body.Data.Inputs.MediaAssetIDs...)
		for i := range assetIDs {
			assetIDs[i] = strings.TrimSpace(assetIDs[i])
		}
		prompt := strings.TrimSpace(body.Data.Prompt)
		if err := workspacetasks.ValidateWorkspacePrompt(prompt); err != nil {
			renderTaskError(c, err)
			return
		}
		// Board binding (#937): malformed board_ shapes fail closed as
		// 422; the domain resolves visibility/generation against the
		// live Board posture (invisible -> 404 board_not_found, archived
		// -> 409 board_archived, no generate capability -> 403
		// task_forbidden). Omitted Board IDs stay non-Canvas tasks.
		boardID := strings.TrimSpace(body.Data.BoardID)
		if err := workspacetasks.ValidateBoardID(boardID); err != nil {
			renderTaskError(c, err)
			return
		}
		var workspaceRole string
		if boardID != "" {
			role, ok := h.taskWorkspaceRoleString(c.Request.Context(), workspace, identityPrincipal.UserID())
			if !ok {
				contract.RenderError(c, http.StatusNotFound, contract.CodeTaskNotFound, runtimehttp.MsgControlNotFound8)
				c.Abort()
				return
			}
			workspaceRole = role
		}
		var quantity int64 = 1
		if body.Data.Quantity != nil {
			quantity = *body.Data.Quantity
		}
		// Canonicalize the request once at ingress. `params.output_resolution`
		// is the durable source of truth; the top-level field is accepted as
		// an envelope convenience and is folded into the params map before
		// compile, hashing, routing, billing, and persistence.
		params := make(map[string]any, len(body.Data.Params)+1)
		for field, value := range body.Data.Params {
			params[field] = value
		}
		resolution := ""
		if body.Data.Resolution != nil {
			resolution = strings.TrimSpace(*body.Data.Resolution)
		}
		if raw, ok := params[paramkeys.OutputResolution]; ok && raw != nil {
			planResolution, isString := raw.(string)
			if isString && strings.TrimSpace(planResolution) != "" {
				canonical, valid := workflow.NormalizeOutputResolution(planResolution)
				if !valid {
					// Preserve malformed values for the canonical compiler or
					// reject them here when the envelope also supplied a value.
					if resolution != "" {
						renderTaskError(c, &workspacetasks.ValidationError{
							Fields: map[string]string{"resolution": "invalid"},
						})
						return
					}
					resolution = strings.TrimSpace(planResolution)
				} else {
					params[paramkeys.OutputResolution] = canonical
					if resolution == "" {
						resolution = canonical
					} else if resolutionCanonical, topValid := workflow.NormalizeOutputResolution(resolution); !topValid || resolutionCanonical != canonical {
						renderTaskError(c, &workspacetasks.ValidationError{
							Fields: map[string]string{"resolution": "mismatch"},
						})
						return
					} else {
						resolution = resolutionCanonical
					}
				}
			} else if resolution != "" {
				// Keep the canonical compiler as the authority for malformed
				// params values; a top-level value must not silently override it.
				renderTaskError(c, &workspacetasks.ValidationError{
					Fields: map[string]string{"resolution": "mismatch"},
				})
				return
			}
		}
		if resolution != "" {
			canonical, valid := workflow.NormalizeOutputResolution(resolution)
			if !valid {
				renderTaskError(c, &workspacetasks.ValidationError{
					Fields: map[string]string{"resolution": "invalid"},
				})
				return
			}
			resolution = canonical
			if _, exists := params[paramkeys.OutputResolution]; !exists {
				params[paramkeys.OutputResolution] = canonical
			}
		}
		// Canonical request hash for idempotent replay (same key+hash ->
		// same task_id; conflict hash -> 409).
		paramsJSON, _ := json.Marshal(params)
		canonicalParams := ""
		if h.deps.TaskCanonicalCompiler != nil {
			var compileErr error
			canonicalInput := make(map[string]interface{}, len(params)+1)
			for key, value := range params {
				canonicalInput[key] = value
			}
			if len(assetIDs) > 0 {
				canonicalInput[taskservice.CompilerWorkspaceMediaAssetIDsParam] = append([]string(nil), assetIDs...)
			}
			canonicalParams, compileErr = h.deps.TaskCanonicalCompiler(kind, prompt, canonicalInput)
			if compileErr != nil {
				renderTaskError(c, compileErr)
				return
			}
		}
		hash := contract.HashIdempotencyRequest("task:create", workspace.PublicID, kind,
			strings.Join(assetIDs, ","), prompt, string(paramsJSON),
			strconv.FormatInt(quantity, 10), resolution, boardID)
		externalUser := tenancy.FormatUserPublicID(identityPrincipal.PublicID())
		// The durable task claim is workspace-scoped. Include the workspace in
		// the process-local fast-path scope as well, otherwise the same user
		// reusing a key in another workspace can be rejected by this cache even
		// though the durable claims are independent.
		scope := contract.IdempotencyScope(externalUser+":"+workspace.PublicID, "task:create")
		service := h.taskService()
		result, _, doErr := h.deps.Idempotency.Do(c.Request.Context(), scope, identityPrincipal.UserID(), key, hash, func() (any, error) {
			created, err := service.Create(c.Request.Context(), workspacetasks.CreateInput{
				WorkspaceID: workspace.PublicID, OwnerUserID: identityPrincipal.UserID(),
				Service: kind, MediaAssetIDs: assetIDs, Params: params,
				Quantity: quantity, Resolution: resolution, CanonicalParams: canonicalParams,
				// The durable claim stores this digest so a repeated key with a
				// different body is a 409 rather than a replay of the wrong task.
				RequestHash: hash, Prompt: prompt,
				BoardID: boardID, WorkspaceRole: workspaceRole,
			}, key)
			if err != nil {
				return nil, err
			}
			// #807: emit AFTER the settlement commit, inside the once-fn so
			// idempotent replays never re-emit. Staged pending maps to
			// task.created; the hold/free ref maps to billing.settled.
			for _, staged := range service.StagedEvents() {
				typ, ok := events.TaskStatusV1Type(events.TaskStatus(staged.Status))
				if !ok {
					continue
				}
				h.emitWorkspaceEventWithID(c.Request.Context(), identityPrincipal, staged.WorkspaceID, typ,
					events.V1Resource{Type: events.V1ResourceTask, ID: staged.TaskID},
					events.V1TaskCreatedData{TaskID: staged.TaskID, Service: kind}, staged.EventID, staged.OccurredAt)
			}
			h.emitTaskBillingSettled(c.Request.Context(), identityPrincipal,
				created.WorkspaceID, created.ID,
				created.BillingSnapshot.SettlementState, created.BillingSnapshot.HoldTransactionID)
			dto := toTaskDTO(created)
			return &dto, nil
		})
		if doErr != nil {
			if contract.IsIdempotencyConflict(doErr) {
				contract.RenderError(c, http.StatusConflict, contract.CodeTaskIdempotencyConflict, runtimehttp.MsgWorkspaceIdempotencyConflict)
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
	})
}

func (h *contractV1Handler) listWorkspaceTasks(c *gin.Context) {
	h.withWorkspace(c, v1WorkspaceOpts{Require: h.requireStoreAndTasks, Resolve: h.resolveTaskWorkspace}, func(wctx v1WorkspaceCtx) {
		identityPrincipal := wctx.Principal
		workspace := wctx.Workspace
		pageSize, ok := v1PageSize(c)
		if !ok {
			return
		}
		offset := 0
		if raw := strings.TrimSpace(c.Query(contract.CursorParam)); raw != "" {
			cursor, decodeErr := contract.DecodeCursor[v1ListCursor](raw)
			if decodeErr != nil || cursor == nil || cursor.Offset < 0 {
				contract.RenderError(c, http.StatusBadRequest, contract.CodePaginationCursorInvalid, runtimehttp.MsgPaginationCursorInvalid3)
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
		// #937 visibility filter: the submitter always sees their own
		// tasks; other tasks surface only when the caller currently holds
		// Board read on the linked Board. Board-less tasks by other
		// submitters are silently skipped (never leaked, never errored),
		// so omitting the Board ID cannot degrade into workspace-wide
		// visibility. The workspace role resolves once for the page.
		callerID := identityPrincipal.UserID()
		workspaceRole, _ := h.taskWorkspaceRoleString(c.Request.Context(), workspace, callerID)
		visible := make([]*workspacetasks.WorkspaceTask, 0, len(tasks))
		for _, task := range tasks {
			if task != nil && task.OwnerUserID == callerID {
				visible = append(visible, task)
				continue
			}
			access := h.resolveTaskBoardAccess(c.Request.Context(), workspace, workspaceRole, callerID, task)
			if workspacetasks.AuthorizeTaskRead(task, callerID, access) {
				visible = append(visible, task)
			}
		}
		page := make([]v1TaskDTO, 0, len(visible))
		for _, task := range visible {
			page = append(page, toTaskDTO(task))
		}
		// Cursor pagination stays store-ordered: hasMore derives from the
		// store page (a filtered page may be shorter without more data).
		storeHasMore := len(tasks) > pageSize
		meta := contract.NewPageMeta("", false)
		if storeHasMore {
			next, encodeErr := contract.EncodeCursor(v1ListCursor{Offset: offset + pageSize})
			if encodeErr != nil {
				renderTaskError(c, encodeErr)
				return
			}
			meta = contract.NewPageMeta(next, true)
		}
		contract.RenderPaginated(c, page, meta)
	})
}

func (h *contractV1Handler) getWorkspaceTask(c *gin.Context) {
	h.withWorkspace(c, v1WorkspaceOpts{Require: h.requireStoreAndTasks, Resolve: h.resolveTaskWorkspace}, func(wctx v1WorkspaceCtx) {
		identityPrincipal := wctx.Principal
		workspace := wctx.Workspace
		raw := strings.TrimSpace(c.Param("task_id"))
		if !contract.IsTaskID(raw) {
			contract.RenderError(c, http.StatusNotFound, contract.CodeTaskNotFound, runtimehttp.MsgControlNotFound8)
			c.Abort()
			return
		}
		task, err := h.deps.Tasks.GetTask(c.Request.Context(), workspace.PublicID, raw)
		if err != nil {
			renderTaskError(c, err)
			return
		}
		// #937: task content is submitter- or Board-reader-only.
		callerID := identityPrincipal.UserID()
		workspaceRole, _ := h.taskWorkspaceRoleString(c.Request.Context(), workspace, callerID)
		if !h.requireVisibleWorkspaceTask(c, workspace, workspaceRole, callerID, task) {
			return
		}
		contract.RenderSuccess(c, toTaskDTO(task))
	})
}

type v1CancelTaskBody struct {
	Data *struct {
		Reason *string `json:"reason"`
	} `json:"data"`
}

func (h *contractV1Handler) cancelWorkspaceTask(c *gin.Context) {
	h.withWorkspace(c, v1WorkspaceOpts{Require: h.requireStoreAndTasks, Resolve: h.resolveTaskWorkspace}, func(wctx v1WorkspaceCtx) {
		identityPrincipal := wctx.Principal
		workspace := wctx.Workspace
		raw := strings.TrimSpace(c.Param("task_id"))
		if !contract.IsTaskID(raw) {
			contract.RenderError(c, http.StatusNotFound, contract.CodeTaskNotFound, runtimehttp.MsgControlNotFound8)
			c.Abort()
			return
		}
		key, ok := requireV1IdempotencyKey(c)
		if !ok {
			return
		}
		// #937: cancellation needs the task to be visible AND the caller
		// to hold cancel rights (submitter or Board writer). Readers and
		// outsiders converge to 404 task_not_found before any idempotency
		// state is recorded.
		existing, err := h.deps.Tasks.GetTask(c.Request.Context(), workspace.PublicID, raw)
		if err != nil {
			renderTaskError(c, err)
			return
		}
		callerID := identityPrincipal.UserID()
		workspaceRole, _ := h.taskWorkspaceRoleString(c.Request.Context(), workspace, callerID)
		access := h.resolveTaskBoardAccess(c.Request.Context(), workspace, workspaceRole, callerID, existing)
		if !workspacetasks.AuthorizeTaskCancel(existing, callerID, access) {
			contract.RenderError(c, http.StatusNotFound, contract.CodeTaskNotFound, runtimehttp.MsgControlNotFound8)
			c.Abort()
			return
		}
		reason := "user cancel"
		if c.Request.ContentLength != 0 {
			var body v1CancelTaskBody
			if bindErr := contract.BindStrictJSON(c, &body); bindErr != nil {
				writeV1InputError(c)
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
					typ, ok := events.TaskStatusV1Type(events.TaskStatus(staged.Status))
					if !ok {
						continue
					}
					h.emitWorkspaceEventWithID(c.Request.Context(), identityPrincipal, staged.WorkspaceID, typ,
						events.V1Resource{Type: events.V1ResourceTask, ID: staged.TaskID},
						events.V1TaskCancelledData{TaskID: staged.TaskID, CancelReason: reason}, staged.EventID, staged.OccurredAt)
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
				contract.RenderError(c, http.StatusConflict, contract.CodeTaskIdempotencyConflict, runtimehttp.MsgWorkspaceIdempotencyConflict)
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
	})
}

func (h *contractV1Handler) listWorkspaceTaskOutputs(c *gin.Context) {
	h.withWorkspace(c, v1WorkspaceOpts{Require: h.requireStoreAndTasks, Resolve: h.resolveTaskWorkspace}, func(wctx v1WorkspaceCtx) {
		identityPrincipal := wctx.Principal
		workspace := wctx.Workspace
		raw := strings.TrimSpace(c.Param("task_id"))
		if !contract.IsTaskID(raw) {
			contract.RenderError(c, http.StatusNotFound, contract.CodeTaskNotFound, runtimehttp.MsgControlNotFound8)
			c.Abort()
			return
		}
		task, err := h.deps.Tasks.GetTask(c.Request.Context(), workspace.PublicID, raw)
		if err != nil {
			renderTaskError(c, err)
			return
		}
		// #937: outputs inherit task visibility.
		callerID := identityPrincipal.UserID()
		workspaceRole, _ := h.taskWorkspaceRoleString(c.Request.Context(), workspace, callerID)
		if !h.requireVisibleWorkspaceTask(c, workspace, workspaceRole, callerID, task) {
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
	})
}

type v1QuoteTaskBody struct {
	Data struct {
		Service    string  `json:"service"`
		Quantity   *int64  `json:"quantity"`
		Resolution *string `json:"resolution"`
		// BoardID optionally binds a Canvas readiness preflight to its
		// Board (#917): the same generate-capability gate submit enforces
		// applies here, so Viewers never read ready for a Board they
		// cannot generate on. Omitted stays a workspace-level price
		// preflight with no Board attribution.
		BoardID string `json:"board_id,omitempty"`
	} `json:"data"`
}

// quoteWorkspaceTask is the preflight seam: it returns an immutable price
// snapshot with no persistence (separate from authoritative create). It
// spends no Credits, but it resolves the SAME stable Billing Account
// submit would hold (#917) and applies the same consumption eligibility:
// workspace membership (404 task_not_found when hidden), the Board
// generate gate when data.board_id is present (404 board_not_found, 409
// board_archived, 403 task_forbidden), the shared pool availability
// decision (422 service_resolution_unavailable), and the Wallet funds
// gate (402 task_insufficient_credits vs 403 billing_wallet_restricted,
// mirroring the submit Hold mapping, including the frozen passthrough).
// Financial management alone grants no Board standing: a caller without
// Board generate capability fails here exactly as on submit. The snapshot
// stays price-only (no balances ride it); callers refresh the accurate
// account balance through the workspace billing reads, which resolve the
// same stable account.
func (h *contractV1Handler) quoteWorkspaceTask(c *gin.Context) {
	withV1WorkspaceBody(h, c, v1WorkspaceOpts{Require: h.requireStoreAndTasks, Resolve: h.resolveTaskWorkspace}, func(wctx v1WorkspaceCtx, body v1QuoteTaskBody) {
		identityPrincipal := wctx.Principal
		workspace := wctx.Workspace
		quantity := int64(1)
		if body.Data.Quantity != nil {
			quantity = *body.Data.Quantity
		}
		resolution := ""
		if body.Data.Resolution != nil && strings.TrimSpace(*body.Data.Resolution) != "" {
			resolution = strings.TrimSpace(*body.Data.Resolution)
		}
		// Board generate gate first (same BoardPolicy submit decision as
		// the settlement transaction): Viewers never read ready.
		boardID := strings.TrimSpace(body.Data.BoardID)
		if boardID != "" {
			if err := workspacetasks.ValidateBoardID(boardID); err != nil {
				renderTaskError(c, err)
				return
			}
			workspaceRole, ok := h.taskWorkspaceRoleString(c.Request.Context(), workspace, identityPrincipal.UserID())
			if !ok {
				contract.RenderError(c, http.StatusNotFound, contract.CodeTaskNotFound, runtimehttp.MsgControlNotFound8)
				c.Abort()
				return
			}
			authorizer := h.taskBoardsAuthorizer()
			if authorizer == nil {
				renderTaskError(c, errors.New("tasks: board authorizer unavailable"))
				return
			}
			access, err := authorizer.ResolveBoardAccess(c.Request.Context(), workspace.PublicID, boardID, identityPrincipal.UserID(), workspaceRole)
			if err != nil {
				renderTaskError(c, err)
				return
			}
			var facts workspacetasks.BoardTaskFacts
			if access != nil {
				facts = workspacetasks.BoardTaskFacts{
					CanRead: access.CanRead, CanGenerate: access.CanGenerate,
					CanWrite: access.CanWrite, Archived: access.Archived,
				}
			}
			switch workspacetasks.BoardPolicy.Check(facts, workspacetasks.BoardOpSubmit) {
			case workspacetasks.BoardDecisionAllow:
			case workspacetasks.BoardDecisionArchived:
				renderTaskError(c, workspacetasks.ErrBoardArchived)
				return
			case workspacetasks.BoardDecisionForbidden:
				renderTaskError(c, workspacetasks.ErrForbidden)
				return
			default:
				renderTaskError(c, workspacetasks.ErrBoardNotFound)
				return
			}
		}
		// Same stable payer submit would hold: no owner-wallet fallback.
		if h.deps.TaskQuoteBillingAccounts == nil || h.deps.TaskQuoteWallets == nil {
			contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable25)
			c.Abort()
			return
		}
		account, err := h.deps.TaskQuoteBillingAccounts.ResolveFunds(c.Request.Context(), workspace.PublicID)
		if err != nil {
			renderBillingAccountError(c, err)
			return
		}
		accountID := account.BillingAccountID
		service := h.taskService()
		snapshot, err := service.Quote(c.Request.Context(), workspacetasks.QuoteParams{
			Service: strings.TrimSpace(body.Data.Service), Quantity: quantity, Resolution: resolution,
		})
		if err != nil {
			renderTaskError(c, err)
			return
		}
		// Funds gate on the resolved account (submit Hold parity via
		// workspacetasks.CheckFundsGate): active restrictions deny first,
		// then sufficiency. Frozen and missing wallets pass through
		// exactly as the submit Hold maps them (no second mapping to drift).
		wallet, err := h.deps.TaskQuoteWallets.GetWallet(c.Request.Context(), accountID)
		if err != nil {
			if errors.Is(err, domain.ErrWalletRestricted) {
				renderTaskError(c, workspacetasks.ErrWalletRestricted)
				return
			}
			renderTaskError(c, err)
			return
		}
		if err := workspacetasks.CheckFundsGate(wallet.Available, snapshot.TotalCredits, len(wallet.ActiveRestrictions) > 0); err != nil {
			renderTaskError(c, err)
			return
		}
		// Member quota gate (#946: same cap submit enforces, reported
		// here as not-ready before any Hold). Unlimited (nil reader or
		// no quota row) passes through; over-cap renders 403
		// member_quota_exhausted via renderTaskError.
		if h.deps.TaskQuoteQuotas != nil && snapshot.TotalCredits > 0 {
			if err := h.deps.TaskQuoteQuotas.CheckQuotaForQuote(c.Request.Context(), accountID, identityPrincipal.UserID(), snapshot.TotalCredits, snapshot.QuotedAt); err != nil {
				renderTaskError(c, err)
				return
			}
		}
		contract.RenderSuccess(c, v1TaskBillingSnapshotDTO{
			SKU: snapshot.SKU, DisplayName: snapshot.DisplayName,
			PriceVersionID: snapshot.PriceVersionID, PriceVersion: snapshot.PriceVersion,
			OutputResolution: snapshot.OutputResolution, UnitCredits: snapshot.UnitCredits,
			Quantity: snapshot.Quantity, TotalCredits: snapshot.TotalCredits,
			QuotedAt: v1FormatTime(snapshot.QuotedAt), SettlementState: "quoted",
		})
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
//	  post: {operationId: quoteWorkspaceTask, parameters: [WorkspaceId]
//	         (no Idempotency-Key: pure read, safe to retry)}
//
// QuoteWorkspaceTaskRequest carries the same stable payer and consumption
// eligibility submit enforces (#917): an optional data.board_id binds the
// preflight to its Board (invisible 404 board_not_found, archived 409
// board_archived, no generate capability 403 task_forbidden); the Wallet
// funds gate renders 402 task_insufficient_credits vs 403
// billing_wallet_restricted exactly as the submit Hold maps them.
//
// Schemas:
//
//	WorkspaceTask: {id: TaskId (task_...), workspace_id: WorkspaceId, service: {enum: [render,
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
//	CreateWorkspaceTaskRequest: {data: {service: string, inputs: {media_asset_ids?: asset_...[]},
//	  prompt?: string (max 8KB bytes; required for generation/inpaint, optional for
//	  render/variations/fusion/outpaint, ignored for cutout/upscale),
//	  params?: object (params.mask_asset_id?: asset_... — inpaint only; data-URL and
//	  storage-URL masks rejected 422, params.prompt still 422 unknown_field),
//	  quantity?: int64>=1, resolution?: string}}
//	  Breaking rename (#847): the old `workflow_kind` field is rejected by
//	  strict decoding as 400 bad_request; a missing `service` renders 422
//	  validation_failed on details.fields.service.
//	  Resolution agreement (#851, ADR-0012 rule 11): params.output_resolution
//	  is the client source of truth. An explicit top-level `resolution` must
//	  agree with it exactly (case-insensitive) or submit fails 422
//	  validation_failed on details.fields.resolution; an absent top level
//	  carries the params value through, and an absent pair applies the
//	  omitted resolutions are normalized to the Service policy's canonical 1k/2k/4k value.
//	QuoteWorkspaceTaskRequest: {data: {service: string, quantity?: int64>=1, resolution?: string, board_id?: BoardId (optional Canvas binding; the preflight enforces the same generate gate as submit)}}
//
// Prompt/mask spec notes for the merger (v1 task creation gap batch):
//   - data.prompt is new; the idempotency hash covers it (same key +
//     different prompt -> 409 task_idempotency_conflict, not a silent replay).
//   - params.mask_asset_id is new; the only durable mask path (mask bytes
//     arrive via the existing upload-session flow, linked here by asset id).
//     Inpaint without a mask, or a mask on any non-inpaint feature, is 422
//     validation_failed (plan media policy). Mask assets gate on the same
//     ready/archived semantics as inputs (404 media_not_found, 409
//     media_not_ready, 410 media_archived).
//   - Worker Generate bridges CanonicalParams Generation.compiled_prompt
//     (fallback user_prompt, then legacy params.prompt) into the gateway
//     TaskPayload.Prompt; provider mask refs resolve from the canonical
//     Media.mask public asset id through the workspace runtime media
//     resolver (no params/Extra URL plumbing).
//
// Error codes frozen: task_not_found (404 hidden), task_forbidden (403),
// task_state_conflict (409), task_insufficient_credits (402, submit Hold
// and quote preflight alike),
// task_idempotency_conflict (409, kernel); reused kernel codes
// workspace_not_found/workspace_forbidden/pagination_cursor_invalid/bad_request/
// validation_failed/idempotency_key_required/service_unavailable plus
// billing_quote_unavailable (503 transient), billing_pricing_unavailable
// (422 unpriced SKU), billing_wallet_restricted (403),
// media_not_found (404), media_not_ready (409), media_archived (410).
