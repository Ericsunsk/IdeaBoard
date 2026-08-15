package tasksubmission

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"ideaboard/api/internal/tasking/taskbilling"
	"ideaboard/api/internal/tasking/tasks"
	"ideaboard/api/internal/workflow"
)

// CanonicalRequestInput is the single representation accepted from Task
// Intake. It contains no Task identity or River-specific fields.
type CanonicalRequestInput struct {
	UserID         int64
	UserPrompt     string
	Prompt         string
	Params         string
	ClientActionID string
	Feature        string
	Priority       int
	BillingRequest *taskbilling.Request
	Generation     *tasks.GenerationRequestSnapshot
}

// CanonicalRequest is immutable after construction. Only this package can
// derive the paired Task persistence fact and River payload from it.
type CanonicalRequest struct {
	userID         int64
	userPrompt     string
	prompt         string
	params         string
	clientActionID string
	feature        string
	priority       int
	billingRequest *taskbilling.Request
	generation     *tasks.GenerationRequestSnapshot
}

func NewCanonicalRequest(input CanonicalRequestInput) (CanonicalRequest, error) {
	if input.UserID <= 0 {
		return CanonicalRequest{}, fmt.Errorf("canonical submission user id is invalid")
	}
	feature := strings.TrimSpace(input.Feature)
	if feature == "" {
		return CanonicalRequest{}, fmt.Errorf("canonical submission feature is required")
	}
	priority := tasks.EffectiveTaskPriority(input.Priority)
	if !tasks.IsValidTaskPriority(priority) {
		return CanonicalRequest{}, fmt.Errorf("canonical submission priority is invalid")
	}
	var billingRequest *taskbilling.Request
	if taskbilling.RequiresBilling(feature) && input.BillingRequest == nil {
		return CanonicalRequest{}, fmt.Errorf("canonical billed submission requires billing request")
	}
	if input.BillingRequest != nil {
		request := *input.BillingRequest
		if strings.TrimSpace(request.Feature) != feature {
			return CanonicalRequest{}, fmt.Errorf("canonical submission billing product does not match feature")
		}
		if _, err := taskbilling.RequestFingerprint(request); err != nil {
			return CanonicalRequest{}, fmt.Errorf("canonical submission billing request: %w", err)
		}
		if strings.TrimSpace(input.ClientActionID) == "" {
			return CanonicalRequest{}, fmt.Errorf("canonical billed submission requires client action id")
		}
		billingRequest = &request
	}
	var generation *tasks.GenerationRequestSnapshot
	if input.Generation != nil {
		copySnapshot := cloneGenerationSnapshot(*input.Generation)
		if err := validateGenerationSnapshot(copySnapshot); err != nil {
			return CanonicalRequest{}, err
		}
		generation = &copySnapshot
	}
	if billingRequest != nil && generation != nil && billingRequest.Quantity != int64(generation.RequestedOutputCount) {
		return CanonicalRequest{}, fmt.Errorf("canonical submission billing quantity does not match generation output count")
	}
	return CanonicalRequest{
		userID: input.UserID, userPrompt: input.UserPrompt, prompt: input.Prompt, params: input.Params,
		clientActionID: strings.TrimSpace(input.ClientActionID), feature: feature,
		priority: priority, billingRequest: billingRequest,
		generation: generation,
	}, nil
}

func (r CanonicalRequest) UserID() int64 {
	return r.userID
}

func (r CanonicalRequest) Feature() string {
	return r.feature
}

func (r CanonicalRequest) Prompt() string {
	return r.prompt
}

func (r CanonicalRequest) UserPrompt() string     { return r.userPrompt }
func (r CanonicalRequest) CompiledPrompt() string { return r.prompt }
func (r CanonicalRequest) GenerationSnapshot() *tasks.GenerationRequestSnapshot {
	if r.generation == nil {
		return nil
	}
	copySnapshot := cloneGenerationSnapshot(*r.generation)
	return &copySnapshot
}

func (r CanonicalRequest) Params() string {
	return r.params
}

