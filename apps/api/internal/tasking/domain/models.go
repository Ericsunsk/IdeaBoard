package domain

import (
	"bytes"
	"encoding/json"
	"strings"
	"time"

	"ideaboard/api/internal/generation/routing"
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
	Service                    string                `json:"service"`
	Priority                   int                   `json:"priority"`
	ResolvedModel              string                `json:"resolved_model"`
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
	Service          string
	OutputResolution string
	Quantity         int64
}

// AdminBillingSnapshot is a read-only projection over immutable Task Billing
// facts. FeatureExists is resolved dynamically from the current Registry.
type AdminBillingSnapshot struct {
	SKU              string     `json:"sku"`
	DisplayName      string     `json:"display_name"`
	PriceVersionID   int64      `json:"price_version_id"`
	PriceVersion     int64      `json:"price_version"`
	OutputResolution string     `json:"output_resolution"`
	UnitCredits      int64      `json:"unit_credits"`
	Quantity         int64      `json:"quantity"`
	TotalCredits     int64      `json:"total_credits"`
	CapturedCredits  int64      `json:"captured_credits"`
	QuotedAt         time.Time  `json:"quoted_at"`
	SettlementState  string     `json:"settlement_state"`
	SettledAt        *time.Time `json:"settled_at"`
	FeatureExists    bool       `json:"feature_exists"`
}

// AdminTaskExecutionSnapshot is the immutable lease snapshot used by Admin
// Task list projections. It intentionally contains no attempt lifecycle data.
type AdminTaskExecutionSnapshot struct {
	ResolvedModel string `json:"resolved_model"`
	Provider      string `json:"provider"`
	GatewayStep   string `json:"gateway_step"`
}

// AdminTaskAttempt is the safe, immutable execution history projection for
// Admin Task details. It intentionally excludes credentials and queue metadata.
// Finalize timing is not recorded per attempt: result materialization is part
// of the terminal write itself, so there is no separate finalize phase to report.
//
// Routing observability (#852, ADR-0012 rule 7): Service, RequestedModel,
// RetryLayer, and GatewayRetryCount distinguish gateway request retries,
// model fallback attempts, and River/task retries in Admin presentation.
// ErrorMsg is the truncated per-send failure reason (bounded at write;
// never prompts, payloads, credentials, or storage URLs).
type AdminTaskAttempt struct {
	AttemptNo          int        `json:"attempt_no"`
	Status             string     `json:"status"`
	Stage              string     `json:"stage"`
	FailureCategory    string     `json:"failure_category"`
	Service            string     `json:"service"`
	RequestedModel     string     `json:"requested_model"`
	ResolvedModel      string     `json:"resolved_model"`
	Provider           string     `json:"provider"`
	GatewayStep        string     `json:"gateway_step"`
	RetryLayer         string     `json:"retry_layer"`
	GatewayRetryCount  int        `json:"gateway_retry_count"`
	ErrorMsg           string     `json:"error_msg,omitempty"`
	ProviderStartedAt  *time.Time `json:"provider_started_at"`
	ProviderFinishedAt *time.Time `json:"provider_finished_at"`
	CompletedAt        *time.Time `json:"completed_at"`
	CancelledAt        *time.Time `json:"cancelled_at"`
	ProviderLatencyMS  int64      `json:"provider_latency_ms"`
	TotalLatencyMS     int64      `json:"total_latency_ms"`
}

// AdminRoutingSnapshot is the read-only Admin projection over the
// immutable submit-time RoutingSnapshot pinned on the task row (#852,
// ADR-0012 rule 9). It carries the requested service/resolution, the
// eligible pool entries, the deterministic selection, the single retry
// candidate, and the pool version — everything needed to replay and
// diagnose selection plus fallback without the live pool. A nil pointer
// means the task never pinned a snapshot (pre-pool row); Admin renders
// it as missing model data, never as an error.
type AdminRoutingSnapshot struct {
	Service             string   `json:"service"`
	RequestedResolution string   `json:"requested_resolution"`
	EligibleModels      []string `json:"eligible_models"`
	SelectedModel       string   `json:"selected_model"`
	RetryCandidate      string   `json:"retry_candidate,omitempty"`
	PoolVersion         uint64   `json:"pool_version"`
}

