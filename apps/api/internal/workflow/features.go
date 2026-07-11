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
	FeatureSlugSketchToImage  = "sketch-to-image"
	FeatureSlugStyleVariation = "style-variation"
	FeatureSlugPartialEdit    = "partial-edit"
	FeatureSlugRemoveBg       = "remove-bg"
	FeatureSlugStyleFusion    = "style-fusion"
	FeatureSlugPatternGen     = "pattern-gen"
	FeatureSlugHDUpscale      = "hd-upscale"
	FeatureSlugImageExtend    = "image-extend"
)

type featureDefinition struct {
	Slug           string
	UsesAIProvider bool
}

var workflowFeatureDefinitions = []featureDefinition{
	{Slug: FeatureSlugSketchToImage, UsesAIProvider: true},
	{Slug: FeatureSlugStyleVariation, UsesAIProvider: true},
	{Slug: FeatureSlugPartialEdit, UsesAIProvider: true},
	{Slug: FeatureSlugRemoveBg, UsesAIProvider: true},
	{Slug: FeatureSlugStyleFusion, UsesAIProvider: true},
	{Slug: FeatureSlugPatternGen, UsesAIProvider: true},
	{Slug: FeatureSlugHDUpscale, UsesAIProvider: true},
	{Slug: FeatureSlugImageExtend, UsesAIProvider: true},
}

var supportedWorkflowFeatureSlugs = featureSlugs(workflowFeatureDefinitions, func(featureDefinition) bool { return true })
var supportedTaskFeatureSlugs = supportedFeatureSlugSet(supportedWorkflowFeatureSlugs)
var aiImageWorkflowFeatureSlugs = featureSlugs(workflowFeatureDefinitions, func(definition featureDefinition) bool {
	return definition.UsesAIProvider
})
var aiImageWorkflowFeatures = supportedFeatureSlugSet(aiImageWorkflowFeatureSlugs)

func featureSlugs(definitions []featureDefinition, include func(featureDefinition) bool) []string {
	slugs := make([]string, 0, len(definitions))
	for _, definition := range definitions {
		if include(definition) {
			slugs = append(slugs, definition.Slug)
		}
	}
	return slugs
}

// AIImageFeatureSlugs is the canonical product registry for workflows that
// invoke an external AI image provider. Local editor operations never enter
// this registry and therefore cannot acquire Task Billing facts.
func AIImageFeatureSlugs() []string {
	slugs := make([]string, len(aiImageWorkflowFeatureSlugs))
	copy(slugs, aiImageWorkflowFeatureSlugs)
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

// NormalizeModelKey is the canonical model identity shared by account routing
// and billing. Provider-specific path aliases are not collapsed here.
func NormalizeModelKey(model string) string {
	return strings.ToLower(strings.TrimSpace(model))
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
