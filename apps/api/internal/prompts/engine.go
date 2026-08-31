package prompts

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"ideaboard/api/internal/workflow"
	"ideaboard/api/internal/workflow/paramkeys"
)

type promptParamReader func(map[string]interface{}, string) (interface{}, bool)
type promptParamFormatter func(interface{}) (string, bool)

type promptTemplateBinding struct {
	placeholder string
	paramKey    string
	reader      promptParamReader
	formatter   promptParamFormatter
}

type FeaturePromptMergePolicy struct {
	AppendUserPromptWhenTemplateMissesPlaceholder bool `json:"append_user_prompt_when_template_misses_placeholder"`
	AppendBasePromptWithoutTemplate               bool `json:"append_base_prompt_without_template"`
}

type featurePromptVariableProvider func(map[string]interface{}) map[string]interface{}

var featurePromptVariableProviders = map[string]featurePromptVariableProvider{
	workflow.FeatureSlugVariations: variationsPromptVariables,
}

var featurePromptBindings = map[string][]promptTemplateBinding{
	workflow.FeatureSlugFusion: {
		{
			placeholder: paramkeys.PromptPlaceholderFusionInputWeights,
			paramKey:    paramkeys.PromptPathFusionInputWeights,
			reader:      readFusionInputWeightsPromptParam,
			formatter:   formatImageWeightsTemplateValue,
		},
	},
}

var defaultPromptMergePolicy = FeaturePromptMergePolicy{
	AppendUserPromptWhenTemplateMissesPlaceholder: true,
	AppendBasePromptWithoutTemplate:               true,
}

// Image nodes always have a media input as their semantic source of truth.
// Their optional text must be requested explicitly by each System Prompt
// instead of being appended implicitly when an operator omits the placeholder.
var imageNodeFeatureSlugs = map[string]struct{}{
	workflow.FeatureSlugInpaint:    {},
	workflow.FeatureSlugRender:     {},
	workflow.FeatureSlugVariations: {},
	workflow.FeatureSlugCutout:     {},
	workflow.FeatureSlugOutpaint:   {},
}

func DefaultFeaturePromptMergePolicy(featureSlug string) FeaturePromptMergePolicy {
	if _, ok := imageNodeFeatureSlugs[featureSlug]; ok {
		return FeaturePromptMergePolicy{}
	}
	return defaultPromptMergePolicy
}

func applyPromptTemplateBindings(
	varMap map[string]string,
	featureSlug string,
	params map[string]interface{},
) {
	if params == nil {
		return
	}
	if provider := featurePromptVariableProviders[featureSlug]; provider != nil {
		for placeholder, value := range provider(params) {
			formatted, ok := stringPromptValue(value)
			if ok {
				varMap[placeholder] = formatted
			}
		}
	}
	for _, binding := range featurePromptBindings[featureSlug] {
		reader := binding.reader
		if reader == nil {
			reader = readDirectPromptParam
		}
		formatter := binding.formatter
		if formatter == nil {
			formatter = stringPromptValue
		}
		value, ok := reader(params, binding.paramKey)
		if !ok {
			continue
		}
		formatted, ok := formatter(value)
		if ok {
			varMap[binding.placeholder] = formatted
		}
	}
}

func readDirectPromptParam(params map[string]interface{}, key string) (interface{}, bool) {
	value, ok := params[key]
	return value, ok
}

func readFusionInputWeightsPromptParam(params map[string]interface{}, _ string) (interface{}, bool) {
	return workflow.ReadFusionInputWeights(params)
}

func variationsPromptVariables(params map[string]interface{}) map[string]interface{} {
	contract, ok := workflow.ReadVariationsPromptContract(params)
	if !ok {
		return nil
	}
	return contract.PromptVariables()
}

func stringPromptValue(value interface{}) (string, bool) {
	return fmt.Sprintf("%v", value), true
}

func ApplyFeatureSystemPromptWithConfig(
	basePrompt string,
	userPrompt string,
	featureSlug string,
	params map[string]interface{},
	config FeaturePromptRuntimeConfig,
) string {
	if featureSlug == "" || config.Prompt == "" {
		return basePrompt
	}
	return applyFeatureSystemPromptWithMergePolicy(basePrompt, userPrompt, featureSlug, params, config.Prompt, config.UserPromptMergePolicy)
}

func applyFeatureSystemPromptWithMergePolicy(
	basePrompt string,
	userPrompt string,
	featureSlug string,
	params map[string]interface{},
	sysPrompt string,
	policy FeaturePromptMergePolicy,
) string {
	varMap := map[string]string{
		"{{user_prompt}}": userPrompt,
	}
	applyPromptTemplateBindings(varMap, featureSlug, params)
	if strings.Contains(sysPrompt, "{{") {
		hasUserPromptPlaceholder := strings.Contains(sysPrompt, "{{user_prompt}}")
		result := sysPrompt
		result = replacePromptTemplateVariables(result, varMap)
		if policy.AppendUserPromptWhenTemplateMissesPlaceholder &&
			!hasUserPromptPlaceholder &&
			strings.TrimSpace(userPrompt) != "" {
			result += " " + userPrompt
		}
		return result
	}
	if !policy.AppendBasePromptWithoutTemplate {
		return sysPrompt
	}
	return sysPrompt + " " + basePrompt
}

func replacePromptTemplateVariables(template string, values map[string]string) string {
	keys := make([]string, 0, len(values))
	for key := range values {
		if key == "{{user_prompt}}" {
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)

	result := template
	for _, key := range keys {
		result = strings.ReplaceAll(result, key, values[key])
	}
	if userPrompt, ok := values["{{user_prompt}}"]; ok {
		result = strings.ReplaceAll(result, "{{user_prompt}}", userPrompt)
	}
	return result
}

func formatImageWeightsTemplateValue(raw interface{}) (string, bool) {
	switch values := raw.(type) {
	case []float64:
		return formatImageWeightFloats(values)
	case string:
		trimmed := strings.TrimSpace(values)
		return trimmed, trimmed != ""
	default:
		return "", false
	}
}

func formatImageWeightFloats(weights []float64) (string, bool) {
	if len(weights) == 0 {
		return "", false
	}
	parts := make([]string, 0, len(weights))
	for _, weight := range weights {
		if math.IsNaN(weight) || math.IsInf(weight, 0) {
			continue
		}
		percent := weight
		if percent >= 0 && percent <= 1 {
			percent *= 100
		}
		percent = math.Max(0, math.Min(100, percent))
		parts = append(parts, fmt.Sprintf("Image %d: %.0f%%", len(parts)+1, percent))
	}
	return strings.Join(parts, ", "), len(parts) > 0
}
