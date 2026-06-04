package accounts

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"ideaboard/api/internal/db/dbutil"
	dbsqlc "ideaboard/api/internal/db/sqlc"
	"ideaboard/api/internal/tasks"
)

type AccountRepository struct {
	db *sql.DB
	q  *dbsqlc.Queries
}

func NewAccountRepository(db *sql.DB) *AccountRepository {
	return &AccountRepository{db: db, q: dbsqlc.New(db)}
}

func (r *AccountRepository) Create(ctx context.Context, account *ApiAccount) error {
	id, err := r.q.CreateAccount(ctx, dbsqlc.CreateAccountParams{
		Name:          account.Name,
		ChannelID:     account.ChannelID,
		Platform:      account.Platform,
		ApiType:       account.ApiType,
		ModelSet:      account.ModelSet,
		ApiKey:        account.ApiKey,
		ProxyUrl:      account.ProxyURL,
		MaxConcurrent: int32(account.MaxConcurrent),
		Timeout:       int32(account.Timeout),
		ExtraConfig:   account.ExtraConfig,
		Status:        account.Status,
		FailCount:     int32(account.FailCount),
		Weight:        int32(account.Weight),
	})
	if err != nil {
		return err
	}
	account.ID = int64(id)
	return nil
}

func (r *AccountRepository) GetByID(ctx context.Context, id int64) (*ApiAccount, error) {
	row, err := r.q.GetAccountByID(ctx, id)
	if err != nil {
		return nil, err
	}
	account := accountFromSQLC(row)
	return &account, nil
}

func (r *AccountRepository) List(ctx context.Context, keyword, platform string, page, pageSize int) ([]*ApiAccount, int64, error) {
	keyword = strings.TrimSpace(keyword)
	platform = strings.TrimSpace(platform)
	keywordLike := "%" + keyword + "%"
	hasKeyword := keyword != ""
	hasPlatform := platform != ""
	params := dbsqlc.CountAccountsParams{
		HasKeyword:  hasKeyword,
		KeywordLike: keywordLike,
		HasPlatform: hasPlatform,
		Platform:    platform,
	}
	total, err := r.q.CountAccounts(ctx, params)
	if err != nil {
		return nil, 0, err
	}
	offset := (page - 1) * pageSize
	rows, err := r.q.ListAccounts(ctx, dbsqlc.ListAccountsParams{
		HasKeyword:  hasKeyword,
		KeywordLike: keywordLike,
		HasPlatform: hasPlatform,
		Platform:    platform,
		PageLimit:   int32(pageSize),
		PageOffset:  int32(offset),
	})
	if err != nil {
		return nil, 0, err
	}
	return accountsFromSQLC(rows), total, nil
}

func (r *AccountRepository) GetAllEnabled(ctx context.Context) ([]*ApiAccount, error) {
	rows, err := r.q.ListEnabledAccounts(ctx)
	if err != nil {
		return nil, err
	}
	return accountsFromSQLC(rows), nil
}

func (r *AccountRepository) ListAccountIDs(ctx context.Context) ([]int64, error) {
	return r.q.ListAccountIDs(ctx)
}

func (r *AccountRepository) Update(ctx context.Context, account *ApiAccount) error {
	return r.q.UpdateAccount(ctx, dbsqlc.UpdateAccountParams{
		Name:          account.Name,
		ChannelID:     account.ChannelID,
		Platform:      account.Platform,
		ApiType:       account.ApiType,
		ModelSet:      account.ModelSet,
		ApiKey:        account.ApiKey,
		ProxyUrl:      account.ProxyURL,
		MaxConcurrent: int32(account.MaxConcurrent),
		Timeout:       int32(account.Timeout),
		ExtraConfig:   account.ExtraConfig,
		Status:        account.Status,
		FailCount:     int32(account.FailCount),
		Weight:        int32(account.Weight),
		ID:            account.ID,
	})
}

func (r *AccountRepository) Delete(ctx context.Context, id int64) error {
	return r.q.DeleteAccount(ctx, id)
}

func (r *AccountRepository) UpdateStatus(ctx context.Context, id int64, status int16) error {
	return r.q.UpdateAccountStatus(ctx, dbsqlc.UpdateAccountStatusParams{Status: status, ID: id})
}

