package tasks

import (
	"encoding/json"
	"time"
)

type Task struct {
	ID          uint64     `gorm:"primaryKey;autoIncrement" json:"id"`
	TaskID      string     `gorm:"type:varchar(64);not null;uniqueIndex" json:"task_id"`
	UserID      uint64     `gorm:"not null;index" json:"user_id"`
	ChannelID   uint64     `gorm:"not null" json:"channel_id"`
	AccountID   uint64     `gorm:"index" json:"account_id"`
	Prompt      string     `gorm:"type:text" json:"prompt"`
	UserPrompt  string     `gorm:"type:text" json:"user_prompt"`
	Params      string     `gorm:"type:mediumtext" json:"params"`
	Status      string     `gorm:"type:varchar(20);default:'pending';index" json:"status"`
	RetryCount  int        `gorm:"type:int;default:0" json:"retry_count"`
	ErrorMsg    string     `gorm:"type:text" json:"error_msg"`
	Result      string     `gorm:"type:text" json:"result"`
	CreatedAt   time.Time  `gorm:"autoCreateTime;index" json:"created_at"`
	UpdatedAt   time.Time  `gorm:"autoUpdateTime" json:"updated_at"`
	CompletedAt *time.Time `json:"completed_at"`
}

func (t *Task) ToUserView() map[string]interface{} {
	safeResult := t.Result
	if safeResult != "" {
		var resultMap map[string]interface{}
		if err := json.Unmarshal([]byte(safeResult), &resultMap); err == nil {
			delete(resultMap, "prompt")
			delete(resultMap, "account_id")
			delete(resultMap, "channel_id")
			delete(resultMap, "channel_name")
			delete(resultMap, "platform")
			delete(resultMap, "api_type")
			delete(resultMap, "model_set")
			if b, err := json.Marshal(resultMap); err == nil {
				safeResult = string(b)
			}
		}
	}

	return map[string]interface{}{
		"id":           t.ID,
		"task_id":      t.TaskID,
		"user_id":      t.UserID,
		"prompt":       t.UserPrompt,
		"status":       t.Status,
		"error_msg":    t.ErrorMsg,
		"result":       safeResult,
		"created_at":   t.CreatedAt,
		"updated_at":   t.UpdatedAt,
		"completed_at": t.CompletedAt,
	}
}

type SubmitTaskRequest struct {
	Prompt    string                 `json:"prompt"`
	Feature   string                 `json:"feature"`
	ImageURLs []string               `json:"image_urls"`
	Params    map[string]interface{} `json:"params"`
}

type TaskListRequest struct {
	UserID    uint64 `form:"user_id"`
	AccountID uint64 `form:"account_id"`
	Status    string `form:"status"`
	Keyword   string `form:"keyword"`
	Page      int    `form:"page,default=1"`
	PageSize  int    `form:"page_size,default=20"`
}
