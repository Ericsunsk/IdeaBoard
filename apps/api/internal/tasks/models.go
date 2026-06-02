package tasks

import (
	"encoding/json"
	"time"
)

type Task struct {
	ID          uint64       `json:"id"`
	TaskID      string       `json:"task_id"`
	UserID      uint64       `json:"user_id"`
	ChannelID   uint64       `json:"channel_id"`
	AccountID   uint64       `json:"account_id"`
	Prompt      string       `json:"prompt"`
	UserPrompt  string       `json:"user_prompt"`
	Params      string       `json:"params"`
	Status      string       `json:"status"`
	LeaseToken  string       `json:"lease_token"`
	RetryCount  int          `json:"retry_count"`
	ErrorMsg    string       `json:"error_msg"`
	Result      string       `json:"result"`
	CreatedAt   time.Time    `json:"created_at"`
	UpdatedAt   time.Time    `json:"updated_at"`
	CompletedAt *time.Time   `json:"completed_at"`
	Outputs     []TaskOutput `json:"outputs,omitempty"`
}

type TaskOutput struct {
	ID         uint64    `json:"id"`
	TaskID     string    `json:"task_id"`
	Role       string    `json:"role"`
	URL        string    `json:"url"`
	StorageKey string    `json:"storage_key"`
	MIMEType   string    `json:"mime_type"`
	Width      int       `json:"width"`
	Height     int       `json:"height"`
	SizeBytes  int64     `json:"size_bytes"`
	SortIndex  int       `json:"sort_index"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
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