func (r *AccountRepository) IncrementFailCount(ctx context.Context, id int64) error {
	return r.q.IncrementAccountFailCount(ctx, id)
}

func (r *AccountRepository) ResetFailCount(ctx context.Context, id int64) error {
	return r.q.ResetAccountFailCount(ctx, id)
}

func accountFromSQLC(row dbsqlc.ApiAccount) ApiAccount {
	return ApiAccount{
		ID:            row.ID,
		Name:          row.Name,
		ChannelID:     row.ChannelID,
		Platform:      row.Platform,
		ApiType:       row.ApiType,
		ModelSet:      row.ModelSet,
		ApiKey:        row.ApiKey,
		ProxyURL:      row.ProxyUrl,
		MaxConcurrent: int(row.MaxConcurrent),
		Timeout:       int(row.Timeout),
		ExtraConfig:   row.ExtraConfig,
		Status:        row.Status,
		FailCount:     int(row.FailCount),
		Weight:        int(row.Weight),
		CreatedAt:     row.CreatedAt,
		UpdatedAt:     row.UpdatedAt,
	}
}

func accountsFromSQLC(rows []dbsqlc.ApiAccount) []*ApiAccount {
	result := make([]*ApiAccount, 0, len(rows))
	for _, row := range rows {
		account := accountFromSQLC(row)
		result = append(result, &account)
	}
	return result
}

// Task Repository
type TaskRepository struct {
	db *sql.DB
	q  *dbsqlc.Queries
}

type TaskStatisticLite struct {
	TaskID      string
	UserID      int64
	AccountID   int64
	Status      string
	Result      string
	CreatedAt   time.Time
	CompletedAt *time.Time
}

type RunningTaskLease struct {
	TaskID     string
	AccountID  int64
	LeaseToken string
}

type DailyStatAggregate struct {
	StatDate          string
	ModelName         string
	UserKey           string
	Total             int64
	Completed         int64
	Failed            int64
	LatencySecondsSum float64
	LatencyCount      int64
}

type StatEventLatencySample struct {
	ModelName      string
	LatencySeconds float64
}

func NewTaskRepository(db *sql.DB) *TaskRepository {
	return &TaskRepository{db: db, q: dbsqlc.New(db)}
}

func (r *TaskRepository) Create(ctx context.Context, task *tasks.Task) error {
	now := time.Now()
	if task.CreatedAt.IsZero() {
		task.CreatedAt = now
	}
	if task.UpdatedAt.IsZero() {
		task.UpdatedAt = task.CreatedAt
	}
	id, err := r.q.CreateTask(ctx, dbsqlc.CreateTaskParams{
		TaskID:      task.TaskID,
		UserID:      task.UserID,
		ChannelID:   task.ChannelID,
		AccountID:   task.AccountID,
		Prompt:      task.Prompt,
		UserPrompt:  task.UserPrompt,
		Params:      task.Params,
		Status:      task.Status,
		LeaseToken:  task.LeaseToken,
		RetryCount:  int32(task.RetryCount),
		ErrorMsg:    task.ErrorMsg,
		Result:      task.Result,
		CreatedAt:   task.CreatedAt,
		UpdatedAt:   task.UpdatedAt,
		CompletedAt: dbutil.NullTime(task.CompletedAt),
	})
	if err != nil {
		return err
	}
	task.ID = int64(id)
	return nil
}

func (r *TaskRepository) GetByTaskID(ctx context.Context, taskID string) (*tasks.Task, error) {
	row, err := r.q.GetTaskByTaskID(ctx, taskID)
	if err != nil {
		return nil, err
	}
	task := taskFromSQLC(row)
	return &task, nil
}

func (r *TaskRepository) GetByTaskIDAndUserID(ctx context.Context, taskID string, userID int64) (*tasks.Task, error) {
	row, err := r.q.GetTaskByTaskIDAndUserID(ctx, dbsqlc.GetTaskByTaskIDAndUserIDParams{TaskID: taskID, UserID: userID})
	if err != nil {
		return nil, err
	}
	task := taskFromSQLC(row)
	return &task, nil
}

