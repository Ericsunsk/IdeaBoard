package taskservice

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"

	"ideaboard/api/internal/platform/jsonlimit"
	"ideaboard/api/internal/platform/mediarefs"
	"ideaboard/api/internal/taskmedia"
	"ideaboard/api/internal/tasks"
	"ideaboard/api/internal/workflow"
)

var unsupportedSubmitTaskParamKeys = []string{
	"image_url",
	"mask_image_urls",
	"variation_strength",
	"extend_top",
	"extend_right",
	"extend_bottom",
	"extend_left",
	"clarity_strength",
	"fusion_input_weights",
	"upscale_mode",
	"upscale_factor",
	"output_format",
	"output_quality",
}

func normalizeSubmitTaskMediaParams(params map[string]interface{}) {
	if params == nil {
		return
	}
	imageURLs := mediarefs.CollectStringList(params["image_urls"])
	if len(imageURLs) > 0 {
		params["image_urls"] = imageURLs
	} else {
		delete(params, "image_urls")
	}
	referenceImageURLs := mediarefs.CollectStringList(params["reference_image_urls"])
	if len(referenceImageURLs) > 0 {
		params["reference_image_urls"] = referenceImageURLs
	} else {
		delete(params, "reference_image_urls")
	}
}

func normalizeStyleFusionParams(params map[string]interface{}) error {
	if params == nil {
		return nil
	}
	imageURLs := mediarefs.CollectStringList(params["image_urls"])
	if len(imageURLs) < 2 || len(imageURLs) > 4 {
		return fmt.Errorf("fusion 节点需要 2 到 4 张输入图")
	}
	rawFusion, ok := params["fusion"]
	if !ok {
		return fmt.Errorf("fusion 参数缺少 fusion.input_weights")
	}
	fusion, ok := rawFusion.(map[string]interface{})
	if !ok {
		return fmt.Errorf("fusion 参数格式错误")
	}
	if _, ok := fusion["input_weights"]; !ok {
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
	normalizedFusion["mode"] = "weighted_reference"
	normalizedFusion["input_weights"] = normalizedWeights
	params["fusion"] = normalizedFusion
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

func validateSubmitTaskImageURLs(imageURLs []string) error {
	return validateSubmitTaskImageURLsWithRefValidator(imageURLs, validateSubmitTaskMediaRef)
}

func (s *Service) validateSubmitTaskImageURLs(ctx context.Context, userID int64, imageURLs []string) error {
	return validateSubmitTaskImageURLsWithRefValidator(imageURLs, func(raw string) error {
		return s.validateSubmitTaskMediaRef(ctx, userID, raw)
	})
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

func validateSubmitTaskMediaParams(params map[string]interface{}) error {
	return validateSubmitTaskMediaParamsWithRefValidator(params, validateSubmitTaskMediaRef)
}

func (s *Service) validateSubmitTaskMediaParams(ctx context.Context, userID int64, params map[string]interface{}) error {
	return validateSubmitTaskMediaParamsWithRefValidator(params, func(raw string) error {
		return s.validateSubmitTaskMediaRef(ctx, userID, raw)
	})
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

func validateSubmitTaskMediaRef(raw string) error {
	return taskmedia.ValidateSubmitMediaRef(raw)
}

func (s *Service) validateSubmitTaskMediaRef(ctx context.Context, userID int64, raw string) error {
	if s == nil || s.mediaValidator == nil {
		return fmt.Errorf("任务媒体校验服务未配置")
	}
	return s.mediaValidator.ValidateSubmitMediaRef(ctx, userID, raw)
}

func marshalSubmitTaskParams(params map[string]interface{}) (string, error) {
	if params == nil {
		return "", nil
	}
	if jsonlimit.ValueDepth(params) > maxSubmitTaskParamsDepth {
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
		if isWorkflowTaskParamsKnownMapKey(key) {
			continue
		}
		if p.Extra == nil {
			p.Extra = map[string]interface{}{}
		}
		p.Extra[key] = value
	}
	if v, ok := workflow.ReadWorkflowStringParam(params, "feature"); ok {
		p.Feature = v
	}
	if v, ok := workflow.ReadWorkflowStringParam(params, "model"); ok {
		p.Model = strings.TrimSpace(v)
	}

	if media := workflowMediaInputsFromMap(params, featureSlug); media != nil {
		p.Media = media
	}

	controls := &tasks.GenerationControls{}
	hasControls := false
	if v, ok := readFloatParam(params, "custom_width"); ok {
		controls.CustomWidth = v
		hasControls = true
	}
	if v, ok := readFloatParam(params, "custom_height"); ok {
		controls.CustomHeight = v
		hasControls = true
	}
	if v, ok := readFloatParam(params, "original_image_width"); ok {
		controls.OriginalWidth = v
		hasControls = true
	}
	if v, ok := readFloatParam(params, "original_image_height"); ok {
		controls.OriginalHeight = v
		hasControls = true
	}
	if v, ok := workflow.ReadWorkflowStringParam(params, "ratio"); ok {
		controls.Ratio = v
		hasControls = true
	}
	if v, ok := workflow.ReadWorkflowStringParam(params, "quality"); ok {
		controls.Quality = v
		hasControls = true
	}
	if v, ok := readIntParam(params, "sample_count"); ok {
		controls.SampleCount = v
		hasControls = true
	}
	if v, ok := readIntParam(params, "output_count"); ok {
		controls.OutputCount = v
		hasControls = true
	}
	if v, ok := readIntParam(params, "image_count"); ok {
		controls.ImageCount = v
		hasControls = true
	}
	if v, ok := readBoolParam(params, "prompt_upsampling"); ok {
		controls.PromptUpsampling = &v
		hasControls = true
	}
	if hasControls {
		p.Controls = controls
	}

	// Feature-specific nested contracts
	switch featureSlug {
	case workflow.FeatureSlugStyleVariation:
		if c, ok := workflow.ReadStyleVariationPromptContract(params); ok {
			p.Variation = &tasks.StyleVariationParams{Strength: c.Strength}
		}
	case workflow.FeatureSlugImageExtend:
		if c, ok := workflow.ReadImageExtendPromptContract(params); ok {
			p.Extend = &tasks.ImageExtendParams{
				TopPercent: c.TopPercent, RightPercent: c.RightPercent,
				BottomPercent: c.BottomPercent, LeftPercent: c.LeftPercent,
			}
		}
	case workflow.FeatureSlugHDUpscale:
		if c, ok := workflow.ReadHDUpscalePromptContract(params); ok {
			p.HD = &tasks.HDUpscaleParams{ClarityStrength: c.ClarityStrength}
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

	imageURLs := mediarefs.CollectStringList(params["image_urls"])
	if len(imageURLs) > 0 {
		media.Inputs = mediaRefsFromURLs(imageURLs, "input", inputWeights)
		hasMedia = true
	}
	referenceImageURLs := mediarefs.CollectStringList(params["reference_image_urls"])
	if len(referenceImageURLs) > 0 {
		media.References = mediaRefsFromURLs(referenceImageURLs, "reference", nil)
		hasMedia = true
	}
	if v, ok := workflow.ReadWorkflowStringParam(params, "mask_image_url"); ok {
		media.Mask = &tasks.MediaRef{URL: v, Role: "mask"}
		hasMedia = true
	}
	if v, ok := workflow.ReadWorkflowStringParam(params, "original_image_url"); ok {
		media.Original = &tasks.MediaRef{URL: v, Role: "original"}
		hasMedia = true
	}
	if v, ok := readBoolParam(params, "use_mask"); ok {
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
			URL:  strings.TrimSpace(raw),
			Role: role,
		}
		if index < len(weights) {
			ref.Weight = weights[index]
		}
		refs = append(refs, ref)
	}
	return refs
}

func isWorkflowTaskParamsKnownMapKey(key string) bool {
	switch key {
	case "feature",
		"model",
		"image_urls",
		"reference_image_urls",
		"mask_image_url",
		"mask_image_urls",
		"original_image_url",
		"use_mask",
		"custom_width",
		"custom_height",
		"original_image_width",
		"original_image_height",
		"ratio",
		"quality",
		"sample_count",
		"output_count",
		"image_count",
		"prompt_upsampling",
		"variation",
		"extend",
		"hd",
		"fusion":
		return true
	default:
		return false
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
	switch v := raw.(type) {
	case int:
		return v, true
	case int64:
		return int(v), true
	case int32:
		return int(v), true
	case float64:
		return int(v), true
	case float32:
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
