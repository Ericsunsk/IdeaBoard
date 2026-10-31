package task

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"

	"ideaboard/api/internal/media/mediarefs"
	"ideaboard/api/internal/platform/errorrender"
	"ideaboard/api/internal/platform/jsonlimit"
	tasksubmission "ideaboard/api/internal/tasking/application/submission"
	tasks "ideaboard/api/internal/tasking/domain"
	"ideaboard/api/internal/workflow"
	"ideaboard/api/internal/workflow/paramkeys"
)

const (
	maxSubmitTaskParamsBytes = 2 * 1024 * 1024
	maxSubmitTaskParamsDepth = 24
)

// Canonical workspace compilation ordering.
//
// Workspace submit owns one map→typed→plan→persisted-JSON sequence. The order
// is load-bearing:
//
//  1. map: stamp feature, strip model, normalize structured prompt contracts,
//     then validate generation/fusion contracts (normalizeSubmitExecutionMap).
//  2. plan inputs + plan: derive FeatureExecutionParams from the normalized map
//     and compile the Workflow plan.
//  3. typed: stamp plan-derived output facts back into the map, convert to the
//     typed WorkflowTaskParams, and attach the encoded plan
//     (finalizeSubmitTypedParams). The write-back must precede the conversion
//     so typed Controls agree with the compiled plan.
//  4. persisted JSON: marshal the typed params with the generation snapshot.
//
// Readiness diverges only at stage 2: it compiles a readiness plan with
// synthesized media facts and never stamps plan facts back into the map.
//
// Workflow stays a pure plan/contract library: it never owns this sequencing,
// performs I/O, or persists anything.
// CompilerWorkspaceMediaAssetIDsParam carries workspace public asset ids into
// canonical compilation without persisting the HTTP envelope field in Extra.
const CompilerWorkspaceMediaAssetIDsParam = "__workspace_media_asset_ids"

// CompileWorkspaceParams applies canonical submit ordering to a workspace v1
// task envelope and returns the durable typed parameter JSON.
//
// Prompt contract (v1 data.prompt, bound: workspacetasks
// .MaxWorkspaceTaskPromptBytes = 8KB): prompt-required
// features (generation, inpaint) reject an empty prompt; prompt-ignored
// features (cutout, upscale) drop user text (binary media operations); the
// rest carry it into the Generation snapshot
// (UserPrompt + CompiledPrompt) that the worker bridges to the provider.
func CompileWorkspaceParams(feature, userPrompt string, params map[string]interface{}) (string, error) {
	feature = strings.TrimSpace(feature)
	if feature == "" {
		return "", errorrender.NewValidationError(errorrender.MsgCompileWorkspaceParams)
	}
	working := make(map[string]interface{}, len(params)+1)
	for key, value := range params {
		working[key] = value
	}
	prompt := strings.TrimSpace(userPrompt)
	if err := tasks.ValidateWorkspacePrompt(prompt); err != nil {
		return "", err
	}
	if err := normalizeSubmitExecutionMap(working, feature); err != nil {
		return "", err
	}
	plan, err := compileSubmitExecutionPlan(working, feature)
	if err != nil {
		return "", err
	}
	if plan.PromptPolicy() == workflow.PromptPolicyRequired && prompt == "" {
		return "", errorrender.NewValidationError(errorrender.MsgCompileWorkspaceParams2)
	}
	if plan.PromptPolicy() == workflow.PromptPolicyIgnored {
		prompt = ""
	}
	typed, err := finalizeSubmitTypedParams(working, feature, plan)
	if err != nil {
		return "", err
	}
	snapshot, err := tasksubmission.CompileGenerationSnapshot(tasksubmission.GenerationCompilerInput{
		Plan: plan, UserPrompt: prompt, CompiledPrompt: prompt,
		TemplateFingerprint: tasksubmission.TemplateFingerprint("", prompt), Params: typed,
	})
	if err != nil {
		return "", err
	}
	typed.Generation = &snapshot
	return marshalWorkflowTaskParams(typed, working)
}