func (r *TaskRepository) GetByID(ctx context.Context, id int64) (*tasks.Task, error) {
	row, err := r.q.GetTaskByID(ctx, id)
	if err != nil {
		return nil, err
	}
	task := taskFromSQLC(row)
	return &task, nil
}

func (r *TaskRepository) Update(ctx context.Context, task *tasks.Task) error {
	return r.q.UpdateTask(ctx, dbsqlc.UpdateTaskParams{
		TaskID:      task.TaskID,
		UserID:      task.UserID,
		ChannelID:   task.ChannelID,
		AccountID:   task.AccountID,
		Prompt:      task.Prompt,
		UserPrompt:  task.UserPrompt,
		Params:      task.Params,
		Status:      task.Status,
		LeaseToken:  task.LeaseToken,
		RetryCount:  int32(task.RetryCount),
		ErrorMsg:    task.ErrorMsg,
		Result:      task.Result,
		CompletedAt: dbutil.NullTime(task.CompletedAt),
		ID:          task.ID,
	})
}

func (r *TaskRepository) MarkTaskFailed(ctx context.Context, taskID string, statuses []string, errMsg string, result string, completedAt time.Time) (int64, error) {
	return r.q.MarkTaskFailed(ctx, dbsqlc.MarkTaskFailedParams{
		ErrorMsg:    errMsg,
		Result:      result,
		CompletedAt: sql.NullTime{Time: completedAt, Valid: true},
		TaskID:      taskID,
		Statuses:    statuses,
	})
}

func (r *TaskRepository) MarkTaskFailedWithLease(ctx context.Context, taskID string, statuses []string, leaseToken string, errMsg string, result string, completedAt time.Time) (int64, error) {
	return r.q.MarkTaskFailedWithLease(ctx, dbsqlc.MarkTaskFailedWithLeaseParams{
		ErrorMsg:    errMsg,
		Result:      result,
		CompletedAt: sql.NullTime{Time: completedAt, Valid: true},
		TaskID:      taskID,
		Statuses:    statuses,
		LeaseToken:  leaseToken,
	})
}

func (r *TaskRepository) ResetTaskRunningToPendingWithLease(ctx context.Context, taskID string, statuses []string, leaseToken string, errMsg string) (int64, error) {
	return r.q.ResetTaskRunningToPendingWithLease(ctx, dbsqlc.ResetTaskRunningToPendingWithLeaseParams{
		ErrorMsg:   errMsg,
		TaskID:     taskID,
		Statuses:   statuses,
		LeaseToken: leaseToken,
	})
}

func (r *TaskRepository) RequeueTaskRunningWithLease(ctx context.Context, taskID string, statuses []string, leaseToken string, errMsg string) (int64, error) {
	return r.q.RequeueTaskRunningWithLease(ctx, dbsqlc.RequeueTaskRunningWithLeaseParams{
		ErrorMsg:   errMsg,
		TaskID:     taskID,
		Statuses:   statuses,
		LeaseToken: leaseToken,
	})
}

func (r *TaskRepository) FinalFailTaskRunningWithLease(ctx context.Context, taskID string, statuses []string, leaseToken string, errMsg string, result string, completedAt time.Time) (int64, error) {
	return r.q.FinalFailTaskRunningWithLease(ctx, dbsqlc.FinalFailTaskRunningWithLeaseParams{
		ErrorMsg:    errMsg,
		Result:      result,
		CompletedAt: sql.NullTime{Time: completedAt, Valid: true},
		TaskID:      taskID,
		Statuses:    statuses,
		LeaseToken:  leaseToken,
	})
}

func (r *TaskRepository) CompleteWithOutputs(ctx context.Context, taskID string, statuses []string, result string, completedAt time.Time, outputs []tasks.TaskOutput) (int64, error) {
	return r.completeWithOutputs(ctx, taskID, statuses, "", false, result, completedAt, outputs)
}

func (r *TaskRepository) CompleteWithOutputsAndLeaseToken(ctx context.Context, taskID string, statuses []string, leaseToken string, result string, completedAt time.Time, outputs []tasks.TaskOutput) (int64, error) {
	return r.completeWithOutputs(ctx, taskID, statuses, leaseToken, true, result, completedAt, outputs)
}

