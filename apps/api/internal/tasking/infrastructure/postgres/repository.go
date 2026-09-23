package postgres

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"ideaboard/api/internal/db/dbutil"
	dbsqlc "ideaboard/api/internal/db/sqlc"
	tasks "ideaboard/api/internal/tasking/domain"
	taskbilling "ideaboard/api/internal/tasking/ports/billing"
	submissioncontracts "ideaboard/api/internal/tasking/ports/submission"
	"ideaboard/api/internal/workflow"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	db *pgxpool.Pool
	q  *dbsqlc.Queries
}

func (r *Repository) ClaimTaskInputMediaRef(ctx context.Context, tx pgx.Tx, claim tasks.MediaRefClaim) error {
	taskID, ownerUserID, mediaAssetID := claim.TaskID, claim.OwnerUserID, claim.AssetID
	preparedRefID, role, inputIndex := claim.PreparedRefID, claim.Role, claim.InputIndex
	if tx == nil {
		return fmt.Errorf("taskstore: pgx tx is nil")
	}
	if strings.TrimSpace(taskID) == "" || ownerUserID <= 0 || mediaAssetID <= 0 || inputIndex < 0 || !tasks.IsGenerationMediaRole(role) {
		return fmt.Errorf("taskstore: invalid task input media ref")
	}
	var lockedAssetID int64
	err := tx.QueryRow(ctx, `
UPDATE media_assets
SET reclaim_after = NULL,
    reclaim_started_at = NULL,
    reclaim_claim_token = NULL,
    updated_at = now()
WHERE id = $1 AND owner_user_id = $2
  AND status = 'ready' AND deleted_at IS NULL
  AND reclaim_started_at IS NULL
RETURNING id`, mediaAssetID, ownerUserID).Scan(&lockedAssetID)
	if err != nil {
		return fmt.Errorf("claim task input media asset: %w", err)
	}
	if _, err := tx.Exec(ctx, `
INSERT INTO media_refs (
  media_asset_id, owner_user_id, ref_type, ref_id, task_id, role, input_index, metadata
)
VALUES ($1, $2, 'task_input', $3, $3, $4, $5, '{}'::jsonb)`, lockedAssetID, ownerUserID, taskID, role, inputIndex); err != nil {
		return fmt.Errorf("create task input media ref: %w", err)
	}
	if strings.TrimSpace(preparedRefID) != "" {
		if _, err := tx.Exec(ctx, `
DELETE FROM media_refs
WHERE owner_user_id = $1 AND media_asset_id = $2
  AND ref_type = 'prompt_reference' AND ref_id = $3
  AND role = $4 AND input_index = $5 AND task_id = ''`, ownerUserID, lockedAssetID, preparedRefID, role, inputIndex); err != nil {
			return fmt.Errorf("release prepared media ref: %w", err)
		}
	}
	return nil
}

type repositoryContextDB struct {
	ctx context.Context
	db  *pgxpool.Pool
}

func (r *Repository) dbWithContext(ctx context.Context) repositoryContextDB {
	return repositoryContextDB{ctx: ctx, db: r.db}
}

func (q repositoryContextDB) Query(query string, args ...any) (pgx.Rows, error) {
	return q.db.Query(q.ctx, query, args...)
}

type StartupRecoveredTask struct {
	TaskID     string
	RiverJobID int64
}

const (
	defaultStorageProfileID = "builtin-local"
)

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db, q: dbsqlc.New(db)}
}

// Create is a thin non-transactional sqlc wrapper kept for fixtures and
// read-path tests. Production submission never uses it; SubmitTask inserts
// through CreateWithPGXTx inside the caller-owned submission transaction so
// task insert, billing hold, media claim, and River enqueue commit atomically.
func (r *Repository) Create(ctx context.Context, task *tasks.Task) (err error) {
	params := createTaskParams(task)
	id, err := r.q.CreateTask(ctx, params)
	if err != nil {
		return err
	}
	task.ID = int64(id)
	return nil
}

func (r *Repository) CreateWithPGXTx(ctx context.Context, tx pgx.Tx, task *tasks.Task) (created bool, err error) {
	if tx == nil {
		return false, fmt.Errorf("taskstore: pgx tx is nil")
	}
	params := dbsqlc.CreateTaskIdempotentParams(createTaskParams(task))
	row, err := r.q.WithTx(tx).CreateTaskIdempotent(ctx, params)
	if errors.Is(err, pgx.ErrNoRows) && strings.TrimSpace(task.ClientActionID) != "" {
		return r.loadIdempotentTaskWithPGXTx(ctx, tx, task)
	}
	if err != nil {
		return false, fmt.Errorf("taskstore: create task with pgx tx: %w", err)
	}
	task.ID = row.ID
	task.TaskID = row.TaskID
	return true, nil
}