// normalizeSubmitExecutionMap runs stage 1 of the canonical submit ordering.
// Contract normalization must precede media-fact derivation so fusion weights
// and structured prompt fields are canonical before anything reads them.
func normalizeSubmitExecutionMap(params map[string]interface{}, featureSlug string) error {
	params["service"] = featureSlug
	delete(params, "model")
	// v1 mask reference fail-closes here before any plan work; shape is
	// owned by tasks.IsValidMaskAssetID (same predicate as
	// tasks.ValidateParams) so durable params never carry media
	// bytes or URLs.
	if raw, ok := params[paramkeys.MaskAssetID]; ok {
		id, _ := raw.(string)
		if !tasks.IsValidMaskAssetID(id) {
			return errorrender.NewValidationError(errorrender.MsgValidateWorkspaceMaskParam)
		}
	}
	if err := workflow.NormalizeStructuredPromptContractParams(params, featureSlug); err != nil {
		return err
	}
	if featureSlug == workflow.FeatureSlugGeneration {
		if err := workflow.ValidateGenerationSubmitContract(params); err != nil {
			return err
		}
	}
	if featureSlug == workflow.FeatureSlugFusion {
		if err := normalizeFusionParams(params); err != nil {
			return err
		}
	}
	return nil
}

// compileSubmitExecutionPlan runs stage 2 of the canonical submit ordering:
// derive plan inputs from the normalized map, then compile the execution plan.
func compileSubmitExecutionPlan(params map[string]interface{}, featureSlug string) (workflow.FeatureExecutionPlan, error) {
	executionParams, err := compileFeatureExecutionParams(params, featureSlug)
	if err != nil {
		return workflow.FeatureExecutionPlan{}, err
	}
	executionPlan, err := workflow.CompileFeatureExecutionPlan(workflow.CompileFeatureExecutionInput{
		Feature: featureSlug,
		Params:  executionParams,
	})
	if err != nil {
		return workflow.FeatureExecutionPlan{}, mapFeatureContractValidationError(err)
	}
	return executionPlan, nil
}

// finalizeSubmitTypedParams runs stage 3 of the canonical submit ordering.
// Plan-derived output facts overwrite caller values in the map before the
// typed conversion, so the persisted typed Controls cannot disagree with the
// compiled plan.
func finalizeSubmitTypedParams(params map[string]interface{}, featureSlug string, executionPlan workflow.FeatureExecutionPlan) (*tasks.WorkflowTaskParams, error) {
	params[paramkeys.OutputCount] = executionPlan.RequestedOutputCount().Int()
	params[paramkeys.OutputResolution] = executionPlan.OutputResolution()
	typedParams, err := workflowTaskParamsFromMap(params, featureSlug)
	if err != nil {
		return nil, err
	}
	executionPlanJSON, err := workflow.EncodeFeatureExecutionPlan(executionPlan)
	if err != nil {
		return nil, err
	}
	typedParams.ExecutionPlan = executionPlanJSON
	return typedParams, nil
}