// ParseAdminRoutingSnapshot projects raw task-row snapshot bytes onto
// the Admin view. Empty bytes report ok=false (never pinned). Malformed
// JSON also reports ok=false: observability degrades to "missing model
// data" instead of failing the whole detail read. Deliberately no
// Verify: Admin diagnosis must render even a non-replaying snapshot.
func ParseAdminRoutingSnapshot(raw []byte) (*AdminRoutingSnapshot, bool) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil, false
	}
	var snap routing.RoutingSnapshot
	if err := json.Unmarshal(raw, &snap); err != nil {
		return nil, false
	}
	return &AdminRoutingSnapshot{
		Service:             strings.TrimSpace(snap.Service),
		RequestedResolution: string(snap.RequestedResolution),
		EligibleModels:      append([]string(nil), snap.EligibleModels...),
		SelectedModel:       strings.TrimSpace(snap.SelectedModel),
		RetryCandidate:      strings.TrimSpace(snap.RetryCandidate),
		PoolVersion:         snap.PoolVersion,
	}, true
}

// FinalAttempt resolves the execution attempt that owns the task's final
// model (#852): the highest attempt number in a terminal status
// (completed/failed/cancelled) wins; a task with no terminal attempt
// falls back to its highest attempt of any status so in-flight tasks
// still attribute to their latest send. Nil when no attempt carries a
// resolved model (missing model data), never a fabricated zero value.
func FinalAttempt(attempts []AdminTaskAttempt) *AdminTaskAttempt {
	var best *AdminTaskAttempt
	for i := range attempts {
		candidate := &attempts[i]
		if strings.TrimSpace(candidate.ResolvedModel) == "" {
			continue
		}
		if best == nil {
			best = candidate
			continue
		}
		bestTerminal := isTerminalAttemptStatus(best.Status)
		candidateTerminal := isTerminalAttemptStatus(candidate.Status)
		if candidateTerminal != bestTerminal {
			if candidateTerminal {
				best = candidate
			}
			continue
		}
		if candidate.AttemptNo >= best.AttemptNo {
			best = candidate
		}
	}
	return best
}

// FinalResolvedModel attributes execution to the resolved (final) model:
// the FinalAttempt's resolved model, or "" when attempts carry no model
// data. Read-only projection: it never changes task or billing state.
func FinalResolvedModel(attempts []AdminTaskAttempt) string {
	if final := FinalAttempt(attempts); final != nil {
		return strings.TrimSpace(final.ResolvedModel)
	}
	return ""
}

func isTerminalAttemptStatus(status string) bool {
	switch strings.TrimSpace(status) {
	case "completed", "failed", "cancelled":
		return true
	default:
		return false
	}
}

type TaskOutput struct {
	ID     int64  `json:"id"`
	TaskID string `json:"task_id"`
	// MediaAssetID is the internal relational FK used only while persisting
	// output lineage. API consumers receive AssetID, the stable public identity.
	MediaAssetID   *int64    `json:"-"`
	AssetID        string    `json:"asset_id,omitempty"`
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
	Animated       bool      `json:"animated,omitempty"`
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
	AssetID     string    `json:"asset_id,omitempty"`
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
			AssetID:     output.AssetID,
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
	Prompt                 string                 `json:"prompt"`
	Service                string                 `json:"service"`
	ClientActionID         string                 `json:"client_action_id"`
	Priority               int                    `json:"priority"`
	ImageURLs              []string               `json:"image_urls"`
	ImageAssetIDs          []int64                `json:"image_asset_ids"`
	ReferenceImageAssetIDs []int64                `json:"reference_image_asset_ids"`
	Params                 map[string]interface{} `json:"params"`
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
