package worker

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"testing"
	"time"

	"ideaboard/api/internal/accountpool/accounts"
	"ideaboard/api/internal/accountpool/providerendpoints"
	"ideaboard/api/internal/tasking/taskqueue"
	"ideaboard/api/internal/tasking/tasks"
	"ideaboard/api/internal/workflow"
)

func marshalWorkflowTaskParamsForTest(t testing.TB, params *tasks.WorkflowTaskParams) string {
	t.Helper()
	if workflow.IsAIImageFeature(params.Feature) && len(params.ExecutionPlan) == 0 {
		imageInputCount := 0
		hasMaskInput := false
		if params.Media != nil {
			imageInputCount = len(params.Media.Inputs) + len(params.Media.References)
			if params.Media.Original != nil {
				imageInputCount++
			}
			hasMaskInput = params.Media.Mask != nil
		}
		var requestedOutputCount *int
		if params.Controls != nil && params.Controls.OutputCount > 0 {
			count := params.Controls.OutputCount
			requestedOutputCount = &count
		}
		plan, err := workflow.CompileFeatureExecutionPlan(workflow.CompileFeatureExecutionInput{
			Feature: params.Feature,
			Params: workflow.FeatureExecutionParams{
				RequestedOutputCount: requestedOutputCount,
				ImageInputCount:      imageInputCount,
				HasMaskInput:         hasMaskInput,
			},
		})
		if err != nil {
			t.Fatalf("compile workflow execution plan: %v", err)
		}
		params.ExecutionPlan, err = workflow.EncodeFeatureExecutionPlan(plan)
		if err != nil {
			t.Fatalf("encode workflow execution plan: %v", err)
		}
		if params.Controls == nil {
			params.Controls = &tasks.GenerationControls{}
		}
		params.Controls.OutputCount = plan.RequestedOutputCount().Int()
		if params.Generation == nil {
			params.Generation = &tasks.GenerationRequestSnapshot{}
		}
		params.Generation.SchemaVersion = tasks.CurrentParamsSchemaVersion
		params.Generation.Feature = plan.Feature()
		params.Generation.Primitive = string(plan.RuntimeInputs().GenerationPrimitive())
		params.Generation.RequestedOutputCount = plan.RequestedOutputCount().Int()
		if params.Generation.TemplateFingerprint == "" {
			params.Generation.TemplateFingerprint = "worker-test"
		}
		params.Generation.Media = generationMediaFromParamsForTest(params.Media)
	}
	payload, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("marshal workflow task params: %v", err)
	}
	return string(payload)
}

func generationMediaFromParamsForTest(media *tasks.MediaInputs) []tasks.GenerationMediaIntent {
	if media == nil {
		return nil
	}
	result := make([]tasks.GenerationMediaIntent, 0, len(media.Inputs)+len(media.References)+2)
	appendRef := func(ref tasks.MediaRef, fallbackRole string) {
		if ref.AssetID <= 0 {
			return
		}
		role := ref.Role
		if role == "" {
			role = fallbackRole
		}
		result = append(result, tasks.GenerationMediaIntent{
			InputIndex: len(result), Role: role, AssetID: ref.AssetID, Weight: ref.Weight,
		})
	}
	for _, ref := range media.Inputs {
		appendRef(ref, "input")
	}
	for _, ref := range media.References {
		appendRef(ref, "reference")
	}
	if media.Original != nil {
		appendRef(*media.Original, "original")
	}
	if media.Mask != nil {
		appendRef(*media.Mask, "mask")
	}
	return result
}

type testRuntimeMediaResolver struct {
	urls map[int64]string
}

