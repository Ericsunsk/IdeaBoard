package tasks

import (
	"encoding/json"
	"time"
)

type Task struct {
	ID          uint64       `gorm:"primaryKey;autoIncrement" json:"id"`
	TaskID      string       `gorm:"type:varchar(64);not null;uniqueIndex" json:"task_id"`
	UserID      uint64       `gorm:"not null;index" json:"user_id"`
	ChannelID   uint64       `gorm:"not null" json:"channel_id"`
	AccountID   uint64       `gorm:"index" json:"account_id"`
	Prompt      string       `gorm:"type:text" json:"prompt"`
	UserPrompt  string       `gorm:"type:text" json:"user_prompt"`
	Params      string       `gorm:"type:mediumtext" json:"params"`
	Status      string       `gorm:"type:varchar(20);default:'pending';index" json:"status"`
	RetryCount  int          `gorm:"type:int;default:0" json:"retry_count"`
	ErrorMsg    string       `gorm:"type:text" json:"error_msg"`
	Result      string       `gorm:"type:mediumtext" json:"result"`
	CreatedAt   time.Time    `gorm:"autoCreateTime;index" json:"created_at"`
	UpdatedAt   time.Time    `gorm:"autoUpdateTime" json:"updated_at"`
	CompletedAt *time.Time   `json:"completed_at"`
	Outputs     []TaskOutput `gorm:"foreignKey:TaskID;references:TaskID" json:"outputs,omitempty"`
}

type TaskOutput struct {
	ID         uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	TaskID     string    `gorm:"type:varchar(64);not null;index:idx_task_outputs_task_sort,priority:1" json:"task_id"`
	Role       string    `gorm:"type:varchar(32);not null;default:'image'" json:"role"`
	URL        string    `gorm:"type:text;not null" json:"url"`
	StorageKey string    `gorm:"type:text" json:"storage_key"`
	MIMEType   string    `gorm:"type:varchar(64)" json:"mime_type"`
	Width      int       `gorm:"type:int;default:0" json:"width"`
	Height     int       `gorm:"type:int;default:0" json:"height"`
	SizeBytes  int64     `gorm:"type:bigint;default:0" json:"size_bytes"`
	SortIndex  int       `gorm:"type:int;not null;default:0;index:idx_task_outputs_task_sort,priority:2" json:"sort_index"`
	CreatedAt  time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt  time.Time `gorm:"autoUpdateTime" json:"updated_at"`
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
		"outputs":      taskOutputsUserView(t.Outputs),
		"created_at":   t.CreatedAt,
		"updated_at":   t.UpdatedAt,
		"completed_at": t.CompletedAt,
	}
}

func taskOutputsUserView(outputs []TaskOutput) []map[string]interface{} {
	if len(outputs) == 0 {
		return nil
	}
	result := make([]map[string]interface{}, 0, len(outputs))
	for _, output := range outputs {
		result = append(result, map[string]interface{}{
			"id":         output.ID,
			"task_id":    output.TaskID,
			"role":       output.Role,
			"url":        output.URL,
			"mime_type":  output.MIMEType,
			"width":      output.Width,
			"height":     output.Height,
			"size_bytes": output.SizeBytes,
			"sort_index": output.SortIndex,
			"created_at": output.CreatedAt,
			"updated_at": output.UpdatedAt,
		})
	}
	return result
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
