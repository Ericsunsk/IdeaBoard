package accounts

import (
	"api/internal/tasks"
	"fmt"
	"log"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type AccountRepository struct {
	db *gorm.DB
}

func NewAccountRepository(db *gorm.DB) *AccountRepository {
	return &AccountRepository{db: db}
}

func (r *AccountRepository) Create(account *ApiAccount) error {
	return r.db.Create(account).Error
}

func (r *AccountRepository) GetByID(id uint64) (*ApiAccount, error) {
	var account ApiAccount
	if err := r.db.First(&account, id).Error; err != nil {
		return nil, err
	}
	return &account, nil
}

func (r *AccountRepository) List(keyword, platform string, page, pageSize int) ([]*ApiAccount, int64, error) {
	var accounts []*ApiAccount
	var total int64

	query := r.db.Model(&ApiAccount{})

	if keyword != "" {
		query = query.Where("name LIKE ? OR platform LIKE ? OR api_type LIKE ?", "%"+keyword+"%", "%"+keyword+"%", "%"+keyword+"%")
	}

	if platform != "" {
		query = query.Where("platform = ?", platform)
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * pageSize
	if err := query.Offset(offset).Limit(pageSize).Order("id DESC").Find(&accounts).Error; err != nil {
		return nil, 0, err
	}

	return accounts, total, nil
}

func (r *AccountRepository) GetAllEnabled() ([]*ApiAccount, error) {
	var accounts []*ApiAccount
	if err := r.db.Where("status = 1").Find(&accounts).Error; err != nil {
		return nil, err
	}
	return accounts, nil
}

func (r *AccountRepository) Update(account *ApiAccount) error {
	return r.db.Save(account).Error
}

func (r *AccountRepository) Delete(id uint64) error {
	return r.db.Delete(&ApiAccount{}, id).Error
}

func (r *AccountRepository) UpdateStatus(id uint64, status int8) error {
	return r.db.Model(&ApiAccount{}).Where("id = ?", id).Update("status", status).Error
}

func (r *AccountRepository) IncrementFailCount(id uint64) error {
	return r.db.Model(&ApiAccount{}).Where("id = ?", id).Update("fail_count", gorm.Expr("fail_count + 1")).Error
}

func (r *AccountRepository) ResetFailCount(id uint64) error {
	return r.db.Model(&ApiAccount{}).Where("id = ?", id).Update("fail_count", 0).Error
}

// Task Repository
type TaskRepository struct {
	db *gorm.DB
}

type TaskStatisticLite struct {
	TaskID      string
	UserID      uint64
	AccountID   uint64
	Status      string
	Result      string
	CreatedAt   time.Time
	CompletedAt *time.Time
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

func NewTaskRepository(db *gorm.DB) *TaskRepository {
	return &TaskRepository{db: db}
}

func (r *TaskRepository) Create(task *tasks.Task) error {
	return r.db.Create(task).Error
}

func (r *TaskRepository) GetByTaskID(taskID string) (*tasks.Task, error) {
	var task tasks.Task
	if err := r.db.Where("task_id = ?", taskID).First(&task).Error; err != nil {
		return nil, err
	}
	return &task, nil
}

func (r *TaskRepository) GetByTaskIDAndUserID(taskID string, userID uint64) (*tasks.Task, error) {
	var task tasks.Task
	if err := r.db.Where("task_id = ? AND user_id = ?", taskID, userID).First(&task).Error; err != nil {
		return nil, err
	}
	return &task, nil
}

func (r *TaskRepository) GetByID(id uint64) (*tasks.Task, error) {
	var task tasks.Task
	if err := r.db.First(&task, id).Error; err != nil {
		return nil, err
	}
	return &task, nil
}

func (r *TaskRepository) Update(task *tasks.Task) error {
	return r.db.Save(task).Error
}

func (r *TaskRepository) UpdateByTaskIDAndStatuses(taskID string, statuses []string, updates map[string]interface{}) (int64, error) {
	result := r.db.Model(&tasks.Task{}).
		Where("task_id = ? AND status IN ?", taskID, statuses).
		Updates(updates)
	return result.RowsAffected, result.Error
}

func (r *TaskRepository) CompleteWithOutputs(taskID string, statuses []string, updates map[string]interface{}, outputs []tasks.TaskOutput) (int64, error) {
	var rowsAffected int64
	err := r.db.Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&tasks.Task{}).
			Where("task_id = ? AND status IN ?", taskID, statuses).
			Updates(updates)
		if result.Error != nil {
			return result.Error
		}
		rowsAffected = result.RowsAffected
		if rowsAffected == 0 {
			return nil
		}

		if err := tx.Where("task_id = ?", taskID).Delete(&tasks.TaskOutput{}).Error; err != nil {
			return err
		}
		if len(outputs) == 0 {
			return nil
		}
		for i := range outputs {
			outputs[i].TaskID = taskID
			outputs[i].SortIndex = i
			if strings.TrimSpace(outputs[i].Role) == "" {
				outputs[i].Role = "image"
			}
		}
		return tx.Create(&outputs).Error
	})
	return rowsAffected, err
}