func (r *TaskRepository) completeWithOutputs(ctx context.Context, taskID string, statuses []string, leaseToken string, requireLeaseToken bool, result string, completedAt time.Time, outputs []tasks.TaskOutput) (int64, error) {
	var rowsAffected int64
	err := dbutil.WithTx(ctx, r.db, func(q *dbsqlc.Queries) error {
		var err error
		if requireLeaseToken {
			rowsAffected, err = q.CompleteTaskWithLease(ctx, dbsqlc.CompleteTaskWithLeaseParams{
				Result:      result,
				CompletedAt: sql.NullTime{Time: completedAt, Valid: true},
				TaskID:      taskID,
				Statuses:    statuses,
				LeaseToken:  leaseToken,
			})
		} else {
			rowsAffected, err = q.CompleteTask(ctx, dbsqlc.CompleteTaskParams{
				Result:      result,
				CompletedAt: sql.NullTime{Time: completedAt, Valid: true},
				TaskID:      taskID,
				Statuses:    statuses,
			})
		}
		if err != nil || rowsAffected == 0 {
			return err
		}
		if err := q.DeleteOutputsByTaskID(ctx, taskID); err != nil {
			return err
		}
		return insertTaskOutputs(ctx, q, taskID, outputs)
	})
	return rowsAffected, err
}

func insertTaskOutputs(ctx context.Context, q *dbsqlc.Queries, taskID string, outputs []tasks.TaskOutput) error {
	for i := range outputs {
		output := outputs[i]
		output.TaskID = taskID
		output.SortIndex = i
		if strings.TrimSpace(output.Role) == "" {
			output.Role = "image"
		}
		if _, err := q.CreateTaskOutput(ctx, dbsqlc.CreateTaskOutputParams{
			TaskID:     output.TaskID,
			Role:       output.Role,
			Url:        output.URL,
			StorageKey: output.StorageKey,
			MimeType:   output.MIMEType,
			Width:      int32(output.Width),
			Height:     int32(output.Height),
			SizeBytes:  output.SizeBytes,
			SortIndex:  int32(output.SortIndex),
		}); err != nil {
			return err
		}
	}
	return nil
}

