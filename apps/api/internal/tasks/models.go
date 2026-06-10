package tasks

import (
	"encoding/json"
	"time"
)

type Task struct {
	ID                int64                 `json:"id"`
	TaskID            string                `json:"task_id"`
	UserID            int64                 `json:"user_id"`
	AssignedChannelID *int64                `json:"assigned_channel_id"`
	AccountID         *int64                `json:"account_id"`
	Prompt            string                `json:"prompt"`
	UserPrompt        string                `json:"user_prompt"`
	Params            string                `json:"params"`
	FeatureSlug       string                `json:"feature_slug"`
	RequestedModel    string                `json:"requested_model"`
	ResolvedModel     string                `json:"resolved_model"`
	APIType           string                `json:"api_type"`
	Status            string                `json:"status"`
	LeaseToken        string                `json:"lease_token"`
	RetryCount        int                   `json:"retry_count"`
	ErrorMsg          string                `json:"error_msg"`
	Result            string                `json:"result"`
	CreatedAt         time.Time             `json:"created_at"`
	UpdatedAt         time.Time             `json:"updated_at"`
	CompletedAt       *time.Time            `json:"completed_at"`
	CancelledAt       *time.Time            `json:"cancelled_at"`
	CancelledBy       *int64                `json:"cancelled_by"`
	CancelReason      string                `json:"cancel_reason"`
	Outputs           []TaskOutput          `json:"outputs,omitempty"`
	ExecutionMetrics  *TaskExecutionMetrics `json:"execution_metrics,omitempty"`
}

type TaskOutput struct {
	ID             int64     `json:"id"`
	TaskID         string    `json:"task_id"`
	MediaAssetID   *int64    `json:"media_asset_id,omitempty"`
	Role           string    `json:"role"`
	URL            string    `json:"url"`
	StorageKey     string    `json:"storage_key"`
	MIMEType       string    `json:"mime_type"`
	Width          int       `json:"width"`
	Height         int       `json:"height"`
	SizeBytes      int64     `json:"size_bytes"`
	SourceURL      string    `json:"source_url,omitempty"`
	ChecksumSHA256 string    `json:"checksum_sha256,omitempty"`
	StorageProfile string    `json:"storage_profile,omitempty"`
	Metadata       string    `json:"metadata,omitempty"`
	SortIndex      int       `json:"sort_index"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

func (t *Task) ToUserView() map[string]interface{} {
	safeResult := t.Result
	if safeResult != "" {
		var resultMap map[string]interface{}
		if err := json.Unmarshal([]byte(safeResult), &resultMap); err == nil {
			delete(resultMap, "prompt")
			delete(resultMap, "account_id")
			delete(resultMap, "channel_id")
			delete(resultMap, "assigned_channel_id")
			delete(resultMap, "channel_name")
			delete(resultMap, "platform")
			delete(resultMap, "api_type")
			if b, err := json.Marshal(resultMap); err == nil {
				safeResult = string(b)
			}
		}
	}

	return map[string]interface{}{
		"id":            t.ID,
		"task_id":       t.TaskID,
		"user_id":       t.UserID,
		"prompt":        t.UserPrompt,
		"status":        t.Status,
		"error_msg":     t.ErrorMsg,
		"cancel_reason": t.CancelReason,
		"result":        safeResult,
		"outputs":       taskOutputsUserView(t.Outputs),
		"created_at":    t.CreatedAt,
		"updated_at":    t.UpdatedAt,
		"completed_at":  t.CompletedAt,
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
	UserID    int64  `form:"user_id"`
	AccountID int64  `form:"account_id"`
	Status    string `form:"status"`
	Keyword   string `form:"keyword"`
	Page      int    `form:"page,default=1"`
	PageSize  int    `form:"page_size,default=20"`
}

type TaskListCounts struct {
	Total          int64 `json:"total"`
	PendingCount   int64 `json:"pending_count"`
	RunningCount   int64 `json:"running_count"`
	CompletedCount int64 `json:"completed_count"`
	FailedCount    int64 `json:"failed_count"`
	CancelledCount int64 `json:"cancelled_count"`
	OtherCount     int64 `json:"other_count"`
}
