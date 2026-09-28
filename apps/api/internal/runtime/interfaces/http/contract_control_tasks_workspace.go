package httpapi

// Control task read model over workspace task stores (#809-close).
//
// The legacy task services were deleted with the #809 hard cut, so the
// control task routes (list/detail/cancel) had no Tasks dep and failed
// closed with 503. workspaceControlTaskAdapter implements
// ControlTaskAdmin from the workspace-scoped task store plus a small set
// of global control queries (see workspace_tasks.sql): operators see
// every workspace (capability auth + audit stay in the guard), unknown
// ids converge to not-found (never 403), and cancel records the
// operator as actor with the guard-enforced reason while releasing any
// credit hold through the settlement Service.
//
// Projection notes (documented for the merger/frontend):
//   - Prompt/params come from the workflow_input envelope (best effort:
//     params.prompt, else params.user_prompt, else empty).
//   - Result has no column in the workspace schema and stays empty;
//     outputs carry asset refs (no delivery URLs: media delivery is the
//     media workstream's seam, never fabricated here).
//   - LatestExecution stays nil (no lease snapshot in the workspace
//     schema); execution attempts and metrics read live from
//     task_execution_attempts / task_execution_metrics.

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"ideaboard/api/internal/db/dbutil"
	dbsqlc "ideaboard/api/internal/db/sqlc"
	"ideaboard/api/internal/platform/database"
	taskapp "ideaboard/api/internal/tasking/application/task"
	tasksdomain "ideaboard/api/internal/tasking/domain"
	taskpostgres "ideaboard/api/internal/tasking/infrastructure/postgres"
	"ideaboard/api/internal/tasking/workspacetasks"
	"ideaboard/api/internal/tenancy"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// workspaceControlTaskAdapter is the ControlTaskAdmin implementation over
// workspace task stores. Repository methods take context.Context first.
type workspaceControlTaskAdapter struct {
	db     *pgxpool.Pool
	q      *dbsqlc.Queries
	store  *taskpostgres.Store
	holder workspacetasks.Holder
	users  ControlCreditDirectory
}

// NewWorkspaceControlTasks builds the control task read model. A nil db
// keeps every method fail-closed (the handlers converge to 503), matching
// the nil-dep convention of the other control seams.
func NewWorkspaceControlTasks(db *pgxpool.Pool, store *taskpostgres.Store, holder workspacetasks.Holder, users ControlCreditDirectory) *workspaceControlTaskAdapter {
	if db == nil {
		return &workspaceControlTaskAdapter{store: store, holder: holder, users: users}
	}
	return &workspaceControlTaskAdapter{db: db, q: dbsqlc.New(db), store: store, holder: holder, users: users}
}

// Compile-time conformance with the control seam.
var _ ControlTaskAdmin = (*workspaceControlTaskAdapter)(nil)

// isControlTaskNotFound reports invisible tasks (unknown id, malformed
// shape, or cross-workspace id).
func isControlTaskNotFound(err error) bool {
	return errors.Is(err, workspacetasks.ErrNotFound) ||
		errors.Is(err, tenancy.ErrNotFound) ||
		database.IsNotFound(err)
}

// isControlTaskConflict reports terminal-state transitions.
func isControlTaskConflict(err error) bool {
	return errors.Is(err, workspacetasks.ErrStateConflict)
}

// controlTaskEnvelope is the tasks.workflow_input JSONB shape read back
// for the control projection (mirrors the store envelope; cancel reason
// rides here because tasks carries no cancel text column).
type controlTaskEnvelope struct {
	Params          map[string]any `json:"params,omitempty"`
	CanonicalParams string         `json:"canonical_params,omitempty"`
	CancelReason    string         `json:"cancel_reason,omitempty"`
}

func parseControlTaskEnvelope(raw []byte) controlTaskEnvelope {
	var envelope controlTaskEnvelope
	if len(raw) == 0 {
		return envelope
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return controlTaskEnvelope{}
	}
	return envelope
}

