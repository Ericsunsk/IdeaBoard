package workflow

import (
	"fmt"
	"math"
	"regexp"
	"strings"
)

var workflowFeatureSlugRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

const (
	FeatureSlugSketchToImage  = "sketch-to-image"
	FeatureSlugStyleVariation = "style-variation"
	FeatureSlugPartialEdit    = "partial-edit"
	FeatureSlugRemoveBg       = "remove-bg"
	FeatureSlugStyleFusion    = "style-fusion"
	FeatureSlugPatternGen     = "pattern-gen"
	FeatureSlugHDUpscale      = "hd-upscale"
	FeatureSlugImageExtend    = "image-extend"
)

var supportedWorkflowFeatureSlugs = []string{
	FeatureSlugSketchToImage,
	FeatureSlugStyleVariation,
	FeatureSlugPartialEdit,
	FeatureSlugRemoveBg,
	FeatureSlugStyleFusion,
	FeatureSlugPatternGen,
	FeatureSlugHDUpscale,
	FeatureSlugImageExtend,
}

var supportedTaskFeatureSlugs = supportedFeatureSlugSet(supportedWorkflowFeatureSlugs)

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
		return "", fmt.Errorf("feature %q 不支持", raw)
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

func ReadStyleFusionInputWeights(params map[string]interface{}) ([]float64, bool) {
	rawFusion, ok := ReadWorkflowParam(params, "fusion")
	if !ok {
		return nil, false
	}
	fusion, ok := rawFusion.(map[string]interface{})
	if !ok {
		return nil, false
	}
	rawWeights, ok := fusion["input_weights"]
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
	rawFeature, ok := ReadWorkflowStringParam(params, "feature")
	if !ok {
		return "", nil
	}
	slug, err := ValidateWorkflowFeatureSlug(rawFeature)
	if err != nil {
		return "", err
	}
	return slug, nil
}