func (r *Repository) LockAndFindSubmissionByClientActionWithPGXTx(ctx context.Context, tx pgx.Tx, lookup submissioncontracts.Lookup) (result tasks.TaskSubmissionResult, found bool, err error) {
	userID, clientActionID, expectedHash := lookup.UserID, lookup.ClientActionID, lookup.RequestHash
	if tx == nil || userID <= 0 || strings.TrimSpace(clientActionID) == "" {
		return tasks.TaskSubmissionResult{}, false, nil
	}
	clientActionID = strings.TrimSpace(clientActionID)
	if strings.TrimSpace(expectedHash) == "" {
		return tasks.TaskSubmissionResult{}, false, fmt.Errorf("taskstore: submission request hash is required")
	}
	if err := r.lockSubmissionClaim(ctx, tx, userID, clientActionID); err != nil {
		return tasks.TaskSubmissionResult{}, false, err
	}
	result, actualHash, found, err := findSubmissionClaim(ctx, tx, userID, clientActionID)
	if err != nil {
		return tasks.TaskSubmissionResult{}, false, err
	}
	if !found {
		return r.findLegacySubmission(ctx, tx, userID, clientActionID)
	}
	if actualHash != expectedHash {
		return tasks.TaskSubmissionResult{}, false, fmt.Errorf("%w: client action %s was reused with a different billed product", taskbilling.ErrIdempotencyConflict, clientActionID)
	}
	if err := r.loadSubmissionBillingFacts(ctx, tx, &result); err != nil {
		return tasks.TaskSubmissionResult{}, false, err
	}
	result.Created = false
	return result, true, nil
}

func (r *Repository) lockSubmissionClaim(ctx context.Context, tx pgx.Tx, userID int64, clientActionID string) error {
	_, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, fmt.Sprintf("task-submit:%d:%s", userID, clientActionID))
	if err != nil {
		return fmt.Errorf("taskstore: lock submission claim: %w", err)
	}
	return nil
}

func findSubmissionClaim(ctx context.Context, tx pgx.Tx, userID int64, clientActionID string) (tasks.TaskSubmissionResult, string, bool, error) {
	var result tasks.TaskSubmissionResult
	var actualHash string
	err := tx.QueryRow(ctx, `
SELECT task_id, request_hash
FROM task_submission_claims
WHERE user_id = $1 AND client_action_id = $2`, userID, clientActionID).Scan(&result.TaskID, &actualHash)
	if errors.Is(err, pgx.ErrNoRows) {
		return tasks.TaskSubmissionResult{}, "", false, nil
	}
	if err != nil {
		return tasks.TaskSubmissionResult{}, "", false, fmt.Errorf("taskstore: find durable submission claim: %w", err)
	}
	return result, actualHash, true, nil
}

func (r *Repository) findLegacySubmission(ctx context.Context, tx pgx.Tx, userID int64, clientActionID string) (tasks.TaskSubmissionResult, bool, error) {
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
	if workflow.IsAIImageFeature(feature) {
		return tasks.TaskSubmissionResult{}, false, fmt.Errorf("%w: billed submission %s has no durable idempotency claim", taskbilling.ErrIntegrity, existingTaskID)
	}
	return tasks.TaskSubmissionResult{}, false, fmt.Errorf("%w: client action already belongs to task %s", taskbilling.ErrIdempotencyConflict, existingTaskID)
}

func (r *Repository) loadSubmissionBillingFacts(ctx context.Context, tx pgx.Tx, result *tasks.TaskSubmissionResult) error {
	var settlementState taskbilling.SettlementState
	if err := tx.QueryRow(ctx, `
SELECT COALESCE(snapshots.hold_transaction_id, ''), settlements.state
FROM task_billing_snapshots snapshots
JOIN task_billing_settlements settlements USING (task_id)
	WHERE snapshots.task_id = $1`, result.TaskID).Scan(&result.HoldTransactionID, &settlementState); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("%w: billed submission %s has incomplete billing facts", taskbilling.ErrIntegrity, result.TaskID)
		}
		return fmt.Errorf("taskstore: load billed submission %s facts: %w", result.TaskID, err)
	}
	if settlementState == taskbilling.SettlementStateHeld && result.HoldTransactionID == "" {
		return fmt.Errorf("%w: billed submission %s has held state without Hold", taskbilling.ErrIntegrity, result.TaskID)
	}
	return nil
}

