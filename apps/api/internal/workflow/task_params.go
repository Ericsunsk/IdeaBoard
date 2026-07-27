package workflow

import (
	"strings"

	"ideaboard/api/internal/tasking/tasks"
	"ideaboard/api/internal/workflow/paramkeys"
)

type ProviderParamsEncoder interface {
	EncodeTaskParams(params *tasks.WorkflowTaskParams) map[string]interface{}
	EncodePromptParams(params map[string]interface{}) map[string]interface{}
}

type providerParamsEncoder struct{}

var defaultProviderParamsEncoder ProviderParamsEncoder = providerParamsEncoder{}

func (providerParamsEncoder) EncodeTaskParams(params *tasks.WorkflowTaskParams) map[string]interface{} {
	return buildProviderTaskParams(params)
}

func (providerParamsEncoder) EncodePromptParams(params map[string]interface{}) map[string]interface{} {
	return buildProviderPromptParams(params)
}

// BuildProviderTaskParams adapts the canonical task params contract to the
// provider request map expected by current integrations.
func BuildProviderTaskParams(params *tasks.WorkflowTaskParams) map[string]interface{} {
	return defaultProviderParamsEncoder.EncodeTaskParams(params)
}

func buildProviderTaskParams(params *tasks.WorkflowTaskParams) map[string]interface{} {
	if params == nil {
		return nil
	}
	out := make(map[string]interface{})
	for key, value := range params.Extra {
		if paramkeys.IsProviderReservedKey(key) {
			continue
		}
		out[key] = value
	}
	if text := strings.TrimSpace(params.Feature); text != "" {
		out[paramkeys.Feature] = text
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
		out[paramkeys.ImageURLs] = urls
	}
	if urls := mediaRefURLs(media.References); len(urls) > 0 {
		out[paramkeys.ReferenceImageURLs] = urls
	}
	if media.Original != nil {
		if url := strings.TrimSpace(media.Original.URL); url != "" {
			out[paramkeys.OriginalImageURL] = url
		}
	}
	if media.Mask != nil {
		if url := strings.TrimSpace(media.Mask.URL); url != "" {
			out[paramkeys.MaskImageURL] = url
		}
	}
	if media.UseMask != nil {
		out[paramkeys.UseMask] = *media.UseMask
	}
}

func flattenWorkflowControls(out map[string]interface{}, controls *tasks.GenerationControls) {
	if controls == nil {
		return
	}
	setPositiveFloatParam(out, paramkeys.CustomWidth, controls.CustomWidth)
	setPositiveFloatParam(out, paramkeys.CustomHeight, controls.CustomHeight)
	setPositiveFloatParam(out, paramkeys.OriginalImageWidth, controls.OriginalWidth)
	setPositiveFloatParam(out, paramkeys.OriginalImageHeight, controls.OriginalHeight)
	if text := strings.TrimSpace(controls.Ratio); text != "" {
		out[paramkeys.Ratio] = text
	}
	if text := strings.TrimSpace(controls.Quality); text != "" {
		out[paramkeys.Quality] = text
	}
	setPositiveIntParam(out, paramkeys.SampleCount, controls.SampleCount)
	setPositiveIntParam(out, paramkeys.OutputCount, controls.OutputCount)
	setPositiveIntParam(out, paramkeys.ImageCount, controls.ImageCount)
	if controls.PromptUpsampling != nil {
		out[paramkeys.PromptUpsampling] = *controls.PromptUpsampling
	}
}

func flattenWorkflowFeatureParams(out map[string]interface{}, params *tasks.WorkflowTaskParams) {
	if params.Variation != nil {
		out[ParamKeyVariation] = map[string]interface{}{
			paramkeys.Strength: params.Variation.Strength,
		}
		out[paramkeys.VariationStrength] = params.Variation.Strength
	}
	if params.Extend != nil {
		out[ParamKeyExtend] = map[string]interface{}{
			paramkeys.TopPercent:    params.Extend.TopPercent,
			paramkeys.RightPercent:  params.Extend.RightPercent,
			paramkeys.BottomPercent: params.Extend.BottomPercent,
			paramkeys.LeftPercent:   params.Extend.LeftPercent,
		}
		out[paramkeys.ExtendTop] = params.Extend.TopPercent
		out[paramkeys.ExtendRight] = params.Extend.RightPercent
		out[paramkeys.ExtendBottom] = params.Extend.BottomPercent
		out[paramkeys.ExtendLeft] = params.Extend.LeftPercent
	}
	if params.HD != nil {
		out[ParamKeyHD] = map[string]interface{}{
			paramkeys.ClarityStrength: params.HD.ClarityStrength,
		}
	}
	if params.Fusion != nil {
		fusion := map[string]interface{}{}
		if text := strings.TrimSpace(params.Fusion.Mode); text != "" {
			fusion[paramkeys.Mode] = text
		}
		if len(params.Fusion.InputWeights) > 0 {
			weights := append([]float64(nil), params.Fusion.InputWeights...)
			fusion[paramkeys.InputWeights] = weights
			out[paramkeys.FusionInputWeights] = weights
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
