package task

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"ideaboard/api/internal/platform/validation"
	"ideaboard/api/internal/prompts"
	taskbilling "ideaboard/api/internal/tasking/application/billing"
	tasksubmission "ideaboard/api/internal/tasking/application/submission"
	tasks "ideaboard/api/internal/tasking/domain"
	taskmedia "ideaboard/api/internal/tasking/ports/media"
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
	CompileReadiness(context.Context, int64, *tasks.SubmitTaskRequest) (TaskReadinessCompilation, error)
	PrepareMedia(context.Context, int64, CanonicalTaskRequest) (CanonicalTaskRequest, error)
}

type CanonicalTaskRequest = tasksubmission.CanonicalRequest

type TaskReadinessCompilation struct {
	Feature        string
	BillingRequest *taskbilling.Request
}

type FeaturePromptStore interface {
	GetRuntimeConfig(ctx context.Context, slug string) (prompts.FeaturePromptRuntimeConfig, error)
}

// ModelStore resolves the admin-configured model for one feature.
// Composition wires the AI gateway settings repository; tests inject fakes.
type ModelStore interface {
	ResolveModel(ctx context.Context, feature string) (string, error)
}

type TaskIntakeDeps struct {
	Models         ModelStore
	Prompts        FeaturePromptStore
	Media          taskmedia.Validator
	MediaReadiness taskmedia.ProcessingInputReadiness
	MediaPreparer  taskmedia.GenerationMediaPreparer
	MediaManifest  taskmedia.TaskInputManifestBuilder
}

type taskIntake struct {
	models         ModelStore
	prompts        FeaturePromptStore
	media          taskmedia.Validator
	mediaReadiness taskmedia.ProcessingInputReadiness
	mediaPreparer  taskmedia.GenerationMediaPreparer
	mediaManifest  taskmedia.TaskInputManifestBuilder
}

func NewTaskIntake(deps TaskIntakeDeps) TaskIntake {
	return &taskIntake{
		models:         deps.Models,
		prompts:        deps.Prompts,
		media:          deps.Media,
		mediaReadiness: deps.MediaReadiness,
		mediaPreparer:  deps.MediaPreparer,
		mediaManifest:  deps.MediaManifest,
	}
}

func (i *taskIntake) CompileReadiness(ctx context.Context, userID int64, req *tasks.SubmitTaskRequest) (TaskReadinessCompilation, error) {
	if req == nil {
		return TaskReadinessCompilation{}, validation.New("请求参数错误")
	}
	params := make(map[string]interface{}, len(req.Params)+1)
	for key, value := range req.Params {
		params[key] = value
	}
	if req.Feature != "" {
		params["feature"] = req.Feature
	}
	featureSlug, err := workflow.ResolveWorkflowFeatureSlug(params)
	if err != nil {
		return TaskReadinessCompilation{}, err
	}
	if featureSlug == "" {
		return TaskReadinessCompilation{}, validation.New("feature 不能为空")
	}
	executionPlan, err := compileReadinessExecutionPlan(params, featureSlug)
	if err != nil {
		return TaskReadinessCompilation{}, err
	}
	typedParams, err := workflowTaskParamsFromMap(params, featureSlug)
	if err != nil {
		return TaskReadinessCompilation{}, err
	}
	if err := validateImageFillDimensions(featureSlug, typedParams); err != nil {
		return TaskReadinessCompilation{}, err
	}
	if err := i.validateProcessingInputReadiness(ctx, userID, submitRequestAssetIDs(req)); err != nil {
		return TaskReadinessCompilation{}, err
	}
	if requiresImageFillSystemPrompt(featureSlug) {
		if err := i.requireSystemPrompt(ctx, featureSlug); err != nil {
			return TaskReadinessCompilation{}, err
		}
	}
	if err := i.ensureProviderAvailable(ctx, featureSlug); err != nil {
		return TaskReadinessCompilation{}, err
	}
	billingRequest, err := resolveTaskBillingRequest(executionPlan)
	if err != nil {
		return TaskReadinessCompilation{}, err
	}
	return TaskReadinessCompilation{
		Feature:        executionPlan.Feature(),
		BillingRequest: billingRequest,
	}, nil
}

