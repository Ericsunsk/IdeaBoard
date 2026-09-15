package task

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"

	"ideaboard/api/internal/media/mediarefs"
	"ideaboard/api/internal/platform/jsonlimit"
	tasks "ideaboard/api/internal/tasking/domain"
	"ideaboard/api/internal/workflow"
	"ideaboard/api/internal/workflow/paramkeys"
)

var unsupportedSubmitTaskParamKeys = []string{
	paramkeys.ImageURL,
	paramkeys.MaskImageURLs,
	paramkeys.VariationStrength,
	paramkeys.FusionInputWeights,
	paramkeys.UpscaleMode,
	paramkeys.UpscaleFactor,
	paramkeys.OutputFormat,
	paramkeys.OutputQuality,
	paramkeys.SampleCount,
	paramkeys.ImageCount,
}

func normalizeSubmitTaskMediaParams(params map[string]interface{}) {
	if params == nil {
		return
	}
	imageURLs := mediarefs.CollectStringList(params[paramkeys.ImageURLs])
	if len(imageURLs) > 0 {
		params[paramkeys.ImageURLs] = imageURLs
	} else {
		delete(params, paramkeys.ImageURLs)
	}
	referenceImageURLs := mediarefs.CollectStringList(params[paramkeys.ReferenceImageURLs])
	if len(referenceImageURLs) > 0 {
		params[paramkeys.ReferenceImageURLs] = referenceImageURLs
	} else {
		delete(params, paramkeys.ReferenceImageURLs)
	}
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

func mediaRefsFromAssetIDs(assetIDs []int64, role string) []tasks.MediaRef {
	refs := make([]tasks.MediaRef, 0, len(assetIDs))
	for _, id := range assetIDs {
		refs = append(refs, tasks.MediaRef{AssetID: id, Role: role})
	}
	return refs
}

func normalizeFusionParams(params map[string]interface{}) error {
	if params == nil {
		return nil
	}
	imageURLs := mediarefs.CollectStringList(params[paramkeys.ImageURLs])
	if len(imageURLs) < 2 || len(imageURLs) > 4 {
		return fmt.Errorf("fusion 节点需要 2 到 4 张输入图")
	}
	rawFusion, ok := params[paramkeys.Fusion]
	if !ok {
		return fmt.Errorf("fusion 参数缺少 fusion.input_weights")
	}
	fusion, ok := rawFusion.(map[string]interface{})
	if !ok {
		return fmt.Errorf("fusion 参数格式错误")
	}
	if _, ok := fusion[paramkeys.InputWeights]; !ok {
		return fmt.Errorf("fusion 参数缺少 fusion.input_weights")
	}
	weights, ok := workflow.ReadFusionInputWeights(params)
	if !ok {
		return fmt.Errorf("fusion 权重必须是数字数组")
	}
	if len(weights) != len(imageURLs) {
		return fmt.Errorf("fusion 权重数量必须与输入图数量一致")
	}
	normalizedWeights, err := normalizeFusionInputWeights(weights)
	if err != nil {
		return err
	}
	normalizedFusion := make(map[string]interface{}, len(fusion)+2)
	for key, value := range fusion {
		normalizedFusion[key] = value
	}
	normalizedFusion[paramkeys.Mode] = "weighted_reference"
	normalizedFusion[paramkeys.InputWeights] = normalizedWeights
	params[paramkeys.Fusion] = normalizedFusion
	return nil
}

func normalizeFusionInputWeights(weights []float64) ([]float64, error) {
	out := make([]float64, 0, len(weights))
	total := 0.0
	for _, weight := range weights {
		if math.IsNaN(weight) || math.IsInf(weight, 0) {
			return nil, fmt.Errorf("fusion 权重必须是有限数字")
		}
		if weight < 0 {
			return nil, fmt.Errorf("fusion 权重不能为负数")
		}
		out = append(out, weight)
		total += weight
	}
	if total <= 0 {
		return nil, fmt.Errorf("fusion 权重总和必须大于 0")
	}
	for index := range out {
		out[index] /= total
	}
	return out, nil
}

func validateSubmitTaskPrompt(prompt string, maxBytes int, label string) error {
	if len([]byte(prompt)) > maxBytes {
		return fmt.Errorf("%s长度不能超过 %dKB", label, maxBytes/1024)
	}
	return nil
}

func validateSubmitTaskImageURLsWithRefValidator(imageURLs []string, validateRef func(string) error) error {
	if len(imageURLs) > maxSubmitTaskImageURLs {
		return fmt.Errorf("图片数量不能超过 %d 张", maxSubmitTaskImageURLs)
	}
	for _, raw := range imageURLs {
		url := strings.TrimSpace(raw)
		if url == "" {
			return fmt.Errorf("图片地址不能为空")
		}
		if len([]byte(url)) > maxSubmitTaskImageURLBytes {
			return fmt.Errorf("图片地址长度不能超过 %d 字符", maxSubmitTaskImageURLBytes)
		}
		if err := validateRef(url); err != nil {
			return err
		}
	}
	return nil
}

func validateSubmitTaskMediaParamsWithRefValidator(params map[string]interface{}, validateRef func(string) error) error {
	if params == nil {
		return nil
	}
	if err := validateSubmitTaskContractKeys(params); err != nil {
		return err
	}
	for _, key := range mediarefs.CanonicalListKeys() {
		values := mediarefs.CollectStringList(params[key])
		if len(values) > maxSubmitTaskImageURLs {
			return fmt.Errorf("%s 数量不能超过 %d 张", key, maxSubmitTaskImageURLs)
		}
	}
	return mediarefs.ForEachParamMediaRef(params, func(key string, ref string) error {
		if err := validateRef(ref); err != nil {
			return fmt.Errorf("%s 无效: %w", key, err)
		}
		return nil
	})
}

func validateSubmitTaskContractKeys(params map[string]interface{}) error {
	for _, key := range unsupportedSubmitTaskParamKeys {
		if _, ok := params[key]; ok {
			return fmt.Errorf("任务参数必须使用结构化字段")
		}
	}
	return nil
}

func marshalWorkflowTaskParams(params *tasks.WorkflowTaskParams, source map[string]interface{}) (string, error) {
	if params == nil {
		return "", nil
	}
	if source != nil && jsonlimit.ValueDepth(source) > maxSubmitTaskParamsDepth {
		return "", fmt.Errorf("任务参数层级过深")
	}
	payload, err := json.Marshal(params)
	if err != nil {
		return "", fmt.Errorf("任务参数格式错误")
	}
	if len(payload) > maxSubmitTaskParamsBytes {
		return "", fmt.Errorf("任务参数不能超过 %dMB", maxSubmitTaskParamsBytes/1024/1024)
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
	if v, ok := workflow.ReadWorkflowStringParam(params, paramkeys.Feature); ok {
		p.Feature = v
	}
	p.Media = workflowMediaInputsFromMap(params, featureSlug)
	p.Controls = generationControlsFromMap(params)
	applyFeatureSpecificTaskParams(p, params, featureSlug)
	return p, nil
}

func workflowExtraParamsFromMap(params map[string]interface{}) map[string]interface{} {
	var extra map[string]interface{}
	for key, value := range params {
		if key == "model" || paramkeys.IsKnownSubmitTaskParamKey(key) {
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
	strings, hasStrings := generationStringControlsFromMap(params)
	if hasStrings {
		controls.Ratio = strings.Ratio
		controls.Quality = strings.Quality
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