func (r *Repository) PersistSubmissionClaim(ctx context.Context, tx pgx.Tx, claim submissioncontracts.Claim) error {
	clientActionID := strings.TrimSpace(claim.ClientActionID)
	taskID := strings.TrimSpace(claim.TaskID)
	if tx == nil || claim.UserID <= 0 || clientActionID == "" || taskID == "" {
		return fmt.Errorf("persist task submission claim: invalid input")
	}
	requestHash := strings.TrimSpace(claim.RequestHash)
	if requestHash == "" {
		return fmt.Errorf("persist task submission claim: request hash is required")
	}
	if _, err := tx.Exec(ctx, `
INSERT INTO task_submission_claims (user_id, client_action_id, task_id, request_hash)
VALUES ($1, $2, $3, $4)`, claim.UserID, clientActionID, taskID, requestHash); err != nil {
		return fmt.Errorf("persist task submission claim: %w", err)
	}
	return nil
}

func (r *Repository) loadIdempotentTaskWithPGXTx(ctx context.Context, tx pgx.Tx, task *tasks.Task) (bool, error) {
	row, err := r.q.WithTx(tx).GetTaskByUserClientActionForUpdate(ctx, dbsqlc.GetTaskByUserClientActionForUpdateParams{
		UserID:         task.UserID,
		ClientActionID: strings.TrimSpace(task.ClientActionID),
	})
	if err != nil {
		return false, fmt.Errorf("taskstore: load idempotent task with pgx tx: %w", err)
	}
	task.ID = row.ID
	task.TaskID = row.TaskID
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
		CreatedAt:                  pgtype.Timestamptz{Time: task.CreatedAt, Valid: true},
		UpdatedAt:                  pgtype.Timestamptz{Time: task.UpdatedAt, Valid: true},
		CompletedAt:                dbutil.NullTime(task.CompletedAt),
		CancelledAt:                dbutil.NullTime(task.CancelledAt),
		CancelledBy:                dbutil.NullInt64(task.CancelledBy),
		CancelReason:               task.CancelReason,
	}
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
	   snapshots.output_resolution, unit_credits, quantity, total_credits, settlements.captured_credits, quoted_at,
       settlements.state, settlements.settled_at
FROM task_billing_snapshots snapshots
JOIN task_billing_settlements settlements ON settlements.task_id = snapshots.task_id
WHERE snapshots.task_id = ANY($1::text[])`, taskIDs)
	if err != nil {
		return nil, fmt.Errorf("taskstore: list Admin Billing snapshots: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var taskID string
		var snapshot tasks.AdminBillingSnapshot
		var settledAt pgtype.Timestamptz
		if err := rows.Scan(
			&taskID, &snapshot.SKU, &snapshot.DisplayName, &snapshot.PriceVersionID,
			&snapshot.PriceVersion, &snapshot.OutputResolution, &snapshot.UnitCredits, &snapshot.Quantity,
			&snapshot.TotalCredits, &snapshot.CapturedCredits, &snapshot.QuotedAt, &snapshot.SettlementState, &settledAt,
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
		AccountID:    pgtype.Int8{Int64: accountID, Valid: hasAccountID},
		// 计数独立于 status 筛选：pill 展示全局分布，列表行才按 status 过滤。
		HasStatus:   false,
		Status:      "",
		HasKeyword:  hasKeyword,
		KeywordLike: keywordLike,
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
		AccountID:    pgtype.Int8{Int64: accountID, Valid: hasAccountID},
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
		CreatedAt:          dbutil.TimeValue(row.CreatedAt),
		UpdatedAt:          dbutil.TimeValue(row.UpdatedAt),
	}
}

func taskOutputFromSQLC(row dbsqlc.ListOutputsByTaskIDsRow) tasks.TaskOutput {
	return tasks.TaskOutput{
		ID:             row.ID,
		TaskID:         row.TaskID,
		MediaAssetID:   dbutil.Int64Ptr(row.MediaAssetID),
		AssetID:        row.AssetID,
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
		ResultIndex:    int(row.ResultIndex),
		CreatedAt:      dbutil.TimeValue(row.CreatedAt),
		UpdatedAt:      dbutil.TimeValue(row.UpdatedAt),
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
		"task_id":      strings.TrimSpace(output.TaskID),
		"role":         strings.TrimSpace(output.Role),
		"result_index": output.ResultIndex,
		"animated":     output.Animated,
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

func optionalString(value *string) pgtype.Text {
	if value == nil {
		return pgtype.Text{}
	}
	return pgtype.Text{String: strings.TrimSpace(*value), Valid: true}
}

func escapeLikeWildcards(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "%", "\\%")
	s = strings.ReplaceAll(s, "_", "\\_")
	return s
}
