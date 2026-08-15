package taskservice

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"ideaboard/api/internal/accountpool/accounts"
	"ideaboard/api/internal/platform/validation"
	"ideaboard/api/internal/prompts"
	"ideaboard/api/internal/tasking/taskbilling"
	"ideaboard/api/internal/tasking/taskmedia"
	"ideaboard/api/internal/tasking/tasks"
	"ideaboard/api/internal/tasking/tasksubmission"
	"ideaboard/api/internal/workflow"
	"ideaboard/api/internal/workflow/paramkeys"
)

const (
	maxSubmitTaskPromptBytes         = 8 * 1024
	maxSubmitTaskFinalPromptBytes    = 32 * 1024
	maxSubmitTaskImageURLs           = 16
	maxSubmitTaskImageURLBytes       = 4096
	maxSubmitTaskClientActionIDBytes = 128
	maxSubmitTaskParamsBytes         = 2 * 1024 * 1024
	maxSubmitTaskParamsDepth         = 24
	providerUnavailableMessage       = "生图服务暂不可用，请稍后重试"
)

var errNoAvailableAPIAccount = errors.New("no available API account")

// TaskIntake is the single public seam that turns an HTTP submission request
// into the canonical Tasking request shared by Readiness and Submit.
type TaskIntake interface {
	Compile(context.Context, int64, *tasks.SubmitTaskRequest) (CanonicalTaskRequest, error)
	PrepareMedia(context.Context, int64, CanonicalTaskRequest) (CanonicalTaskRequest, error)
}

type CanonicalTaskRequest = tasksubmission.CanonicalRequest

type FeaturePromptStore interface {
	GetRuntimeConfig(ctx context.Context, slug string) (prompts.FeaturePromptRuntimeConfig, error)
}

type AccountStore interface {
	GetAllEnabled(ctx context.Context) ([]*accounts.ApiAccount, error)
}

type TaskIntakeDeps struct {
	Accounts      AccountStore
	Prompts       FeaturePromptStore
	Media         taskmedia.Validator
	MediaPreparer taskmedia.GenerationMediaPreparer
}

type taskIntake struct {
	accounts      AccountStore
	prompts       FeaturePromptStore
	media         taskmedia.Validator
	mediaPreparer taskmedia.GenerationMediaPreparer
}

func NewTaskIntake(deps TaskIntakeDeps) TaskIntake {
	return &taskIntake{
		accounts:      deps.Accounts,
		prompts:       deps.Prompts,
		media:         deps.Media,
		mediaPreparer: deps.MediaPreparer,
	}
}

