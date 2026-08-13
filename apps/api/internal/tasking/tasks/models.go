package tasks

import (
	"encoding/json"
	"strings"
	"time"
)

type Task struct {
	ID                         int64                 `json:"id"`
	TaskID                     string                `json:"task_id"`
	CorrelationID              string                `json:"correlation_id"`
	UserID                     int64                 `json:"user_id"`
	ClientActionID             string                `json:"client_action_id"`
	AssignedProviderEndpointID *int64                `json:"assigned_provider_endpoint_id"`
	AssignedCredentialID       *int64                `json:"assigned_credential_id"`
	AccountID                  *int64                `json:"account_id"`
	Prompt                     string                `json:"prompt"`
	UserPrompt                 string                `json:"user_prompt"`
	Params                     string                `json:"params"`
	FeatureSlug                string                `json:"feature_slug"`
	Priority                   int                   `json:"priority"`
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
	BillingRequest             *TaskBillingRequest   `json:"-"`
}

type TaskBillingRequest struct {
	Feature  string
	Quantity int64
}

// AdminBillingSnapshot is a read-only projection over immutable Task Billing
// facts. FeatureExists is resolved dynamically from the current Registry.
type AdminBillingSnapshot struct {
	SKU             string     `json:"sku"`
	DisplayName     string     `json:"display_name"`
	PriceVersionID  int64      `json:"price_version_id"`
	PriceVersion    int64      `json:"price_version"`
	UnitCredits     int64      `json:"unit_credits"`
	Quantity        int64      `json:"quantity"`
	TotalCredits    int64      `json:"total_credits"`
	QuotedAt        time.Time  `json:"quoted_at"`
	SettlementState string     `json:"settlement_state"`
	SettledAt       *time.Time `json:"settled_at"`
	FeatureExists   bool       `json:"feature_exists"`
}

// AdminTaskExecutionSnapshot is the immutable lease snapshot used by Admin
// Task list projections. It intentionally contains no attempt lifecycle data.
type AdminTaskExecutionSnapshot struct {
	AccountID            int64  `json:"account_id"`
	ProviderEndpointID   int64  `json:"provider_endpoint_id"`
	ProviderEndpointName string `json:"provider_endpoint_name"`
	ResolvedModel        string `json:"resolved_model"`
	APIType              string `json:"api_type"`
}

// AdminTaskAttempt is the safe, immutable execution history projection for
// Admin Task details. It intentionally excludes credentials and queue metadata.
type AdminTaskAttempt struct {
	AttemptNo            int        `json:"attempt_no"`
	AccountID            int64      `json:"account_id"`
	Status               string     `json:"status"`
	Stage                string     `json:"stage"`
	FailureCategory      string     `json:"failure_category"`
	ProviderEndpointID   int64      `json:"provider_endpoint_id"`
	ProviderEndpointName string     `json:"provider_endpoint_name"`
	ResolvedModel        string     `json:"resolved_model"`
	APIType              string     `json:"api_type"`
	ProviderStartedAt    *time.Time `json:"provider_started_at"`
	ProviderFinishedAt   *time.Time `json:"provider_finished_at"`
	FinalizeStartedAt    *time.Time `json:"finalize_started_at"`
	FinalizeFinishedAt   *time.Time `json:"finalize_finished_at"`
	CompletedAt          *time.Time `json:"completed_at"`
	CancelledAt          *time.Time `json:"cancelled_at"`
	ProviderLatencyMS    int64      `json:"provider_latency_ms"`
	FinalizeLatencyMS    int64      `json:"finalize_latency_ms"`
	TotalLatencyMS       int64      `json:"total_latency_ms"`
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
	ResultIndex    int       `json:"result_index"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type TaskUserView struct {
	TaskID           string                `json:"task_id"`
	CorrelationID    string                `json:"correlation_id"`
	Prompt           string                `json:"prompt"`
	Status           string                `json:"status"`
	ErrorMsg         string                `json:"error_msg"`
	CancelReason     string                `json:"cancel_reason"`
	Result           string                `json:"result"`
	Outputs          []TaskOutputUserView  `json:"outputs,omitempty"`
	ExecutionMetrics *TaskExecutionMetrics `json:"execution_metrics,omitempty"`
	CreatedAt        time.Time             `json:"created_at"`
	UpdatedAt        time.Time             `json:"updated_at"`
	CompletedAt      *time.Time            `json:"completed_at"`
}

type TaskOutputUserView struct {
	ID          int64     `json:"id"`
	TaskID      string    `json:"task_id"`
	OutputKind  string    `json:"output_kind"`
	Role        string    `json:"role"`
	URL         string    `json:"url"`
	MIMEType    string    `json:"mime_type"`
	Width       int       `json:"width"`
	Height      int       `json:"height"`
	SizeBytes   int64     `json:"size_bytes"`
	ResultIndex int       `json:"result_index"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
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
		TaskID:           t.TaskID,
		CorrelationID:    t.EffectiveCorrelationID(),
		Prompt:           t.UserPrompt,
		Status:           t.Status,
		ErrorMsg:         t.ErrorMsg,
		CancelReason:     t.CancelReason,
		Result:           SafeTaskResultJSON(t.Result),
		Outputs:          taskOutputsUserView(t.Outputs),
		ExecutionMetrics: t.ExecutionMetrics,
		CreatedAt:        t.CreatedAt,
		UpdatedAt:        t.UpdatedAt,
		CompletedAt:      t.CompletedAt,
	}
}

func (t *Task) EffectiveCorrelationID() string {
	if t == nil {
		return ""
	}
	if correlationID := NormalizeCorrelationID(t.CorrelationID); correlationID != "" {
		return correlationID
	}
	return CorrelationIDForTaskID(t.TaskID)
}

func CorrelationIDForTaskID(taskID string) string {
	return NormalizeCorrelationID(taskID)
}

func NormalizeCorrelationID(correlationID string) string {
	return strings.TrimSpace(correlationID)
}

func taskOutputsUserView(outputs []TaskOutput) []TaskOutputUserView {
	if len(outputs) == 0 {
		return nil
	}
	result := make([]TaskOutputUserView, 0, len(outputs))
	for _, output := range outputs {
		result = append(result, TaskOutputUserView{
			ID:          output.ID,
			TaskID:      output.TaskID,
			OutputKind:  output.OutputKind,
			Role:        output.Role,
			URL:         output.URL,
			MIMEType:    output.MIMEType,
			Width:       output.Width,
			Height:      output.Height,
			SizeBytes:   output.SizeBytes,
			ResultIndex: output.ResultIndex,
			CreatedAt:   output.CreatedAt,
			UpdatedAt:   output.UpdatedAt,
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
	Priority       int                    `json:"priority"`
	ImageURLs      []string               `json:"image_urls"`
	Params         map[string]interface{} `json:"params"`
}

type TaskSubmissionResult struct {
	TaskID            string
	HoldTransactionID string
	Created           bool
}

type TaskListRequest struct {
	UserID       int64  `form:"-" json:"-"`
	UserPublicID string `form:"user_public_id"`
	AccountID    int64  `form:"account_id"`
	Status       string `form:"status"`
	Keyword      string `form:"keyword"`
	Page         int    `form:"page,default=1"`
	PageSize     int    `form:"page_size,default=20"`
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
