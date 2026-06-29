package taskstore

import (
	"context"
	"database/sql"
	"fmt"
	"ideaboard/api/internal/db/dbutil"
	dbsqlc "ideaboard/api/internal/db/sqlc"
	"ideaboard/api/internal/tasking/tasks"
	"strconv"
	"strings"
	"time"
)

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
			MediaAssetID:   dbutil.NullInt64(output.MediaAssetID),
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

func (r *Repository) FailRunningByAccountIDWithDBTX(ctx context.Context, tx dbutil.DBTX, accountID int64, errMsg string, completedAt time.Time) ([]string, error) {
	if tx == nil {
		return nil, fmt.Errorf("tx is nil")
	}
	return r.FailRunningByAccountIDTx(ctx, dbsqlc.New(tx), accountID, errMsg, completedAt)
}