func (r CanonicalRequest) WithGenerationSnapshot(snapshot tasks.GenerationRequestSnapshot) (CanonicalRequest, error) {
	copySnapshot := cloneGenerationSnapshot(snapshot)
	if err := validateGenerationSnapshot(copySnapshot); err != nil {
		return CanonicalRequest{}, err
	}
	params := map[string]json.RawMessage{}
	if err := json.Unmarshal([]byte(r.params), &params); err != nil {
		return CanonicalRequest{}, fmt.Errorf("update canonical generation snapshot: %w", err)
	}
	generationJSON, err := json.Marshal(copySnapshot)
	if err != nil {
		return CanonicalRequest{}, fmt.Errorf("encode canonical generation snapshot: %w", err)
	}
	params["generation"] = generationJSON
	paramsJSON, err := json.Marshal(params)
	if err != nil {
		return CanonicalRequest{}, fmt.Errorf("encode canonical params: %w", err)
	}
	r.generation = &copySnapshot
	r.params = string(paramsJSON)
	return r, nil
}

func (r CanonicalRequest) Priority() int {
	return r.priority
}

func (r CanonicalRequest) ClientActionID() string {
	return r.clientActionID
}

func (r CanonicalRequest) BillingRequest() *taskbilling.Request {
	if r.billingRequest == nil {
		return nil
	}
	request := *r.billingRequest
	return &request
}

func (r CanonicalRequest) valid() bool {
	if r.userID <= 0 || strings.TrimSpace(r.feature) == "" || !tasks.IsValidTaskPriority(r.priority) {
		return false
	}
	if taskbilling.RequiresBilling(r.feature) && r.billingRequest == nil {
		return false
	}
	if r.billingRequest != nil && strings.TrimSpace(r.billingRequest.Feature) != strings.TrimSpace(r.feature) {
		return false
	}
	if r.generation != nil && validateGenerationSnapshot(*r.generation) != nil {
		return false
	}
	return true
}

func cloneGenerationSnapshot(snapshot tasks.GenerationRequestSnapshot) tasks.GenerationRequestSnapshot {
	copySnapshot := snapshot
	copySnapshot.Media = append([]tasks.GenerationMediaIntent(nil), snapshot.Media...)
	if snapshot.Controls != nil {
		controls := *snapshot.Controls
		copySnapshot.Controls = &controls
	}
	return copySnapshot
}

func validateGenerationSnapshot(snapshot tasks.GenerationRequestSnapshot) error {
	if strings.TrimSpace(snapshot.Feature) == "" || snapshot.TemplateFingerprint == "" {
		return fmt.Errorf("canonical generation snapshot is invalid")
	}
	if _, err := workflow.ParseGenerationPrimitive(snapshot.Primitive); err != nil {
		return err
	}
	count, err := workflow.ParseRequestedOutputCount(snapshot.RequestedOutputCount)
	if err != nil || count.Int() != snapshot.RequestedOutputCount {
		return fmt.Errorf("canonical generation snapshot output count is invalid")
	}
	for index, media := range snapshot.Media {
		if media.InputIndex != index || strings.TrimSpace(media.Ref) == "" || strings.TrimSpace(media.Role) == "" {
			return fmt.Errorf("canonical generation snapshot media order is invalid")
		}
	}
	return nil
}

func (r CanonicalRequest) materialize(taskID string, createdAt time.Time) (*tasks.Task, *tasks.TaskPayload) {
	correlationID := tasks.CorrelationIDForTaskID(taskID)
	task := &tasks.Task{
		TaskID: taskID, CorrelationID: correlationID, UserID: r.userID,
		ClientActionID: r.clientActionID, Prompt: r.prompt, UserPrompt: r.userPrompt,
		Params: r.params, FeatureSlug: r.feature, Priority: r.priority, Status: tasks.StatusPending,
	}
	if r.billingRequest != nil {
		task.BillingRequest = &tasks.TaskBillingRequest{
			Feature: r.billingRequest.Feature, OutputResolution: string(r.billingRequest.OutputResolution), Quantity: r.billingRequest.Quantity,
		}
	}
	payload := &tasks.TaskPayload{
		TaskID: taskID, CorrelationID: correlationID, UserID: r.userID,
		Prompt: r.prompt, Params: r.params, Priority: r.priority,
		RetryCount: 0, CreatedAt: createdAt.Unix(),
	}
	return task, payload
}
