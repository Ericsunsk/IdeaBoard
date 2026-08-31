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

type AccountStore interface {
	GetAllEnabled(ctx context.Context) ([]*accounts.ApiAccount, error)
}

type TaskIntakeDeps struct {
	Accounts       AccountStore
	Prompts        FeaturePromptStore
	Media          taskmedia.Validator
	MediaReadiness taskmedia.ProcessingInputReadiness
	MediaPreparer  taskmedia.GenerationMediaPreparer
}

type taskIntake struct {
	accounts       AccountStore
	prompts        FeaturePromptStore
	media          taskmedia.Validator
	mediaReadiness taskmedia.ProcessingInputReadiness
	mediaPreparer  taskmedia.GenerationMediaPreparer
}

func NewTaskIntake(deps TaskIntakeDeps) TaskIntake {
	return &taskIntake{
		accounts:       deps.Accounts,
		prompts:        deps.Prompts,
		media:          deps.Media,
		mediaReadiness: deps.MediaReadiness,
		mediaPreparer:  deps.MediaPreparer,
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
	executionParams, err := compileFeatureExecutionParams(params, featureSlug)
	if err != nil {
		return TaskReadinessCompilation{}, err
	}
	executionPlan, err := workflow.CompileFeatureReadinessPlan(workflow.CompileFeatureExecutionInput{
		Feature: featureSlug,
		Params:  executionParams,
	})
	if err != nil {
		return TaskReadinessCompilation{}, mapFeatureContractValidationError(err)
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
	if err := i.ensureProviderAvailable(ctx, executionPlan.RoutingRequirement()); err != nil {
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
	assetIDs := submitRequestAssetIDs(req)
	if err := i.validateImageAssetIDs(ctx, userID, assetIDs); err != nil {
		return CanonicalTaskRequest{}, err
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
	executionParams, err := compileFeatureExecutionParams(params, featureSlug)
	if err != nil {
		return CanonicalTaskRequest{}, err
	}
	executionPlan, err := workflow.CompileFeatureExecutionPlan(workflow.CompileFeatureExecutionInput{
		Feature: featureSlug,
		Params:  executionParams,
	})
	if err != nil {
		return CanonicalTaskRequest{}, mapFeatureContractValidationError(err)
	}
	params[paramkeys.OutputCount] = executionPlan.RequestedOutputCount().Int()
	params[paramkeys.OutputResolution] = executionPlan.OutputResolution()
	typedParams, err := workflowTaskParamsFromMap(params, featureSlug)
	if err != nil {
		return CanonicalTaskRequest{}, err
	}
	if err := validateImageFillDimensions(featureSlug, typedParams); err != nil {
		return CanonicalTaskRequest{}, err
	}
	executionPlanJSON, err := workflow.EncodeFeatureExecutionPlan(executionPlan)
	if err != nil {
		return CanonicalTaskRequest{}, err
	}
	typedParams.ExecutionPlan = executionPlanJSON
	if err := i.ensureProviderAvailable(ctx, executionPlan.RoutingRequirement()); err != nil {
		return CanonicalTaskRequest{}, err
	}

	promptConfig := prompts.FeaturePromptRuntimeConfig{
		UserPromptMergePolicy: prompts.DefaultFeaturePromptMergePolicy(featureSlug),
	}
	if i.prompts != nil && executionPlan.PromptPolicy() != workflow.PromptPolicyIgnored {
		promptConfig, err = i.prompts.GetRuntimeConfig(ctx, featureSlug)
		if err != nil {
			return CanonicalTaskRequest{}, newSubmissionError("prompt_unavailable", "系统提示词暂不可用", err)
		}
	}
	if requiresImageFillSystemPrompt(featureSlug) && strings.TrimSpace(promptConfig.Prompt) == "" {
		return CanonicalTaskRequest{}, validation.New("该功能未配置 System Prompt")
	}
	var fullPrompt, systemPrompt string
	// Dedicated image transforms are binary media operations. Their Modal
	// contracts do not consume user text or a System Prompt.
	if executionPlan.PromptPolicy() != workflow.PromptPolicyIgnored {
		systemPrompt = strings.TrimSpace(prompts.ApplyFeatureSystemPromptWithConfig("", "", featureSlug, params, promptConfig))
		fullPrompt = strings.TrimSpace(prompts.ApplyFeatureSystemPromptWithConfig(req.Prompt, req.Prompt, featureSlug, params, promptConfig))
	}
	if executionPlan.PromptPolicy() == workflow.PromptPolicyRequired && fullPrompt == "" {
		return CanonicalTaskRequest{}, validation.New("Prompt Node 需要系统模板或用户描述")
	}
	if err := validateSubmitTaskPrompt(fullPrompt, maxSubmitTaskFinalPromptBytes, "最终提示词"); err != nil {
		return CanonicalTaskRequest{}, err
	}
	generationSnapshot, err := tasksubmission.CompileGenerationSnapshot(tasksubmission.GenerationCompilerInput{
		Plan:                executionPlan,
		UserPrompt:          req.Prompt,
		SystemPrompt:        systemPrompt,
		CompiledPrompt:      fullPrompt,
		TemplateFingerprint: tasksubmission.TemplateFingerprint("", promptConfig.Prompt),
		SeedMaterial:        clientActionID,
		Params:              typedParams,
	})
	if err != nil {
		return CanonicalTaskRequest{}, err
	}
	billingRequest, err := resolveTaskBillingRequest(executionPlan)
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
		ExecutionPlan:  &executionPlan,
	})
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
	snapshot := canonical.GenerationSnapshot()
	if snapshot == nil || len(snapshot.Media) == 0 {
		return canonical, nil
	}
	if i.mediaPreparer == nil {
		return CanonicalTaskRequest{}, newSubmissionError("media_preparation_failed", "参考图片准备失败，请稍后重试", fmt.Errorf("generation media preparer is unavailable"))
	}
	prepared, err := i.mediaPreparer.PrepareGenerationMedia(ctx, userID, canonical.ClientActionID(), snapshot)
	if err != nil {
		return CanonicalTaskRequest{}, newSubmissionError("media_preparation_failed", "参考图片准备失败，请稍后重试", err)
	}
	assetIDs := make([]int64, 0, len(prepared.Media))
	for _, media := range prepared.Media {
		assetIDs = append(assetIDs, media.AssetID)
	}
	if err := i.validateProcessingInputReadiness(ctx, userID, assetIDs); err != nil {
		return CanonicalTaskRequest{}, err
	}
	updated, err := canonical.WithGenerationSnapshot(prepared)
	if err != nil {
		return CanonicalTaskRequest{}, newSubmissionError("media_preparation_failed", "参考图片准备失败，请稍后重试", err)
	}
	return updated, nil
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

func (i *taskIntake) ensureProviderAvailable(ctx context.Context, requirement workflow.RoutingRequirement) error {
	if i == nil || i.accounts == nil {
		return newSubmissionError("provider_unavailable", providerUnavailableMessage, errNoAvailableAPIAccount)
	}
	apiAccounts, err := i.accounts.GetAllEnabled(ctx)
	if err != nil {
		return newSubmissionError("provider_unavailable", providerUnavailableMessage, err)
	}
	apiAccounts = accounts.FilterAccountsWithCapability(apiAccounts, string(requirement.Capability()))
	if requirement.ModelRequirement() == workflow.RoutingModelRequired {
		apiAccounts = accounts.FilterAccountsWithOutputResolution(apiAccounts, requirement.OutputResolution())
	}
	if len(apiAccounts) == 0 {
		return newSubmissionError("provider_unavailable", providerUnavailableMessage, errNoAvailableAPIAccount)
	}
	return nil
}

func resolveTaskBillingRequest(plan workflow.FeatureExecutionPlan) (*taskbilling.Request, error) {
	if !taskbilling.RequiresBilling(plan.Feature()) {
		return nil, nil
	}
	request, err := taskbilling.AIImageRequest(
		plan.Feature(),
		plan.OutputResolution(),
		int64(plan.RequestedOutputCount().Int()),
	)
	if err != nil {
		return nil, err
	}
	return &request, nil
}

func compileFeatureExecutionParams(params map[string]interface{}, featureSlug string) (workflow.FeatureExecutionParams, error) {
	result := workflow.FeatureExecutionParams{}
	if raw, ok := params[paramkeys.OutputCount]; ok {
		value, ok := readExactIntParam(raw)
		if !ok {
			return result, validation.New("output_count 仅支持整数 1-4")
		}
		result.RequestedOutputCount = &value
	}
	if raw, ok := params[paramkeys.OutputResolution]; ok {
		value, ok := raw.(string)
		if !ok {
			return result, validation.New("output_resolution 必须是字符串")
		}
		result.OutputResolution = &value
	}
	media := workflowMediaInputsFromMap(params, featureSlug)
	if media == nil {
		return result, nil
	}
	result.ImageInputCount = len(media.Inputs) + len(media.References)
	if media.Original != nil {
		result.ImageInputCount++
	}
	result.HasMaskInput = media.Mask != nil
	return result, nil
}

func mapFeatureContractValidationError(err error) error {
	var contractErr *workflow.FeatureContractValidationError
	if !errors.As(err, &contractErr) {
		return err
	}
	switch contractErr.Code {
	case workflow.FeatureContractUnknownFeature:
		return validation.New("feature 不受支持")
	case workflow.FeatureContractInvalidOutputCount:
		return validation.New("output_count 不符合功能要求")
	case workflow.FeatureContractUnsupportedResolution:
		return validation.New("该功能的 output_resolution 不受支持")
	case workflow.FeatureContractInvalidRuntimeInput:
		return validation.New("该功能的图片输入不符合要求")
	default:
		return validation.New("功能执行参数无效")
	}
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
