package tasksubmission

import (
	"fmt"
	"strings"
	"time"

	"ideaboard/api/internal/tasking/taskbilling"
	"ideaboard/api/internal/tasking/tasks"
)

// CanonicalRequestInput is the single representation accepted from Task
// Intake. It contains no Task identity or River-specific fields.
type CanonicalRequestInput struct {
	UserID             int64
	UserPrompt         string
	Prompt             string
	Params             string
	ClientActionID     string
	Feature            string
	Priority           int
	BillingRequest     *taskbilling.Request
	MaxBillingQuantity int64
}

// CanonicalRequest is immutable after construction. Only this package can
// derive the paired Task persistence fact and River payload from it.
type CanonicalRequest struct {
	userID             int64
	userPrompt         string
	prompt             string
	params             string
	clientActionID     string
	feature            string
	priority           int
	billingRequest     *taskbilling.Request
	maxBillingQuantity int64
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
	return CanonicalRequest{
		userID: input.UserID, userPrompt: input.UserPrompt, prompt: input.Prompt, params: input.Params,
		clientActionID: strings.TrimSpace(input.ClientActionID), feature: feature,
		priority: priority, billingRequest: billingRequest,
		maxBillingQuantity: input.MaxBillingQuantity,
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

func (r CanonicalRequest) Params() string {
	return r.params
}

func (r CanonicalRequest) Priority() int {
	return r.priority
}

func (r CanonicalRequest) ClientActionID() string {
	return r.clientActionID
}

func (r CanonicalRequest) BillingRequest(quantity int64) (*taskbilling.Request, error) {
	if r.billingRequest == nil {
		return nil, nil
	}
	if quantity <= 0 || r.maxBillingQuantity > 0 && quantity > r.maxBillingQuantity {
		return nil, fmt.Errorf("billing quantity is invalid")
	}
	request := *r.billingRequest
	request.Quantity = quantity
	return &request, nil
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
	return true
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
			Feature: r.billingRequest.Feature, Quantity: r.billingRequest.Quantity,
		}
	}
	payload := &tasks.TaskPayload{
		TaskID: taskID, CorrelationID: correlationID, UserID: r.userID,
		Prompt: r.prompt, Params: r.params, Priority: r.priority,
		RetryCount: 0, CreatedAt: createdAt.Unix(),
	}
	return task, payload
}
