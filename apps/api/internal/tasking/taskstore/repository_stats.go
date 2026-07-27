package taskstore

import (
	"context"
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

func (r *Repository) UpsertDailyStatByTask(ctx context.Context, task *tasks.Task) error {
	return dbutil.WithTx(ctx, r.db, func(q *dbsqlc.Queries) error {
		return upsertDailyStatByTaskTx(ctx, q, task, nil)
	})
}

func upsertDailyStatByTaskIDTx(ctx context.Context, q *dbsqlc.Queries, taskID string, attemptFinish *tasks.TaskAttemptFinish) error {
	row, err := q.GetTaskByTaskID(ctx, taskID)
	if err != nil {
		return err
	}
	task := taskFromSQLC(row)
	return upsertDailyStatByTaskTx(ctx, q, &task, attemptFinish)
}

func upsertDailyStatByTaskTx(ctx context.Context, q *dbsqlc.Queries, task *tasks.Task, attemptFinish *tasks.TaskAttemptFinish) error {
	event, ok := taskStatEventFromTask(task, attemptFinish)
	if !ok {
		return nil
	}

	inserted, err := q.InsertTaskStatEventIgnore(ctx, dbsqlc.InsertTaskStatEventIgnoreParams{
		TaskID:             event.TaskID,
		OccurredAt:         event.OccurredAt,
		StatDate:           event.StatDate,
		UserID:             event.UserID,
		AccountID:          event.AccountID,
		ProviderEndpointID: event.ProviderEndpointID,
		CredentialID:       event.CredentialID,
		FeatureSlug:        event.FeatureSlug,
		ApiType:            event.APIType,
		ModelName:          event.ModelName,
		UserKey:            event.UserKey,
		Status:             event.Status,
		LatencySeconds:     event.LatencySeconds,
		CostMicros:         event.CostMicros,
	})
	if err != nil || inserted == 0 {
		return err
	}

	return upsertTaskDailyStatDeltaTx(ctx, q, event)
}

type normalizedTaskStatEvent struct {
	TaskID             string
	OccurredAt         time.Time
	StatDate           time.Time
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

func taskStatEventFromTask(task *tasks.Task, attemptFinish *tasks.TaskAttemptFinish) (normalizedTaskStatEvent, bool) {
	if task == nil || strings.TrimSpace(task.Status) == "" || strings.TrimSpace(task.TaskID) == "" {
		return normalizedTaskStatEvent{}, false
	}
	if task.Status != tasks.StatusCompleted && task.Status != tasks.StatusFailed && task.Status != tasks.StatusCancelled {
		return normalizedTaskStatEvent{}, false
	}
	occurredAt := taskStatOccurredAt(task)
	loc := time.Now().Location()
	event := normalizedTaskStatEvent{
		TaskID:             strings.TrimSpace(task.TaskID),
		OccurredAt:         occurredAt,
		StatDate:           taskStatDateFromTime(occurredAt, loc),
		UserID:             nonNegativeIDValue(task.UserID),
		AccountID:          positiveIDValue(task.AccountID),
		ProviderEndpointID: positiveIDValue(task.AssignedProviderEndpointID),
		CredentialID:       positiveIDValue(task.AssignedCredentialID),
		FeatureSlug:        normalizeStatFeature(task.FeatureSlug),
		APIType:            normalizeStatAPIType(task.APIType),
		ModelName:          taskStatModel(task),
		UserKey:            taskStatUserKey(task.UserID),
		Status:             task.Status,
		LatencySeconds:     taskStatLatencySeconds(task, attemptFinish, occurredAt),
		CostMicros:         0,
	}
	return event, true
}

func taskStatOccurredAt(task *tasks.Task) time.Time {
	switch task.Status {
	case tasks.StatusCancelled:
		if task.CancelledAt != nil && !task.CancelledAt.IsZero() {
			return *task.CancelledAt
		}
	}
	if task.CompletedAt != nil && !task.CompletedAt.IsZero() {
		return *task.CompletedAt
	}
	if task.UpdatedAt.IsZero() {
		return time.Now()
	}
	return task.UpdatedAt
}

func taskStatLatencySeconds(task *tasks.Task, attemptFinish *tasks.TaskAttemptFinish, occurredAt time.Time) float64 {
	if task.Status != tasks.StatusCompleted {
		return 0
	}
	if attemptFinish != nil && attemptFinish.TotalLatencyMS > 0 {
		return float64(attemptFinish.TotalLatencyMS) / 1000
	}
	if !task.CreatedAt.IsZero() && occurredAt.After(task.CreatedAt) {
		return occurredAt.Sub(task.CreatedAt).Seconds()
	}
	return 0
}

func upsertTaskDailyStatDeltaTx(ctx context.Context, q *dbsqlc.Queries, event normalizedTaskStatEvent) error {
	completedDelta := int64(0)
	failedDelta := int64(0)
	cancelledDelta := int64(0)
	latencyCountDelta := int64(0)
	switch event.Status {
	case tasks.StatusCompleted:
		completedDelta = 1
		if event.LatencySeconds > 0 {
			latencyCountDelta = 1
		}
	case tasks.StatusFailed:
		failedDelta = 1
	case tasks.StatusCancelled:
		cancelledDelta = 1
	}

	return q.UpsertTaskDailyStatDelta(ctx, dbsqlc.UpsertTaskDailyStatDeltaParams{
		StatDate:           event.StatDate,
		UserID:             event.UserID,
		AccountID:          event.AccountID,
		ProviderEndpointID: event.ProviderEndpointID,
		CredentialID:       event.CredentialID,
		FeatureSlug:        event.FeatureSlug,
		ApiType:            event.APIType,
		ModelName:          event.ModelName,
		UserKey:            event.UserKey,
		Total:              1,
		Completed:          completedDelta,
		Failed:             failedDelta,
		Cancelled:          cancelledDelta,
		LatencySecondsSum:  event.LatencySeconds,
		LatencyCount:       latencyCountDelta,
		CostMicrosSum:      event.CostMicros,
	})
}

func positiveIDValue(value *int64) int64 {
	if value == nil || *value <= 0 {
		return 0
	}
	return *value
}

func nonNegativeIDValue(value int64) int64 {
	if value < 0 {
		return 0
	}
	return value
}

func taskStatModel(task *tasks.Task) string {
	if task == nil {
		return unknownModelName
	}
	if model := normalizeStatDimension(task.ResolvedModel); model != "" {
		return model
	}
	return unknownModelName
}

func taskStatUserKey(userID int64) string {
	if userID <= 0 {
		return "unknown"
	}
	return normalizeStatDimension(fmt.Sprintf("%d", userID))
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
			CostMicrosSum:     row.CostMicrosSum,
		})
	}
	return result, nil
}