func (r testRuntimeMediaResolver) ResolveRuntimeMedia(_ context.Context, params *tasks.WorkflowTaskParams) (*tasks.RuntimeMediaInputs, error) {
	if params == nil || params.Media == nil {
		return nil, nil
	}
	resolve := func(ref tasks.MediaRef) tasks.RuntimeMediaRef {
		return tasks.RuntimeMediaRef{URL: r.urls[ref.AssetID], Role: ref.Role, Weight: ref.Weight}
	}
	result := &tasks.RuntimeMediaInputs{UseMask: params.Media.UseMask}
	for _, ref := range params.Media.Inputs {
		result.Inputs = append(result.Inputs, resolve(ref))
	}
	for _, ref := range params.Media.References {
		result.References = append(result.References, resolve(ref))
	}
	if params.Media.Original != nil {
		ref := resolve(*params.Media.Original)
		result.Original = &ref
	}
	if params.Media.Mask != nil {
		ref := resolve(*params.Media.Mask)
		result.Mask = &ref
	}
	return result, nil
}

func roundTripDrawingTaskForTest(t *testing.T, task *tasks.TaskPayload) *tasks.TaskPayload {
	t.Helper()
	payload, err := json.Marshal(taskqueue.DrawingTaskArgs{Task: *task})
	if err != nil {
		t.Fatalf("marshal drawing task args: %v", err)
	}
	var decoded taskqueue.DrawingTaskArgs
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("unmarshal drawing task args: %v", err)
	}
	return &decoded.Task
}

func (c *ProviderExecutor) callOpenAIForTest(ctx context.Context, account *accounts.ApiAccount, prompt string, width, height int, timeout time.Duration, params map[string]interface{}, resolvedModel string) (providerCallResult, error) {
	operation, invocation, controls := providerRuntimeIntentForTest(params)
	compiledParams := workflow.BuildProviderPromptParams(params)
	return c.callOpenAIWithRequest(ctx, account, newProviderRequest(prompt, width, height, compiledParams, resolvedModel, providerRequestFactsForTest(compiledParams, operation), operation, invocation, controls), timeout)
}

func (c *ProviderExecutor) callGoogleAIForTest(ctx context.Context, account *accounts.ApiAccount, prompt string, timeout time.Duration, params map[string]interface{}, resolvedModel string) (providerCallResult, error) {
	operation, invocation, controls := providerRuntimeIntentForTest(params)
	compiledParams := workflow.BuildProviderPromptParams(params)
	return c.callGoogleAIWithRequest(ctx, account, newProviderRequest(prompt, 0, 0, compiledParams, resolvedModel, providerRequestFactsForTest(compiledParams, operation), operation, invocation, controls), timeout)
}

func (c *ProviderExecutor) callGoogleCloudForTest(ctx context.Context, account *accounts.ApiAccount, prompt string, timeout time.Duration, params map[string]interface{}, resolvedModel string) (providerCallResult, error) {
	operation, invocation, controls := providerRuntimeIntentForTest(params)
	compiledParams := workflow.BuildProviderPromptParams(params)
	return c.callGoogleProviderWithRequest(ctx, account, "google-cloud", newProviderRequest(prompt, 0, 0, compiledParams, resolvedModel, providerRequestFactsForTest(compiledParams, operation), operation, invocation, controls), timeout)
}

func providerRequestFactsForTest(params map[string]interface{}, operation workflow.RuntimeOperation) resolvedProviderRequestFacts {
	count := 1
	switch raw := params["output_count"].(type) {
	case int:
		count = raw
	case int64:
		count = int(raw)
	case float64:
		count = int(raw)
	case json.Number:
		count, _ = strconv.Atoi(string(raw))
	case string:
		count, _ = strconv.Atoi(strings.TrimSpace(raw))
	}
	resolution := workflow.OutputResolution1K
	if raw, ok := params["output_resolution"].(string); ok {
		if normalized, valid := workflow.NormalizeOutputResolution(raw); valid && normalized != workflow.OutputResolutionDefault {
			resolution = normalized
		}
	}
	primitive := workflow.GenerationPrimitiveTextToImage
	if operation != workflow.RuntimeOperationPromptGeneration {
		primitive = workflow.GenerationPrimitiveImageToImage
	}
	return resolvedProviderRequestFacts{primitive: primitive, outputCount: count, outputResolution: resolution}
}

