package taskstore

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"ideaboard/api/internal/db/dbutil"
	dbsqlc "ideaboard/api/internal/db/sqlc"
	"ideaboard/api/internal/tasking/taskbilling"
	"ideaboard/api/internal/tasking/tasks"

	"github.com/jackc/pgx/v5"
	"github.com/lib/pq"
)

type Repository struct {
	db *sql.DB
	q  *dbsqlc.Queries
}

type repositoryContextDB struct {
	ctx context.Context
	db  *sql.DB
}

func (r *Repository) dbWithContext(ctx context.Context) repositoryContextDB {
	return repositoryContextDB{ctx: ctx, db: r.db}
}

func (q repositoryContextDB) Query(query string, args ...any) (*sql.Rows, error) {
	return q.db.QueryContext(q.ctx, query, args...)
}

type RunningTaskLease struct {
	TaskID     string
	AccountID  int64
	LeaseToken string
}

type StartupRecoveredTask struct {
	TaskID     string
	RiverJobID int64
}

type ActiveTaskLease struct {
	TaskID                  string
	LeaseToken              string
	RiverJobID              int64
	AccountID               int64
	ProviderEndpointID      int64
	CredentialID            int64
	FeatureSlug             string
	ResolvedModel           string
	APIType                 string
	ProviderEndpointName    string
	ProviderEndpointBaseURL string
	CredentialFingerprint   string
	CredentialKeyVersion    string
	AcquiredAt              time.Time
}

type DailyStatAggregate struct {
	StatDate          time.Time
	ModelName         string
	UserKey           string
	Total             int64
	Completed         int64
	Failed            int64
	Cancelled         int64
	LatencySecondsSum float64
	LatencyCount      int64
	CostMicrosSum     int64
}

type BackfillDailyStatsResult struct {
	InsertedEvents int64
	TouchedGroups  int64
	MarkedDays     int64
}

type RebuildDailyStatsResult struct {
	DeletedGroups int64
	RebuiltGroups int64
}

type TaskStatEventCorrection struct {
	TaskID             string
	OccurredAt         time.Time
	UserID             int64
	AccountID          int64
	ProviderEndpointID int64
	CredentialID       int64
	FeatureSlug        string
	APIType            string
	ModelName          string
	UserKey            string
	Status             string
	LatencySeconds     float64
	CostMicros         int64
}

type StatEventTrendRow struct {
	OccurredAt     time.Time
	StatDate       time.Time
	ModelName      string
	UserKey        string
	Status         string
	LatencySeconds float64
	CostMicros     int64
}

type StatEventLatencySample struct {
	ModelName      string
	LatencySeconds float64
}

type ExecutionBottleneckAggregate struct {
	Source       string
	Total        int64
	Failed       int64
	AvgLatencyMS float64
}

type ExecutionFailureCategoryAggregate struct {
	Category     string
	Total        int64
	AvgLatencyMS float64
}

type MonitorExecutionPressureAggregate struct {
	Key                string
	Total              int64
	AvgDurationSeconds float64
	MaxDurationSeconds float64
	Failed             int64
	Retry              int64
}

type MonitorGoldenSignalSample struct {
	Status            string
	FailureStage      string
	FailureCategory   string
	QueueWaitMS       int64
	ProviderLatencyMS int64
	FinalizeLatencyMS int64
	ProviderObserved  bool
	ProviderError     bool
	ProviderAttempts  int64
	ProviderErrors    int64
}

const (
	defaultStorageProfileID = "builtin-local"
)

const createTaskPGXSQL = `
INSERT INTO tasks (
  task_id, user_id, client_action_id, assigned_provider_endpoint_id, assigned_credential_id, account_id, prompt, user_prompt, params,
  feature_slug, priority,
  status, lease_token, retry_count, error_msg, result, created_at, updated_at, completed_at,
  cancelled_at, cancelled_by, cancel_reason
) VALUES (
  $1, $2, $3, $4, $5, $6,
  $7, $8, $9,
  $10, $11,
  $12, $13, $14, $15, $16, $17, $18, $19,
  $20, $21, $22
)
ON CONFLICT (user_id, client_action_id) WHERE client_action_id <> '' DO NOTHING
RETURNING id, task_id`