func (i *taskIntake) Compile(ctx context.Context, userID int64, req *tasks.SubmitTaskRequest) (CanonicalTaskRequest, error) {
	if req == nil {
		return CanonicalTaskRequest{}, validation.New("请求参数错误")
	}
	normalized, err := i.normalizeCompileRequest(ctx, userID, req)
	if err != nil {
		return CanonicalTaskRequest{}, err
	}
	execution, err := i.compileTaskExecution(ctx, userID, req, normalized.params)
	if err != nil {
		return CanonicalTaskRequest{}, err
	}
	prompt, err := i.compileTaskPrompts(ctx, req, execution.featureSlug, execution.plan, normalized.params)
	if err != nil {
		return CanonicalTaskRequest{}, err
	}
	return assembleCanonicalTaskRequest(userID, req, normalized, execution, prompt)
}

type normalizedTaskCompile struct {
	clientActionID string
	priority       int
	params         map[string]interface{}
}

func (i *taskIntake) normalizeCompileRequest(ctx context.Context, userID int64, req *tasks.SubmitTaskRequest) (normalizedTaskCompile, error) {
	clientActionID, err := normalizeSubmitTaskClientActionID(req.ClientActionID)
	if err != nil {
		return normalizedTaskCompile{}, err
	}
	priority := tasks.EffectiveTaskPriority(req.Priority)
	if !tasks.IsValidTaskPriority(priority) {
		return normalizedTaskCompile{}, validation.New("priority 仅支持 1-4")
	}
	if err := validateSubmitTaskPrompt(req.Prompt, maxSubmitTaskPromptBytes, "提示词"); err != nil {
		return normalizedTaskCompile{}, err
	}
	if err := i.validateImageURLs(ctx, userID, req.ImageURLs); err != nil {
		return normalizedTaskCompile{}, err
	}
	if err := i.validateImageAssetIDs(ctx, userID, submitRequestAssetIDs(req)); err != nil {
		return normalizedTaskCompile{}, err
	}

	params := make(map[string]interface{}, len(req.Params)+3)
	for key, value := range req.Params {
		params[key] = value
	}
	if req.Feature != "" {
		params["feature"] = req.Feature
	}
	if len(req.ImageURLs) > 0 {
		params["image_urls"] = req.ImageURLs
	}
	if len(req.ImageAssetIDs) > 0 {
		params["image_asset_ids"] = req.ImageAssetIDs
	}
	if len(req.ReferenceImageAssetIDs) > 0 {
		params[paramkeys.ReferenceImageAssetIDs] = req.ReferenceImageAssetIDs
	}
	normalizeSubmitTaskMediaParams(params)
	if err := i.validateMediaParams(ctx, userID, params); err != nil {
		return normalizedTaskCompile{}, err
	}
	return normalizedTaskCompile{clientActionID: clientActionID, priority: priority, params: params}, nil
}

func (i *taskIntake) compileTaskExecution(ctx context.Context, userID int64, req *tasks.SubmitTaskRequest, params map[string]interface{}) (compiledTaskExecution, error) {
	featureSlug, err := workflow.ResolveWorkflowFeatureSlug(params)
	if err != nil {
		return compiledTaskExecution{}, err
	}
	if featureSlug == "" {
		return compiledTaskExecution{}, validation.New("feature 不能为空")
	}
	if err := normalizeSubmitExecutionMap(params, featureSlug); err != nil {
		return compiledTaskExecution{}, err
	}
	executionPlan, err := compileSubmitExecutionPlan(params, featureSlug)
	if err != nil {
		return compiledTaskExecution{}, err
	}
	typedParams, err := finalizeSubmitTypedParams(params, featureSlug, executionPlan)
	if err != nil {
		return compiledTaskExecution{}, err
	}
	if err := validateImageFillDimensions(featureSlug, typedParams); err != nil {
		return compiledTaskExecution{}, err
	}
	if err := i.validateProcessingInputReadiness(ctx, userID, submitRequestAssetIDs(req)); err != nil {
		return compiledTaskExecution{}, err
	}
	if err := i.ensureProviderAvailable(ctx, featureSlug); err != nil {
		return compiledTaskExecution{}, err
	}
	return compiledTaskExecution{featureSlug: featureSlug, plan: executionPlan, typedParams: typedParams}, nil
}