func compileFeatureExecutionParams(params map[string]interface{}, featureSlug string) (workflow.FeatureExecutionParams, error) {
	result := workflow.FeatureExecutionParams{}
	if raw, ok := params[paramkeys.OutputCount]; ok {
		value, ok := readExactIntParam(raw)
		if !ok {
			return result, errorrender.NewValidationError(errorrender.MsgCompileFeatureExecutionParams)
		}
		result.RequestedOutputCount = &value
	}
	if raw, ok := params[paramkeys.OutputResolution]; ok {
		value, ok := raw.(string)
		if !ok {
			return result, errorrender.NewValidationError(errorrender.MsgCompileFeatureExecutionParams2)
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
		return errorrender.NewValidationError(errorrender.MsgMapFeatureContractValidationError)
	case workflow.FeatureContractInvalidOutputCount:
		return errorrender.NewValidationError(errorrender.MsgMapFeatureContractValidationError2)
	case workflow.FeatureContractUnsupportedResolution:
		return errorrender.NewValidationError(errorrender.MsgMapFeatureContractValidationError3)
	case workflow.FeatureContractInvalidRuntimeInput:
		return errorrender.NewValidationError(errorrender.MsgMapFeatureContractValidationError4)
	default:
		return errorrender.NewValidationError(errorrender.MsgMapFeatureContractValidationError5)
	}
}

func marshalWorkflowTaskParams(params *tasks.WorkflowTaskParams, source map[string]interface{}) (string, error) {
	if params == nil {
		return "", nil
	}
	if source != nil && jsonlimit.ValueDepth(source) > maxSubmitTaskParamsDepth {
		return "", errors.New(errorrender.MsgMarshalWorkflowTaskParams)
	}
	payload, err := json.Marshal(params)
	if err != nil {
		return "", errors.New(errorrender.MsgMarshalWorkflowTaskParams2)
	}
	if len(payload) > maxSubmitTaskParamsBytes {
		return "", fmt.Errorf(errorrender.MsgMarshalWorkflowTaskParams3, maxSubmitTaskParamsBytes/1024/1024)
	}
	return string(payload), nil
}

// workflowTaskParamsFromMap converts submit params into the canonical typed
// WorkflowTaskParams stored on the task and passed through the queue.
func workflowTaskParamsFromMap(params map[string]interface{}, featureSlug string) (*tasks.WorkflowTaskParams, error) {
	p := &tasks.WorkflowTaskParams{SchemaVersion: tasks.CurrentParamsSchemaVersion}
	if params == nil {
		return p, nil
	}
	p.Extra = workflowExtraParamsFromMap(params)
	if v, ok := workflow.ReadWorkflowStringParam(params, paramkeys.Service); ok {
		p.Service = v
	}
	p.Media = workflowMediaInputsFromMap(params, featureSlug)
	p.Controls = generationControlsFromMap(params)
	applyFeatureSpecificTaskParams(p, params, featureSlug)
	return p, nil
}

func workflowExtraParamsFromMap(params map[string]interface{}) map[string]interface{} {
	var extra map[string]interface{}
	for key, value := range params {
		if key == "model" || key == CompilerWorkspaceMediaAssetIDsParam || paramkeys.IsKnownSubmitTaskParamKey(key) {
			continue
		}
		if extra == nil {
			extra = map[string]interface{}{}
		}
		extra[key] = value
	}
	return extra
}

func generationControlsFromMap(params map[string]interface{}) *tasks.GenerationControls {
	controls := &tasks.GenerationControls{}
	hasControls := false
	dimensions, hasDimensions := generationDimensionControlsFromMap(params)
	if hasDimensions {
		controls.CustomWidth = dimensions.CustomWidth
		controls.CustomHeight = dimensions.CustomHeight
		controls.OriginalWidth = dimensions.OriginalWidth
		controls.OriginalHeight = dimensions.OriginalHeight
		hasControls = true
	}
	stringsControls, hasStrings := generationStringControlsFromMap(params)
	if hasStrings {
		controls.Ratio = stringsControls.Ratio
		controls.Quality = stringsControls.Quality
		hasControls = true
	}
	output, hasOutput := generationOutputControlsFromMap(params)
	if hasOutput {
		controls.OutputCount = output.OutputCount
		controls.PromptUpsampling = output.PromptUpsampling
		hasControls = true
	}
	if !hasControls {
		return nil
	}
	return controls
}

func generationDimensionControlsFromMap(params map[string]interface{}) (tasks.GenerationControls, bool) {
	controls := tasks.GenerationControls{}
	hasControls := false
	if v, ok := readFloatParam(params, paramkeys.CustomWidth); ok {
		controls.CustomWidth = v
		hasControls = true
	}
	if v, ok := readFloatParam(params, paramkeys.CustomHeight); ok {
		controls.CustomHeight = v
		hasControls = true
	}
	if v, ok := readFloatParam(params, paramkeys.OriginalImageWidth); ok {
		controls.OriginalWidth = v
		hasControls = true
	}
	if v, ok := readFloatParam(params, paramkeys.OriginalImageHeight); ok {
		controls.OriginalHeight = v
		hasControls = true
	}
	return controls, hasControls
}

func generationStringControlsFromMap(params map[string]interface{}) (tasks.GenerationControls, bool) {
	controls := tasks.GenerationControls{}
	hasControls := false
	if v, ok := workflow.ReadWorkflowStringParam(params, paramkeys.Ratio); ok {
		controls.Ratio = v
		hasControls = true
	}
	if v, ok := workflow.ReadWorkflowStringParam(params, paramkeys.Quality); ok {
		controls.Quality = v
		hasControls = true
	}
	return controls, hasControls
}

func generationOutputControlsFromMap(params map[string]interface{}) (tasks.GenerationControls, bool) {
	controls := tasks.GenerationControls{}
	hasControls := false
	if v, ok := readIntParam(params, paramkeys.OutputCount); ok {
		controls.OutputCount = v
		hasControls = true
	}
	if v, ok := readBoolParam(params, paramkeys.PromptUpsampling); ok {
		controls.PromptUpsampling = &v
		hasControls = true
	}
	return controls, hasControls
}

func applyFeatureSpecificTaskParams(params *tasks.WorkflowTaskParams, raw map[string]interface{}, featureSlug string) {
	switch featureSlug {
	case workflow.FeatureSlugVariations:
		if c, ok := workflow.ReadVariationsPromptContract(raw); ok {
			params.Variation = &tasks.VariationsParams{Strength: c.Strength}
		}
	case workflow.FeatureSlugOutpaint:
		if c, ok := workflow.ReadOutpaintContract(raw); ok {
			params.Outpaint = &tasks.OutpaintParams{
				TargetWidth: c.TargetWidth, TargetHeight: c.TargetHeight,
				ImageOffsetX: c.ImageOffsetX, ImageOffsetY: c.ImageOffsetY,
			}
		}
	case workflow.FeatureSlugUpscale:
		if c, ok := workflow.ReadUpscaleContract(raw); ok {
			params.Upscale = &tasks.UpscaleParams{Scale: c.Scale}
		}
	case workflow.FeatureSlugFusion:
		if c, ok := workflow.ReadFusionPromptContract(raw); ok {
			params.Fusion = &tasks.FusionParams{
				Mode:         c.Mode,
				InputWeights: c.InputWeights,
			}
		}
	}
}

func workflowMediaInputsFromMap(params map[string]interface{}, featureSlug string) *tasks.MediaInputs {
	media := &tasks.MediaInputs{}
	hasMedia := false

	inputWeights := []float64(nil)
	if featureSlug == workflow.FeatureSlugFusion {
		if weights, ok := workflow.ReadFusionInputWeights(params); ok {
			inputWeights = weights
		}
	}

	imageURLs := mediarefs.CollectStringList(params[paramkeys.ImageURLs])
	if len(imageURLs) > 0 {
		media.Inputs = mediaRefsFromURLs(imageURLs, tasks.MediaRoleInput, inputWeights)
		hasMedia = true
	}
	assetIDs := collectInt64List(params[paramkeys.ImageAssetIDs])
	if len(assetIDs) > 0 {
		media.Inputs = append(media.Inputs, mediaRefsFromAssetIDs(assetIDs, tasks.MediaRoleInput)...)
		hasMedia = true
	}
	publicAssetIDs := collectPublicAssetIDs(params[CompilerWorkspaceMediaAssetIDsParam])
	if len(publicAssetIDs) > 0 {
		media.Inputs = append(media.Inputs, mediaRefsFromPublicAssetIDs(publicAssetIDs, tasks.MediaRoleInput)...)
		hasMedia = true
	}
	referenceAssetIDs := collectInt64List(params[paramkeys.ReferenceImageAssetIDs])
	if len(referenceAssetIDs) > 0 {
		media.References = append(media.References, mediaRefsFromAssetIDs(referenceAssetIDs, tasks.MediaRoleReference)...)
		hasMedia = true
	}
	referenceImageURLs := mediarefs.CollectStringList(params[paramkeys.ReferenceImageURLs])
	if len(referenceImageURLs) > 0 {
		media.References = mediaRefsFromURLs(referenceImageURLs, tasks.MediaRoleReference, nil)
		hasMedia = true
	}
	if v, ok := workflow.ReadWorkflowStringParam(params, paramkeys.MaskImageURL); ok {
		media.Mask = &tasks.MediaRef{Source: v, Role: tasks.MediaRoleMask}
		hasMedia = true
	}
	if assetID, ok := readInt64Param(params[paramkeys.OriginalImageAssetID]); ok {
		media.Original = &tasks.MediaRef{AssetID: assetID, Role: tasks.MediaRoleOriginal}
		hasMedia = true
	}
	if assetID, ok := readInt64Param(params[paramkeys.MaskImageAssetID]); ok {
		media.Mask = &tasks.MediaRef{AssetID: assetID, Role: tasks.MediaRoleMask}
		hasMedia = true
	}
	// v1 mask reference (workspace public asset_... id from the
	// upload-session flow). Wins over the legacy numeric MaskImageAssetID
	// when both are present; shape was fail-closed in
	// normalizeSubmitExecutionMap above via tasks.IsValidMaskAssetID.
	if id, ok := params[paramkeys.MaskAssetID].(string); ok && strings.TrimSpace(id) != "" {
		media.Mask = &tasks.MediaRef{PublicAssetID: strings.TrimSpace(id), Role: tasks.MediaRoleMask}
		hasMedia = true
	}
	if v, ok := readBoolParam(params, paramkeys.UseMask); ok {
		media.UseMask = &v
		hasMedia = true
	}
	if !hasMedia {
		return nil
	}
	return media
}

func mediaRefsFromURLs(urls []string, role string, weights []float64) []tasks.MediaRef {
	if len(urls) == 0 {
		return nil
	}
	refs := make([]tasks.MediaRef, 0, len(urls))
	for index, raw := range urls {
		ref := tasks.MediaRef{
			Source: strings.TrimSpace(raw),
			Role:   role,
		}
		if index < len(weights) {
			ref.Weight = weights[index]
		}
		refs = append(refs, ref)
	}
	return refs
}

func mediaRefsFromAssetIDs(assetIDs []int64, role string) []tasks.MediaRef {
	refs := make([]tasks.MediaRef, 0, len(assetIDs))
	for _, id := range assetIDs {
		refs = append(refs, tasks.MediaRef{AssetID: id, Role: role})
	}
	return refs
}

func collectPublicAssetIDs(raw interface{}) []string {
	var values []string
	switch typed := raw.(type) {
	case []string:
		values = append(values, typed...)
	case []interface{}:
		for _, value := range typed {
			if id, ok := value.(string); ok {
				values = append(values, id)
			}
		}
	case string:
		values = append(values, typed)
	}
	result := make([]string, 0, len(values))
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

func mediaRefsFromPublicAssetIDs(assetIDs []string, role string) []tasks.MediaRef {
	refs := make([]tasks.MediaRef, 0, len(assetIDs))
	for _, id := range assetIDs {
		refs = append(refs, tasks.MediaRef{PublicAssetID: strings.TrimSpace(id), Role: role})
	}
	return refs
}

func normalizeFusionParams(params map[string]interface{}) error {
	// Workflow normalizes the fusion shape and mode first; submission only
	// enforces image/weight counts and normalizes the weights to sum 1.
	if params == nil {
		return nil
	}
	imageURLs := mediarefs.CollectStringList(params[paramkeys.ImageURLs])
	if len(imageURLs) < 2 || len(imageURLs) > 4 {
		return errors.New(errorrender.MsgNormalizeFusionParams)
	}
	weights, ok := workflow.ReadFusionInputWeights(params)
	if !ok {
		return errors.New(errorrender.MsgNormalizeFusionParams4)
	}
	if len(weights) != len(imageURLs) {
		return errors.New(errorrender.MsgNormalizeFusionParams5)
	}
	normalizedWeights, err := normalizeFusionInputWeights(weights)
	if err != nil {
		return err
	}
	fusion := params[paramkeys.Fusion].(map[string]interface{})
	normalizedFusion := make(map[string]interface{}, len(fusion))
	for key, value := range fusion {
		normalizedFusion[key] = value
	}
	normalizedFusion[paramkeys.InputWeights] = normalizedWeights
	params[paramkeys.Fusion] = normalizedFusion
	return nil
}

func normalizeFusionInputWeights(weights []float64) ([]float64, error) {
	out := make([]float64, 0, len(weights))
	total := 0.0
	for _, weight := range weights {
		if math.IsNaN(weight) || math.IsInf(weight, 0) {
			return nil, errors.New(errorrender.MsgNormalizeFusionInputWeights)
		}
		if weight < 0 {
			return nil, errors.New(errorrender.MsgNormalizeFusionInputWeights2)
		}
		out = append(out, weight)
		total += weight
	}
	if total <= 0 {
		return nil, errors.New(errorrender.MsgNormalizeFusionInputWeights3)
	}
	for index := range out {
		out[index] /= total
	}
	return out, nil
}

func collectInt64List(raw interface{}) []int64 {
	var values []int64
	switch v := raw.(type) {
	case []int64:
		values = append(values, v...)
	case []int:
		for _, n := range v {
			values = append(values, int64(n))
		}
	case []interface{}:
		for _, item := range v {
			switch n := item.(type) {
			case int64:
				values = append(values, n)
			case int:
				values = append(values, int64(n))
			case float64:
				if n == float64(int64(n)) {
					values = append(values, int64(n))
				}
			}
		}
	}
	return values
}

func readInt64Param(raw interface{}) (int64, bool) {
	switch value := raw.(type) {
	case int64:
		return value, value > 0
	case int:
		return int64(value), value > 0
	case float64:
		if value <= 0 || value != float64(int64(value)) {
			return 0, false
		}
		return int64(value), true
	case json.Number:
		n, err := value.Int64()
		return n, err == nil && n > 0
	default:
		return 0, false
	}
}

func readBoolParam(params map[string]interface{}, key string) (bool, bool) {
	raw, ok := params[key]
	if !ok {
		return false, false
	}
	switch v := raw.(type) {
	case bool:
		return v, true
	case string:
		normalized := strings.ToLower(strings.TrimSpace(v))
		switch normalized {
		case "true", "1", "yes":
			return true, true
		case "false", "0", "no":
			return false, true
		default:
			return false, false
		}
	default:
		return false, false
	}
}

func readFloatParam(params map[string]interface{}, key string) (float64, bool) {
	raw, ok := params[key]
	if !ok {
		return 0, false
	}
	switch v := raw.(type) {
	case float64:
		return v, true
	case float32:
		return float64(v), true
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	case json.Number:
		f, err := v.Float64()
		if err != nil {
			return 0, false
		}
		return f, true
	default:
		return 0, false
	}
}

func readIntParam(params map[string]interface{}, key string) (int, bool) {
	raw, ok := params[key]
	if !ok {
		return 0, false
	}
	return readExactIntParam(raw)
}

func readExactIntParam(raw interface{}) (int, bool) {
	switch v := raw.(type) {
	case int:
		return v, true
	case int64:
		return int(v), true
	case int32:
		return int(v), true
	case float64:
		if math.IsNaN(v) || math.IsInf(v, 0) || math.Trunc(v) != v {
			return 0, false
		}
		return int(v), true
	case float32:
		if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) || float32(math.Trunc(float64(v))) != v {
			return 0, false
		}
		return int(v), true
	case json.Number:
		n, err := v.Int64()
		if err != nil {
			return 0, false
		}
		return int(n), true
	default:
		return 0, false
	}
}
