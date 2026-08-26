package taskservice

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"

	"ideaboard/api/internal/media/mediarefs"
	"ideaboard/api/internal/platform/jsonlimit"
	"ideaboard/api/internal/tasking/tasks"
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

func normalizeStyleFusionParams(params map[string]interface{}) error {
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
	weights, ok := workflow.ReadStyleFusionInputWeights(params)
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
		out[index] = out[index] / total
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
	for key, value := range params {
		if key == "model" || paramkeys.IsKnownSubmitTaskParamKey(key) {
			continue
		}
		if p.Extra == nil {
			p.Extra = map[string]interface{}{}
		}
		p.Extra[key] = value
	}
	if v, ok := workflow.ReadWorkflowStringParam(params, paramkeys.Feature); ok {
		p.Feature = v
	}
	if media := workflowMediaInputsFromMap(params, featureSlug); media != nil {
		p.Media = media
	}

	controls := &tasks.GenerationControls{}
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
	if v, ok := workflow.ReadWorkflowStringParam(params, paramkeys.Ratio); ok {
		controls.Ratio = v
		hasControls = true
	}
	if v, ok := workflow.ReadWorkflowStringParam(params, paramkeys.Quality); ok {
		controls.Quality = v
		hasControls = true
	}
	if v, ok := readIntParam(params, paramkeys.OutputCount); ok {
		controls.OutputCount = v
		hasControls = true
	}
	if v, ok := readBoolParam(params, paramkeys.PromptUpsampling); ok {
		controls.PromptUpsampling = &v
		hasControls = true
	}
	if hasControls {
		p.Controls = controls
	}

	// Feature-specific nested contracts
	switch featureSlug {
	case workflow.FeatureSlugVariations:
		if c, ok := workflow.ReadVariationsPromptContract(params); ok {
			p.Variation = &tasks.VariationsParams{Strength: c.Strength}
		}
	case workflow.FeatureSlugOutpaint:
		if c, ok := workflow.ReadOutpaintContract(params); ok {
			p.Extend = &tasks.OutpaintParams{
				TargetWidth: c.TargetWidth, TargetHeight: c.TargetHeight,
				ImageOffsetX: c.ImageOffsetX, ImageOffsetY: c.ImageOffsetY,
			}
		}
	case workflow.FeatureSlugHDUpscale:
		if c, ok := workflow.ReadHDUpscaleContract(params); ok {
			p.HD = &tasks.HDUpscaleParams{Scale: c.Scale}
		}
	case workflow.FeatureSlugStyleFusion:
		if c, ok := workflow.ReadStyleFusionPromptContract(params); ok {
			p.Fusion = &tasks.StyleFusionParams{
				Mode:         c.Mode,
				InputWeights: c.InputWeights,
			}
		}
	}

	return p, nil
}

func workflowMediaInputsFromMap(params map[string]interface{}, featureSlug string) *tasks.MediaInputs {
	media := &tasks.MediaInputs{}
	hasMedia := false

	inputWeights := []float64(nil)
	if featureSlug == workflow.FeatureSlugStyleFusion {
		if weights, ok := workflow.ReadStyleFusionInputWeights(params); ok {
			inputWeights = weights
		}
	}

	imageURLs := mediarefs.CollectStringList(params[paramkeys.ImageURLs])
	if len(imageURLs) > 0 {
		media.Inputs = mediaRefsFromURLs(imageURLs, tasks.MediaRoleInput, inputWeights)
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
	if v, ok := workflow.ReadWorkflowStringParam(params, paramkeys.OriginalImageURL); ok {
		media.Original = &tasks.MediaRef{Source: v, Role: tasks.MediaRoleOriginal}
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