func (r *TaskRepository) ListOutputsByTaskIDs(ctx context.Context, taskIDs []string) (map[string][]tasks.TaskOutput, error) {
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

func (r *TaskRepository) List(ctx context.Context, userID int64, accountID int64, status string, keyword string, page, pageSize int) ([]*tasks.Task, int64, error) {
	hasUserID := userID > 0
	hasAccountID := accountID > 0
	hasStatus := status != ""
	keywordLike := "%" + keyword + "%"
	hasKeyword := keyword != ""
	countParams := dbsqlc.CountTasksParams{
		HasUserID:    hasUserID,
		UserID:       userID,
		HasAccountID: hasAccountID,
		AccountID:    accountID,
		HasStatus:    hasStatus,
		Status:       status,
		HasKeyword:   hasKeyword,
		KeywordLike:  keywordLike,
	}
	total, err := r.q.CountTasks(ctx, countParams)
	if err != nil {
		return nil, 0, err
	}
	offset := (page - 1) * pageSize
	rows, err := r.q.ListTasks(ctx, dbsqlc.ListTasksParams{
		HasUserID:    hasUserID,
		UserID:       userID,
		HasAccountID: hasAccountID,
		AccountID:    accountID,
		HasStatus:    hasStatus,
		Status:       status,
		HasKeyword:   hasKeyword,
		KeywordLike:  keywordLike,
		PageLimit:    int32(pageSize),
		PageOffset:   int32(offset),
	})
	if err != nil {
		return nil, 0, err
	}
	return tasksFromSQLC(rows), total, nil
}

func (r *TaskRepository) GetByUserID(ctx context.Context, userID int64, page, pageSize int) ([]*tasks.Task, int64, error) {
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

func (r *TaskRepository) ListTaskIDsByAccountIDAndStatuses(ctx context.Context, accountID int64, statuses ...string) ([]string, error) {
	if len(statuses) == 0 {
		return r.q.ListTaskIDsByAccountID(ctx, accountID)
	}
	return r.q.ListTaskIDsByAccountIDAndStatuses(ctx, dbsqlc.ListTaskIDsByAccountIDAndStatusesParams{
		AccountID: accountID,
		Statuses:  statuses,
	})
}

func (r *TaskRepository) ListRunningTaskLeases(ctx context.Context, limit int) ([]RunningTaskLease, error) {
	if limit <= 0 {
		return nil, fmt.Errorf("limit must be positive")
	}
	rows, err := r.q.ListRunningTaskLeases(ctx, int32(limit))
	if err != nil {
		return nil, err
	}
	result := make([]RunningTaskLease, 0, len(rows))
	for _, row := range rows {
		result = append(result, RunningTaskLease{TaskID: row.TaskID, AccountID: row.AccountID, LeaseToken: row.LeaseToken})
	}
	return result, nil
}

func (r *TaskRepository) CountRunningTasksMissingLeaseToken(ctx context.Context) (int64, error) {
	return r.q.CountRunningTasksMissingLeaseToken(ctx)
}

func (r *TaskRepository) ListRunningTasksMissingLeaseToken(ctx context.Context, limit int) ([]RunningTaskLease, error) {
	if limit <= 0 {
		return nil, fmt.Errorf("limit must be positive")
	}
	rows, err := r.q.ListRunningTasksMissingLeaseToken(ctx, int32(limit))
	if err != nil {
		return nil, err
	}
	result := make([]RunningTaskLease, 0, len(rows))
	for _, row := range rows {
		result = append(result, RunningTaskLease{TaskID: row.TaskID, AccountID: row.AccountID, LeaseToken: row.LeaseToken})
	}
	return result, nil
}

func (r *TaskRepository) ResetRunningTasksMissingLeaseTokenToPending(ctx context.Context, limit int) (int64, error) {
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

func (r *TaskRepository) DeleteByTaskIDAndUserID(ctx context.Context, taskID string, userID int64) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	result, err := tx.ExecContext(ctx, `DELETE FROM tasks WHERE task_id = $1 AND user_id = $2`, taskID, userID)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows > 0 {
		if _, err := tx.ExecContext(ctx, `DELETE FROM task_outputs WHERE task_id = $1`, taskID); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	committed = true
	return nil
}

func (r *TaskRepository) MarkQueueFailed(ctx context.Context, taskID string, errMsg string) error {
	now := time.Now()
	return r.q.MarkQueueFailed(ctx, dbsqlc.MarkQueueFailedParams{
		ErrorMsg:    errMsg,
		CompletedAt: sql.NullTime{Time: now, Valid: true},
		TaskID:      taskID,
	})
}

// FailRunningByAccountID 将指定账号下所有 running/pending 状态的任务标记为 failed
func (r *TaskRepository) FailRunningByAccountID(ctx context.Context, accountID int64, errMsg string) ([]string, error) {
	now := time.Now()
	var failedTasks []dbsqlc.FailRunningTasksByAccountIDRow
	err := dbutil.WithTx(ctx, r.db, func(q *dbsqlc.Queries) error {
		updated, err := q.FailRunningTasksByAccountID(ctx, dbsqlc.FailRunningTasksByAccountIDParams{
			ErrorMsg:    errMsg,
			CompletedAt: sql.NullTime{Time: now, Valid: true},
			AccountID:   accountID,
		})
		if err != nil || len(updated) == 0 {
			return err
		}
		failedTasks = updated
		modelSetMap, err := listSingleModelSetByAccountIDsTx(ctx, q, []int64{accountID})
		if err != nil {
			return err
		}
		fallbackModelSet := modelSetMap[accountID]
		for _, c := range updated {
			completedAt := dbutil.TimePtr(c.CompletedAt)
			if completedAt == nil {
				fallback := now
				completedAt = &fallback
			}
			task := &tasks.Task{
				TaskID:      c.TaskID,
				UserID:      c.UserID,
				AccountID:   c.AccountID,
				Status:      tasks.StatusFailed,
				Result:      c.Result,
				CreatedAt:   c.CreatedAt,
				CompletedAt: completedAt,
			}
			if err := upsertDailyStatByTaskWithFallbackModelTx(ctx, q, task, fallbackModelSet); err != nil {
				return fmt.Errorf("upsert daily stat for task %s: %w", c.TaskID, err)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	taskIDs := make([]string, 0, len(failedTasks))
	for _, task := range failedTasks {
		taskIDs = append(taskIDs, task.TaskID)
	}
	return taskIDs, nil
}

func (r *TaskRepository) CountByStatus(ctx context.Context, status string) (int64, error) {
	return r.q.CountTasksByStatus(ctx, status)
}

func (r *TaskRepository) CountAllTasks(ctx context.Context) (int64, error) {
	return r.q.CountAllTasks(ctx)
}

func (r *TaskRepository) ListStatisticsByCreatedAtRange(ctx context.Context, start, end time.Time) ([]TaskStatisticLite, error) {
	rows, err := r.q.ListTaskStatisticsByCreatedRange(ctx, dbsqlc.ListTaskStatisticsByCreatedRangeParams{StartedAt: start, EndedAt: end})
	if err != nil {
		return nil, err
	}
	result := make([]TaskStatisticLite, 0, len(rows))
	for _, row := range rows {
		result = append(result, TaskStatisticLite{
			TaskID:      row.TaskID,
			UserID:      row.UserID,
			AccountID:   row.AccountID,
			Status:      row.Status,
			Result:      row.Result,
			CreatedAt:   row.CreatedAt,
			CompletedAt: dbutil.TimePtr(row.CompletedAt),
		})
	}
	return result, nil
}

func (r *TaskRepository) UpsertDailyStatByTask(ctx context.Context, task *tasks.Task) error {
	return r.UpsertDailyStatByTaskWithFallbackModel(ctx, task, "")
}

func (r *TaskRepository) UpsertDailyStatByTaskWithFallbackModel(ctx context.Context, task *tasks.Task, fallbackModelSet string) error {
	if task != nil && strings.TrimSpace(fallbackModelSet) == "" && !resultContainsModel(task.Result) {
		fallbackModelSet = r.resolveAccountSingleModelSet(ctx, task.AccountID)
	}
	return dbutil.WithTx(ctx, r.db, func(q *dbsqlc.Queries) error {
		return upsertDailyStatByTaskWithFallbackModelTx(ctx, q, task, fallbackModelSet)
	})
}

func upsertDailyStatByTaskWithFallbackModelTx(ctx context.Context, q *dbsqlc.Queries, task *tasks.Task, fallbackModelSet string) error {
	if task == nil || task.Status == "" || task.TaskID == "" {
		return nil
	}
	if task.Status != tasks.StatusCompleted && task.Status != tasks.StatusFailed {
		return nil
	}
	loc := time.Now().Location()
	createdLocal := task.CreatedAt.In(loc)
	statDate := createdLocal.Format("2006-01-02")
	model := extractTaskModelWithFallback(task.Result, fallbackModelSet)
	user := extractTaskUser(task.Result, task.UserID)

	completedDelta := int64(0)
	failedDelta := int64(0)
	latencySeconds := float64(0)
	latencyCountDelta := int64(0)
	if task.Status == tasks.StatusCompleted {
		completedDelta = 1
		if task.CompletedAt != nil {
			end := task.CompletedAt.In(loc)
			if !end.Before(createdLocal) {
				latencySeconds = end.Sub(createdLocal).Seconds()
				latencyCountDelta = 1
			}
		}
	}
	if task.Status == tasks.StatusFailed {
		failedDelta = 1
	}

	inserted, err := q.InsertTaskStatEventIgnore(ctx, dbsqlc.InsertTaskStatEventIgnoreParams{
		TaskID:         task.TaskID,
		StatDate:       statDate,
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
		ModelName:         model,
		UserKey:           user,
		Total:             1,
		Completed:         completedDelta,
		Failed:            failedDelta,
		LatencySecondsSum: latencySeconds,
		LatencyCount:      latencyCountDelta,
	})
}

func (r *TaskRepository) resolveAccountSingleModelSet(ctx context.Context, accountID int64) string {
	if accountID == 0 {
		return ""
	}
	modelSet, err := r.q.GetAccountModelSetByID(ctx, accountID)
	if err != nil {
		return ""
	}
	return singleModelFromSet(modelSet)
}

func (r *TaskRepository) listSingleModelSetByAccountIDs(ctx context.Context, accountIDs []int64) (map[int64]string, error) {
	return listSingleModelSetByAccountIDsTx(ctx, r.q, accountIDs)
}

func listSingleModelSetByAccountIDsTx(ctx context.Context, q *dbsqlc.Queries, accountIDs []int64) (map[int64]string, error) {
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

	rows, err := q.ListAccountModelSetsByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		result[row.ID] = singleModelFromSet(row.ModelSet)
	}
	return result, nil
}

func (r *TaskRepository) ListDailyStatsByDateRange(ctx context.Context, startDate, endDate string) ([]DailyStatAggregate, error) {
	rows, err := r.q.ListDailyStatsByDateRange(ctx, dbsqlc.ListDailyStatsByDateRangeParams{StartDate: startDate, EndDate: endDate})
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
			LatencySecondsSum: row.LatencySecondsSum,
			LatencyCount:      row.LatencyCount,
		})
	}
	return result, nil
}

func (r *TaskRepository) ListStatEventLatencySamplesByDateRange(ctx context.Context, startDate, endDate string) ([]StatEventLatencySample, error) {
	rows, err := r.q.ListStatEventLatencySamplesByDateRange(ctx, dbsqlc.ListStatEventLatencySamplesByDateRangeParams{StartDate: startDate, EndDate: endDate})
	if err != nil {
		return nil, err
	}
	result := make([]StatEventLatencySample, 0, len(rows))
	for _, row := range rows {
		result = append(result, StatEventLatencySample{ModelName: row.ModelName, LatencySeconds: row.LatencySeconds})
	}
	return result, nil
}

func (r *TaskRepository) ListBackfilledDatesByRange(ctx context.Context, startDate, endDate string) ([]string, error) {
	return r.q.ListBackfilledDatesByRange(ctx, dbsqlc.ListBackfilledDatesByRangeParams{StartDate: startDate, EndDate: endDate})
}

func (r *TaskRepository) MarkDailyBackfillDone(ctx context.Context, statDate string) error {
	if statDate == "" {
		return nil
	}
	return r.q.MarkDailyBackfillDone(ctx, statDate)
}

func taskFromSQLC(row dbsqlc.Task) tasks.Task {
	return tasks.Task{
		ID:          row.ID,
		TaskID:      row.TaskID,
		UserID:      row.UserID,
		ChannelID:   row.ChannelID,
		AccountID:   row.AccountID,
		Prompt:      row.Prompt,
		UserPrompt:  row.UserPrompt,
		Params:      row.Params,
		Status:      row.Status,
		LeaseToken:  row.LeaseToken,
		RetryCount:  int(row.RetryCount),
		ErrorMsg:    row.ErrorMsg,
		Result:      row.Result,
		CreatedAt:   row.CreatedAt,
		UpdatedAt:   row.UpdatedAt,
		CompletedAt: dbutil.TimePtr(row.CompletedAt),
	}
}

func tasksFromSQLC(rows []dbsqlc.Task) []*tasks.Task {
	result := make([]*tasks.Task, 0, len(rows))
	for _, row := range rows {
		task := taskFromSQLC(row)
		result = append(result, &task)
	}
	return result
}

func taskOutputFromSQLC(row dbsqlc.TaskOutput) tasks.TaskOutput {
	return tasks.TaskOutput{
		ID:         row.ID,
		TaskID:     row.TaskID,
		Role:       row.Role,
		URL:        row.Url,
		StorageKey: row.StorageKey,
		MIMEType:   row.MimeType,
		Width:      int(row.Width),
		Height:     int(row.Height),
		SizeBytes:  row.SizeBytes,
		SortIndex:  int(row.SortIndex),
		CreatedAt:  row.CreatedAt,
		UpdatedAt:  row.UpdatedAt,
	}
}
