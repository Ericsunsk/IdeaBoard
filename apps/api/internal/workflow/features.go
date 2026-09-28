package workflow

import (
	"fmt"
	"math"
	"regexp"
	"strings"

	"ideaboard/api/internal/workflow/paramkeys"
)

var workflowFeatureSlugRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

const (
	FeatureSlugGeneration = "generation"
	FeatureSlugInpaint    = "inpaint"

	FeatureSlugRender     = "render"
	FeatureSlugVariations = "variations"
	FeatureSlugCutout     = "cutout"
	FeatureSlugFusion     = "fusion"
	FeatureSlugUpscale    = "upscale"
	FeatureSlugOutpaint   = "outpaint"
)

// ReservedVisionSlug is permanently reserved for vision search
// (SKU ai-search:vision, admin mapping bare key "vision").
// It must never be registered as a workflow feature slug: the gateway mapping
// storage isolates vision at ai_gateway_vision_model precisely because the API
// reuses the bare string, and a workflow "vision" would reintroduce the
// ai-image:vision / ai-search:vision overwrite. Locked by test.
const ReservedVisionSlug = "vision"

var supportedWorkflowFeatureSlugs = registeredImageFeatureSlugs()
var supportedTaskFeatureSlugs = supportedFeatureSlugSet(supportedWorkflowFeatureSlugs)
var aiImageWorkflowFeatures = supportedFeatureSlugSet(supportedWorkflowFeatureSlugs)

// AIImageFeatureSlugs is the canonical product registry for workflows that
// invoke an external AI image provider. Local editor operations never enter
// this registry and therefore cannot acquire Task Billing facts.
func AIImageFeatureSlugs() []string {
	slugs := make([]string, len(supportedWorkflowFeatureSlugs))
	copy(slugs, supportedWorkflowFeatureSlugs)
	return slugs
}

func IsAIImageFeature(raw string) bool {
	_, ok := ParseAIImageFeatureSlug(raw)
	return ok
}

func ParseAIImageFeatureSlug(raw string) (string, bool) {
	slug, ok := ParseWorkflowFeatureSlug(raw)
	if !ok {
		return "", false
	}
	_, ok = aiImageWorkflowFeatures[slug]
	if !ok {
		return "", false
	}
	return slug, true
}

func SupportedFeatureSlugs() []string {
	slugs := make([]string, len(supportedWorkflowFeatureSlugs))
	copy(slugs, supportedWorkflowFeatureSlugs)
	return slugs
}

func supportedFeatureSlugSet(slugs []string) map[string]struct{} {
	result := make(map[string]struct{}, len(slugs))
	for _, slug := range slugs {
		result[slug] = struct{}{}
	}
	return result
}

func normalizeWorkflowFeatureSlug(raw string) string {
	return strings.TrimSpace(raw)
}

func ParseWorkflowFeatureSlug(raw string) (string, bool) {
	slug := normalizeWorkflowFeatureSlug(raw)
	if slug == "" || !workflowFeatureSlugRe.MatchString(slug) {
		return "", false
	}
	if _, ok := supportedTaskFeatureSlugs[slug]; !ok {
		return "", false
	}
	return slug, true
}

func ValidateWorkflowFeatureSlug(raw string) (string, error) {
	slug, ok := ParseWorkflowFeatureSlug(raw)
	if !ok {
		return "", fmt.Errorf(msgValidateWorkflowFeatureSlug, raw)
	}
	return slug, nil
}

func ReadWorkflowParam(params map[string]interface{}, canonicalKey string) (interface{}, bool) {
	if params == nil {
		return nil, false
	}
	v, ok := params[canonicalKey]
	return v, ok
}

func ReadWorkflowStringParam(params map[string]interface{}, canonicalKey string) (string, bool) {
	v, ok := ReadWorkflowParam(params, canonicalKey)
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	if !ok {
		return "", false
	}
	s = strings.TrimSpace(s)
	if s == "" {
		return "", false
	}
	return s, true
}

func ReadFusionInputWeights(params map[string]interface{}) ([]float64, bool) {
	rawFusion, ok := ReadWorkflowParam(params, paramkeys.Fusion)
	if !ok {
		return nil, false
	}
	fusion, ok := rawFusion.(map[string]interface{})
	if !ok {
		return nil, false
	}
	rawWeights, ok := fusion[paramkeys.InputWeights]
	if !ok {
		return nil, false
	}
	weights, ok := readNumberList(rawWeights)
	if !ok || len(weights) == 0 {
		return nil, false
	}
	return weights, true
}

func readNumberList(raw interface{}) ([]float64, bool) {
	switch values := raw.(type) {
	case []float64:
		return copyFiniteNumbers(values)
	case []interface{}:
		weights := make([]float64, 0, len(values))
		for _, value := range values {
			weight, ok := readFiniteNumber(value)
			if !ok {
				return nil, false
			}
			weights = append(weights, weight)
		}
		return weights, true
	default:
		return nil, false
	}
}

func copyFiniteNumbers(values []float64) ([]float64, bool) {
	weights := make([]float64, 0, len(values))
	for _, value := range values {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return nil, false
		}
		weights = append(weights, value)
	}
	return weights, true
}

func readFiniteNumber(value interface{}) (float64, bool) {
	switch typed := value.(type) {
	case float64:
		return typed, !math.IsNaN(typed) && !math.IsInf(typed, 0)
	case float32:
		value := float64(typed)
		return value, !math.IsNaN(value) && !math.IsInf(value, 0)
	case int:
		return float64(typed), true
	case int8:
		return float64(typed), true
	case int16:
		return float64(typed), true
	case int32:
		return float64(typed), true
	case int64:
		return float64(typed), true
	case uint:
		return float64(typed), true
	case uint8:
		return float64(typed), true
	case uint16:
		return float64(typed), true
	case uint32:
		return float64(typed), true
	case uint64:
		return float64(typed), true
	default:
		return 0, false
	}
}

func ResolveWorkflowFeatureSlug(params map[string]interface{}) (string, error) {
	rawFeature, ok := ReadWorkflowStringParam(params, paramkeys.Feature)
	if !ok {
		return "", nil
	}
	slug, err := ValidateWorkflowFeatureSlug(rawFeature)
	if err != nil {
		return "", err
	}
	return slug, nil
}