type compiledTaskPrompts struct {
	fullPrompt   string
	systemPrompt string
	config       prompts.FeaturePromptRuntimeConfig
}

func (i *taskIntake) compileTaskPrompts(ctx context.Context, req *tasks.SubmitTaskRequest, featureSlug string, executionPlan workflow.FeatureExecutionPlan, params map[string]interface{}) (compiledTaskPrompts, error) {
	promptConfig := prompts.FeaturePromptRuntimeConfig{
		UserPromptMergePolicy: prompts.DefaultFeaturePromptMergePolicy(featureSlug),
	}
	if i.prompts != nil && executionPlan.PromptPolicy() != workflow.PromptPolicyIgnored {
		var err error
		promptConfig, err = i.prompts.GetRuntimeConfig(ctx, featureSlug)
		if err != nil {
			return compiledTaskPrompts{}, newSubmissionError("prompt_unavailable", "系统提示词暂不可用", err)
		}
	}
	if requiresImageFillSystemPrompt(featureSlug) && strings.TrimSpace(promptConfig.Prompt) == "" {
		return compiledTaskPrompts{}, validation.New("该功能未配置 System Prompt")
	}
	var fullPrompt, systemPrompt string
	// Dedicated image transforms are binary media operations. Their Modal
	// contracts do not consume user text or a System Prompt.
	if executionPlan.PromptPolicy() != workflow.PromptPolicyIgnored {
		systemPrompt = strings.TrimSpace(prompts.ApplyFeatureSystemPromptWithConfig("", "", featureSlug, params, promptConfig))
		fullPrompt = strings.TrimSpace(prompts.ApplyFeatureSystemPromptWithConfig(req.Prompt, req.Prompt, featureSlug, params, promptConfig))
	}
	if executionPlan.PromptPolicy() == workflow.PromptPolicyRequired && fullPrompt == "" {
		return compiledTaskPrompts{}, validation.New("Prompt Node 需要系统模板或用户描述")
	}
	if err := validateSubmitTaskPrompt(fullPrompt, maxSubmitTaskFinalPromptBytes, "最终提示词"); err != nil {
		return compiledTaskPrompts{}, err
	}
	return compiledTaskPrompts{fullPrompt: fullPrompt, systemPrompt: systemPrompt, config: promptConfig}, nil
}

func mediaAssetIDsFromParams(params map[string]interface{}) []int64 {
	if len(params) == 0 {
		return nil
	}
	assetIDs := collectInt64List(params[paramkeys.ImageAssetIDs])
	assetIDs = append(assetIDs, collectInt64List(params[paramkeys.ReferenceImageAssetIDs])...)
	for _, key := range []string{paramkeys.OriginalImageAssetID, paramkeys.MaskImageAssetID} {
		if assetID, ok := readInt64Param(params[key]); ok {
			assetIDs = append(assetIDs, assetID)
		}
	}
	return assetIDs
}

func submitRequestAssetIDs(req *tasks.SubmitTaskRequest) []int64 {
	if req == nil {
		return nil
	}
	assetIDs := append([]int64(nil), req.ImageAssetIDs...)
	assetIDs = append(assetIDs, req.ReferenceImageAssetIDs...)
	return append(assetIDs, mediaAssetIDsFromParams(req.Params)...)
}

func requiresImageFillSystemPrompt(featureSlug string) bool {
	return featureSlug == workflow.FeatureSlugInpaint || featureSlug == workflow.FeatureSlugOutpaint
}

func (i *taskIntake) requireSystemPrompt(ctx context.Context, featureSlug string) error {
	if i.prompts == nil {
		return validation.New("该功能未配置 System Prompt")
	}
	config, err := i.prompts.GetRuntimeConfig(ctx, featureSlug)
	if err != nil {
		return newSubmissionError("prompt_unavailable", "系统提示词暂不可用", err)
	}
	if strings.TrimSpace(config.Prompt) == "" {
		return validation.New("该功能未配置 System Prompt")
	}
	return nil
}