func controlTaskPrompt(envelope controlTaskEnvelope) string {
	for _, key := range []string{"prompt", "user_prompt"} {
		if raw, ok := envelope.Params[key].(string); ok && strings.TrimSpace(raw) != "" {
			return strings.TrimSpace(raw)
		}
	}
	return ""
}

func controlTaskParamsString(envelope controlTaskEnvelope) string {
	if strings.TrimSpace(envelope.CanonicalParams) != "" {
		return envelope.CanonicalParams
	}
	if len(envelope.Params) == 0 {
		return ""
	}
	raw, err := json.Marshal(envelope.Params)
	if err != nil {
		return ""
	}
	return string(raw)
}

func (a *workspaceControlTaskAdapter) queries() *dbsqlc.Queries {
	if a == nil || a.q == nil {
		return nil
	}
	return a.q
}

// resolveControlTaskIdentity maps an owner user id to its public identity
// for control projections. Unresolvable owners degrade to zero identity
// (never fail the whole page); cancelling never depends on it.
func (a *workspaceControlTaskAdapter) resolveControlTaskIdentity(ctx context.Context, userID int64) (uuid.UUID, string) {
	if a == nil || a.users == nil || userID <= 0 {
		return uuid.Nil, ""
	}
	identity, err := a.users.ResolveID(ctx, userID)
	if err != nil {
		return uuid.Nil, ""
	}
	return identity.PublicID, identity.DisplayName
}

func (a *workspaceControlTaskAdapter) GetTask(ctx context.Context, taskID string) (*taskapp.AdminMonitorDetail, error) {
	q := a.queries()
	if q == nil {
		return nil, workspacetasks.ErrNotFound
	}
	taskID = strings.TrimSpace(taskID)
	row, err := q.GetControlTaskByPublicID(ctx, taskID)
	if err != nil {
		return nil, workspacetasks.ErrNotFound
	}
	envelope := parseControlTaskEnvelope(row.WorkflowInput)
	publicID, displayName := a.resolveControlTaskIdentity(ctx, row.SubmitterUserID)
	completed := dbutil.TimePtr(row.CompletedAt)
	if row.Status == string(workspacetasks.StatusCancelled) && completed == nil {
		completed = dbutil.TimePtr(row.CancelledAt)
	}
	detail := &taskapp.AdminMonitorDetail{
		TaskID:          row.PublicID,
		UserPublicID:    publicID,
		UserDisplayName: displayName,
		Prompt:          controlTaskPrompt(envelope),
		Params:          controlTaskParamsString(envelope),
		FeatureSlug:     row.WorkflowType,
		Priority:        int(row.Priority),
		Status:          row.Status,
		RetryCount:      int(row.RetryCount),
		ErrorMsg:        row.ErrorMessage,
		CreatedAt:       dbutil.TimeValue(row.CreatedAt),
		UpdatedAt:       dbutil.TimeValue(row.UpdatedAt),
		CompletedAt:     completed,
		CancelReason:    envelope.CancelReason,
	}
	if row.Status == string(workspacetasks.StatusCancelled) {
		detail.CancelledAt = dbutil.TimePtr(row.CancelledAt)
	}
	if outputs, err := a.loadControlTaskOutputs(ctx, row.WorkspacePublicID, row.PublicID); err == nil {
		detail.Outputs = outputs
	}
	if billing, ok, err := a.loadControlTaskBilling(ctx, row.PublicID); err == nil && ok {
		detail.Billing = billing
	}
	if metrics, err := a.loadControlTaskMetrics(ctx, row.PublicID); err == nil {
		detail.ExecutionMetrics = metrics
	}
	if attempts, err := a.loadControlTaskAttempts(ctx, row.PublicID); err == nil {
		detail.ExecutionAttempts = attempts
		if len(attempts) > 0 {
			latest := attempts[len(attempts)-1]
			detail.LatestExecution = &tasksdomain.AdminTaskExecutionSnapshot{
				ResolvedModel: latest.ResolvedModel,
				Provider:      latest.Provider,
				GatewayStep:   latest.GatewayStep,
			}
		}
	}
	if detail.ExecutionAttempts == nil {
		detail.ExecutionAttempts = []tasksdomain.AdminTaskAttempt{}
	}
	return detail, nil
}

