package tasksubmission

import (
	"bytes"
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
	ExecutionPlan  *workflow.FeatureExecutionPlan
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
	executionPlan  *workflow.FeatureExecutionPlan
}

func NewCanonicalRequest(input CanonicalRequestInput) (CanonicalRequest, error) {
	feature, priority, err := validateCanonicalMetadata(input)
	if err != nil {
		return CanonicalRequest{}, err
	}
	executionPlan, err := validateCanonicalExecutionPlan(feature, input.Params, input.ExecutionPlan)
	if err != nil {
		return CanonicalRequest{}, err
	}
	billingRequest, err := validateCanonicalBilling(feature, input.ClientActionID, executionPlan, input.BillingRequest)
	if err != nil {
		return CanonicalRequest{}, err
	}
	generation, err := validateCanonicalGeneration(input.Generation, executionPlan)
	if err != nil {
		return CanonicalRequest{}, err
	}
	if billingRequest != nil && generation != nil && billingRequest.Quantity != int64(generation.RequestedOutputCount) {
		return CanonicalRequest{}, fmt.Errorf("canonical submission billing quantity does not match generation output count")
	}
	return CanonicalRequest{
		userID: input.UserID, userPrompt: input.UserPrompt, prompt: input.Prompt, params: input.Params,
		clientActionID: strings.TrimSpace(input.ClientActionID), feature: feature,
		priority: priority, billingRequest: billingRequest,
		generation: generation, executionPlan: executionPlan,
	}, nil
}

func validateCanonicalMetadata(input CanonicalRequestInput) (string, int, error) {
	if input.UserID <= 0 {
		return "", 0, fmt.Errorf("canonical submission user id is invalid")
	}
	feature := strings.TrimSpace(input.Feature)
	if feature == "" {
		return "", 0, fmt.Errorf("canonical submission feature is required")
	}
	priority := tasks.EffectiveTaskPriority(input.Priority)
	if !tasks.IsValidTaskPriority(priority) {
		return "", 0, fmt.Errorf("canonical submission priority is invalid")
	}
	return feature, priority, nil
}

func validateCanonicalExecutionPlan(feature, params string, planInput *workflow.FeatureExecutionPlan) (*workflow.FeatureExecutionPlan, error) {
	if !workflow.IsAIImageFeature(feature) {
		if planInput != nil {
			return nil, fmt.Errorf("canonical non-image submission must not carry execution plan")
		}
		return nil, nil
	}
	if planInput == nil {
		return nil, fmt.Errorf("canonical AI image submission requires execution plan")
	}
	encodedPlan, err := workflow.EncodeFeatureExecutionPlan(*planInput)
	if err != nil {
		return nil, fmt.Errorf("canonical submission execution plan: %w", err)
	}
	if planInput.Feature() != feature {
		return nil, fmt.Errorf("canonical submission execution plan does not match feature")
	}
	typedParams, err := tasks.UnmarshalParams(params)
	if err != nil {
		return nil, fmt.Errorf("canonical submission params: %w", err)
	}
	persistedPlan, err := workflow.DecodeFeatureExecutionPlan(typedParams.ExecutionPlan)
	if err != nil {
		return nil, fmt.Errorf("canonical submission persisted execution plan: %w", err)
	}
	persistedPlanJSON, err := workflow.EncodeFeatureExecutionPlan(persistedPlan)
	if err != nil || !bytes.Equal(encodedPlan, persistedPlanJSON) {
		return nil, fmt.Errorf("canonical submission persisted execution plan does not match compiled plan")
	}
	planCopy := *planInput
	return &planCopy, nil
}

func validateCanonicalBilling(feature, clientActionID string, executionPlan *workflow.FeatureExecutionPlan, input *taskbilling.Request) (*taskbilling.Request, error) {
	if taskbilling.RequiresBilling(feature) && input == nil {
		return nil, fmt.Errorf("canonical billed submission requires billing request")
	}
	if input == nil {
		return nil, nil
	}
	request := *input
	if strings.TrimSpace(request.Feature) != feature {
		return nil, fmt.Errorf("canonical submission billing product does not match feature")
	}
	if _, err := taskbilling.RequestFingerprint(request); err != nil {
		return nil, fmt.Errorf("canonical submission billing request: %w", err)
	}
	if strings.TrimSpace(clientActionID) == "" {
		return nil, fmt.Errorf("canonical billed submission requires client action id")
	}
	if executionPlan != nil && (string(request.OutputResolution) != executionPlan.OutputResolution() || request.Quantity != int64(executionPlan.RequestedOutputCount().Int())) {
		return nil, fmt.Errorf("canonical submission billing request does not match execution plan")
	}
	return &request, nil
}

