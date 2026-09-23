package postgres

import (
	"context"
	"fmt"
	statisticscontracts "ideaboard/api/internal/tasking/ports/statistics"
	"strings"
	"time"

	"ideaboard/api/internal/db/dbutil"
	dbsqlc "ideaboard/api/internal/db/sqlc"
	tasks "ideaboard/api/internal/tasking/domain"
	"ideaboard/api/internal/workflow"
)

func (r *Repository) CountByStatus(ctx context.Context, status string) (int64, error) {
	return r.q.CountTasksByStatus(ctx, status)
}

func (r *Repository) CountAllTaskCalls(ctx context.Context) (int64, error) {
	return r.q.CountAllTaskCalls(ctx)
}

func (r *Repository) UpsertDailyStatByTask(ctx context.Context, task *tasks.Task) error {
	return dbutil.WithTx(ctx, r.db, func(q *dbsqlc.Queries) error {
		return upsertDailyStatByTaskTx(ctx, q, task, nil)
	})
}

func upsertDailyStatByTaskTx(ctx context.Context, q *dbsqlc.Queries, task *tasks.Task, attemptFinish *tasks.TaskAttemptFinish) error {
	event, ok, err := taskStatEventFromTask(task, attemptFinish)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}

	inserted, err := q.InsertTaskStatEventIgnore(ctx, dbsqlc.InsertTaskStatEventIgnoreParams{
		TaskID:             event.TaskID,
		OccurredAt:         dbutil.TimestamptzFromTime(event.OccurredAt),
		StatDate:           dbutil.DateFromTime(event.StatDate),
		UserID:             event.UserID,
		AccountID:          event.AccountID,
		ProviderEndpointID: event.ProviderEndpointID,
		CredentialID:       event.CredentialID,
		FeatureSlug:        event.FeatureSlug,
		ApiType:            event.APIType,
		ModelName:          event.ModelName,
		UserKey:            event.UserKey,
		Status:             event.Status,
		OutputCount:        int16(event.OutputCount),
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
	OutputCount        int64
	LatencySeconds     float64
	CostMicros         int64
}

func taskStatEventFromTask(task *tasks.Task, attemptFinish *tasks.TaskAttemptFinish) (normalizedTaskStatEvent, bool, error) {
	if task == nil || strings.TrimSpace(task.Status) == "" || strings.TrimSpace(task.TaskID) == "" {
		return normalizedTaskStatEvent{}, false, nil
	}
	if task.Status != tasks.StatusCompleted && task.Status != tasks.StatusFailed && !tasks.IsTerminalStatus(task.Status) {
		return normalizedTaskStatEvent{}, false, nil
	}
	outputCount, err := taskStatOutputCount(task.Params)
	if err != nil {
		return normalizedTaskStatEvent{}, false, fmt.Errorf("resolve task stat output count: %w", err)
	}
	occurredAt := taskStatOccurredAt(task)
	loc := time.Now().Location()
	statStatus := task.Status
	if statStatus == tasks.StatusCancelledWithResults {
		statStatus = tasks.StatusCancelled
	}
	event := normalizedTaskStatEvent{
		TaskID:             strings.TrimSpace(task.TaskID),
		OccurredAt:         occurredAt,
		StatDate:           taskStatDateFromTime(occurredAt, loc),
		UserID:             nonNegativeIDValue(task.UserID),
		AccountID:          positiveIDValue(task.AccountID),
		ProviderEndpointID: positiveIDValue(task.AssignedProviderEndpointID),
		CredentialID:       positiveIDValue(task.AssignedCredentialID),
		FeatureSlug:        normalizeStatFeature(task.FeatureSlug),
		// Gateway cutover: tasks no longer carry provider api_type; stat
		// history keeps its column, new events record empty.
		APIType:        "",
		ModelName:      taskStatModel(task),
		UserKey:        taskStatUserKey(task.UserID),
		Status:         statStatus,
		OutputCount:    outputCount,
		LatencySeconds: taskStatLatencySeconds(task, attemptFinish, occurredAt),
		CostMicros:     0,
	}
	return event, true, nil
}

func taskStatOutputCount(rawParams string) (int64, error) {
	params, err := tasks.UnmarshalParams(rawParams)
	if err != nil {
		return 0, err
	}
	if params.Generation == nil {
		return 1, nil
	}
	count, err := workflow.ParseRequestedOutputCount(params.Generation.RequestedOutputCount)
	if err != nil {
		return 0, err
	}
	return int64(count.Int()), nil
}

func taskStatOccurredAt(task *tasks.Task) time.Time {
	if task.Status == tasks.StatusCancelled {
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
		completedDelta = event.OutputCount
		if event.LatencySeconds > 0 {
			latencyCountDelta = 1
		}
	case tasks.StatusFailed:
		failedDelta = event.OutputCount
	case tasks.StatusCancelled, tasks.StatusCancelledWithResults:
		cancelledDelta = event.OutputCount
	}

	return q.UpsertTaskDailyStatDelta(ctx, dbsqlc.UpsertTaskDailyStatDeltaParams{
		StatDate:           dbutil.DateFromTime(event.StatDate),
		UserID:             event.UserID,
		AccountID:          event.AccountID,
		ProviderEndpointID: event.ProviderEndpointID,
		CredentialID:       event.CredentialID,
		FeatureSlug:        event.FeatureSlug,
		ApiType:            event.APIType,
		ModelName:          event.ModelName,
		UserKey:            event.UserKey,
		Total:              event.OutputCount,
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

func (r *Repository) ListDailyStatsByDateRange(ctx context.Context, startDate, endDate time.Time) ([]statisticscontracts.DailyStatAggregate, error) {
	if startDate.IsZero() || endDate.IsZero() {
		return nil, fmt.Errorf("stat date range is required")
	}
	start := normalizeTaskStatDate(startDate)
	end := normalizeTaskStatDate(endDate)
	rows, err := r.q.ListDailyStatsByDateRange(ctx, dbsqlc.ListDailyStatsByDateRangeParams{StartDate: dbutil.DateFromTime(start), EndDate: dbutil.DateFromTime(end)})
	if err != nil {
		return nil, err
	}
	result := make([]statisticscontracts.DailyStatAggregate, 0, len(rows))
	for _, row := range rows {
		result = append(result, statisticscontracts.DailyStatAggregate{
			StatDate:          dbutil.DateValue(row.StatDate),
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

func (r *Repository) ListDailyCaptureCreditsByRange(ctx context.Context, start, end time.Time, location *time.Location) (map[string]int64, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("taskstore: database not configured")
	}
	if start.IsZero() || end.IsZero() || !end.After(start) {
		return nil, fmt.Errorf("taskstore: valid daily capture range is required")
	}
	if location == nil {
		return nil, fmt.Errorf("taskstore: daily capture location is required")
	}
	timezoneName := location.String()
	if timezoneName == "" || timezoneName == "Local" {
		// Go reports unnamed local zones as "Local", which PostgreSQL cannot
		// resolve. Fall back to the numeric UTC offset at the range start so
		// day buckets stay aligned; named zones keep DST-aware grouping.
		// PostgreSQL reads numeric AT TIME ZONE offsets with POSIX sign
		// convention (positive = west), opposite of Go/ISO offsets, so the
		// sign is flipped: "+08:00" must be passed as "-08:00".
		_, offset := start.In(location).Zone()
		sign := "-"
		if offset < 0 {
			sign = "+"
			offset = -offset
		}
		timezoneName = fmt.Sprintf("%s%02d:%02d", sign, offset/3600, (offset%3600)/60)
	}
	rows, err := r.dbWithContext(ctx).Query(`
SELECT ((created_at AT TIME ZONE $3)::date)::text AS capture_day,
       SUM(CASE WHEN command_type = 'capture' THEN command_amount
                WHEN command_type = 'refund' THEN -command_amount
                ELSE 0 END)::bigint AS amount
FROM credit_transactions
WHERE command_type IN ('capture', 'refund')
  AND created_at >= $1
  AND created_at < $2
GROUP BY (created_at AT TIME ZONE $3)::date
ORDER BY capture_day`, start, end, timezoneName)
	if err != nil {
		return nil, fmt.Errorf("taskstore: query daily capture credits: %w", err)
	}
	defer rows.Close()
	dailyCredits := make(map[string]int64)
	for rows.Next() {
		var date string
		var amount int64
		if err := rows.Scan(&date, &amount); err != nil {
			return nil, fmt.Errorf("taskstore: scan daily capture credits: %w", err)
		}
		dailyCredits[date] = amount
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("taskstore: read daily capture credits: %w", err)
	}
	return dailyCredits, nil
}

func (r *Repository) ListStatEventsByOccurredRange(ctx context.Context, start, end time.Time) ([]statisticscontracts.StatEventTrendRow, error) {
	if start.IsZero() || end.IsZero() {
		return nil, fmt.Errorf("stat event occurred range is required")
	}
	rows, err := r.q.ListStatEventsByOccurredRange(ctx, dbsqlc.ListStatEventsByOccurredRangeParams{StartedAt: dbutil.TimestamptzFromTime(start), EndedAt: dbutil.TimestamptzFromTime(end)})
	if err != nil {
		return nil, err
	}
	result := make([]statisticscontracts.StatEventTrendRow, 0, len(rows))
	for _, row := range rows {
		result = append(result, statisticscontracts.StatEventTrendRow{
			OccurredAt:     dbutil.TimeValue(row.OccurredAt),
			StatDate:       dbutil.DateValue(row.StatDate),
			ModelName:      row.ModelName,
			UserKey:        row.UserKey,
			Status:         row.Status,
			OutputCount:    int64(row.OutputCount),
			LatencySeconds: row.LatencySeconds,
			CostMicros:     row.CostMicros,
		})
	}
	return result, nil
}

func (r *Repository) ListStatEventLatencySamplesByDateRange(ctx context.Context, startDate, endDate time.Time) ([]statisticscontracts.StatEventLatencySample, error) {
	if startDate.IsZero() || endDate.IsZero() {
		return nil, fmt.Errorf("stat date range is required")
	}
	start := normalizeTaskStatDate(startDate)
	end := normalizeTaskStatDate(endDate)
	rows, err := r.q.ListStatEventLatencySamplesByDateRange(ctx, dbsqlc.ListStatEventLatencySamplesByDateRangeParams{StartDate: dbutil.DateFromTime(start), EndDate: dbutil.DateFromTime(end)})
	if err != nil {
		return nil, err
	}
	result := make([]statisticscontracts.StatEventLatencySample, 0, len(rows))
	for _, row := range rows {
		result = append(result, statisticscontracts.StatEventLatencySample{ModelName: row.ModelName, LatencySeconds: row.LatencySeconds})
	}
	return result, nil
}

func (r *Repository) ListBackfilledDatesByRange(ctx context.Context, startDate, endDate time.Time) ([]time.Time, error) {
	if startDate.IsZero() || endDate.IsZero() {
		return nil, fmt.Errorf("stat date range is required")
	}
	start := normalizeTaskStatDate(startDate)
	end := normalizeTaskStatDate(endDate)
	dateRows, err := r.q.ListBackfilledDatesByRange(ctx, dbsqlc.ListBackfilledDatesByRangeParams{StartDate: dbutil.DateFromTime(start), EndDate: dbutil.DateFromTime(end)})
	if err != nil {
		return nil, err
	}
	result := make([]time.Time, 0, len(dateRows))
	for _, d := range dateRows {
		result = append(result, dbutil.DateValue(d))
	}
	return result, nil
}

func (r *Repository) RebuildDailyStatsFromEventsByDateRange(ctx context.Context, startDate, endDate time.Time) (statisticscontracts.RebuildDailyStatsResult, error) {
	if startDate.IsZero() || endDate.IsZero() {
		return statisticscontracts.RebuildDailyStatsResult{}, fmt.Errorf("stat date range is required")
	}
	start := normalizeTaskStatDate(startDate)
	end := normalizeTaskStatDate(endDate)
	row, err := r.q.RebuildTaskDailyStatsByEventDateRange(ctx, dbsqlc.RebuildTaskDailyStatsByEventDateRangeParams{StartDate: dbutil.DateFromTime(start), EndDate: dbutil.DateFromTime(end)})
	if err != nil {
		return statisticscontracts.RebuildDailyStatsResult{}, err
	}
	return statisticscontracts.RebuildDailyStatsResult{DeletedGroups: row.DeletedGroups, RebuiltGroups: row.RebuiltGroups}, nil
}

func (r *Repository) CorrectTaskStatEvent(ctx context.Context, correction statisticscontracts.TaskStatEventCorrection) (int64, error) {
	event, ok := taskStatEventFromCorrection(correction)
	if !ok {
		return 0, fmt.Errorf("task stat event correction is invalid")
	}
	return r.q.UpsertTaskStatEventCorrection(ctx, dbsqlc.UpsertTaskStatEventCorrectionParams{
		TaskID:             event.TaskID,
		OccurredAt:         dbutil.TimestamptzFromTime(event.OccurredAt),
		StatDate:           dbutil.DateFromTime(event.StatDate),
		UserID:             event.UserID,
		AccountID:          event.AccountID,
		ProviderEndpointID: event.ProviderEndpointID,
		CredentialID:       event.CredentialID,
		FeatureSlug:        event.FeatureSlug,
		ApiType:            event.APIType,
		ModelName:          event.ModelName,
		UserKey:            event.UserKey,
		Status:             event.Status,
		OutputCount:        int16(event.OutputCount),
		LatencySeconds:     event.LatencySeconds,
		CostMicros:         event.CostMicros,
	})
}

func taskStatEventFromCorrection(correction statisticscontracts.TaskStatEventCorrection) (normalizedTaskStatEvent, bool) {
	taskID := strings.TrimSpace(correction.TaskID)
	if taskID == "" {
		return normalizedTaskStatEvent{}, false
	}
	if correction.Status != tasks.StatusCompleted && correction.Status != tasks.StatusFailed && !tasks.IsTerminalStatus(correction.Status) {
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
	outputCount := correction.OutputCount
	if outputCount == 0 {
		outputCount = 1
	}
	if _, err := workflow.ParseRequestedOutputCount(int(outputCount)); err != nil {
		return normalizedTaskStatEvent{}, false
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
		OutputCount:        outputCount,
		LatencySeconds:     latencySeconds,
		CostMicros:         costMicros,
	}, true
}

func (r *Repository) BackfillDailyStatsByCreatedRange(ctx context.Context, startedAt, endedAt, statDate time.Time, markDone bool) (statisticscontracts.BackfillDailyStatsResult, error) {
	if startedAt.IsZero() || endedAt.IsZero() || statDate.IsZero() {
		return statisticscontracts.BackfillDailyStatsResult{}, fmt.Errorf("backfill date range is required")
	}
	if !endedAt.After(startedAt) {
		return statisticscontracts.BackfillDailyStatsResult{}, fmt.Errorf("backfill ended_at must be after started_at")
	}

	var result statisticscontracts.BackfillDailyStatsResult
	err := dbutil.WithTx(ctx, r.db, func(q *dbsqlc.Queries) error {
		row, err := q.BackfillDailyStatsByCreatedRange(ctx, dbsqlc.BackfillDailyStatsByCreatedRangeParams{
			StartedAt: dbutil.NullTime(&startedAt),
			EndedAt:   dbutil.NullTime(&endedAt),
			StatDate:  dbutil.DateFromTime(normalizeTaskStatDate(statDate)),
			MarkDone:  markDone,
		})
		if err != nil {
			return err
		}
		result = statisticscontracts.BackfillDailyStatsResult{
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
	ResolvedModel string
}

func taskFromSQLC(row any) tasks.Task {
	switch r := row.(type) {
	case dbsqlc.GetTaskByTaskIDRow:
		return taskFromSQLCModel(r.Task, taskLeaseSnapshot{
			ResolvedModel: r.ResolvedModel,
		})
	case dbsqlc.GetTaskByTaskIDAndUserIDRow:
		return taskFromSQLCModel(r.Task, taskLeaseSnapshot{
			ResolvedModel: r.ResolvedModel,
		})
	case dbsqlc.ListTasksRow:
		return taskFromSQLCModel(r.Task, taskLeaseSnapshot{
			ResolvedModel: r.ResolvedModel,
		})
	case dbsqlc.ListTasksByUserIDRow:
		return taskFromSQLCModel(r.Task, taskLeaseSnapshot{
			ResolvedModel: r.ResolvedModel,
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
		Status:                     row.Status,
		LeaseToken:                 row.LeaseToken,
		RetryCount:                 int(row.RetryCount),
		ErrorMsg:                   row.ErrorMsg,
		Result:                     row.Result,
		CreatedAt:                  dbutil.TimeValue(row.CreatedAt),
		UpdatedAt:                  dbutil.TimeValue(row.UpdatedAt),
		CompletedAt:                dbutil.TimePtr(row.CompletedAt),
		CancelledAt:                dbutil.TimePtr(row.CancelledAt),
		CancelledBy:                dbutil.Int64Ptr(row.CancelledBy),
		CancelReason:               row.CancelReason,
	}
}
