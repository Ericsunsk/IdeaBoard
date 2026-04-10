package repository

import (
	"backend/internal/model"
	"time"

	"gorm.io/gorm"
)

type AccountRepository struct {
	db *gorm.DB
}

func NewAccountRepository(db *gorm.DB) *AccountRepository {
	return &AccountRepository{db: db}
}

func (r *AccountRepository) Create(account *model.ApiAccount) error {
	return r.db.Create(account).Error
}

func (r *AccountRepository) GetByID(id uint64) (*model.ApiAccount, error) {
	var account model.ApiAccount
	if err := r.db.First(&account, id).Error; err != nil {
		return nil, err
	}
	return &account, nil
}

func (r *AccountRepository) List(keyword, platform string, page, pageSize int) ([]*model.ApiAccount, int64, error) {
	var accounts []*model.ApiAccount
	var total int64

	query := r.db.Model(&model.ApiAccount{})

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

func (r *AccountRepository) GetAllEnabled() ([]*model.ApiAccount, error) {
	var accounts []*model.ApiAccount
	if err := r.db.Where("status = 1").Find(&accounts).Error; err != nil {
		return nil, err
	}
	return accounts, nil
}

func (r *AccountRepository) Update(account *model.ApiAccount) error {
	return r.db.Save(account).Error
}

func (r *AccountRepository) Delete(id uint64) error {
	return r.db.Delete(&model.ApiAccount{}, id).Error
}

func (r *AccountRepository) UpdateStatus(id uint64, status int8) error {
	return r.db.Model(&model.ApiAccount{}).Where("id = ?", id).Update("status", status).Error
}

func (r *AccountRepository) IncrementFailCount(id uint64) error {
	return r.db.Model(&model.ApiAccount{}).Where("id = ?", id).Update("fail_count", gorm.Expr("fail_count + 1")).Error
}

func (r *AccountRepository) ResetFailCount(id uint64) error {
	return r.db.Model(&model.ApiAccount{}).Where("id = ?", id).Update("fail_count", 0).Error
}

// Task Repository
type TaskRepository struct {
	db *gorm.DB
}

func NewTaskRepository(db *gorm.DB) *TaskRepository {
	return &TaskRepository{db: db}
}

func (r *TaskRepository) Create(task *model.Task) error {
	return r.db.Create(task).Error
}

func (r *TaskRepository) GetByTaskID(taskID string) (*model.Task, error) {
	var task model.Task
	if err := r.db.Where("task_id = ?", taskID).First(&task).Error; err != nil {
		return nil, err
	}
	return &task, nil
}

func (r *TaskRepository) GetByID(id uint64) (*model.Task, error) {
	var task model.Task
	if err := r.db.First(&task, id).Error; err != nil {
		return nil, err
	}
	return &task, nil
}

func (r *TaskRepository) Update(task *model.Task) error {
	return r.db.Save(task).Error
}

func (r *TaskRepository) List(accountID uint64, status string, page, pageSize int) ([]*model.Task, int64, error) {
	var tasks []*model.Task
	var total int64

	query := r.db.Model(&model.Task{})

	if accountID > 0 {
		query = query.Where("account_id = ?", accountID)
	}
	if status != "" {
		query = query.Where("status = ?", status)
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * pageSize
	if err := query.Offset(offset).Limit(pageSize).Order("created_at DESC").Find(&tasks).Error; err != nil {
		return nil, 0, err
	}

	return tasks, total, nil
}

func (r *TaskRepository) GetByUserID(userID uint64, page, pageSize int) ([]*model.Task, int64, error) {
	var tasks []*model.Task
	var total int64

	query := r.db.Model(&model.Task{}).Where("user_id = ?", userID)

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * pageSize
	if err := query.Offset(offset).Limit(pageSize).Order("created_at DESC").Find(&tasks).Error; err != nil {
		return nil, 0, err
	}

	return tasks, total, nil
}

func (r *TaskRepository) DeleteByTaskIDAndUserID(taskID string, userID uint64) error {
	return r.db.Where("task_id = ? AND user_id = ?", taskID, userID).Delete(&model.Task{}).Error
}

// FailRunningByAccountID 将指定账号下所有 running/pending 状态的任务标记为 failed
func (r *TaskRepository) FailRunningByAccountID(accountID uint64, errMsg string) (int64, error) {
	now := time.Now()
	result := r.db.Model(&model.Task{}).
		Where("account_id = ? AND status IN ?", accountID, []string{"running", "pending"}).
		Updates(map[string]interface{}{
			"status":       "failed",
			"error_msg":    errMsg,
			"completed_at": now,
		})
	return result.RowsAffected, result.Error
}

func (r *TaskRepository) CountByStatus(status string) (int64, error) {
	var count int64
	if err := r.db.Model(&model.Task{}).Where("status = ?", status).Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}