func (r *TaskRepository) ListOutputsByTaskIDs(taskIDs []string) (map[string][]tasks.TaskOutput, error) {
	result := make(map[string][]tasks.TaskOutput, len(taskIDs))
	if len(taskIDs) == 0 {
		return result, nil
	}

	var outputs []tasks.TaskOutput
	if err := r.db.
		Where("task_id IN ?", taskIDs).
		Order("task_id ASC, sort_index ASC, id ASC").
		Find(&outputs).Error; err != nil {
		return nil, err
	}
	for _, output := range outputs {
		result[output.TaskID] = append(result[output.TaskID], output)
	}
	return result, nil
}

func (r *TaskRepository) List(userID uint64, accountID uint64, status string, keyword string, page, pageSize int) ([]*tasks.Task, int64, error) {
	var items []*tasks.Task
	var total int64

	query := r.db.Model(&tasks.Task{})

	if userID > 0 {
		query = query.Where("user_id = ?", userID)
	}
	if accountID > 0 {
		query = query.Where("account_id = ?", accountID)
	}
	if status != "" {
		query = query.Where("status = ?", status)
	}
	if keyword != "" {
		query = query.Where("prompt LIKE ?", "%"+keyword+"%")
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * pageSize
	if err := query.Offset(offset).Limit(pageSize).Order("created_at DESC").Find(&items).Error; err != nil {
		return nil, 0, err
	}

	return items, total, nil
}

func (r *TaskRepository) GetByUserID(userID uint64, page, pageSize int) ([]*tasks.Task, int64, error) {
	var items []*tasks.Task
	var total int64

	query := r.db.Model(&tasks.Task{}).Where("user_id = ?", userID)

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * pageSize
	if err := query.Offset(offset).Limit(pageSize).Order("created_at DESC").Find(&items).Error; err != nil {
		return nil, 0, err
	}

	return items, total, nil
}

func (r *TaskRepository) ListTaskIDsByAccountIDAndStatuses(accountID uint64, statuses ...string) ([]string, error) {
	var taskIDs []string

	query := r.db.Model(&tasks.Task{}).
		Where("account_id = ?", accountID)
	if len(statuses) > 0 {
		query = query.Where("status IN ?", statuses)
	}

	if err := query.Pluck("task_id", &taskIDs).Error; err != nil {
		return nil, err
	}

	return taskIDs, nil
}

func (r *TaskRepository) DeleteByTaskIDAndUserID(taskID string, userID uint64) error {
	return r.db.Where("task_id = ? AND user_id = ?", taskID, userID).Delete(&tasks.Task{}).Error
}

func (r *TaskRepository) MarkQueueFailed(taskID string, errMsg string) error {
	now := time.Now()
	return r.db.Model(&tasks.Task{}).
		Where("task_id = ?", taskID).
		Updates(map[string]interface{}{
			"status":       "failed",
			"error_msg":    errMsg,
			"completed_at": now,
		}).Error
}

// FailRunningByAccountID 将指定账号下所有 running/pending 状态的任务标记为 failed
func (r *TaskRepository) FailRunningByAccountID(accountID uint64, errMsg string) (int64, error) {
	now := time.Now()
	type candidate struct {
		TaskID      string
		UserID      uint64
		AccountID   uint64
		Result      string
		CreatedAt   time.Time
		CompletedAt *time.Time
	}

	var rowsAffected int64
	err := r.db.Transaction(func(tx *gorm.DB) error {
		var candidates []candidate
		if err := tx.Model(&tasks.Task{}).
			Select("task_id", "user_id", "account_id", "result", "created_at", "completed_at").
			Where("account_id = ? AND status IN ?", accountID, []string{"running", "pending"}).
			Find(&candidates).Error; err != nil {
			return err
		}

		result := tx.Model(&tasks.Task{}).
			Where("account_id = ? AND status IN ?", accountID, []string{"running", "pending"}).
			Updates(map[string]interface{}{
				"status":       "failed",
				"error_msg":    errMsg,
				"completed_at": now,
			})
		if result.Error != nil {
			return result.Error
		}
		rowsAffected = result.RowsAffected
		if len(candidates) == 0 {
			return nil
		}
		if result.RowsAffected != int64(len(candidates)) {
			log.Printf("[TaskRepository] FailRunningByAccountID affected mismatch account_id=%d candidates=%d updated=%d", accountID, len(candidates), result.RowsAffected)
		}

		modelSetMap, err := r.listSingleModelSetByAccountIDsTx(tx, []uint64{accountID})
		if err != nil {
			return err
		}
		fallbackModelSet := modelSetMap[accountID]

		for _, c := range candidates {
			completedAt := c.CompletedAt
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
			if err := r.upsertDailyStatByTaskWithFallbackModelTx(tx, task, fallbackModelSet); err != nil {
				return fmt.Errorf("upsert daily stat for task %s: %w", c.TaskID, err)
			}
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return rowsAffected, nil
}

func (r *TaskRepository) CountByStatus(status string) (int64, error) {
	var count int64
	if err := r.db.Model(&tasks.Task{}).Where("status = ?", status).Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

func (r *TaskRepository) CountAllTasks() (int64, error) {
	var count int64
	if err := r.db.Model(&tasks.Task{}).Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

func (r *TaskRepository) ListStatisticsByCreatedAtRange(start, end time.Time) ([]TaskStatisticLite, error) {
	var rows []TaskStatisticLite
	err := r.db.Model(&tasks.Task{}).
		Select("task_id", "user_id", "account_id", "status", "result", "created_at", "completed_at").
		Where("created_at >= ? AND created_at < ?", start, end).
		Order("created_at ASC").
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *TaskRepository) UpsertDailyStatByTask(task *tasks.Task) error {
	return r.UpsertDailyStatByTaskWithFallbackModel(task, "")
}

func (r *TaskRepository) UpsertDailyStatByTaskWithFallbackModel(task *tasks.Task, fallbackModelSet string) error {
	if task != nil && strings.TrimSpace(fallbackModelSet) == "" && !resultContainsModel(task.Result) {
		fallbackModelSet = r.resolveAccountSingleModelSet(task.AccountID)
	}
	return r.db.Transaction(func(tx *gorm.DB) error {
		return r.upsertDailyStatByTaskWithFallbackModelTx(tx, task, fallbackModelSet)
	})
}

func (r *TaskRepository) upsertDailyStatByTaskWithFallbackModelTx(tx *gorm.DB, task *tasks.Task, fallbackModelSet string) error {
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

	row := &TaskDailyStat{
		StatDate:          statDate,
		ModelName:         model,
		UserKey:           user,
		Total:             1,
		Completed:         completedDelta,
		Failed:            failedDelta,
		LatencySecondsSum: latencySeconds,
		LatencyCount:      latencyCountDelta,
	}

	event := &TaskStatEvent{
		TaskID:         task.TaskID,
		StatDate:       statDate,
		ModelName:      model,
		UserKey:        user,
		Status:         task.Status,
		LatencySeconds: latencySeconds,
	}
	insert := tx.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "task_id"}},
		DoNothing: true,
	}).Create(event)
	if insert.Error != nil {
		return insert.Error
	}
	if insert.RowsAffected == 0 {
		return nil
	}

	return tx.Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "stat_date"},
			{Name: "model_name"},
			{Name: "user_key"},
		},
		DoUpdates: clause.Assignments(map[string]interface{}{
			"total":               gorm.Expr("total + ?", row.Total),
			"completed":           gorm.Expr("completed + ?", row.Completed),
			"failed":              gorm.Expr("failed + ?", row.Failed),
			"latency_seconds_sum": gorm.Expr("latency_seconds_sum + ?", row.LatencySecondsSum),
			"latency_count":       gorm.Expr("latency_count + ?", row.LatencyCount),
		}),
	}).Create(row).Error
}

