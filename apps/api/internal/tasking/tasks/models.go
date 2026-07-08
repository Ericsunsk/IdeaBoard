package tasks

import (
	"encoding/json"
	"time"
)

type Task struct {
	ID                         int64                 `json:"id"`
	TaskID                     string                `json:"task_id"`
	UserID                     int64                 `json:"user_id"`
	ClientActionID             string                `json:"client_action_id"`
	AssignedProviderEndpointID *int64                `json:"assigned_provider_endpoint_id"`
	AssignedCredentialID       *int64                `json:"assigned_credential_id"`
	AccountID                  *int64                `json:"account_id"`
	Prompt                     string                `json:"prompt"`
	UserPrompt                 string                `json:"user_prompt"`
	Params                     string                `json:"params"`
	FeatureSlug                string                `json:"feature_slug"`
	RequestedModel             string                `json:"requested_model"`
	ResolvedModel              string                `json:"resolved_model"`
	APIType                    string                `json:"api_type"`
	ProviderEndpointName       string                `json:"provider_endpoint_name"`
	ProviderEndpointBaseURL    string                `json:"provider_endpoint_base_url"`
	CredentialFingerprint      string                `json:"credential_fingerprint"`
	CredentialKeyVersion       string                `json:"credential_key_version"`
	Status                     string                `json:"status"`
	LeaseToken                 string                `json:"lease_token"`
	RetryCount                 int                   `json:"retry_count"`
	ErrorMsg                   string                `json:"error_msg"`
	Result                     string                `json:"result"`
	CreatedAt                  time.Time             `json:"created_at"`
	UpdatedAt                  time.Time             `json:"updated_at"`
	CompletedAt                *time.Time            `json:"completed_at"`
	CancelledAt                *time.Time            `json:"cancelled_at"`
	CancelledBy                *int64                `json:"cancelled_by"`
	CancelReason               string                `json:"cancel_reason"`
	Outputs                    []TaskOutput          `json:"outputs,omitempty"`
	ExecutionMetrics           *TaskExecutionMetrics `json:"execution_metrics,omitempty"`
}

type TaskOutput struct {
	ID             int64     `json:"id"`
	TaskID         string    `json:"task_id"`
	MediaAssetID   *int64    `json:"media_asset_id,omitempty"`
	OutputKind     string    `json:"output_kind"`
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

type TaskUserView struct {
	ID           int64                `json:"id"`
	TaskID       string               `json:"task_id"`
	UserID       int64                `json:"user_id"`
	Prompt       string               `json:"prompt"`
	Status       string               `json:"status"`
	ErrorMsg     string               `json:"error_msg"`
	CancelReason string               `json:"cancel_reason"`
	Result       string               `json:"result"`
	Outputs      []TaskOutputUserView `json:"outputs,omitempty"`
	CreatedAt    time.Time            `json:"created_at"`
	UpdatedAt    time.Time            `json:"updated_at"`
	CompletedAt  *time.Time           `json:"completed_at"`
}

type TaskOutputUserView struct {
	ID         int64     `json:"id"`
	TaskID     string    `json:"task_id"`
	OutputKind string    `json:"output_kind"`
	Role       string    `json:"role"`
	URL        string    `json:"url"`
	MIMEType   string    `json:"mime_type"`
	Width      int       `json:"width"`
	Height     int       `json:"height"`
	SizeBytes  int64     `json:"size_bytes"`
	SortIndex  int       `json:"sort_index"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type TaskResultUserView struct {
	TaskID    string   `json:"task_id,omitempty"`
	Status    string   `json:"status,omitempty"`
	ImageURL  string   `json:"image_url,omitempty"`
	ImageURLs []string `json:"image_urls,omitempty"`
	Model     string   `json:"model,omitempty"`
}

func (t *Task) ToUserView() TaskUserView {
	return TaskUserView{
		ID:           t.ID,
		TaskID:       t.TaskID,
		UserID:       t.UserID,
		Prompt:       t.UserPrompt,
		Status:       t.Status,
		ErrorMsg:     t.ErrorMsg,
		CancelReason: t.CancelReason,
		Result:       SafeTaskResultJSON(t.Result),
		Outputs:      taskOutputsUserView(t.Outputs),
		CreatedAt:    t.CreatedAt,
		UpdatedAt:    t.UpdatedAt,
		CompletedAt:  t.CompletedAt,
	}
}

func taskOutputsUserView(outputs []TaskOutput) []TaskOutputUserView {
	if len(outputs) == 0 {
		return nil
	}
	result := make([]TaskOutputUserView, 0, len(outputs))
	for _, output := range outputs {
		result = append(result, TaskOutputUserView{
			ID:         output.ID,
			TaskID:     output.TaskID,
			OutputKind: output.OutputKind,
			Role:       output.Role,
			URL:        output.URL,
			MIMEType:   output.MIMEType,
			Width:      output.Width,
			Height:     output.Height,
			SizeBytes:  output.SizeBytes,
			SortIndex:  output.SortIndex,
			CreatedAt:  output.CreatedAt,
			UpdatedAt:  output.UpdatedAt,
		})
	}
	return result
}

func SafeTaskResultJSON(raw string) string {
	if raw == "" {
		return ""
	}
	var resultMap map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &resultMap); err != nil {
		return ""
	}
	safe := TaskResultUserView{}
	if value, ok := resultMap["task_id"].(string); ok {
		safe.TaskID = value
	}
	if value, ok := resultMap["status"].(string); ok {
		safe.Status = value
	}
	if value, ok := resultMap["image_url"].(string); ok {
		safe.ImageURL = value
	}
	if values, ok := resultMap["image_urls"].([]interface{}); ok {
		for _, value := range values {
			if text, ok := value.(string); ok {
				safe.ImageURLs = append(safe.ImageURLs, text)
			}
		}
	}
	if value, ok := resultMap["model"].(string); ok {
		safe.Model = value
	}
	payload, err := json.Marshal(safe)
	if err != nil {
		return ""
	}
	return string(payload)
}

type SubmitTaskRequest struct {
	Prompt         string                 `json:"prompt"`
	Feature        string                 `json:"feature"`
	ClientActionID string                 `json:"client_action_id"`
	ImageURLs      []string               `json:"image_urls"`
	Params         map[string]interface{} `json:"params"`
}

type TaskSubmissionResult struct {
	TaskID  string
	Created bool
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