func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db, q: dbsqlc.New(db)}
}

func (r *Repository) Create(ctx context.Context, task *tasks.Task) error {
	params := createTaskParams(task)
	id, err := r.q.CreateTask(ctx, params)
	if err != nil {
		return err
	}
	task.ID = int64(id)
	return nil
}

func (r *Repository) CreateWithPGXTx(ctx context.Context, tx pgx.Tx, task *tasks.Task) (bool, error) {
	if tx == nil {
		return false, fmt.Errorf("taskstore: pgx tx is nil")
	}
	params := createTaskParams(task)
	var taskID string
	err := tx.QueryRow(ctx, createTaskPGXSQL,
		params.TaskID,
		params.UserID,
		params.ClientActionID,
		nullableInt64(params.AssignedProviderEndpointID),
		nullableInt64(params.AssignedCredentialID),
		nullableInt64(params.AccountID),
		params.Prompt,
		params.UserPrompt,
		params.Params,
		params.FeatureSlug,
		params.Priority,
		params.Status,
		params.LeaseToken,
		params.RetryCount,
		params.ErrorMsg,
		params.Result,
		params.CreatedAt,
		params.UpdatedAt,
		nullableTime(params.CompletedAt),
		nullableTime(params.CancelledAt),
		nullableInt64(params.CancelledBy),
		params.CancelReason,
	).Scan(&task.ID, &taskID)
	if errors.Is(err, pgx.ErrNoRows) && strings.TrimSpace(task.ClientActionID) != "" {
		return r.loadIdempotentTaskWithPGXTx(ctx, tx, task)
	}
	if err != nil {
		return false, fmt.Errorf("taskstore: create task with pgx tx: %w", err)
	}
	task.TaskID = taskID
	return true, nil
}

