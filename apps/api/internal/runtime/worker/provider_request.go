package worker

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"

	"ideaboard/api/internal/media/mediarefs"
	"ideaboard/api/internal/tasking/tasks"
	"ideaboard/api/internal/workflow"
)

var (
	openAIImageInputRefKeys = []string{"image_urls", "reference_image_urls"}
)

type providerRequest struct {
	Primitive   workflow.GenerationPrimitive
	Prompt      string
	Model       string
	Size        string
	OutputCount int
	ImageRefs   []string
	MaskRefs    []string
	CanvasRef   string
	OpenAI      openAIProviderControls
	Google      googleProviderControls
}

type openAIProviderControls struct {
	UseEditEndpoint      bool
	DropOriginalImageRef bool
	RemoveBackground     bool
	VariationStrength    *float64
	ExtendTop            *float64
	ExtendRight          *float64
	ExtendBottom         *float64
	ExtendLeft           *float64
	HDOutputQuality      *int
}

type googleProviderControls struct {
	AspectRatio string
}

type providerTaskParams struct {
	Canonical *tasks.WorkflowTaskParams
	Provider  map[string]interface{}
}

func newProviderTaskParams(params *tasks.WorkflowTaskParams) providerTaskParams {
	return providerTaskParams{
		Canonical: params,
		Provider:  workflow.BuildProviderTaskParams(params),
	}
}

func (p providerTaskParams) featureSlug() string {
	if p.Canonical != nil {
		if feature := strings.TrimSpace(p.Canonical.Feature); feature != "" {
			return feature
		}
	}
	if slug, err := workflow.ResolveWorkflowFeatureSlug(p.Provider); err == nil {
		return slug
	}
	return ""
}

func (p providerTaskParams) imageSize() (int, int) {
	return parseSizeFromParams(p.Provider)
}

func (p providerTaskParams) providerRequest(prompt string, resolvedModel string) providerRequest {
	width, height := p.imageSize()
	request := newProviderRequest(prompt, width, height, p.Provider, resolvedModel)
	if p.Canonical != nil && p.Canonical.Generation != nil {
		request.Primitive, _ = workflow.ParseGenerationPrimitive(p.Canonical.Generation.Primitive)
	}
	return request
}

func newProviderRequest(prompt string, width, height int, params map[string]interface{}, resolvedModel string) providerRequest {
	providerParams := workflow.BuildProviderPromptParams(params)
	featureSlug, _ := workflow.ResolveWorkflowFeatureSlug(providerParams)
	imageRefs := collectOrderedProviderRefs(providerParams, openAIImageInputRefKeys)
	maskRefs := collectOrderedProviderRefs(providerParams, []string{"mask_image_urls", "mask_image_url"})
	canvasRef := firstProviderRef(providerParams, "original_image_url")
	controls := compileProviderControls(providerParams, featureSlug)
	return providerRequest{
		Prompt:      prompt,
		Model:       resolvedModel,
		Size:        providerImageSize(width, height),
		OutputCount: readProviderOutputCount(providerParams),
		ImageRefs:   imageRefs,
		MaskRefs:    maskRefs,
		CanvasRef:   canvasRef,
		OpenAI:      controls.OpenAI,
		Google:      controls.Google,
	}
}

type compiledProviderControls struct {
	OpenAI openAIProviderControls
	Google googleProviderControls
}