func (i *taskIntake) Compile(ctx context.Context, userID int64, req *tasks.SubmitTaskRequest) (CanonicalTaskRequest, error) {
	if req == nil {
		return CanonicalTaskRequest{}, validation.New("请求参数错误")
	}
	clientActionID, err := normalizeSubmitTaskClientActionID(req.ClientActionID)
	if err != nil {
		return CanonicalTaskRequest{}, err
	}
	priority := tasks.EffectiveTaskPriority(req.Priority)
	if !tasks.IsValidTaskPriority(priority) {
		return CanonicalTaskRequest{}, validation.New("priority 仅支持 1-4")
	}
	if err := validateSubmitTaskPrompt(req.Prompt, maxSubmitTaskPromptBytes, "提示词"); err != nil {
		return CanonicalTaskRequest{}, err
	}
	if err := i.validateImageURLs(ctx, userID, req.ImageURLs); err != nil {
		return CanonicalTaskRequest{}, err
	}

	params := make(map[string]interface{}, len(req.Params)+2)
	for key, value := range req.Params {
		params[key] = value
	}
	if req.Feature != "" {
		params["feature"] = req.Feature
	}
	if len(req.ImageURLs) > 0 {
		params["image_urls"] = req.ImageURLs
	}
	normalizeSubmitTaskMediaParams(params)
	if err := i.validateMediaParams(ctx, userID, params); err != nil {
		return CanonicalTaskRequest{}, err
	}

	featureSlug, err := workflow.ResolveWorkflowFeatureSlug(params)
	if err != nil {
		return CanonicalTaskRequest{}, err
	}
	if featureSlug == "" {
		return CanonicalTaskRequest{}, validation.New("feature 不能为空")
	}
	params["feature"] = featureSlug
	delete(params, "model")
	if err := workflow.NormalizeStructuredPromptContractParams(params, featureSlug); err != nil {
		return CanonicalTaskRequest{}, err
	}
	if featureSlug == workflow.FeatureSlugPromptGeneration {
		if err := workflow.ValidatePromptGenerationSubmitContract(params); err != nil {
			return CanonicalTaskRequest{}, err
		}
	}
	if featureSlug == workflow.FeatureSlugStyleFusion {
		if err := normalizeStyleFusionParams(params); err != nil {
			return CanonicalTaskRequest{}, err
		}
	}
	if rawOutputCount, ok := params[paramkeys.OutputCount]; ok {
		outputCount, ok := readExactIntParam(rawOutputCount)
		if !ok {
			return CanonicalTaskRequest{}, validation.New("output_count 仅支持整数 1-4")
		}
		requestedOutputCount, err := workflow.ParseRequestedOutputCount(outputCount)
		if err != nil {
			return CanonicalTaskRequest{}, validation.New("output_count 仅支持整数 1-4")
		}
		params[paramkeys.OutputCount] = requestedOutputCount.Int()
	}
	if featureSlug == workflow.FeatureSlugRemoveBg {
		params[paramkeys.OutputCount] = 1
	}
	resolution, ok := workflow.NormalizeOutputResolutionForFeature(featureSlug, readStringParam(params, paramkeys.OutputResolution))
	if !ok {
		return CanonicalTaskRequest{}, validation.New("该功能的 output_resolution 不受支持")
	}
	params[paramkeys.OutputResolution] = resolution
	typedParams, err := workflowTaskParamsFromMap(params, featureSlug)
	if err != nil {
		return CanonicalTaskRequest{}, err
	}
	if err := i.ensureProviderAvailable(ctx, resolution); err != nil {
		return CanonicalTaskRequest{}, err
	}

	promptConfig := prompts.FeaturePromptRuntimeConfig{
		UserPromptMergePolicy: prompts.DefaultFeaturePromptMergePolicy(featureSlug),
	}
	if i.prompts != nil {
		promptConfig, err = i.prompts.GetRuntimeConfig(ctx, featureSlug)
		if err != nil {
			return CanonicalTaskRequest{}, newSubmissionError("prompt_unavailable", "系统提示词暂不可用", err)
		}
	}
	fullPrompt := strings.TrimSpace(prompts.ApplyFeatureSystemPromptWithConfig(req.Prompt, req.Prompt, featureSlug, params, promptConfig))
	if featureSlug == workflow.FeatureSlugPromptGeneration && fullPrompt == "" {
		return CanonicalTaskRequest{}, validation.New("Prompt Node 需要系统模板或用户描述")
	}
	if err := validateSubmitTaskPrompt(fullPrompt, maxSubmitTaskFinalPromptBytes, "最终提示词"); err != nil {
		return CanonicalTaskRequest{}, err
	}
	generationSnapshot, err := tasksubmission.CompileGenerationSnapshot(tasksubmission.GenerationCompilerInput{
		Feature:             featureSlug,
		UserPrompt:          req.Prompt,
		CompiledPrompt:      fullPrompt,
		TemplateFingerprint: tasksubmission.TemplateFingerprint("", promptConfig.Prompt),
		Params:              typedParams,
	})
	if err != nil {
		return CanonicalTaskRequest{}, err
	}
	billingRequest, err := resolveTaskBillingRequest(featureSlug, resolution, generationSnapshot.RequestedOutputCount)
	if err != nil {
		return CanonicalTaskRequest{}, err
	}
	if billingRequest != nil && clientActionID == "" {
		return CanonicalTaskRequest{}, validation.New("AI 生图任务必须提供 client_action_id")
	}
	typedParams.Generation = &generationSnapshot
	paramsJSON, err := marshalWorkflowTaskParams(typedParams, params)
	if err != nil {
		return CanonicalTaskRequest{}, err
	}
	return tasksubmission.NewCanonicalRequest(tasksubmission.CanonicalRequestInput{
		UserID: userID, Feature: featureSlug, UserPrompt: req.Prompt, Prompt: fullPrompt,
		Params: paramsJSON, ClientActionID: clientActionID, Priority: priority,
		Generation:     &generationSnapshot,
		BillingRequest: billingRequest,
	})
}