func (r *Repository) ListStatEventsByOccurredRange(ctx context.Context, start, end time.Time) ([]StatEventTrendRow, error) {
	if start.IsZero() || end.IsZero() {
		return nil, fmt.Errorf("stat event occurred range is required")
	}
	rows, err := r.q.ListStatEventsByOccurredRange(ctx, dbsqlc.ListStatEventsByOccurredRangeParams{StartedAt: start, EndedAt: end})
	if err != nil {
		return nil, err
	}
	result := make([]StatEventTrendRow, 0, len(rows))
	for _, row := range rows {
		result = append(result, StatEventTrendRow{
			OccurredAt:     row.OccurredAt,
			StatDate:       row.StatDate,
			ModelName:      row.ModelName,
			UserKey:        row.UserKey,
			Status:         row.Status,
			LatencySeconds: row.LatencySeconds,
			CostMicros:     row.CostMicros,
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

func (r *Repository) RebuildDailyStatsFromEventsByDateRange(ctx context.Context, startDate, endDate time.Time) (RebuildDailyStatsResult, error) {
	if startDate.IsZero() || endDate.IsZero() {
		return RebuildDailyStatsResult{}, fmt.Errorf("stat date range is required")
	}
	start := normalizeTaskStatDate(startDate)
	end := normalizeTaskStatDate(endDate)
	row, err := r.q.RebuildTaskDailyStatsByEventDateRange(ctx, dbsqlc.RebuildTaskDailyStatsByEventDateRangeParams{StartDate: start, EndDate: end})
	if err != nil {
		return RebuildDailyStatsResult{}, err
	}
	return RebuildDailyStatsResult{DeletedGroups: row.DeletedGroups, RebuiltGroups: row.RebuiltGroups}, nil
}

func (r *Repository) CorrectTaskStatEvent(ctx context.Context, correction TaskStatEventCorrection) (int64, error) {
	event, ok := taskStatEventFromCorrection(correction)
	if !ok {
		return 0, fmt.Errorf("task stat event correction is invalid")
	}
	return r.q.UpsertTaskStatEventCorrection(ctx, dbsqlc.UpsertTaskStatEventCorrectionParams{
		TaskID:             event.TaskID,
		OccurredAt:         event.OccurredAt,
		StatDate:           event.StatDate,
		UserID:             event.UserID,
		AccountID:          event.AccountID,
		ProviderEndpointID: event.ProviderEndpointID,
		CredentialID:       event.CredentialID,
		FeatureSlug:        event.FeatureSlug,
		ApiType:            event.APIType,
		ModelName:          event.ModelName,
		UserKey:            event.UserKey,
		Status:             event.Status,
		LatencySeconds:     event.LatencySeconds,
		CostMicros:         event.CostMicros,
	})
}

func taskStatEventFromCorrection(correction TaskStatEventCorrection) (normalizedTaskStatEvent, bool) {
	taskID := strings.TrimSpace(correction.TaskID)
	if taskID == "" {
		return normalizedTaskStatEvent{}, false
	}
	if correction.Status != tasks.StatusCompleted && correction.Status != tasks.StatusFailed && correction.Status != tasks.StatusCancelled {
		return normalizedTaskStatEvent{}, false
	}
	occurredAt := correction.OccurredAt
	if occurredAt.IsZero() {
		occurredAt = time.Now()
	}
	model := normalizeStatDimension(correction.ModelName)
	if model == "" {
		model = unknownModelName
	}
	userKey := normalizeStatDimension(correction.UserKey)
	if userKey == "" {
		userKey = taskStatUserKey(correction.UserID)
	}
	latencySeconds := correction.LatencySeconds
	if latencySeconds < 0 {
		latencySeconds = 0
	}
	costMicros := correction.CostMicros
	if costMicros < 0 {
		costMicros = 0
	}
	return normalizedTaskStatEvent{
		TaskID:             taskID,
		OccurredAt:         occurredAt,
		StatDate:           taskStatDateFromTime(occurredAt, time.Now().Location()),
		UserID:             nonNegativeIDValue(correction.UserID),
		AccountID:          nonNegativeIDValue(correction.AccountID),
		ProviderEndpointID: nonNegativeIDValue(correction.ProviderEndpointID),
		CredentialID:       nonNegativeIDValue(correction.CredentialID),
		FeatureSlug:        normalizeStatFeature(correction.FeatureSlug),
		APIType:            normalizeStatAPIType(correction.APIType),
		ModelName:          model,
		UserKey:            userKey,
		Status:             correction.Status,
		LatencySeconds:     latencySeconds,
		CostMicros:         costMicros,
	}, true
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
			StartedAt: dbutil.NullTime(&startedAt),
			EndedAt:   dbutil.NullTime(&endedAt),
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

type taskLeaseSnapshot struct {
	ResolvedModel           string
	APIType                 string
	ProviderEndpointName    string
	ProviderEndpointBaseURL string
	CredentialFingerprint   string
	CredentialKeyVersion    string
}

func taskFromSQLC(row any) tasks.Task {
	switch r := row.(type) {
	case dbsqlc.GetTaskByTaskIDRow:
		return taskFromSQLCModel(r.Task, taskLeaseSnapshot{
			ResolvedModel:           r.ResolvedModel,
			APIType:                 r.ApiType,
			ProviderEndpointName:    r.ProviderEndpointName,
			ProviderEndpointBaseURL: r.ProviderEndpointBaseUrl,
			CredentialFingerprint:   r.CredentialFingerprint,
			CredentialKeyVersion:    r.CredentialKeyVersion,
		})
	case dbsqlc.GetTaskByTaskIDAndUserIDRow:
		return taskFromSQLCModel(r.Task, taskLeaseSnapshot{
			ResolvedModel:           r.ResolvedModel,
			APIType:                 r.ApiType,
			ProviderEndpointName:    r.ProviderEndpointName,
			ProviderEndpointBaseURL: r.ProviderEndpointBaseUrl,
			CredentialFingerprint:   r.CredentialFingerprint,
			CredentialKeyVersion:    r.CredentialKeyVersion,
		})
	case dbsqlc.ListTasksRow:
		return taskFromSQLCModel(r.Task, taskLeaseSnapshot{
			ResolvedModel:           r.ResolvedModel,
			APIType:                 r.ApiType,
			ProviderEndpointName:    r.ProviderEndpointName,
			ProviderEndpointBaseURL: r.ProviderEndpointBaseUrl,
			CredentialFingerprint:   r.CredentialFingerprint,
			CredentialKeyVersion:    r.CredentialKeyVersion,
		})
	case dbsqlc.ListTasksByTaskIDsRow:
		return taskFromSQLCModel(r.Task, taskLeaseSnapshot{
			ResolvedModel:           r.ResolvedModel,
			APIType:                 r.ApiType,
			ProviderEndpointName:    r.ProviderEndpointName,
			ProviderEndpointBaseURL: r.ProviderEndpointBaseUrl,
			CredentialFingerprint:   r.CredentialFingerprint,
			CredentialKeyVersion:    r.CredentialKeyVersion,
		})
	case dbsqlc.ListTasksByUserIDRow:
		return taskFromSQLCModel(r.Task, taskLeaseSnapshot{
			ResolvedModel:           r.ResolvedModel,
			APIType:                 r.ApiType,
			ProviderEndpointName:    r.ProviderEndpointName,
			ProviderEndpointBaseURL: r.ProviderEndpointBaseUrl,
			CredentialFingerprint:   r.CredentialFingerprint,
			CredentialKeyVersion:    r.CredentialKeyVersion,
		})
	case dbsqlc.Task:
		return taskFromSQLCModel(r, taskLeaseSnapshot{})
	default:
		panic(fmt.Sprintf("unsupported task sqlc row %T", row))
	}
}

func taskFromSQLCModel(row dbsqlc.Task, lease taskLeaseSnapshot) tasks.Task {
	return tasks.Task{
		ID:                         row.ID,
		TaskID:                     row.TaskID,
		CorrelationID:              tasks.CorrelationIDForTaskID(row.TaskID),
		UserID:                     row.UserID,
		ClientActionID:             row.ClientActionID,
		AssignedProviderEndpointID: dbutil.Int64Ptr(row.AssignedProviderEndpointID),
		AssignedCredentialID:       dbutil.Int64Ptr(row.AssignedCredentialID),
		AccountID:                  dbutil.Int64Ptr(row.AccountID),
		Prompt:                     row.Prompt,
		UserPrompt:                 row.UserPrompt,
		Params:                     row.Params,
		FeatureSlug:                row.FeatureSlug,
		Priority:                   int(row.Priority),
		ResolvedModel:              lease.ResolvedModel,
		APIType:                    lease.APIType,
		ProviderEndpointName:       lease.ProviderEndpointName,
		ProviderEndpointBaseURL:    lease.ProviderEndpointBaseURL,
		CredentialFingerprint:      lease.CredentialFingerprint,
		CredentialKeyVersion:       lease.CredentialKeyVersion,
		Status:                     row.Status,
		LeaseToken:                 row.LeaseToken,
		RetryCount:                 int(row.RetryCount),
		ErrorMsg:                   row.ErrorMsg,
		Result:                     row.Result,
		CreatedAt:                  row.CreatedAt,
		UpdatedAt:                  row.UpdatedAt,
		CompletedAt:                dbutil.TimePtr(row.CompletedAt),
		CancelledAt:                dbutil.TimePtr(row.CancelledAt),
		CancelledBy:                dbutil.Int64Ptr(row.CancelledBy),
		CancelReason:               row.CancelReason,
	}
}