func (a *workspaceControlTaskAdapter) ListTasks(ctx context.Context, req *tasksdomain.TaskListRequest) ([]taskapp.AdminMonitorListItem, tasksdomain.TaskListCounts, error) {
	q := a.queries()
	if q == nil {
		return nil, tasksdomain.TaskListCounts{}, workspacetasks.ErrNotFound
	}
	if req == nil {
		req = &tasksdomain.TaskListRequest{}
	}
	page := req.Page
	if page < 1 {
		page = 1
	}
	pageSize := req.PageSize
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	status := strings.ToLower(strings.TrimSpace(req.Status))
	switch status {
	case "", string(workspacetasks.StatusPending), string(workspacetasks.StatusRunning),
		string(workspacetasks.StatusCompleted), string(workspacetasks.StatusFailed), string(workspacetasks.StatusCancelled):
	default:
		return nil, tasksdomain.TaskListCounts{}, workspacetasks.ErrNotFound
	}
	keyword := strings.TrimSpace(req.Keyword)
	if len([]rune(keyword)) > 200 {
		keyword = string([]rune(keyword)[:200])
	}
	rows, err := q.ListControlTasks(ctx, dbsqlc.ListControlTasksParams{
		Status:          status,
		SubmitterUserID: req.UserID,
		Keyword:         keyword,
		RowLimit:        int32(pageSize),
		RowOffset:       int32((page - 1) * pageSize),
	})
	if err != nil {
		return nil, tasksdomain.TaskListCounts{}, err
	}
	countRows, err := q.CountControlTasks(ctx, dbsqlc.CountControlTasksParams{
		SubmitterUserID: req.UserID,
		Keyword:         keyword,
	})
	if err != nil {
		return nil, tasksdomain.TaskListCounts{}, err
	}
	counts := tasksdomain.TaskListCounts{}
	for _, countRow := range countRows {
		counts.Total += countRow.Count
		switch countRow.Status {
		case string(workspacetasks.StatusPending):
			counts.PendingCount += countRow.Count
		case string(workspacetasks.StatusRunning):
			counts.RunningCount += countRow.Count
		case string(workspacetasks.StatusCompleted):
			counts.CompletedCount += countRow.Count
		case string(workspacetasks.StatusFailed):
			counts.FailedCount += countRow.Count
		case string(workspacetasks.StatusCancelled):
			counts.CancelledCount += countRow.Count
		default:
			counts.OtherCount += countRow.Count
		}
	}
	items := make([]taskapp.AdminMonitorListItem, 0, len(rows))
	for _, row := range rows {
		envelope := parseControlTaskEnvelope(row.WorkflowInput)
		publicID, displayName := a.resolveControlTaskIdentity(ctx, row.SubmitterUserID)
		completed := dbutil.TimePtr(row.CompletedAt)
		if row.Status == string(workspacetasks.StatusCancelled) && completed == nil {
			completed = dbutil.TimePtr(row.CancelledAt)
		}
		item := taskapp.AdminMonitorListItem{
			TaskID:          row.PublicID,
			UserPublicID:    publicID,
			UserDisplayName: displayName,
			Prompt:          controlTaskPrompt(envelope),
			FeatureSlug:     row.WorkflowType,
			Priority:        int(row.Priority),
			Status:          row.Status,
			ErrorMsg:        row.ErrorMessage,
			CreatedAt:       dbutil.TimeValue(row.CreatedAt),
			CompletedAt:     completed,
		}
		if row.SettlementState.Valid && row.SettlementState.String == "captured" && row.SettlementCapturedCredits.Valid {
			item.Credits = row.SettlementCapturedCredits.Int64
		}
		items = append(items, item)
	}
	return items, counts, nil
}