func (i *taskIntake) PrepareMedia(ctx context.Context, userID int64, canonical CanonicalTaskRequest) (CanonicalTaskRequest, error) {
	if userID <= 0 || canonical.UserID() != userID {
		return CanonicalTaskRequest{}, validation.New("请求用户与任务不匹配")
	}
	snapshot := canonical.GenerationSnapshot()
	if snapshot == nil || len(snapshot.Media) == 0 || i.mediaPreparer == nil {
		return canonical, nil
	}
	prepared, err := i.mediaPreparer.PrepareGenerationMedia(ctx, userID, canonical.ClientActionID(), snapshot)
	if err != nil {
		return CanonicalTaskRequest{}, newSubmissionError("media_preparation_failed", "参考图片准备失败，请稍后重试", err)
	}
	return canonical.WithGenerationSnapshot(prepared)
}

func (i *taskIntake) validateImageURLs(ctx context.Context, userID int64, imageURLs []string) error {
	return validateSubmitTaskImageURLsWithRefValidator(imageURLs, func(raw string) error {
		return i.validateMediaRef(ctx, userID, raw)
	})
}

func (i *taskIntake) validateMediaParams(ctx context.Context, userID int64, params map[string]interface{}) error {
	return validateSubmitTaskMediaParamsWithRefValidator(params, func(raw string) error {
		return i.validateMediaRef(ctx, userID, raw)
	})
}

func (i *taskIntake) validateMediaRef(ctx context.Context, userID int64, raw string) error {
	if i == nil || i.media == nil {
		return fmt.Errorf("任务媒体校验服务未配置")
	}
	return i.media.ValidateSubmitMediaRef(ctx, userID, raw)
}

func (i *taskIntake) ensureProviderAvailable(ctx context.Context, resolution string) error {
	if i == nil || i.accounts == nil {
		return newSubmissionError("provider_unavailable", providerUnavailableMessage, errNoAvailableAPIAccount)
	}
	apiAccounts, err := i.accounts.GetAllEnabled(ctx)
	if err != nil {
		return newSubmissionError("provider_unavailable", providerUnavailableMessage, err)
	}
	apiAccounts = accounts.FilterAccountsWithOutputResolution(apiAccounts, resolution)
	if len(apiAccounts) == 0 {
		return newSubmissionError("provider_unavailable", providerUnavailableMessage, errNoAvailableAPIAccount)
	}
	return nil
}

func readStringParam(params map[string]interface{}, key string) string {
	raw, ok := params[key]
	if !ok {
		return ""
	}
	value, ok := raw.(string)
	if !ok {
		return "\x00"
	}
	return value
}

func resolveTaskBillingRequest(feature, outputResolution string, quantity int) (*taskbilling.Request, error) {
	if !taskbilling.RequiresBilling(feature) {
		return nil, nil
	}
	request, err := taskbilling.AIImageRequest(feature, outputResolution, int64(quantity))
	if err != nil {
		return nil, err
	}
	return &request, nil
}

func normalizeSubmitTaskClientActionID(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", nil
	}
	if len(value) > maxSubmitTaskClientActionIDBytes {
		return "", fmt.Errorf("client_action_id 过长")
	}
	for _, r := range value {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' {
			continue
		}
		switch r {
		case '-', '_', '.', ':':
			continue
		default:
			return "", fmt.Errorf("client_action_id 只能包含字母、数字、横线、下划线、点或冒号")
		}
	}
	return value, nil
}