func (r *Repository) LockAndFindSubmissionByClientActionWithPGXTx(ctx context.Context, tx pgx.Tx, userID int64, clientActionID string, request taskbilling.Request) (tasks.TaskSubmissionResult, bool, error) {
	if tx == nil || userID <= 0 || strings.TrimSpace(clientActionID) == "" {
		return tasks.TaskSubmissionResult{}, false, nil
	}
	clientActionID = strings.TrimSpace(clientActionID)
	expectedHash, err := taskbilling.RequestFingerprint(request)
	if err != nil {
		return tasks.TaskSubmissionResult{}, false, err
	}
	if _, err := tx.Exec(
		ctx,
		`SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`,
		fmt.Sprintf("task-submit:%d:%s", userID, clientActionID),
	); err != nil {
		return tasks.TaskSubmissionResult{}, false, fmt.Errorf("taskstore: lock submission claim: %w", err)
	}
	var result tasks.TaskSubmissionResult
	var actualHash string
	err = tx.QueryRow(ctx, `
SELECT task_id, request_hash
FROM task_submission_claims
WHERE user_id = $1 AND client_action_id = $2`, userID, clientActionID).Scan(&result.TaskID, &actualHash)
	if errors.Is(err, pgx.ErrNoRows) {
		var existingTaskID, feature string
		taskErr := tx.QueryRow(ctx, `
SELECT task_id, feature_slug
FROM tasks
WHERE user_id = $1 AND client_action_id = $2`, userID, clientActionID).Scan(&existingTaskID, &feature)
		if errors.Is(taskErr, pgx.ErrNoRows) {
			return tasks.TaskSubmissionResult{}, false, nil
		}
		if taskErr != nil {
			return tasks.TaskSubmissionResult{}, false, fmt.Errorf("taskstore: inspect existing submission without claim: %w", taskErr)
		}
		if taskbilling.RequiresBilling(feature) {
			return tasks.TaskSubmissionResult{}, false, fmt.Errorf("%w: billed submission %s has no durable idempotency claim", taskbilling.ErrIntegrity, existingTaskID)
		}
		return tasks.TaskSubmissionResult{}, false, fmt.Errorf("%w: client action already belongs to task %s", taskbilling.ErrIdempotencyConflict, existingTaskID)
	}
	if err != nil {
		return tasks.TaskSubmissionResult{}, false, fmt.Errorf("taskstore: find durable submission claim: %w", err)
	}
	if actualHash != expectedHash {
		return tasks.TaskSubmissionResult{}, false, fmt.Errorf("%w: client action %s was reused with a different billed product", taskbilling.ErrIdempotencyConflict, clientActionID)
	}
	var settlementState taskbilling.SettlementState
	if err := tx.QueryRow(ctx, `
SELECT COALESCE(snapshots.hold_transaction_id, ''), settlements.state
FROM task_billing_snapshots snapshots
JOIN task_billing_settlements settlements USING (task_id)
WHERE snapshots.task_id = $1`, result.TaskID).Scan(&result.HoldTransactionID, &settlementState); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return tasks.TaskSubmissionResult{}, false, fmt.Errorf("%w: billed submission %s has incomplete billing facts", taskbilling.ErrIntegrity, result.TaskID)
		}
		return tasks.TaskSubmissionResult{}, false, fmt.Errorf("taskstore: load billed submission %s facts: %w", result.TaskID, err)
	}
	if settlementState == taskbilling.SettlementStateHeld && result.HoldTransactionID == "" {
		return tasks.TaskSubmissionResult{}, false, fmt.Errorf("%w: billed submission %s has held state without Hold", taskbilling.ErrIntegrity, result.TaskID)
	}
	result.Created = false
	return result, true, nil
}

func (r *Repository) loadIdempotentTaskWithPGXTx(ctx context.Context, tx pgx.Tx, task *tasks.Task) (bool, error) {
	err := tx.QueryRow(ctx, `
SELECT id, task_id
FROM tasks
WHERE user_id = $1 AND client_action_id = $2
FOR UPDATE`,
		task.UserID,
		strings.TrimSpace(task.ClientActionID),
	).Scan(&task.ID, &task.TaskID)
	if err != nil {
		return false, fmt.Errorf("taskstore: load idempotent task with pgx tx: %w", err)
	}
	return false, nil
}

func createTaskParams(task *tasks.Task) dbsqlc.CreateTaskParams {
	now := time.Now()
	if task.CreatedAt.IsZero() {
		task.CreatedAt = now
	}
	if task.UpdatedAt.IsZero() {
		task.UpdatedAt = task.CreatedAt
	}
	task.Priority = tasks.EffectiveTaskPriority(task.Priority)
	return dbsqlc.CreateTaskParams{
		TaskID:                     task.TaskID,
		UserID:                     task.UserID,
		ClientActionID:             strings.TrimSpace(task.ClientActionID),
		AssignedProviderEndpointID: dbutil.NullInt64(task.AssignedProviderEndpointID),
		AssignedCredentialID:       dbutil.NullInt64(task.AssignedCredentialID),
		AccountID:                  dbutil.NullInt64(task.AccountID),
		Prompt:                     task.Prompt,
		UserPrompt:                 task.UserPrompt,
		Params:                     task.Params,
		FeatureSlug:                task.FeatureSlug,
		Priority:                   int32(task.Priority),
		Status:                     task.Status,
		LeaseToken:                 task.LeaseToken,
		RetryCount:                 int32(task.RetryCount),
		ErrorMsg:                   task.ErrorMsg,
		Result:                     task.Result,
		CreatedAt:                  task.CreatedAt,
		UpdatedAt:                  task.UpdatedAt,
		CompletedAt:                dbutil.NullTime(task.CompletedAt),
		CancelledAt:                dbutil.NullTime(task.CancelledAt),
		CancelledBy:                dbutil.NullInt64(task.CancelledBy),
		CancelReason:               task.CancelReason,
	}
}