func providerRuntimeIntentForTest(params map[string]interface{}) (workflow.RuntimeOperation, workflow.RuntimeInvocation, workflow.RuntimeControlContract) {
	feature, _ := workflow.ResolveWorkflowFeatureSlug(params)
	switch feature {
	case workflow.FeatureSlugLocalInpaint:
		return workflow.RuntimeOperationLocalInpaint, workflow.RuntimeInvocationEdit, workflow.RuntimeControlsNone
	case workflow.FeatureSlugSketchToImage:
		return workflow.RuntimeOperationSketchToImage, workflow.RuntimeInvocationEdit, workflow.RuntimeControlsNone
	case workflow.FeatureSlugStyleVariation:
		return workflow.RuntimeOperationStyleVariation, workflow.RuntimeInvocationEdit, workflow.RuntimeControlsVariation
	case workflow.FeatureSlugStyleFusion:
		return workflow.RuntimeOperationStyleFusion, workflow.RuntimeInvocationEdit, workflow.RuntimeControlsFusion
	case workflow.FeatureSlugImageExtend:
		return workflow.RuntimeOperationImageExtend, workflow.RuntimeInvocationEdit, workflow.RuntimeControlsExtend
	case workflow.FeatureSlugRemoveBg:
		return workflow.RuntimeOperationBackgroundRemoval, workflow.RuntimeInvocationTransform, workflow.RuntimeControlsNone
	case workflow.FeatureSlugHDUpscale:
		return workflow.RuntimeOperationImageUpscale, workflow.RuntimeInvocationTransform, workflow.RuntimeControlsUpscale
	default:
		return workflow.RuntimeOperationPromptGeneration, workflow.RuntimeInvocationGenerate, workflow.RuntimeControlsNone
	}
}

func (c *ProviderExecutor) callDrawingAPIForTest(ctx context.Context, account *accounts.ApiAccount, task *tasks.TaskPayload, assignedModel string) (providerCallOutput, error) {
	return c.callDrawingAPIWithAssignment(ctx, runtimeAccountAccessForTest(account), task, assignmentSnapshotForTest(account, assignedModel))
}

func runtimeAccountAccessForTest(account *accounts.ApiAccount) *accounts.RuntimeAccountAccess {
	if account == nil {
		return nil
	}
	accountID := account.ID
	if accountID <= 0 {
		accountID = 1
	}
	return &accounts.RuntimeAccountAccess{
		ID: accountID, Name: account.Name, APIKey: account.ApiKey, Timeout: account.Timeout,
	}
}

func assignmentSnapshotForTest(account *accounts.ApiAccount, resolvedModel string) taskqueue.TaskAssignment {
	if account == nil {
		return taskqueue.TaskAssignment{}
	}
	accountID := account.ID
	if accountID <= 0 {
		accountID = 1
	}
	endpointID := account.ProviderEndpointID
	if endpointID <= 0 {
		endpointID = 1
	}
	apiType := strings.TrimSpace(account.ApiType)
	if apiType == "" {
		apiType = workflow.ProviderOpenAICompatible
	}
	protocol := strings.TrimSpace(account.Protocol)
	if protocol == "" {
		protocol = providerendpoints.ProtocolModelAPI
	}
	name := strings.TrimSpace(account.Name)
	if name == "" {
		name = "test-provider"
	}
	return taskqueue.TaskAssignment{
		AccountID:                accountID,
		ProviderEndpointID:       endpointID,
		ResolvedModel:            strings.TrimSpace(resolvedModel),
		APIType:                  apiType,
		ProviderEndpointProtocol: protocol,
		ProviderEndpointName:     name,
		ProviderEndpointBaseURL:  strings.TrimSpace(account.ProxyURL),
	}
}
