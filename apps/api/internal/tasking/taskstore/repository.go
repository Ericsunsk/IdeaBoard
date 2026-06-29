package taskstore

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"ideaboard/api/internal/db/dbutil"
	dbsqlc "ideaboard/api/internal/db/sqlc"
	"ideaboard/api/internal/tasking/tasks"
)

type Repository struct {
	db *sql.DB
	q  *dbsqlc.Queries
}

type TaskStatisticLite struct {
	TaskID         string
	UserID         int64
	AccountID      *int64
	Status         string
	Result         string
	FeatureSlug    string
	RequestedModel string
	ResolvedModel  string
	APIType        string
	CreatedAt      time.Time
	CompletedAt    *time.Time
}

type RunningTaskLease struct {
	TaskID     string
	AccountID  int64
	LeaseToken string
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
}

type BackfillDailyStatsResult struct {
	InsertedEvents int64
	TouchedGroups  int64
	MarkedDays     int64
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

const (
	defaultStorageProfileID = "builtin-local"
)

func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db, q: dbsqlc.New(db)}
}

func (r *Repository) Create(ctx context.Context, task *tasks.Task) error {
	now := time.Now()
	if task.CreatedAt.IsZero() {
		task.CreatedAt = now
	}
	if task.UpdatedAt.IsZero() {
		task.UpdatedAt = task.CreatedAt
	}
	id, err := r.q.CreateTask(ctx, dbsqlc.CreateTaskParams{
		TaskID:            task.TaskID,
		UserID:            task.UserID,
		AssignedChannelID: dbutil.NullInt64(task.AssignedChannelID),
		AccountID:         dbutil.NullInt64(task.AccountID),
		Prompt:            task.Prompt,
		UserPrompt:        task.UserPrompt,
		Params:            task.Params,
		FeatureSlug:       task.FeatureSlug,
		RequestedModel:    task.RequestedModel,
		ResolvedModel:     task.ResolvedModel,
		ApiType:           task.APIType,
		Status:            task.Status,
		LeaseToken:        task.LeaseToken,
		RetryCount:        int32(task.RetryCount),
		ErrorMsg:          task.ErrorMsg,
		Result:            task.Result,
		CreatedAt:         task.CreatedAt,
		UpdatedAt:         task.UpdatedAt,
		CompletedAt:       dbutil.NullTime(task.CompletedAt),
		CancelledAt:       dbutil.NullTime(task.CancelledAt),
		CancelledBy:       dbutil.NullInt64(task.CancelledBy),
		CancelReason:      task.CancelReason,
	})
	if err != nil {
		return err
	}
	task.ID = int64(id)
	return nil
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

func (r *Repository) GetByTaskIDAndUserID(ctx context.Context, taskID string, userID int64) (*tasks.Task, error) {
	row, err := r.q.GetTaskByTaskIDAndUserID(ctx, dbsqlc.GetTaskByTaskIDAndUserIDParams{TaskID: taskID, UserID: userID})
	if err != nil {
		return nil, err
	}
	task := taskFromSQLC(row)
	return &task, nil
}

func (r *Repository) MarkTaskFailed(ctx context.Context, taskID string, statuses []string, errMsg string, result string, completedAt time.Time, attemptFinish *tasks.TaskAttemptFinish) (int64, error) {
	var rowsAffected int64
	err := dbutil.WithTx(ctx, r.db, func(q *dbsqlc.Queries) error {
		updated, err := q.MarkTaskFailed(ctx, dbsqlc.MarkTaskFailedParams{
			ErrorMsg:    errMsg,
			Result:      result,
			CompletedAt: sql.NullTime{Time: completedAt, Valid: true},
			TaskID:      taskID,
			Statuses:    statuses,
		})
		if err != nil || updated == 0 {
			return err
		}
		if err := upsertDailyStatByTaskIDTx(ctx, q, taskID); err != nil {
			return err
		}
		if err := finishTaskAttemptTx(ctx, q, attemptFinish); err != nil {
			return err
		}
		if err := patchTerminalExecutionMetricsTx(ctx, q, taskID, tasks.ExecutionStageFailed, taskAttemptFailureStage(attemptFinish, tasks.ExecutionStageAssigned), taskAttemptFailureCategory(attemptFinish, tasks.FailureCategoryUnknown), completedAt); err != nil {
			return err
		}
		rowsAffected = updated
		return nil
	})
	return rowsAffected, err
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

func (r *Repository) DeleteByTaskIDAndUserID(ctx context.Context, taskID string, userID int64) error {
	return r.q.DeleteTaskByTaskIDAndUserID(ctx, dbsqlc.DeleteTaskByTaskIDAndUserIDParams{
		TaskID: taskID,
		UserID: userID,
	})
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

func tasksFromSQLC(rows []dbsqlc.Task) []*tasks.Task {
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

func taskOutputFromSQLC(row dbsqlc.TaskOutput) tasks.TaskOutput {
	return tasks.TaskOutput{
		ID:             row.ID,
		TaskID:         row.TaskID,
		MediaAssetID:   dbutil.Int64Ptr(row.MediaAssetID),
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
