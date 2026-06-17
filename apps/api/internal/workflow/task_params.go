package workflow

import (
	"strings"

	"ideaboard/api/internal/tasks"
)

// FlattenWorkflowTaskParams adapts the canonical task params contract to the
// flat provider-facing map expected by current provider integrations.
func FlattenWorkflowTaskParams(params *tasks.WorkflowTaskParams) map[string]interface{} {
	if params == nil {
		return nil
	}
	out := make(map[string]interface{})
	for key, value := range params.Extra {
		if isReservedProviderParamKey(key) {
			continue
		}
		out[key] = value
	}
	if text := strings.TrimSpace(params.Feature); text != "" {
		out["feature"] = text
	}
	if text := strings.TrimSpace(params.Model); text != "" {
		out["model"] = text
	}
	flattenWorkflowMediaInputs(out, params.Media)
	flattenWorkflowControls(out, params.Controls)
	flattenWorkflowFeatureParams(out, params)
	return out
}

func flattenWorkflowMediaInputs(out map[string]interface{}, media *tasks.MediaInputs) {
	if media == nil {
		return
	}
	if urls := mediaRefURLs(media.Inputs); len(urls) > 0 {
		out["image_urls"] = urls
	}
	if urls := mediaRefURLs(media.References); len(urls) > 0 {
		out["reference_image_urls"] = urls
	}
	if media.Original != nil {
		if url := strings.TrimSpace(media.Original.URL); url != "" {
			out["original_image_url"] = url
		}
	}
	if media.Mask != nil {
		if url := strings.TrimSpace(media.Mask.URL); url != "" {
			out["mask_image_url"] = url
		}
	}
	if media.UseMask != nil {
		out["use_mask"] = *media.UseMask
	}
}

func flattenWorkflowControls(out map[string]interface{}, controls *tasks.GenerationControls) {
	if controls == nil {
		return
	}
	setPositiveFloatParam(out, "custom_width", controls.CustomWidth)
	setPositiveFloatParam(out, "custom_height", controls.CustomHeight)
	setPositiveFloatParam(out, "original_image_width", controls.OriginalWidth)
	setPositiveFloatParam(out, "original_image_height", controls.OriginalHeight)
	if text := strings.TrimSpace(controls.Ratio); text != "" {
		out["ratio"] = text
	}
	if text := strings.TrimSpace(controls.Quality); text != "" {
		out["quality"] = text
	}
	setPositiveIntParam(out, "sample_count", controls.SampleCount)
	setPositiveIntParam(out, "output_count", controls.OutputCount)
	setPositiveIntParam(out, "image_count", controls.ImageCount)
	if controls.PromptUpsampling != nil {
		out["prompt_upsampling"] = *controls.PromptUpsampling
	}
}

func flattenWorkflowFeatureParams(out map[string]interface{}, params *tasks.WorkflowTaskParams) {
	if params.Variation != nil {
		out[ParamKeyVariation] = map[string]interface{}{
			"strength": params.Variation.Strength,
		}
		out["variation_strength"] = params.Variation.Strength
	}
	if params.Extend != nil {
		out[ParamKeyExtend] = map[string]interface{}{
			"top_percent":    params.Extend.TopPercent,
			"right_percent":  params.Extend.RightPercent,
			"bottom_percent": params.Extend.BottomPercent,
			"left_percent":   params.Extend.LeftPercent,
		}
		out["extend_top"] = params.Extend.TopPercent
		out["extend_right"] = params.Extend.RightPercent
		out["extend_bottom"] = params.Extend.BottomPercent
		out["extend_left"] = params.Extend.LeftPercent
	}
	if params.HD != nil {
		out[ParamKeyHD] = map[string]interface{}{
			"clarity_strength": params.HD.ClarityStrength,
		}
	}
	if params.Fusion != nil {
		fusion := map[string]interface{}{}
		if text := strings.TrimSpace(params.Fusion.Mode); text != "" {
			fusion["mode"] = text
		}
		if len(params.Fusion.InputWeights) > 0 {
			weights := append([]float64(nil), params.Fusion.InputWeights...)
			fusion["input_weights"] = weights
			out["fusion_input_weights"] = weights
		}
		if len(fusion) > 0 {
			out[ParamKeyFusion] = fusion
		}
	}
}

func mediaRefURLs(refs []tasks.MediaRef) []string {
	if len(refs) == 0 {
		return nil
	}
	urls := make([]string, 0, len(refs))
	for _, ref := range refs {
		if url := strings.TrimSpace(ref.URL); url != "" {
			urls = append(urls, url)
		}
	}
	return urls
}

func setPositiveFloatParam(out map[string]interface{}, key string, value float64) {
	if value > 0 {
		out[key] = value
	}
}

func setPositiveIntParam(out map[string]interface{}, key string, value int) {
	if value > 0 {
		out[key] = value
	}
}

func isReservedProviderParamKey(key string) bool {
	switch key {
	case "schema_version",
		"feature",
		"model",
		"media",
		"controls",
		ParamKeyVariation,
		ParamKeyExtend,
		ParamKeyHD,
		ParamKeyFusion,
		"image_url",
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
		"variation_strength",
		"extend_top",
		"extend_right",
		"extend_bottom",
		"extend_left",
		"fusion_input_weights",
		"output_quality",
		"upscale_mode",
		"upscale_factor":
		return true
	default:
		return false
	}
}