func (i *taskIntake) PrepareMedia(ctx context.Context, userID int64, canonical CanonicalTaskRequest) (CanonicalTaskRequest, error) {
	if userID <= 0 || canonical.UserID() != userID {
		return CanonicalTaskRequest{}, validation.New("请求用户与任务不匹配")
	}
	typedParams, err := tasks.UnmarshalParams(canonical.Params())
	if err != nil {
		return CanonicalTaskRequest{}, newSubmissionError("media_preparation_failed", "参考图片准备失败，请稍后重试", err)
	}
	updated := canonical
	snapshot := canonical.GenerationSnapshot()
	if snapshot != nil && len(snapshot.Media) > 0 {
		if i.mediaPreparer == nil {
			return CanonicalTaskRequest{}, newSubmissionError("media_preparation_failed", "参考图片准备失败，请稍后重试", fmt.Errorf("generation media preparer is unavailable"))
		}
		prepared, prepareErr := i.mediaPreparer.PrepareGenerationMedia(ctx, userID, canonical.ClientActionID(), snapshot)
		if prepareErr != nil {
			return CanonicalTaskRequest{}, newSubmissionError("media_preparation_failed", "参考图片准备失败，请稍后重试", prepareErr)
		}
		updated, err = canonical.WithGenerationSnapshot(prepared)
		if err != nil {
			return CanonicalTaskRequest{}, newSubmissionError("media_preparation_failed", "参考图片准备失败，请稍后重试", err)
		}
		typedParams, err = tasks.UnmarshalParams(updated.Params())
		if err != nil {
			return CanonicalTaskRequest{}, newSubmissionError("media_preparation_failed", "参考图片准备失败，请稍后重试", err)
		}
	}
	if typedParams.Media == nil || i.mediaManifest == nil {
		if typedParams.Media == nil {
			return updated, nil
		}
		return CanonicalTaskRequest{}, newSubmissionError("media_preparation_failed", "参考图片准备失败，请稍后重试", fmt.Errorf("task input manifest builder is unavailable"))
	}
	manifest, err := i.mediaManifest.BuildTaskInputManifest(ctx, userID, typedParams.Media)
	if err != nil {
		return CanonicalTaskRequest{}, newSubmissionError("media_processing_input_not_ready", "参考图片仍在处理中，请稍后重试", err)
	}
	return updated.WithInputManifest(manifest)
}

func (i *taskIntake) validateProcessingInputReadiness(ctx context.Context, userID int64, assetIDs []int64) error {
	if len(assetIDs) == 0 || i == nil || i.mediaReadiness == nil {
		return nil
	}
	if err := i.mediaReadiness.ValidateProcessingInputReadiness(ctx, userID, assetIDs); err != nil {
		return newSubmissionError("media_processing_input_not_ready", "参考图片仍在处理中，请稍后重试", err)
	}
	return nil
}

func (i *taskIntake) validateImageURLs(ctx context.Context, userID int64, imageURLs []string) error {
	return validateSubmitTaskImageURLsWithRefValidator(imageURLs, func(raw string) error {
		return i.validateMediaRef(ctx, userID, raw)
	})
}

func (i *taskIntake) validateImageAssetIDs(ctx context.Context, userID int64, assetIDs []int64) error {
	if len(assetIDs) == 0 {
		return nil
	}
	validator, ok := i.media.(interface {
		ValidateSubmitMediaAssetIDs(context.Context, int64, []int64) error
	})
	if !ok {
		return fmt.Errorf("媒体资产校验服务未配置")
	}
	return validator.ValidateSubmitMediaAssetIDs(ctx, userID, assetIDs)
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

func (i *taskIntake) ensureProviderAvailable(ctx context.Context, featureSlug string) error {
	if i == nil || i.models == nil {
		return newSubmissionError("provider_unavailable", providerUnavailableMessage, errNoAvailableAPIAccount)
	}
	model, err := i.models.ResolveModel(ctx, featureSlug)
	if err != nil || model == "" {
		return newSubmissionError("provider_unavailable", providerUnavailableMessage, err)
	}
	return nil
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