func compileProviderControls(params map[string]interface{}, featureSlug string) compiledProviderControls {
	controls := compiledProviderControls{Google: googleProviderControls{AspectRatio: parseGoogleAspectRatioValue(params)}}
	switch featureSlug {
	case workflow.FeatureSlugImageExtend,
		workflow.FeatureSlugPartialEdit,
		workflow.FeatureSlugRemoveBg,
		workflow.FeatureSlugSketchToImage,
		workflow.FeatureSlugStyleVariation,
		workflow.FeatureSlugStyleFusion:
		controls.OpenAI.UseEditEndpoint = true
	}
	controls.OpenAI.DropOriginalImageRef = featureSlug == workflow.FeatureSlugStyleVariation
	controls.OpenAI.RemoveBackground = featureSlug == workflow.FeatureSlugRemoveBg
	if params == nil {
		return controls
	}
	if raw, ok := readProviderNumber(params, "variation_strength"); ok {
		controls.OpenAI.VariationStrength = float64Ptr(raw)
	}
	for key, target := range map[string]**float64{
		"extend_top":    &controls.OpenAI.ExtendTop,
		"extend_right":  &controls.OpenAI.ExtendRight,
		"extend_bottom": &controls.OpenAI.ExtendBottom,
		"extend_left":   &controls.OpenAI.ExtendLeft,
	} {
		if raw, ok := readProviderNumber(params, key); ok {
			*target = float64Ptr(raw)
		}
	}
	if featureSlug == workflow.FeatureSlugHDUpscale {
		if hd, ok := workflow.ReadHDUpscalePromptContract(params); ok {
			quality := resolveOpenAIHDUpscaleOutputQuality(hd.ClarityStrength)
			controls.OpenAI.HDOutputQuality = &quality
		}
	}
	return controls
}

func float64Ptr(value float64) *float64 { return &value }

func readProviderNumber(params map[string]interface{}, key string) (float64, bool) {
	raw, ok := workflow.ReadWorkflowParam(params, key)
	if !ok {
		return 0, false
	}
	switch value := raw.(type) {
	case float64:
		return value, !math.IsNaN(value) && !math.IsInf(value, 0)
	case float32:
		return float64(value), !math.IsNaN(float64(value)) && !math.IsInf(float64(value), 0)
	case int:
		return float64(value), true
	case int64:
		return float64(value), true
	case json.Number:
		parsed, err := value.Float64()
		return parsed, err == nil && !math.IsNaN(parsed) && !math.IsInf(parsed, 0)
	default:
		return 0, false
	}
}

func firstProviderRef(params map[string]interface{}, key string) string {
	refs := collectOrderedProviderRefs(params, []string{key})
	if len(refs) == 0 {
		return ""
	}
	return refs[0]
}

func parseGoogleAspectRatioValue(params map[string]interface{}) string {
	raw, ok := workflow.ReadWorkflowStringParam(params, "ratio")
	if !ok {
		return ""
	}
	ratio := strings.TrimSpace(raw)
	switch ratio {
	case "1:1", "16:9", "9:16", "3:4", "4:3":
		return ratio
	default:
		return ""
	}
}

func readProviderOutputCount(params map[string]interface{}) int {
	if params == nil {
		return 1
	}
	raw, ok := params["output_count"]
	if !ok {
		return 1
	}
	var count int
	switch value := raw.(type) {
	case int:
		count = value
	case int64:
		count = int(value)
	case float64:
		count = int(value)
	case json.Number:
		count, _ = strconv.Atoi(string(value))
	case string:
		count, _ = strconv.Atoi(strings.TrimSpace(value))
	}
	if count < 1 || count > 4 {
		return 0
	}
	return count
}

func (r providerRequest) withOutputCount(count int) providerRequest {
	clone := r
	clone.OutputCount = count
	return clone
}

func providerImageSize(width, height int) string {
	if width <= 0 || height <= 0 {
		return ""
	}
	return fmt.Sprintf("%dx%d", width, height)
}

func collectOrderedProviderRefs(params map[string]interface{}, keys []string) []string {
	refs := make([]string, 0, len(keys))
	seenFromEarlierKeys := make(map[string]struct{})
	for _, key := range keys {
		var values []string
		switch raw := params[key].(type) {
		case string:
			values = []string{raw}
		default:
			values = mediarefs.CollectStringList(raw)
		}
		currentKeyRefs := make([]string, 0, len(values))
		for _, raw := range values {
			if ref := strings.TrimSpace(raw); ref != "" {
				if _, duplicateAlias := seenFromEarlierKeys[ref]; !duplicateAlias {
					refs = append(refs, ref)
					currentKeyRefs = append(currentKeyRefs, ref)
				}
			}
		}
		for _, ref := range currentKeyRefs {
			seenFromEarlierKeys[ref] = struct{}{}
		}
	}
	return refs
}