func (a *workspaceControlTaskAdapter) ResolveAdminTaskUserID(ctx context.Context, rawPublicID string) (int64, error) {
	rawPublicID = strings.TrimSpace(rawPublicID)
	if rawPublicID == "" {
		return 0, tenancy.ErrNotFound
	}
	publicID, err := tenancy.ParseUserPublicID(rawPublicID)
	if err != nil || publicID == uuid.Nil {
		return 0, tenancy.ErrNotFound
	}
	if a == nil || a.users == nil {
		return 0, tenancy.ErrNotFound
	}
	identity, err := a.users.ResolvePublicID(ctx, rawPublicID)
	if err != nil {
		return 0, tenancy.ErrNotFound
	}
	if identity.ID <= 0 {
		return 0, tenancy.ErrNotFound
	}
	return identity.ID, nil
}

func (a *workspaceControlTaskAdapter) CancelAdminTask(ctx context.Context, taskID string, adminID int64, reason string) error {
	q := a.queries()
	if q == nil || a.store == nil {
		return workspacetasks.ErrNotFound
	}
	taskID = strings.TrimSpace(taskID)
	row, err := q.GetControlTaskByPublicID(ctx, taskID)
	if err != nil {
		return workspacetasks.ErrNotFound
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = "cancelled"
	}
	settler := workspacetasks.NewService(workspacetasks.ServiceDeps{
		Tasks: a.store, Holder: a.holder,
	})
	_, _, err = settler.Cancel(ctx, row.WorkspacePublicID, taskID, adminID, reason)
	return err
}

// controlTaskOutputMeta is the task_outputs.metadata shape (asset refs
// ride in metadata; the row carries only the internal FK).
type controlTaskOutputMeta struct {
	AssetID        string `json:"asset_id,omitempty"`
	ChecksumSHA256 string `json:"checksum_sha256,omitempty"`
	Role           string `json:"role,omitempty"`
}

func (a *workspaceControlTaskAdapter) loadControlTaskOutputs(ctx context.Context, workspacePublicID, taskPublicID string) ([]tasksdomain.TaskOutput, error) {
	q := a.queries()
	if q == nil {
		return nil, workspacetasks.ErrNotFound
	}
	rows, err := q.ListWorkspaceTaskOutputs(ctx, dbsqlc.ListWorkspaceTaskOutputsParams{
		PublicID:          taskPublicID,
		WorkspacePublicID: workspacePublicID,
	})
	if err != nil {
		return nil, err
	}
	outputs := make([]tasksdomain.TaskOutput, 0, len(rows))
	for _, row := range rows {
		var meta controlTaskOutputMeta
		if len(row.Metadata) > 0 {
			_ = json.Unmarshal(row.Metadata, &meta)
		}
		output := tasksdomain.TaskOutput{
			ID:             row.InternalID,
			TaskID:         taskPublicID,
			Role:           meta.Role,
			ResultIndex:    int(row.OutputIndex),
			ChecksumSHA256: meta.ChecksumSHA256,
			CreatedAt:      dbutil.TimeValue(row.CreatedAt),
			UpdatedAt:      dbutil.TimeValue(row.CreatedAt),
		}
		switch strings.TrimSpace(row.OutputKind) {
		case "media_asset":
			output.OutputKind = workspacetasks.OutputKindAsset
			output.AssetID = meta.AssetID
		case "remote_url":
			output.OutputKind = workspacetasks.OutputKindRemote
			output.URL = row.RemoteUrl.String
		default:
			output.OutputKind = strings.TrimSpace(row.OutputKind)
			output.URL = row.RemoteUrl.String
		}
		outputs = append(outputs, output)
	}
	return outputs, nil
}

func (a *workspaceControlTaskAdapter) loadControlTaskBilling(ctx context.Context, taskPublicID string) (*tasksdomain.AdminBillingSnapshot, bool, error) {
	q := a.queries()
	if q == nil {
		return nil, false, workspacetasks.ErrNotFound
	}
	row, err := q.GetControlTaskBilling(ctx, taskPublicID)
	if err != nil {
		return nil, false, nil
	}
	snapshot := &tasksdomain.AdminBillingSnapshot{
		SKU:              row.Sku,
		DisplayName:      row.DisplayName,
		PriceVersionID:   row.PriceVersionID,
		PriceVersion:     row.PriceVersion,
		OutputResolution: row.OutputResolution,
		UnitCredits:      row.UnitCredits,
		Quantity:         row.Quantity,
		TotalCredits:     row.TotalCredits,
		QuotedAt:         dbutil.TimeValue(row.QuotedAt),
	}
	if row.SettlementState.Valid {
		snapshot.SettlementState = row.SettlementState.String
	}
	if row.SettlementCapturedCredits.Valid {
		snapshot.CapturedCredits = row.SettlementCapturedCredits.Int64
	}
	if row.SettlementSettledAt.Valid {
		settled := row.SettlementSettledAt.Time
		snapshot.SettledAt = &settled
	}
	return snapshot, true, nil
}