func nullableInt64(value sql.NullInt64) interface{} {
	if !value.Valid {
		return nil
	}
	return value.Int64
}

func nullableTime(value sql.NullTime) interface{} {
	if !value.Valid {
		return nil
	}
	return value.Time
}

func (r *Repository) GetByTaskID(ctx context.Context, taskID string) (*tasks.Task, error) {
	row, err := r.q.GetTaskByTaskID(ctx, taskID)
	if err != nil {
		return nil, err
	}
	task := taskFromSQLC(row)
	if err := r.attachExecutionMetrics(ctx, []*tasks.Task{&task}); err != nil {
		return nil, err
	}
	return &task, nil
}

func (r *Repository) ListAdminBillingSnapshots(ctx context.Context, taskIDs []string) (map[string]tasks.AdminBillingSnapshot, error) {
	result := make(map[string]tasks.AdminBillingSnapshot)
	if len(taskIDs) == 0 {
		return result, nil
	}
	rows, err := r.dbWithContext(ctx).Query(`
SELECT snapshots.task_id, snapshots.sku, snapshots.display_name, snapshots.price_version_id, snapshots.price_version,
       unit_credits, quantity, total_credits, quoted_at,
       settlements.state, settlements.settled_at
FROM task_billing_snapshots snapshots
JOIN task_billing_settlements settlements ON settlements.task_id = snapshots.task_id
WHERE snapshots.task_id = ANY($1::text[])`, pq.Array(taskIDs))
	if err != nil {
		return nil, fmt.Errorf("taskstore: list Admin Billing snapshots: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var taskID string
		var snapshot tasks.AdminBillingSnapshot
		var settledAt sql.NullTime
		if err := rows.Scan(
			&taskID, &snapshot.SKU, &snapshot.DisplayName, &snapshot.PriceVersionID,
			&snapshot.PriceVersion, &snapshot.UnitCredits, &snapshot.Quantity,
			&snapshot.TotalCredits, &snapshot.QuotedAt, &snapshot.SettlementState, &settledAt,
		); err != nil {
			return nil, fmt.Errorf("taskstore: scan Admin Billing snapshot: %w", err)
		}
		snapshot.SettledAt = nullTimePointer(settledAt)
		result[taskID] = snapshot
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("taskstore: read Admin Billing snapshots: %w", err)
	}
	return result, nil
}

func (r *Repository) GetByTaskIDAndUserID(ctx context.Context, taskID string, userID int64) (*tasks.Task, error) {
	row, err := r.q.GetTaskByTaskIDAndUserID(ctx, dbsqlc.GetTaskByTaskIDAndUserIDParams{TaskID: taskID, UserID: userID})
	if err != nil {
		return nil, err
	}
	task := taskFromSQLC(row)
	return &task, nil
}

func (r *Repository) MarkTaskFailed(ctx context.Context, taskID string, statuses []string, errMsg string, result string, completedAt time.Time, attemptFinish *tasks.TaskAttemptFinish) (int64, error) {
	completedAt = terminalTransitionTime(completedAt)
	return r.runTerminalTransition(ctx, terminalTransitionCommand{
		taskID:        taskID,
		kind:          terminalTransitionFailed,
		finishedAt:    completedAt,
		errorMessage:  errMsg,
		attemptFinish: attemptFinish,
		writeTaskFact: func(ctx context.Context, q *dbsqlc.Queries) (int64, error) {
			return q.MarkTaskFailed(ctx, dbsqlc.MarkTaskFailedParams{
				ErrorMsg:    errMsg,
				Result:      result,
				CompletedAt: sql.NullTime{Time: completedAt, Valid: true},
				TaskID:      taskID,
				Statuses:    statuses,
			})
		},
	})
}

func (r *Repository) List(ctx context.Context, userID int64, accountID int64, status string, keyword string, page, pageSize int) ([]*tasks.Task, tasks.TaskListCounts, error) {
	hasUserID := userID > 0
	hasAccountID := accountID > 0
	hasStatus := status != ""
	keywordLike := "%" + escapeLikeWildcards(keyword) + "%"
	hasKeyword := keyword != ""
	summaryParams := dbsqlc.SummarizeTasksParams{
		HasUserID:    hasUserID,
		UserID:       userID,
		HasAccountID: hasAccountID,
		AccountID:    sql.NullInt64{Int64: accountID, Valid: hasAccountID},
		HasStatus:    hasStatus,
		Status:       status,
		HasKeyword:   hasKeyword,
		KeywordLike:  keywordLike,
	}
	summary, err := r.q.SummarizeTasks(ctx, summaryParams)
	if err != nil {
		return nil, tasks.TaskListCounts{}, err
	}
	offset := (page - 1) * pageSize
	rows, err := r.q.ListTasks(ctx, dbsqlc.ListTasksParams{
		HasUserID:    hasUserID,
		UserID:       userID,
		HasAccountID: hasAccountID,
		AccountID:    sql.NullInt64{Int64: accountID, Valid: hasAccountID},
		HasStatus:    hasStatus,
		Status:       status,
		HasKeyword:   hasKeyword,
		KeywordLike:  keywordLike,
		PageLimit:    int32(pageSize),
		PageOffset:   int32(offset),
	})
	if err != nil {
		return nil, tasks.TaskListCounts{}, err
	}
	items := tasksFromSQLC(rows)
	if err := r.attachExecutionMetrics(ctx, items); err != nil {
		return nil, tasks.TaskListCounts{}, err
	}
	return items, tasks.TaskListCounts{
		Total:          summary.Total,
		PendingCount:   summary.PendingCount,
		RunningCount:   summary.RunningCount,
		CompletedCount: summary.CompletedCount,
		FailedCount:    summary.FailedCount,
		CancelledCount: summary.CancelledCount,
		OtherCount:     summary.OtherCount,
	}, nil
}

func (r *Repository) GetByUserID(ctx context.Context, userID int64, page, pageSize int) ([]*tasks.Task, int64, error) {
	total, err := r.q.CountTasksByUserID(ctx, userID)
	if err != nil {
		return nil, 0, err
	}
	offset := (page - 1) * pageSize
	rows, err := r.q.ListTasksByUserID(ctx, dbsqlc.ListTasksByUserIDParams{
		UserID:     userID,
		PageLimit:  int32(pageSize),
		PageOffset: int32(offset),
	})
	if err != nil {
		return nil, 0, err
	}
	return tasksFromSQLC(rows), total, nil
}

// FailRunningByAccountID 将指定账号下所有 running 状态的任务标记为 failed
func (r *Repository) attachExecutionMetrics(ctx context.Context, items []*tasks.Task) error {
	if len(items) == 0 {
		return nil
	}
	taskIDs := make([]string, 0, len(items))
	byTaskID := make(map[string]*tasks.Task, len(items))
	for _, task := range items {
		if task == nil || task.TaskID == "" {
			continue
		}
		taskIDs = append(taskIDs, task.TaskID)
		byTaskID[task.TaskID] = task
	}
	if len(taskIDs) == 0 {
		return nil
	}
	metricsByTaskID, err := r.ListExecutionMetricsByTaskIDs(ctx, taskIDs)
	if err != nil {
		return err
	}
	for taskID, metrics := range metricsByTaskID {
		task := byTaskID[taskID]
		if task == nil {
			continue
		}
		task.ExecutionMetrics = metrics
	}
	return nil
}

func tasksFromSQLC[T any](rows []T) []*tasks.Task {
	result := make([]*tasks.Task, 0, len(rows))
	for _, row := range rows {
		task := taskFromSQLC(row)
		result = append(result, &task)
	}
	return result
}

func taskExecutionMetricsFromSQLC(row dbsqlc.TaskExecutionMetric) tasks.TaskExecutionMetrics {
	return tasks.TaskExecutionMetrics{
		TaskID:             row.TaskID,
		CurrentStage:       row.CurrentStage,
		BottleneckSource:   row.BottleneckSource,
		FailureStage:       row.FailureStage,
		FailureCategory:    row.FailureCategory,
		AssignedAt:         dbutil.TimePtr(row.AssignedAt),
		ProviderStartedAt:  dbutil.TimePtr(row.ProviderStartedAt),
		ProviderFinishedAt: dbutil.TimePtr(row.ProviderFinishedAt),
		FinalizeStartedAt:  dbutil.TimePtr(row.FinalizeStartedAt),
		FinalizeFinishedAt: dbutil.TimePtr(row.FinalizeFinishedAt),
		QueueWaitMS:        row.QueueWaitMs,
		WorkerOverheadMS:   row.WorkerOverheadMs,
		ProviderLatencyMS:  row.ProviderLatencyMs,
		FinalizeLatencyMS:  row.FinalizeLatencyMs,
		TotalLatencyMS:     row.TotalLatencyMs,
		CreatedAt:          row.CreatedAt,
		UpdatedAt:          row.UpdatedAt,
	}
}

func taskOutputFromSQLC(row dbsqlc.ListOutputsByTaskIDsRow) tasks.TaskOutput {
	return tasks.TaskOutput{
		ID:             row.ID,
		TaskID:         row.TaskID,
		MediaAssetID:   dbutil.Int64Ptr(row.MediaAssetID),
		OutputKind:     row.OutputKind,
		Role:           row.Role,
		URL:            row.Url,
		StorageKey:     row.StorageKey,
		MIMEType:       row.MimeType,
		Width:          int(row.Width),
		Height:         int(row.Height),
		SizeBytes:      row.SizeBytes,
		SourceURL:      row.SourceUrl,
		ChecksumSHA256: row.ChecksumSha256,
		StorageProfile: row.StorageProfile,
		Metadata:       string(row.Metadata),
		SortIndex:      int(row.SortIndex),
		CreatedAt:      row.CreatedAt,
		UpdatedAt:      row.UpdatedAt,
	}
}

func outputMetadataJSON(value string) json.RawMessage {
	value = strings.TrimSpace(value)
	if value == "" {
		return json.RawMessage("{}")
	}
	return json.RawMessage(value)
}

func mediaAssetMetadataJSON(output tasks.TaskOutput) json.RawMessage {
	metadata := map[string]interface{}{
		"task_id":    strings.TrimSpace(output.TaskID),
		"role":       strings.TrimSpace(output.Role),
		"sort_index": output.SortIndex,
	}
	if strings.TrimSpace(output.Metadata) != "" {
		metadata["output_metadata"] = json.RawMessage(output.Metadata)
	}
	raw, err := json.Marshal(metadata)
	if err != nil {
		return json.RawMessage("{}")
	}
	return raw
}

func mediaRefMetadataJSON(taskOutputID int64) json.RawMessage {
	raw, err := json.Marshal(map[string]interface{}{"task_output_id": taskOutputID})
	if err != nil {
		return json.RawMessage("{}")
	}
	return raw
}

func newMediaAssetID() (string, error) {
	var buf [12]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", fmt.Errorf("generate media asset id: %w", err)
	}
	return "asset_" + hex.EncodeToString(buf[:]), nil
}

func optionalString(value *string) sql.NullString {
	if value == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: strings.TrimSpace(*value), Valid: true}
}

func escapeLikeWildcards(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "%", "\\%")
	s = strings.ReplaceAll(s, "_", "\\_")
	return s
}
