package taskstore

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"ideaboard/api/internal/db/dbutil"
	dbsqlc "ideaboard/api/internal/db/sqlc"
	"ideaboard/api/internal/tasks"
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

func (r *Repository) MarkTaskFailedWithLease(ctx context.Context, taskID string, statuses []string, accountID int64, channelID int64, leaseToken string, errMsg string, result string, completedAt time.Time, attemptFinish *tasks.TaskAttemptFinish) (int64, error) {
	var rowsAffected int64
	err := dbutil.WithTx(ctx, r.db, func(q *dbsqlc.Queries) error {
		updated, err := q.MarkTaskFailedWithLease(ctx, dbsqlc.MarkTaskFailedWithLeaseParams{
			ErrorMsg:          errMsg,
			Result:            result,
			CompletedAt:       sql.NullTime{Time: completedAt, Valid: true},
			TaskID:            taskID,
			Statuses:          statuses,
			AccountID:         sql.NullInt64{Int64: accountID, Valid: accountID > 0},
			AssignedChannelID: sql.NullInt64{Int64: channelID, Valid: channelID > 0},
			LeaseToken:        leaseToken,
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

func (r *Repository) ResetTaskRunningToPendingWithLease(ctx context.Context, taskID string, statuses []string, accountID int64, channelID int64, leaseToken string, errMsg string, attemptFinish *tasks.TaskAttemptFinish) (int64, error) {
	params := dbsqlc.ResetTaskRunningToPendingWithLeaseParams{
		ErrorMsg:          errMsg,
		TaskID:            taskID,
		Statuses:          statuses,
		AccountID:         sql.NullInt64{Int64: accountID, Valid: accountID > 0},
		AssignedChannelID: sql.NullInt64{Int64: channelID, Valid: channelID > 0},
		LeaseToken:        leaseToken,
	}
	if attemptFinish == nil || attemptFinish.ID == 0 {
		return r.q.ResetTaskRunningToPendingWithLease(ctx, params)
	}
	var rowsAffected int64
	err := dbutil.WithTx(ctx, r.db, func(q *dbsqlc.Queries) error {
		updated, err := q.ResetTaskRunningToPendingWithLease(ctx, params)
		if err != nil || updated == 0 {
			return err
		}
		if err := finishTaskAttemptTx(ctx, q, attemptFinish); err != nil {
			return err
		}
		rowsAffected = updated
		return nil
	})
	return rowsAffected, err
}

func (r *Repository) RequeueTaskRunningWithLease(ctx context.Context, taskID string, statuses []string, accountID int64, channelID int64, leaseToken string, errMsg string, attemptFinish *tasks.TaskAttemptFinish) (int64, error) {
	params := dbsqlc.RequeueTaskRunningWithLeaseParams{
		ErrorMsg:          errMsg,
		TaskID:            taskID,
		Statuses:          statuses,
		AccountID:         sql.NullInt64{Int64: accountID, Valid: accountID > 0},
		AssignedChannelID: sql.NullInt64{Int64: channelID, Valid: channelID > 0},
		LeaseToken:        leaseToken,
	}
	if attemptFinish == nil || attemptFinish.ID == 0 {
		return r.q.RequeueTaskRunningWithLease(ctx, params)
	}
	var rowsAffected int64
	err := dbutil.WithTx(ctx, r.db, func(q *dbsqlc.Queries) error {
		updated, err := q.RequeueTaskRunningWithLease(ctx, params)
		if err != nil || updated == 0 {
			return err
		}
		if err := finishTaskAttemptTx(ctx, q, attemptFinish); err != nil {
			return err
		}
		rowsAffected = updated
		return nil
	})
	return rowsAffected, err
}

func (r *Repository) FinalFailTaskRunningWithLease(ctx context.Context, taskID string, statuses []string, accountID int64, channelID int64, leaseToken string, errMsg string, result string, completedAt time.Time, attemptFinish *tasks.TaskAttemptFinish) (int64, error) {
	var rowsAffected int64
	err := dbutil.WithTx(ctx, r.db, func(q *dbsqlc.Queries) error {
		updated, err := q.FinalFailTaskRunningWithLease(ctx, dbsqlc.FinalFailTaskRunningWithLeaseParams{
			ErrorMsg:          errMsg,
			Result:            result,
			CompletedAt:       sql.NullTime{Time: completedAt, Valid: true},
			TaskID:            taskID,
			Statuses:          statuses,
			AccountID:         sql.NullInt64{Int64: accountID, Valid: accountID > 0},
			AssignedChannelID: sql.NullInt64{Int64: channelID, Valid: channelID > 0},
			LeaseToken:        leaseToken,
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

func (r *Repository) CompleteWithOutputsAndLeaseToken(ctx context.Context, taskID string, statuses []string, accountID int64, channelID int64, leaseToken string, result string, completedAt time.Time, outputs []tasks.TaskOutput, attemptFinish *tasks.TaskAttemptFinish) (int64, error) {
	var rowsAffected int64
	err := dbutil.WithTx(ctx, r.db, func(q *dbsqlc.Queries) error {
		updated, err := q.CompleteTaskWithLease(ctx, dbsqlc.CompleteTaskWithLeaseParams{
			Result:            result,
			CompletedAt:       sql.NullTime{Time: completedAt, Valid: true},
			TaskID:            taskID,
			Statuses:          statuses,
			AccountID:         sql.NullInt64{Int64: accountID, Valid: accountID > 0},
			AssignedChannelID: sql.NullInt64{Int64: channelID, Valid: channelID > 0},
			LeaseToken:        leaseToken,
		})
		if err != nil || updated == 0 {
			return err
		}
		if err := q.DeleteTaskOutputMediaRefsByTaskID(ctx, taskID); err != nil {
			return err
		}
		if err := q.DeleteOutputsByTaskID(ctx, taskID); err != nil {
			return err
		}
		taskRow, err := q.GetTaskByTaskID(ctx, taskID)
		if err != nil {
			return err
		}
		if err := insertTaskOutputs(ctx, q, taskID, taskRow.UserID, outputs); err != nil {
			return err
		}
		if err := upsertDailyStatByTaskIDTx(ctx, q, taskID); err != nil {
			return err
		}
		if err := finishTaskAttemptTx(ctx, q, attemptFinish); err != nil {
			return err
		}
		metricsByTaskID, err := listExecutionMetricsByTaskIDsTx(ctx, q, []string{taskID})
		if err != nil {
			return err
		}
		if err := patchTerminalExecutionMetricsForTaskTx(ctx, q, taskID, taskRow.CreatedAt, metricsByTaskID[taskID], tasks.ExecutionStageCompleted, "", "", completedAt); err != nil {
			return err
		}
		rowsAffected = updated
		return nil
	})
	return rowsAffected, err
}

func insertTaskOutputs(ctx context.Context, q *dbsqlc.Queries, taskID string, ownerUserID int64, outputs []tasks.TaskOutput) error {
	for i := range outputs {
		output := outputs[i]
		output.TaskID = taskID
		output.SortIndex = i
		if strings.TrimSpace(output.Role) == "" {
			output.Role = "image"
		}
		mediaAssetID, err := upsertTaskOutputMediaAsset(ctx, q, ownerUserID, output)
		if err != nil {
			return err
		}
		output.MediaAssetID = mediaAssetID
		taskOutputID, err := q.CreateTaskOutput(ctx, dbsqlc.CreateTaskOutputParams{
			TaskID:         output.TaskID,
			MediaAssetID:   nullInt64Ptr(output.MediaAssetID),
			Role:           output.Role,
			Url:            output.URL,
			StorageKey:     output.StorageKey,
			MimeType:       output.MIMEType,
			Width:          int32(output.Width),
			Height:         int32(output.Height),
			SizeBytes:      output.SizeBytes,
			SourceUrl:      output.SourceURL,
			ChecksumSha256: output.ChecksumSHA256,
			StorageProfile: output.StorageProfile,
			Metadata:       outputMetadataJSON(output.Metadata),
			SortIndex:      int32(output.SortIndex),
		})
		if err != nil {
			return err
		}
		if output.MediaAssetID != nil {
			if _, err := q.UpsertMediaRef(ctx, dbsqlc.UpsertMediaRefParams{
				MediaAssetID: *output.MediaAssetID,
				OwnerUserID:  ownerUserID,
				RefType:      "task_output",
				RefID:        strconv.FormatInt(taskOutputID, 10),
				BoardID:      "",
				TaskID:       taskID,
				Role:         output.Role,
				Metadata:     mediaRefMetadataJSON(taskOutputID),
			}); err != nil {
				return err
			}
		}
	}
	return nil
}

func upsertTaskOutputMediaAsset(ctx context.Context, q *dbsqlc.Queries, ownerUserID int64, output tasks.TaskOutput) (*int64, error) {
	storageKey := strings.TrimSpace(output.StorageKey)
	if ownerUserID == 0 || storageKey == "" {
		return nil, nil
	}
	assetID, err := newMediaAssetID()
	if err != nil {
		return nil, err
	}
	storageProfileID := strings.TrimSpace(output.StorageProfile)
	if storageProfileID == "" {
		storageProfileID = defaultStorageProfileID
	}
	id, err := q.UpsertMediaAsset(ctx, dbsqlc.UpsertMediaAssetParams{
		AssetID:          assetID,
		OwnerUserID:      ownerUserID,
		StorageProfileID: storageProfileID,
		StorageKey:       storageKey,
		Url:              strings.TrimSpace(output.URL),
		SourceUrl:        strings.TrimSpace(output.SourceURL),
		MediaKind:        "image",
		MimeType:         strings.TrimSpace(output.MIMEType),
		Width:            int32(output.Width),
		Height:           int32(output.Height),
		SizeBytes:        output.SizeBytes,
		ChecksumSha256:   strings.TrimSpace(output.ChecksumSHA256),
		Status:           "ready",
		Metadata:         mediaAssetMetadataJSON(output),
	})
	if err != nil {
		return nil, err
	}
	return &id, nil
}

func (r *Repository) ListOutputsByTaskIDs(ctx context.Context, taskIDs []string) (map[string][]tasks.TaskOutput, error) {
	result := make(map[string][]tasks.TaskOutput, len(taskIDs))
	if len(taskIDs) == 0 {
		return result, nil
	}
	rows, err := r.q.ListOutputsByTaskIDs(ctx, taskIDs)
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		output := taskOutputFromSQLC(row)
		result[output.TaskID] = append(result[output.TaskID], output)
	}
	return result, nil
}

func (r *Repository) PatchExecutionMetrics(ctx context.Context, patch tasks.TaskExecutionMetricsPatch) error {
	taskID := strings.TrimSpace(patch.TaskID)
	if taskID == "" {
		return fmt.Errorf("task_id is required")
	}
	return r.q.PatchTaskExecutionMetrics(ctx, dbsqlc.PatchTaskExecutionMetricsParams{
		TaskID:             taskID,
		CurrentStage:       optionalNonEmptyString(patch.CurrentStage),
		BottleneckSource:   optionalNonEmptyString(patch.BottleneckSource),
		FailureStage:       optionalString(patch.FailureStage),
		FailureCategory:    optionalString(patch.FailureCategory),
		AssignedAt:         dbutil.NullTime(patch.AssignedAt),
		ProviderStartedAt:  dbutil.NullTime(patch.ProviderStartedAt),
		ProviderFinishedAt: dbutil.NullTime(patch.ProviderFinishedAt),
		FinalizeStartedAt:  dbutil.NullTime(patch.FinalizeStartedAt),
		FinalizeFinishedAt: dbutil.NullTime(patch.FinalizeFinishedAt),
		QueueWaitMs:        optionalInt64(patch.QueueWaitMS),
		WorkerOverheadMs:   optionalInt64(patch.WorkerOverheadMS),
		ProviderLatencyMs:  optionalInt64(patch.ProviderLatencyMS),
		FinalizeLatencyMs:  optionalInt64(patch.FinalizeLatencyMS),
		TotalLatencyMs:     optionalInt64(patch.TotalLatencyMS),
	})
}

func patchTerminalExecutionMetricsTx(ctx context.Context, q *dbsqlc.Queries, taskID string, currentStage string, failureStage string, failureCategory string, finishedAt time.Time) error {
	taskRow, err := q.GetTaskByTaskID(ctx, taskID)
	if err != nil {
		return err
	}
	metricsByTaskID, err := listExecutionMetricsByTaskIDsTx(ctx, q, []string{taskID})
	if err != nil {
		return err
	}
	return patchTerminalExecutionMetricsForTaskTx(ctx, q, taskID, taskRow.CreatedAt, metricsByTaskID[taskID], currentStage, failureStage, failureCategory, finishedAt)
}

func patchTerminalExecutionMetricsForTaskTx(
	ctx context.Context,
	q *dbsqlc.Queries,
	taskID string,
	createdAt time.Time,
	metrics *tasks.TaskExecutionMetrics,
	currentStage string,
	failureStage string,
	failureCategory string,
	finishedAt time.Time,
) error {
	currentStage = strings.TrimSpace(currentStage)
	if taskID == "" || currentStage == "" {
		return nil
	}
	if finishedAt.IsZero() {
		finishedAt = time.Now()
	}
	totalLatencyMS := finishedAt.Sub(createdAt).Milliseconds()
	if totalLatencyMS < 0 {
		totalLatencyMS = 0
	}
	bottleneckSource := tasks.ResolveExecutionBottleneck(metrics, failureStage, failureCategory)
	if currentStage == tasks.ExecutionStageCancelled {
		bottleneckSource = tasks.BottleneckSourceUnknown
	}
	if currentStage != tasks.ExecutionStageFailed && currentStage != tasks.ExecutionStageCancelled {
		failureStage = ""
		failureCategory = ""
	}
	return q.PatchTaskExecutionMetrics(ctx, dbsqlc.PatchTaskExecutionMetricsParams{
		TaskID:           taskID,
		CurrentStage:     optionalNonEmptyString(&currentStage),
		BottleneckSource: optionalNonEmptyString(&bottleneckSource),
		FailureStage:     optionalString(&failureStage),
		FailureCategory:  optionalString(&failureCategory),
		TotalLatencyMs:   optionalInt64(&totalLatencyMS),
	})
}

func listExecutionMetricsByTaskIDsTx(ctx context.Context, q *dbsqlc.Queries, taskIDs []string) (map[string]*tasks.TaskExecutionMetrics, error) {
	result := make(map[string]*tasks.TaskExecutionMetrics, len(taskIDs))
	if len(taskIDs) == 0 {
		return result, nil
	}
	rows, err := q.ListTaskExecutionMetricsByTaskIDs(ctx, taskIDs)
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		metrics := taskExecutionMetricsFromSQLC(row)
		result[metrics.TaskID] = &metrics
	}
	return result, nil
}

func taskAttemptFailureStage(attemptFinish *tasks.TaskAttemptFinish, fallback string) string {
	if attemptFinish != nil {
		if stage := strings.TrimSpace(attemptFinish.FailureStage); stage != "" {
			return stage
		}
	}
	return fallback
}

func taskAttemptFailureCategory(attemptFinish *tasks.TaskAttemptFinish, fallback string) string {
	if attemptFinish != nil {
		if category := strings.TrimSpace(attemptFinish.FailureCategory); category != "" {
			return category
		}
	}
	return fallback
}

func (r *Repository) PatchExecutionSnapshot(ctx context.Context, snapshot tasks.TaskExecutionSnapshot) (int64, error) {
	taskID := strings.TrimSpace(snapshot.TaskID)
	if taskID == "" {
		return 0, fmt.Errorf("task_id is required")
	}
	return r.q.PatchTaskExecutionSnapshot(ctx, dbsqlc.PatchTaskExecutionSnapshotParams{
		TaskID:         taskID,
		FeatureSlug:    strings.TrimSpace(snapshot.FeatureSlug),
		RequestedModel: strings.TrimSpace(snapshot.RequestedModel),
		ResolvedModel:  strings.TrimSpace(snapshot.ResolvedModel),
		ApiType:        strings.TrimSpace(snapshot.APIType),
	})
}

func (r *Repository) MarkTaskCancelled(ctx context.Context, snapshot tasks.TaskCancellationSnapshot, attemptFinish *tasks.TaskAttemptFinish) (int64, error) {
	taskID := strings.TrimSpace(snapshot.TaskID)
	if taskID == "" {
		return 0, fmt.Errorf("task_id is required")
	}
	cancelledAt := snapshot.CancelledAt
	if cancelledAt.IsZero() {
		cancelledAt = time.Now()
	}
	var rowsAffected int64
	err := dbutil.WithTx(ctx, r.db, func(q *dbsqlc.Queries) error {
		updated, err := q.MarkTaskCancelled(ctx, dbsqlc.MarkTaskCancelledParams{
			CancelledAt:  sql.NullTime{Time: cancelledAt, Valid: true},
			CancelledBy:  dbutil.NullInt64(snapshot.CancelledBy),
			CancelReason: strings.TrimSpace(snapshot.Reason),
			TaskID:       taskID,
		})
		if err != nil || updated == 0 {
			return err
		}
		if err := finishTaskAttemptTx(ctx, q, attemptFinish); err != nil {
			return err
		}
		cancelStage := tasks.ExecutionStageQueued
		if attemptFinish != nil && strings.TrimSpace(attemptFinish.FailureStage) != "" {
			cancelStage = strings.TrimSpace(attemptFinish.FailureStage)
		}
		if err := patchTerminalExecutionMetricsTx(ctx, q, taskID, tasks.ExecutionStageCancelled, cancelStage, tasks.FailureCategoryCancelled, cancelledAt); err != nil {
			return err
		}
		if err := upsertDailyStatByTaskIDTx(ctx, q, taskID); err != nil {
			return err
		}
		rowsAffected = updated
		return nil
	})
	return rowsAffected, err
}

func (r *Repository) CreateTaskAttempt(ctx context.Context, start tasks.TaskAttemptStart) (tasks.TaskAttemptRef, error) {
	taskID := strings.TrimSpace(start.TaskID)
	if taskID == "" {
		return tasks.TaskAttemptRef{}, fmt.Errorf("task_id is required")
	}
	providerStartedAt := start.ProviderStartedAt
	if providerStartedAt.IsZero() {
		providerStartedAt = time.Now()
	}
	row, err := r.q.CreateTaskAttempt(ctx, dbsqlc.CreateTaskAttemptParams{
		TaskID:            taskID,
		RiverJobID:        start.RiverJobID,
		LeaseToken:        strings.TrimSpace(start.LeaseToken),
		AccountID:         start.AccountID,
		ChannelID:         start.ChannelID,
		FeatureSlug:       strings.TrimSpace(start.FeatureSlug),
		RequestedModel:    strings.TrimSpace(start.RequestedModel),
		ResolvedModel:     strings.TrimSpace(start.ResolvedModel),
		ApiType:           strings.TrimSpace(start.APIType),
		ProviderStartedAt: sql.NullTime{Time: providerStartedAt, Valid: true},
	})
	if err != nil {
		return tasks.TaskAttemptRef{}, err
	}
	return tasks.TaskAttemptRef{ID: row.ID, AttemptNo: int(row.AttemptNo)}, nil
}

func (r *Repository) FinishTaskAttempt(ctx context.Context, finish tasks.TaskAttemptFinish) (int64, error) {
	if finish.ID == 0 {
		return 0, fmt.Errorf("task_attempt id is required")
	}
	return r.q.FinishTaskAttempt(ctx, finishTaskAttemptParams(finish))
}

func finishTaskAttemptTx(ctx context.Context, q *dbsqlc.Queries, finish *tasks.TaskAttemptFinish) error {
	if finish == nil || finish.ID == 0 {
		return nil
	}
	updated, err := q.FinishTaskAttempt(ctx, finishTaskAttemptParams(*finish))
	if err != nil {
		return err
	}
	if updated != 1 {
		return fmt.Errorf("finish task attempt affected %d rows, want 1", updated)
	}
	return nil
}

func finishTaskAttemptParams(finish tasks.TaskAttemptFinish) dbsqlc.FinishTaskAttemptParams {
	status := strings.TrimSpace(finish.Status)
	if status == "" {
		status = tasks.TaskAttemptStatusFailed
	}
	completedAt := finish.CompletedAt
	if completedAt.IsZero() {
		completedAt = time.Now()
	}
	return dbsqlc.FinishTaskAttemptParams{
		Status:             status,
		FailureStage:       strings.TrimSpace(finish.FailureStage),
		FailureCategory:    strings.TrimSpace(finish.FailureCategory),
		ErrorMsg:           strings.TrimSpace(finish.ErrorMsg),
		ProviderFinishedAt: dbutil.NullTime(finish.ProviderFinishedAt),
		FinalizeStartedAt:  dbutil.NullTime(finish.FinalizeStartedAt),
		FinalizeFinishedAt: dbutil.NullTime(finish.FinalizeFinishedAt),
		ProviderLatencyMs:  finish.ProviderLatencyMS,
		FinalizeLatencyMs:  finish.FinalizeLatencyMS,
		TotalLatencyMs:     finish.TotalLatencyMS,
		CompletedAt:        sql.NullTime{Time: completedAt, Valid: true},
		ID:                 finish.ID,
	}
}

func (r *Repository) ListMonitorExecutionPressureByRange(ctx context.Context, start, now time.Time) ([]MonitorExecutionPressureAggregate, error) {
	if start.IsZero() || now.IsZero() {
		return nil, fmt.Errorf("monitor execution pressure date range is required")
	}
	if !now.After(start) {
		return nil, fmt.Errorf("monitor execution pressure now_at must be after started_at")
	}
	rows, err := r.q.ListMonitorExecutionPressureByRange(ctx, dbsqlc.ListMonitorExecutionPressureByRangeParams{
		NowAt:     now,
		StartedAt: start,
	})
	if err != nil {
		return nil, err
	}
	result := make([]MonitorExecutionPressureAggregate, 0, len(rows))
	for _, row := range rows {
		result = append(result, MonitorExecutionPressureAggregate{
			Key:                row.BucketKey,
			Total:              row.Total,
			AvgDurationSeconds: row.AvgDurationSeconds,
			MaxDurationSeconds: row.MaxDurationSeconds,
			Failed:             row.Failed,
			Retry:              row.Retry,
		})
	}
	return result, nil
}

func (r *Repository) ListExecutionMetricsByTaskIDs(ctx context.Context, taskIDs []string) (map[string]*tasks.TaskExecutionMetrics, error) {
	result := make(map[string]*tasks.TaskExecutionMetrics, len(taskIDs))
	if len(taskIDs) == 0 {
		return result, nil
	}
	rows, err := r.q.ListTaskExecutionMetricsByTaskIDs(ctx, taskIDs)
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		metrics := taskExecutionMetricsFromSQLC(row)
		result[metrics.TaskID] = &metrics
	}
	return result, nil
}

func (r *Repository) ListExecutionBottleneckStatsByCreatedRange(ctx context.Context, start, end time.Time) ([]ExecutionBottleneckAggregate, error) {
	rows, err := r.q.ListTaskExecutionBottleneckStatsByCreatedRange(ctx, dbsqlc.ListTaskExecutionBottleneckStatsByCreatedRangeParams{
		StartedAt: start,
		EndedAt:   end,
	})
	if err != nil {
		return nil, err
	}
	result := make([]ExecutionBottleneckAggregate, 0, len(rows))
	for _, row := range rows {
		result = append(result, ExecutionBottleneckAggregate{
			Source:       row.BottleneckSource,
			Total:        row.Total,
			Failed:       row.Failed,
			AvgLatencyMS: row.AvgLatencyMs,
		})
	}
	return result, nil
}

func (r *Repository) ListExecutionFailureCategoryStatsByCreatedRange(ctx context.Context, start, end time.Time) ([]ExecutionFailureCategoryAggregate, error) {
	rows, err := r.q.ListTaskExecutionFailureCategoryStatsByCreatedRange(ctx, dbsqlc.ListTaskExecutionFailureCategoryStatsByCreatedRangeParams{
		StartedAt: start,
		EndedAt:   end,
	})
	if err != nil {
		return nil, err
	}
	result := make([]ExecutionFailureCategoryAggregate, 0, len(rows))
	for _, row := range rows {
		result = append(result, ExecutionFailureCategoryAggregate{
			Category:     row.FailureCategory,
			Total:        row.Total,
			AvgLatencyMS: row.AvgLatencyMs,
		})
	}
	return result, nil
}

func (r *Repository) List(ctx context.Context, userID int64, accountID int64, status string, keyword string, page, pageSize int) ([]*tasks.Task, tasks.TaskListCounts, error) {
	hasUserID := userID > 0
	hasAccountID := accountID > 0
	hasStatus := status != ""
	keywordLike := "%" + keyword + "%"
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

func (r *Repository) ListTaskIDsByAccountIDAndStatuses(ctx context.Context, accountID int64, statuses ...string) ([]string, error) {
	if len(statuses) == 0 {
		return r.q.ListTaskIDsByAccountID(ctx, sql.NullInt64{Int64: accountID, Valid: accountID > 0})
	}
	return r.q.ListTaskIDsByAccountIDAndStatuses(ctx, dbsqlc.ListTaskIDsByAccountIDAndStatusesParams{
		AccountID: sql.NullInt64{Int64: accountID, Valid: accountID > 0},
		Statuses:  statuses,
	})
}

func (r *Repository) CountRunningTasksMissingLeaseToken(ctx context.Context) (int64, error) {
	return r.q.CountRunningTasksMissingLeaseToken(ctx)
}

func (r *Repository) ListRunningTasksMissingLeaseToken(ctx context.Context, limit int) ([]RunningTaskLease, error) {
	if limit <= 0 {
		return nil, fmt.Errorf("limit must be positive")
	}
	rows, err := r.q.ListRunningTasksMissingLeaseToken(ctx, int32(limit))
	if err != nil {
		return nil, err
	}
	result := make([]RunningTaskLease, 0, len(rows))
	for _, row := range rows {
		result = append(result, RunningTaskLease{TaskID: row.TaskID, AccountID: nullableAccountIDValue(row.AccountID), LeaseToken: row.LeaseToken})
	}
	return result, nil
}

func (r *Repository) ResetRunningTasksMissingLeaseTokenToPending(ctx context.Context, limit int) (int64, error) {
	if limit <= 0 {
		return 0, fmt.Errorf("limit must be positive")
	}
	var rowsAffected int64
	err := dbutil.WithTx(ctx, r.db, func(q *dbsqlc.Queries) error {
		taskIDs, err := q.ListRunningMissingLeaseTaskIDs(ctx, int32(limit))
		if err != nil || len(taskIDs) == 0 {
			return err
		}
		rowsAffected, err = q.ResetRunningMissingLeaseTasksByIDs(ctx, taskIDs)
		return err
	})
	return rowsAffected, err
}

func (r *Repository) DeleteByTaskIDAndUserID(ctx context.Context, taskID string, userID int64) error {
	return r.q.DeleteTaskByTaskIDAndUserID(ctx, dbsqlc.DeleteTaskByTaskIDAndUserIDParams{
		TaskID: taskID,
		UserID: userID,
	})
}

// FailRunningByAccountID 将指定账号下所有 running 状态的任务标记为 failed
func (r *Repository) FailRunningByAccountID(ctx context.Context, accountID int64, errMsg string) ([]string, error) {
	now := time.Now()
	var taskIDs []string
	err := dbutil.WithTx(ctx, r.db, func(q *dbsqlc.Queries) error {
		var err error
		taskIDs, err = r.FailRunningByAccountIDTx(ctx, q, accountID, errMsg, now)
		return err
	})
	if err != nil {
		return nil, err
	}
	return taskIDs, nil
}

func (r *Repository) FailRunningByAccountIDTx(ctx context.Context, q *dbsqlc.Queries, accountID int64, errMsg string, completedAt time.Time) ([]string, error) {
	if q == nil {
		return nil, fmt.Errorf("queries is nil")
	}
	if completedAt.IsZero() {
		completedAt = time.Now()
	}
	updated, err := q.FailRunningTasksByAccountID(ctx, dbsqlc.FailRunningTasksByAccountIDParams{
		ErrorMsg:    errMsg,
		CompletedAt: sql.NullTime{Time: completedAt, Valid: true},
		AccountID:   sql.NullInt64{Int64: accountID, Valid: accountID > 0},
	})
	if err != nil || len(updated) == 0 {
		return nil, err
	}

	taskIDs := make([]string, 0, len(updated))
	for _, task := range updated {
		taskIDs = append(taskIDs, task.TaskID)
	}
	if _, err := q.FailRunningTaskAttemptsByAccountID(ctx, dbsqlc.FailRunningTaskAttemptsByAccountIDParams{
		FailureStage:    tasks.ExecutionStageAssigned,
		FailureCategory: tasks.FailureCategoryCancelled,
		ErrorMsg:        errMsg,
		CompletedAt:     sql.NullTime{Time: completedAt, Valid: true},
		AccountID:       accountID,
		TaskIds:         taskIDs,
	}); err != nil {
		return nil, err
	}

	singleModelByAccountID, err := listSingleEnabledModelByAccountIDsTx(ctx, q, []int64{accountID})
	if err != nil {
		return nil, err
	}
	fallbackModel := singleModelByAccountID[accountID]
	metricsByTaskID, err := listExecutionMetricsByTaskIDsTx(ctx, q, taskIDs)
	if err != nil {
		return nil, err
	}
	for _, c := range updated {
		task := &tasks.Task{
			TaskID:            c.TaskID,
			UserID:            c.UserID,
			AssignedChannelID: dbutil.Int64Ptr(c.AssignedChannelID),
			AccountID:         dbutil.Int64Ptr(c.AccountID),
			Status:            tasks.StatusFailed,
			Result:            c.Result,
			FeatureSlug:       c.FeatureSlug,
			RequestedModel:    c.RequestedModel,
			ResolvedModel:     c.ResolvedModel,
			APIType:           c.ApiType,
			CreatedAt:         c.CreatedAt,
			CompletedAt:       &completedAt,
		}
		if err := upsertDailyStatByTaskWithFallbackModelTx(ctx, q, task, fallbackModel); err != nil {
			return nil, fmt.Errorf("upsert daily stat for task %s: %w", c.TaskID, err)
		}
		if err := patchTerminalExecutionMetricsForTaskTx(ctx, q, c.TaskID, c.CreatedAt, metricsByTaskID[c.TaskID], tasks.ExecutionStageFailed, tasks.ExecutionStageAssigned, tasks.FailureCategoryCancelled, completedAt); err != nil {
			return nil, fmt.Errorf("patch terminal execution metrics for task %s: %w", c.TaskID, err)
		}
	}
	return taskIDs, nil
}

func (r *Repository) CountByStatus(ctx context.Context, status string) (int64, error) {
	return r.q.CountTasksByStatus(ctx, status)
}

func (r *Repository) CountAllTasks(ctx context.Context) (int64, error) {
	return r.q.CountAllTasks(ctx)
}

func (r *Repository) ListStatisticsByCreatedAtRange(ctx context.Context, start, end time.Time) ([]TaskStatisticLite, error) {
	rows, err := r.q.ListTaskStatisticsByCreatedRange(ctx, dbsqlc.ListTaskStatisticsByCreatedRangeParams{StartedAt: start, EndedAt: end})
	if err != nil {
		return nil, err
	}
	result := make([]TaskStatisticLite, 0, len(rows))
	for _, row := range rows {
		result = append(result, TaskStatisticLite{
			TaskID:         row.TaskID,
			UserID:         row.UserID,
			AccountID:      dbutil.Int64Ptr(row.AccountID),
			Status:         row.Status,
			Result:         row.Result,
			FeatureSlug:    row.FeatureSlug,
			RequestedModel: row.RequestedModel,
			ResolvedModel:  row.ResolvedModel,
			APIType:        row.ApiType,
			CreatedAt:      row.CreatedAt,
			CompletedAt:    dbutil.TimePtr(row.CompletedAt),
		})
	}
	return result, nil
}

func (r *Repository) UpsertDailyStatByTask(ctx context.Context, task *tasks.Task) error {
	return r.UpsertDailyStatByTaskWithFallbackModel(ctx, task, "")
}

func (r *Repository) UpsertDailyStatByTaskWithFallbackModel(ctx context.Context, task *tasks.Task, fallbackModel string) error {
	return dbutil.WithTx(ctx, r.db, func(q *dbsqlc.Queries) error {
		if task != nil && strings.TrimSpace(fallbackModel) == "" && taskNeedsFallbackModel(task) {
			fallbackModel = resolveAccountSingleEnabledModelTx(ctx, q, task.AccountID)
		}
		return upsertDailyStatByTaskWithFallbackModelTx(ctx, q, task, fallbackModel)
	})
}

func upsertDailyStatByTaskIDTx(ctx context.Context, q *dbsqlc.Queries, taskID string) error {
	row, err := q.GetTaskByTaskID(ctx, taskID)
	if err != nil {
		return err
	}
	task := taskFromSQLC(row)
	fallbackModel := ""
	if taskNeedsFallbackModel(&task) {
		fallbackModel = resolveAccountSingleEnabledModelTx(ctx, q, task.AccountID)
	}
	return upsertDailyStatByTaskWithFallbackModelTx(ctx, q, &task, fallbackModel)
}

func upsertDailyStatByTaskWithFallbackModelTx(ctx context.Context, q *dbsqlc.Queries, task *tasks.Task, fallbackModel string) error {
	if task == nil || task.Status == "" || task.TaskID == "" {
		return nil
	}
	if task.Status != tasks.StatusCompleted && task.Status != tasks.StatusFailed && task.Status != tasks.StatusCancelled {
		return nil
	}
	loc := time.Now().Location()
	createdLocal := task.CreatedAt.In(loc)
	statDate := taskStatDateFromTime(task.CreatedAt, loc)
	model := taskStatModel(task, fallbackModel)
	user := extractTaskUser(task.Result, task.UserID)
	userID := nonNegativeIDValue(task.UserID)
	accountID := positiveIDValue(task.AccountID)
	channelID := positiveIDValue(task.AssignedChannelID)
	featureSlug := normalizeStatFeature(task.FeatureSlug)
	apiType := normalizeStatAPIType(task.APIType)

	completedDelta := int64(0)
	failedDelta := int64(0)
	cancelledDelta := int64(0)
	latencySeconds := float64(0)
	latencyCountDelta := int64(0)
	switch task.Status {
	case tasks.StatusCompleted:
		completedDelta = 1
		if task.CompletedAt != nil {
			end := task.CompletedAt.In(loc)
			if end.After(createdLocal) {
				latencySeconds = end.Sub(createdLocal).Seconds()
				latencyCountDelta = 1
			}
		}
	case tasks.StatusFailed:
		failedDelta = 1
	case tasks.StatusCancelled:
		cancelledDelta = 1
	}

	inserted, err := q.InsertTaskStatEventIgnore(ctx, dbsqlc.InsertTaskStatEventIgnoreParams{
		TaskID:         task.TaskID,
		StatDate:       statDate,
		UserID:         userID,
		AccountID:      accountID,
		ChannelID:      channelID,
		FeatureSlug:    featureSlug,
		ApiType:        apiType,
		ModelName:      model,
		UserKey:        user,
		Status:         task.Status,
		LatencySeconds: latencySeconds,
	})
	if err != nil || inserted == 0 {
		return err
	}

	return q.UpsertTaskDailyStatDelta(ctx, dbsqlc.UpsertTaskDailyStatDeltaParams{
		StatDate:          statDate,
		UserID:            userID,
		AccountID:         accountID,
		ChannelID:         channelID,
		FeatureSlug:       featureSlug,
		ApiType:           apiType,
		ModelName:         model,
		UserKey:           user,
		Total:             1,
		Completed:         completedDelta,
		Failed:            failedDelta,
		Cancelled:         cancelledDelta,
		LatencySecondsSum: latencySeconds,
		LatencyCount:      latencyCountDelta,
	})
}

func nonNegativeIDValue(value int64) int64 {
	if value < 0 {
		return 0
	}
	return value
}

func positiveIDValue(value *int64) int64 {
	if value == nil || *value <= 0 {
		return 0
	}
	return *value
}

func taskNeedsFallbackModel(task *tasks.Task) bool {
	return task != nil &&
		strings.TrimSpace(task.ResolvedModel) == "" &&
		strings.TrimSpace(task.RequestedModel) == ""
}

func taskStatModel(task *tasks.Task, fallbackModel string) string {
	if task == nil {
		return unknownModelName
	}
	if model := normalizeStatDimension(task.ResolvedModel); model != "" {
		return model
	}
	if model := normalizeStatDimension(task.RequestedModel); model != "" {
		return model
	}
	if model := ExtractTaskModel(task.Result); model != unknownModelName {
		return model
	}
	if fallbackModel = normalizeStatDimension(fallbackModel); fallbackModel != "" {
		return fallbackModel
	}
	return unknownModelName
}

func resolveAccountSingleEnabledModelTx(ctx context.Context, q *dbsqlc.Queries, accountID *int64) string {
	if accountID == nil || *accountID <= 0 {
		return ""
	}
	model, err := q.GetSingleEnabledModelByAccountID(ctx, *accountID)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(model)
}

func nullableAccountIDValue(value sql.NullInt64) int64 {
	if !value.Valid {
		return 0
	}
	return value.Int64
}

func listSingleEnabledModelByAccountIDsTx(ctx context.Context, q *dbsqlc.Queries, accountIDs []int64) (map[int64]string, error) {
	result := make(map[int64]string)
	if len(accountIDs) == 0 {
		return result, nil
	}

	uniq := make(map[int64]struct{}, len(accountIDs))
	ids := make([]int64, 0, len(accountIDs))
	for _, id := range accountIDs {
		if id == 0 {
			continue
		}
		if _, ok := uniq[id]; ok {
			continue
		}
		uniq[id] = struct{}{}
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		return result, nil
	}

	rows, err := q.ListSingleEnabledModelsByAccountIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		result[row.ID] = strings.TrimSpace(row.ModelName)
	}
	return result, nil
}

func (r *Repository) ListDailyStatsByDateRange(ctx context.Context, startDate, endDate time.Time) ([]DailyStatAggregate, error) {
	if startDate.IsZero() || endDate.IsZero() {
		return nil, fmt.Errorf("stat date range is required")
	}
	start := normalizeTaskStatDate(startDate)
	end := normalizeTaskStatDate(endDate)
	rows, err := r.q.ListDailyStatsByDateRange(ctx, dbsqlc.ListDailyStatsByDateRangeParams{StartDate: start, EndDate: end})
	if err != nil {
		return nil, err
	}
	result := make([]DailyStatAggregate, 0, len(rows))
	for _, row := range rows {
		result = append(result, DailyStatAggregate{
			StatDate:          row.StatDate,
			ModelName:         row.ModelName,
			UserKey:           row.UserKey,
			Total:             row.Total,
			Completed:         row.Completed,
			Failed:            row.Failed,
			Cancelled:         row.Cancelled,
			LatencySecondsSum: row.LatencySecondsSum,
			LatencyCount:      row.LatencyCount,
		})
	}
	return result, nil
}

func (r *Repository) ListStatEventLatencySamplesByDateRange(ctx context.Context, startDate, endDate time.Time) ([]StatEventLatencySample, error) {
	if startDate.IsZero() || endDate.IsZero() {
		return nil, fmt.Errorf("stat date range is required")
	}
	start := normalizeTaskStatDate(startDate)
	end := normalizeTaskStatDate(endDate)
	rows, err := r.q.ListStatEventLatencySamplesByDateRange(ctx, dbsqlc.ListStatEventLatencySamplesByDateRangeParams{StartDate: start, EndDate: end})
	if err != nil {
		return nil, err
	}
	result := make([]StatEventLatencySample, 0, len(rows))
	for _, row := range rows {
		result = append(result, StatEventLatencySample{ModelName: row.ModelName, LatencySeconds: row.LatencySeconds})
	}
	return result, nil
}

func (r *Repository) ListBackfilledDatesByRange(ctx context.Context, startDate, endDate time.Time) ([]time.Time, error) {
	if startDate.IsZero() || endDate.IsZero() {
		return nil, fmt.Errorf("stat date range is required")
	}
	start := normalizeTaskStatDate(startDate)
	end := normalizeTaskStatDate(endDate)
	return r.q.ListBackfilledDatesByRange(ctx, dbsqlc.ListBackfilledDatesByRangeParams{StartDate: start, EndDate: end})
}

func (r *Repository) BackfillDailyStatsByCreatedRange(ctx context.Context, startedAt, endedAt, statDate time.Time, markDone bool) (BackfillDailyStatsResult, error) {
	if startedAt.IsZero() || endedAt.IsZero() || statDate.IsZero() {
		return BackfillDailyStatsResult{}, fmt.Errorf("backfill date range is required")
	}
	if !endedAt.After(startedAt) {
		return BackfillDailyStatsResult{}, fmt.Errorf("backfill ended_at must be after started_at")
	}

	var result BackfillDailyStatsResult
	err := dbutil.WithTx(ctx, r.db, func(q *dbsqlc.Queries) error {
		row, err := q.BackfillDailyStatsByCreatedRange(ctx, dbsqlc.BackfillDailyStatsByCreatedRangeParams{
			StartedAt: startedAt,
			EndedAt:   endedAt,
			StatDate:  normalizeTaskStatDate(statDate),
			MarkDone:  markDone,
		})
		if err != nil {
			return err
		}
		result = BackfillDailyStatsResult{
			InsertedEvents: row.InsertedEvents,
			TouchedGroups:  row.TouchedGroups,
			MarkedDays:     row.MarkedDays,
		}
		return nil
	})
	return result, err
}

func taskStatDateFromTime(value time.Time, loc *time.Location) time.Time {
	if loc == nil {
		loc = time.Local
	}
	local := value.In(loc)
	return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, time.UTC)
}

func normalizeTaskStatDate(value time.Time) time.Time {
	return taskStatDateFromTime(value, value.Location())
}

func taskFromSQLC(row dbsqlc.Task) tasks.Task {
	return tasks.Task{
		ID:                row.ID,
		TaskID:            row.TaskID,
		UserID:            row.UserID,
		AssignedChannelID: dbutil.Int64Ptr(row.AssignedChannelID),
		AccountID:         dbutil.Int64Ptr(row.AccountID),
		Prompt:            row.Prompt,
		UserPrompt:        row.UserPrompt,
		Params:            row.Params,
		FeatureSlug:       row.FeatureSlug,
		RequestedModel:    row.RequestedModel,
		ResolvedModel:     row.ResolvedModel,
		APIType:           row.ApiType,
		Status:            row.Status,
		LeaseToken:        row.LeaseToken,
		RetryCount:        int(row.RetryCount),
		ErrorMsg:          row.ErrorMsg,
		Result:            row.Result,
		CreatedAt:         row.CreatedAt,
		UpdatedAt:         row.UpdatedAt,
		CompletedAt:       dbutil.TimePtr(row.CompletedAt),
		CancelledAt:       dbutil.TimePtr(row.CancelledAt),
		CancelledBy:       dbutil.Int64Ptr(row.CancelledBy),
		CancelReason:      row.CancelReason,
	}
}

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
		MediaAssetID:   int64Ptr(row.MediaAssetID),
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

func nullInt64Ptr(value *int64) sql.NullInt64 {
	if value == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: *value, Valid: true}
}

func int64Ptr(value sql.NullInt64) *int64 {
	if !value.Valid {
		return nil
	}
	v := value.Int64
	return &v
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

func optionalNonEmptyString(value *string) sql.NullString {
	if value == nil {
		return sql.NullString{}
	}
	v := strings.TrimSpace(*value)
	return sql.NullString{String: v, Valid: v != ""}
}

func optionalString(value *string) sql.NullString {
	if value == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: strings.TrimSpace(*value), Valid: true}
}

func optionalInt64(value *int64) sql.NullInt64 {
	if value == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: *value, Valid: true}
}