func (r *TaskRepository) resolveAccountSingleModelSet(accountID uint64) string {
	if accountID == 0 {
		return ""
	}
	var account ApiAccount
	if err := r.db.Select("model_set").Where("id = ?", accountID).First(&account).Error; err != nil {
		return ""
	}
	return singleModelFromSet(account.ModelSet)
}

func (r *TaskRepository) listSingleModelSetByAccountIDs(accountIDs []uint64) (map[uint64]string, error) {
	return r.listSingleModelSetByAccountIDsTx(r.db, accountIDs)
}

func (r *TaskRepository) listSingleModelSetByAccountIDsTx(tx *gorm.DB, accountIDs []uint64) (map[uint64]string, error) {
	result := make(map[uint64]string)
	if len(accountIDs) == 0 {
		return result, nil
	}

	uniq := make(map[uint64]struct{}, len(accountIDs))
	ids := make([]uint64, 0, len(accountIDs))
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

	var rows []struct {
		ID       uint64
		ModelSet string
	}
	if err := tx.Model(&ApiAccount{}).
		Select("id", "model_set").
		Where("id IN ?", ids).
		Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		result[row.ID] = singleModelFromSet(row.ModelSet)
	}
	return result, nil
}

func (r *TaskRepository) ListDailyStatsByDateRange(startDate, endDate string) ([]DailyStatAggregate, error) {
	var rows []DailyStatAggregate
	err := r.db.Model(&TaskDailyStat{}).
		Select("stat_date", "model_name", "user_key", "total", "completed", "failed", "latency_seconds_sum", "latency_count").
		Where("stat_date >= ? AND stat_date <= ?", startDate, endDate).
		Order("stat_date ASC").
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *TaskRepository) ListBackfilledDatesByRange(startDate, endDate string) ([]string, error) {
	var dates []string
	if err := r.db.Model(&TaskDailyStatBackfill{}).
		Where("stat_date >= ? AND stat_date <= ?", startDate, endDate).
		Order("stat_date ASC").
		Pluck("stat_date", &dates).Error; err != nil {
		return nil, err
	}
	return dates, nil
}

func (r *TaskRepository) MarkDailyBackfillDone(statDate string) error {
	if statDate == "" {
		return nil
	}
	return r.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "stat_date"}},
		DoNothing: true,
	}).Create(&TaskDailyStatBackfill{StatDate: statDate}).Error
}
