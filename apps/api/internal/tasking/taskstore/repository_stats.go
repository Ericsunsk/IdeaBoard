package taskstore

import (
	"context"
	"database/sql"
	"fmt"
	"ideaboard/api/internal/db/dbutil"
	dbsqlc "ideaboard/api/internal/db/sqlc"
	"ideaboard/api/internal/tasking/tasks"
	"strings"
	"time"
)

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
