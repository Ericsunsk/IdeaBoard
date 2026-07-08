package taskstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"ideaboard/api/internal/db/dbutil"
	dbsqlc "ideaboard/api/internal/db/sqlc"
	"ideaboard/api/internal/tasking/tasks"
	"strconv"
	"strings"
	"time"
)

// ErrTaskLeaseStateConflict means the task row matched, but its active lease fact did not.
var ErrTaskLeaseStateConflict = errors.New("task lease state conflict")

func (r *Repository) MarkTaskFailedWithLease(ctx context.Context, taskID string, statuses []string, accountID int64, providerEndpointID int64, credentialID int64, leaseToken string, errMsg string, result string, completedAt time.Time, attemptFinish *tasks.TaskAttemptFinish) (int64, error) {
	var rowsAffected int64
	err := dbutil.WithTx(ctx, r.db, func(q *dbsqlc.Queries) error {
		updated, err := q.MarkTaskFailedWithLease(ctx, dbsqlc.MarkTaskFailedWithLeaseParams{
			ErrorMsg:                   errMsg,
			Result:                     result,
			CompletedAt:                sql.NullTime{Time: completedAt, Valid: true},
			TaskID:                     taskID,
			Statuses:                   statuses,
			AccountID:                  sql.NullInt64{Int64: accountID, Valid: accountID > 0},
			AssignedProviderEndpointID: sql.NullInt64{Int64: providerEndpointID, Valid: providerEndpointID > 0},
			AssignedCredentialID:       sql.NullInt64{Int64: credentialID, Valid: credentialID > 0},
			LeaseToken:                 leaseToken,
		})
		if err != nil || updated == 0 {
			return err
		}
		if err := releaseTaskLeaseTx(ctx, q, taskID, accountID, providerEndpointID, credentialID, leaseToken, tasks.TaskAttemptStatusFailed, completedAt); err != nil {
			return err
		}
		if err := upsertDailyStatByTaskIDTx(ctx, q, taskID, attemptFinish); err != nil {
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

func (r *Repository) ResetTaskRunningToPendingWithLease(ctx context.Context, taskID string, statuses []string, accountID int64, providerEndpointID int64, credentialID int64, leaseToken string, errMsg string, attemptFinish *tasks.TaskAttemptFinish) (int64, error) {
	params := dbsqlc.ResetTaskRunningToPendingWithLeaseParams{
		ErrorMsg:                   errMsg,
		TaskID:                     taskID,
		Statuses:                   statuses,
		AccountID:                  sql.NullInt64{Int64: accountID, Valid: accountID > 0},
		AssignedProviderEndpointID: sql.NullInt64{Int64: providerEndpointID, Valid: providerEndpointID > 0},
		AssignedCredentialID:       sql.NullInt64{Int64: credentialID, Valid: credentialID > 0},
		LeaseToken:                 leaseToken,
	}
	var rowsAffected int64
	err := dbutil.WithTx(ctx, r.db, func(q *dbsqlc.Queries) error {
		updated, err := q.ResetTaskRunningToPendingWithLease(ctx, params)
		if err != nil || updated == 0 {
			return err
		}
		if err := releaseTaskLeaseTx(ctx, q, taskID, accountID, providerEndpointID, credentialID, leaseToken, "reset", time.Now()); err != nil {
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

func (r *Repository) RequeueTaskRunningWithLease(ctx context.Context, taskID string, statuses []string, accountID int64, providerEndpointID int64, credentialID int64, leaseToken string, errMsg string, attemptFinish *tasks.TaskAttemptFinish) (int64, error) {
	params := dbsqlc.RequeueTaskRunningWithLeaseParams{
		ErrorMsg:                   errMsg,
		TaskID:                     taskID,
		Statuses:                   statuses,
		AccountID:                  sql.NullInt64{Int64: accountID, Valid: accountID > 0},
		AssignedProviderEndpointID: sql.NullInt64{Int64: providerEndpointID, Valid: providerEndpointID > 0},
		AssignedCredentialID:       sql.NullInt64{Int64: credentialID, Valid: credentialID > 0},
		LeaseToken:                 leaseToken,
	}
	var rowsAffected int64
	err := dbutil.WithTx(ctx, r.db, func(q *dbsqlc.Queries) error {
		updated, err := q.RequeueTaskRunningWithLease(ctx, params)
		if err != nil || updated == 0 {
			return err
		}
		releasedAt := time.Now()
		if attemptFinish != nil && !attemptFinish.CompletedAt.IsZero() {
			releasedAt = attemptFinish.CompletedAt
		}
		if err := releaseTaskLeaseTx(ctx, q, taskID, accountID, providerEndpointID, credentialID, leaseToken, tasks.TaskAttemptStatusRequeued, releasedAt); err != nil {
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

func (r *Repository) FinalFailTaskRunningWithLease(ctx context.Context, taskID string, statuses []string, accountID int64, providerEndpointID int64, credentialID int64, leaseToken string, errMsg string, result string, completedAt time.Time, attemptFinish *tasks.TaskAttemptFinish) (int64, error) {
	var rowsAffected int64
	err := dbutil.WithTx(ctx, r.db, func(q *dbsqlc.Queries) error {
		updated, err := q.FinalFailTaskRunningWithLease(ctx, dbsqlc.FinalFailTaskRunningWithLeaseParams{
			ErrorMsg:                   errMsg,
			Result:                     result,
			CompletedAt:                sql.NullTime{Time: completedAt, Valid: true},
			TaskID:                     taskID,
			Statuses:                   statuses,
			AccountID:                  sql.NullInt64{Int64: accountID, Valid: accountID > 0},
			AssignedProviderEndpointID: sql.NullInt64{Int64: providerEndpointID, Valid: providerEndpointID > 0},
			AssignedCredentialID:       sql.NullInt64{Int64: credentialID, Valid: credentialID > 0},
			LeaseToken:                 leaseToken,
		})
		if err != nil || updated == 0 {
			return err
		}
		if err := releaseTaskLeaseTx(ctx, q, taskID, accountID, providerEndpointID, credentialID, leaseToken, tasks.TaskAttemptStatusFailed, completedAt); err != nil {
			return err
		}
		if err := upsertDailyStatByTaskIDTx(ctx, q, taskID, attemptFinish); err != nil {
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

func (r *Repository) CompleteWithOutputsAndLeaseToken(ctx context.Context, taskID string, statuses []string, accountID int64, providerEndpointID int64, credentialID int64, leaseToken string, result string, completedAt time.Time, outputs []tasks.TaskOutput, attemptFinish *tasks.TaskAttemptFinish) (int64, error) {
	var rowsAffected int64
	err := dbutil.WithTx(ctx, r.db, func(q *dbsqlc.Queries) error {
		updated, err := q.CompleteTaskWithLease(ctx, dbsqlc.CompleteTaskWithLeaseParams{
			Result:                     result,
			CompletedAt:                sql.NullTime{Time: completedAt, Valid: true},
			TaskID:                     taskID,
			Statuses:                   statuses,
			AccountID:                  sql.NullInt64{Int64: accountID, Valid: accountID > 0},
			AssignedProviderEndpointID: sql.NullInt64{Int64: providerEndpointID, Valid: providerEndpointID > 0},
			AssignedCredentialID:       sql.NullInt64{Int64: credentialID, Valid: credentialID > 0},
			LeaseToken:                 leaseToken,
		})
		if err != nil || updated == 0 {
			return err
		}
		if err := releaseTaskLeaseTx(ctx, q, taskID, accountID, providerEndpointID, credentialID, leaseToken, tasks.TaskAttemptStatusCompleted, completedAt); err != nil {
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
		if err := upsertDailyStatByTaskIDTx(ctx, q, taskID, attemptFinish); err != nil {
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
		output.OutputKind = normalizeTaskOutputKind(output)
		if strings.TrimSpace(output.Role) == "" {
			output.Role = "image"
		}
		if output.OutputKind == "asset" {
			mediaAssetID, err := upsertTaskOutputMediaAsset(ctx, q, ownerUserID, output)
			if err != nil {
				return err
			}
			output.MediaAssetID = mediaAssetID
			output.URL = ""
		} else {
			output.MediaAssetID = nil
		}
		taskOutputID, err := q.CreateTaskOutput(ctx, dbsqlc.CreateTaskOutputParams{
			TaskID:       output.TaskID,
			MediaAssetID: dbutil.NullInt64(output.MediaAssetID),
			OutputKind:   output.OutputKind,
			Role:         output.Role,
			Url:          output.URL,
			SourceUrl:    output.SourceURL,
			Metadata:     outputMetadataJSON(output.Metadata),
			SortIndex:    int32(output.SortIndex),
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

func normalizeTaskOutputKind(output tasks.TaskOutput) string {
	switch strings.TrimSpace(output.OutputKind) {
	case "remote":
		return "remote"
	default:
		return "asset"
	}
}

func releaseTaskLeaseTx(ctx context.Context, q *dbsqlc.Queries, taskID string, accountID int64, providerEndpointID int64, credentialID int64, leaseToken string, reason string, releasedAt time.Time) error {
	if q == nil {
		return fmt.Errorf("queries is nil")
	}
	taskID = strings.TrimSpace(taskID)
	leaseToken = strings.TrimSpace(leaseToken)
	if taskID == "" || leaseToken == "" {
		return fmt.Errorf("%w: release task lease requires task_id and lease_token", ErrTaskLeaseStateConflict)
	}
	if releasedAt.IsZero() {
		releasedAt = time.Now()
	}
	released, err := q.ReleaseTaskLease(ctx, dbsqlc.ReleaseTaskLeaseParams{
		ReleasedAt:         sql.NullTime{Time: releasedAt, Valid: true},
		ReleaseReason:      strings.TrimSpace(reason),
		TaskID:             taskID,
		LeaseToken:         leaseToken,
		AccountID:          accountID,
		ProviderEndpointID: providerEndpointID,
		CredentialID:       credentialID,
	})
	if err != nil {
		return err
	}
	if released == 0 {
		return fmt.Errorf("%w: release task lease matched 0 rows for task_id=%s", ErrTaskLeaseStateConflict, taskID)
	}
	return nil
}

func releaseActiveTaskLeaseByTaskIDTx(ctx context.Context, q *dbsqlc.Queries, taskID string, reason string, releasedAt time.Time) error {
	if q == nil {
		return fmt.Errorf("queries is nil")
	}
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return nil
	}
	if releasedAt.IsZero() {
		releasedAt = time.Now()
	}
	_, err := q.ReleaseActiveTaskLeaseByTaskID(ctx, dbsqlc.ReleaseActiveTaskLeaseByTaskIDParams{
		ReleasedAt:    sql.NullTime{Time: releasedAt, Valid: true},
		ReleaseReason: strings.TrimSpace(reason),
		TaskID:        taskID,
	})
	return err
}

func releaseActiveTaskLeasesByTaskIDsTx(ctx context.Context, q *dbsqlc.Queries, taskIDs []string, reason string, releasedAt time.Time) error {
	if q == nil {
		return fmt.Errorf("queries is nil")
	}
	if len(taskIDs) == 0 {
		return nil
	}
	if releasedAt.IsZero() {
		releasedAt = time.Now()
	}
	_, err := q.ReleaseActiveTaskLeasesByTaskIDs(ctx, dbsqlc.ReleaseActiveTaskLeasesByTaskIDsParams{
		ReleasedAt:    sql.NullTime{Time: releasedAt, Valid: true},
		ReleaseReason: strings.TrimSpace(reason),
		TaskIds:       taskIDs,
	})
	return err
}

func (r *Repository) GetActiveTaskLeaseByTaskID(ctx context.Context, taskID string) (*ActiveTaskLease, error) {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return nil, fmt.Errorf("task_id is required")
	}
	row, err := r.q.GetActiveTaskLeaseByTaskID(ctx, taskID)
	if err != nil {
		return nil, err
	}
	return &ActiveTaskLease{
		TaskID:                  row.TaskID,
		LeaseToken:              row.LeaseToken,
		RiverJobID:              row.RiverJobID,
		AccountID:               row.AccountID,
		ProviderEndpointID:      row.ProviderEndpointID,
		CredentialID:            row.CredentialID,
		FeatureSlug:             row.FeatureSlug,
		RequestedModel:          row.RequestedModel,
		ResolvedModel:           row.ResolvedModel,
		APIType:                 row.ApiType,
		ProviderEndpointName:    row.ProviderEndpointName,
		ProviderEndpointBaseURL: row.ProviderEndpointBaseUrl,
		CredentialFingerprint:   row.CredentialFingerprint,
		CredentialKeyVersion:    row.CredentialKeyVersion,
		AcquiredAt:              row.AcquiredAt,
	}, nil
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
		if err := releaseActiveTaskLeaseByTaskIDTx(ctx, q, taskID, tasks.TaskAttemptStatusCancelled, cancelledAt); err != nil {
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
		if err := upsertDailyStatByTaskIDTx(ctx, q, taskID, attemptFinish); err != nil {
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

func nullableAccountIDValue(value sql.NullInt64) int64 {
	if !value.Valid {
		return 0
	}
	return value.Int64
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
	if err := releaseActiveTaskLeasesByTaskIDsTx(ctx, q, taskIDs, "account_failed", completedAt); err != nil {
		return nil, err
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

	metricsByTaskID, err := listExecutionMetricsByTaskIDsTx(ctx, q, taskIDs)
	if err != nil {
		return nil, err
	}
	for _, c := range updated {
		task := &tasks.Task{
			TaskID:                     c.TaskID,
			UserID:                     c.UserID,
			AssignedProviderEndpointID: dbutil.Int64Ptr(c.AssignedProviderEndpointID),
			AssignedCredentialID:       dbutil.Int64Ptr(c.AssignedCredentialID),
			AccountID:                  dbutil.Int64Ptr(c.AccountID),
			Status:                     tasks.StatusFailed,
			FeatureSlug:                c.FeatureSlug,
			RequestedModel:             c.RequestedModel,
			ResolvedModel:              c.ResolvedModel,
			APIType:                    c.ApiType,
			CreatedAt:                  c.CreatedAt,
			CompletedAt:                &completedAt,
		}
		if err := upsertDailyStatByTaskTx(ctx, q, task, nil); err != nil {
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