func validateCanonicalGeneration(input *tasks.GenerationRequestSnapshot, executionPlan *workflow.FeatureExecutionPlan) (*tasks.GenerationRequestSnapshot, error) {
	if input == nil {
		return nil, nil
	}
	copySnapshot := cloneGenerationSnapshot(*input)
	if err := validateGenerationSnapshot(copySnapshot); err != nil {
		return nil, err
	}
	if executionPlan != nil {
		if err := validateGenerationSnapshotAgainstPlan(copySnapshot, *executionPlan); err != nil {
			return nil, err
		}
	}
	return &copySnapshot, nil
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

func (r CanonicalRequest) ExecutionPlan() (workflow.FeatureExecutionPlan, bool) {
	if r.executionPlan == nil {
		return workflow.FeatureExecutionPlan{}, false
	}
	return *r.executionPlan, true
}

func (r CanonicalRequest) Params() string {
	return r.params
}

func (r CanonicalRequest) WithGenerationSnapshot(snapshot tasks.GenerationRequestSnapshot) (CanonicalRequest, error) {
	copySnapshot := cloneGenerationSnapshot(snapshot)
	for index := range copySnapshot.Media {
		copySnapshot.Media[index].Source = ""
	}
	if err := validateGenerationSnapshot(copySnapshot); err != nil {
		return CanonicalRequest{}, err
	}
	if r.executionPlan == nil {
		return CanonicalRequest{}, fmt.Errorf("update canonical generation snapshot requires execution plan")
	}
	if err := validateGenerationSnapshotAgainstPlan(copySnapshot, *r.executionPlan); err != nil {
		return CanonicalRequest{}, err
	}
	params := map[string]json.RawMessage{}
	if err := json.Unmarshal([]byte(r.params), &params); err != nil {
		return CanonicalRequest{}, fmt.Errorf("update canonical generation snapshot: %w", err)
	}
	typedParams, err := tasks.UnmarshalParams(r.params)
	if err != nil {
		return CanonicalRequest{}, fmt.Errorf("decode canonical params: %w", err)
	}
	media, err := tasks.ApplyGenerationMediaFacts(typedParams.Media, copySnapshot.Media)
	if err != nil {
		return CanonicalRequest{}, err
	}
	generationJSON, err := json.Marshal(copySnapshot)
	if err != nil {
		return CanonicalRequest{}, fmt.Errorf("encode canonical generation snapshot: %w", err)
	}
	params["generation"] = generationJSON
	mediaJSON, err := json.Marshal(media)
	if err != nil {
		return CanonicalRequest{}, fmt.Errorf("encode canonical media: %w", err)
	}
	params["media"] = mediaJSON
	paramsJSON, err := json.Marshal(params)
	if err != nil {
		return CanonicalRequest{}, fmt.Errorf("encode canonical params: %w", err)
	}
	r.generation = &copySnapshot
	r.params = string(paramsJSON)
	return r, nil
}

func (r CanonicalRequest) WithInputManifest(manifest *tasks.TaskInputManifest) (CanonicalRequest, error) {
	if manifest == nil {
		return r, nil
	}
	if err := manifest.Validate(); err != nil {
		return CanonicalRequest{}, err
	}
	typedParams, err := tasks.UnmarshalParams(r.params)
	if err != nil {
		return CanonicalRequest{}, fmt.Errorf("decode canonical params: %w", err)
	}
	typedParams.InputManifest = manifest
	paramsJSON, err := json.Marshal(typedParams)
	if err != nil {
		return CanonicalRequest{}, fmt.Errorf("encode canonical input manifest: %w", err)
	}
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
	if workflow.IsAIImageFeature(r.feature) {
		if r.executionPlan == nil || r.executionPlan.Feature() != r.feature {
			return false
		}
		if _, err := workflow.EncodeFeatureExecutionPlan(*r.executionPlan); err != nil {
			return false
		}
		if r.generation != nil && validateGenerationSnapshotAgainstPlan(*r.generation, *r.executionPlan) != nil {
			return false
		}
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
		if media.InputIndex != index || (media.AssetID <= 0 && strings.TrimSpace(media.Source) == "") || !tasks.IsGenerationMediaRole(media.Role) {
			return fmt.Errorf("canonical generation snapshot media order is invalid")
		}
	}
	return nil
}

// ValidatePreparedGenerationMedia requires persisted generation inputs to be
// backed by registered assets rather than durable URL references.
func ValidatePreparedGenerationMedia(snapshot *tasks.GenerationRequestSnapshot) error {
	if snapshot == nil {
		return nil
	}
	for index, media := range snapshot.Media {
		if media.AssetID <= 0 {
			return fmt.Errorf("prepared generation media asset is missing: index=%d", index)
		}
	}
	return nil
}

func validateGenerationSnapshotAgainstPlan(snapshot tasks.GenerationRequestSnapshot, plan workflow.FeatureExecutionPlan) error {
	runtimeInputs := plan.RuntimeInputs()
	if snapshot.Feature != plan.Feature() ||
		snapshot.Primitive != string(runtimeInputs.GenerationPrimitive()) ||
		snapshot.RequestedOutputCount != plan.RequestedOutputCount().Int() {
		return fmt.Errorf("canonical generation snapshot does not match execution plan")
	}
	imageInputCount := 0
	hasMaskInput := false
	for _, media := range snapshot.Media {
		if media.Role == tasks.MediaRoleMask {
			hasMaskInput = true
			continue
		}
		imageInputCount++
	}
	if imageInputCount != runtimeInputs.ImageInputCount() || hasMaskInput != runtimeInputs.HasMaskInput() {
		return fmt.Errorf("canonical generation snapshot media facts do not match execution plan")
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