func (a *workspaceControlTaskAdapter) loadControlTaskMetrics(ctx context.Context, taskPublicID string) (*tasksdomain.TaskExecutionMetrics, error) {
	if a == nil || a.db == nil {
		return nil, workspacetasks.ErrNotFound
	}
	var metrics tasksdomain.TaskExecutionMetrics
	err := a.db.QueryRow(ctx, `
SELECT task_id, current_stage, bottleneck_source, failure_stage, failure_category,
  assigned_at, provider_started_at, provider_finished_at, finalize_started_at, finalize_finished_at,
  queue_wait_ms, worker_overhead_ms, provider_latency_ms, finalize_latency_ms, total_latency_ms,
  created_at, updated_at
FROM task_execution_metrics
WHERE task_id = $1
LIMIT 1`, taskPublicID).Scan(
		&metrics.TaskID, &metrics.CurrentStage, &metrics.BottleneckSource, &metrics.FailureStage, &metrics.FailureCategory,
		&metrics.AssignedAt, &metrics.ProviderStartedAt, &metrics.ProviderFinishedAt, &metrics.FinalizeStartedAt, &metrics.FinalizeFinishedAt,
		&metrics.QueueWaitMS, &metrics.WorkerOverheadMS, &metrics.ProviderLatencyMS, &metrics.FinalizeLatencyMS, &metrics.TotalLatencyMS,
		&metrics.CreatedAt, &metrics.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &metrics, nil
}

func (a *workspaceControlTaskAdapter) loadControlTaskAttempts(ctx context.Context, taskPublicID string) ([]tasksdomain.AdminTaskAttempt, error) {
	if a == nil || a.db == nil {
		return nil, workspacetasks.ErrNotFound
	}
	rows, err := a.db.Query(ctx, `
SELECT attempt_no, status, failure_stage, failure_category, resolved_model, provider, gateway_step,
  provider_started_at, provider_finished_at, completed_at, created_at
FROM task_execution_attempts
WHERE task_id = $1
ORDER BY attempt_no ASC`, taskPublicID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	attempts := []tasksdomain.AdminTaskAttempt{}
	for rows.Next() {
		var attempt tasksdomain.AdminTaskAttempt
		var failureStage string
		var createdAt time.Time
		if err := rows.Scan(
			&attempt.AttemptNo, &attempt.Status, &failureStage, &attempt.FailureCategory,
			&attempt.ResolvedModel, &attempt.Provider, &attempt.GatewayStep,
			&attempt.ProviderStartedAt, &attempt.ProviderFinishedAt, &attempt.CompletedAt, &createdAt,
		); err != nil {
			return nil, err
		}
		attempt.Stage = failureStage
		if attempt.ProviderStartedAt != nil && attempt.ProviderFinishedAt != nil {
			attempt.ProviderLatencyMS = attempt.ProviderFinishedAt.Sub(*attempt.ProviderStartedAt).Milliseconds()
			if attempt.ProviderLatencyMS < 0 {
				attempt.ProviderLatencyMS = 0
			}
		}
		if attempt.CompletedAt != nil {
			attempt.TotalLatencyMS = attempt.CompletedAt.Sub(createdAt).Milliseconds()
			if attempt.TotalLatencyMS < 0 {
				attempt.TotalLatencyMS = 0
			}
			if strings.TrimSpace(attempt.Status) == string(workspacetasks.StatusCancelled) {
				completed := *attempt.CompletedAt
				attempt.CancelledAt = &completed
			}
		}
		attempts = append(attempts, attempt)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return attempts, nil
}
