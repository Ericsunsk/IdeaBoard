package prompts

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"ideaboard/api/internal/workflow"
)

type promptParamReader func(map[string]interface{}, string) (interface{}, bool)
type promptParamFormatter func(interface{}) (string, bool)

type promptTemplateBinding struct {
	placeholder string
	paramKey    string
	reader      promptParamReader
	formatter   promptParamFormatter
}

type featurePromptMergePolicy struct {
	appendUserPromptWhenTemplateMissesPlaceholder bool
	appendBasePromptWithoutTemplate               bool
}

var featurePromptBindings = map[string][]promptTemplateBinding{
	workflow.FeatureSlugStyleVariation: {
		stringPromptBinding("{{variation_strength}}", "variation_strength"),
	},
	workflow.FeatureSlugStyleFusion: {
		{
			placeholder: "{{fusion_input_weights}}",
			paramKey:    "fusion.input_weights",
			reader:      readStyleFusionInputWeightsPromptParam,
			formatter:   formatImageWeightsTemplateValue,
		},
	},
	workflow.FeatureSlugImageExtend: {
		stringPromptBinding("{{extend_top_percent}}", "extend_top"),
		stringPromptBinding("{{extend_right_percent}}", "extend_right"),
		stringPromptBinding("{{extend_bottom_percent}}", "extend_bottom"),
		stringPromptBinding("{{extend_left_percent}}", "extend_left"),
	},
	workflow.FeatureSlugHDUpscale: {
		stringPromptBinding("{{clarity_strength}}", "clarity_strength"),
	},
}

var defaultFeaturePromptMergePolicy = featurePromptMergePolicy{
	appendUserPromptWhenTemplateMissesPlaceholder: true,
	appendBasePromptWithoutTemplate:               true,
}

func resolveFeaturePromptMergePolicy(featureSlug string) featurePromptMergePolicy {
	if featureSlug == workflow.FeatureSlugStyleVariation {
		return featurePromptMergePolicy{}
	}
	return defaultFeaturePromptMergePolicy
}

func stringPromptBinding(placeholder string, paramKey string) promptTemplateBinding {
	return promptTemplateBinding{
		placeholder: placeholder,
		paramKey:    paramKey,
		formatter:   stringPromptValue,
	}
}

func applyPromptTemplateBindings(
	varMap map[string]string,
	featureSlug string,
	params map[string]interface{},
) {
	if params == nil {
		return
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

func readStyleFusionInputWeightsPromptParam(params map[string]interface{}, _ string) (interface{}, bool) {
	return workflow.ReadStyleFusionInputWeights(params)
}

func stringPromptValue(value interface{}) (string, bool) {
	return fmt.Sprintf("%v", value), true
}

func ApplyFeatureSystemPrompt(
	basePrompt string,
	userPrompt string,
	featureSlug string,
	params map[string]interface{},
	loadPrompt func(string) (string, error),
) string {
	if featureSlug == "" || loadPrompt == nil {
		return basePrompt
	}
	sysPrompt, err := loadPrompt(featureSlug)
	if err != nil || sysPrompt == "" {
		return basePrompt
	}

	varMap := map[string]string{
		"{{user_prompt}}": userPrompt,
	}
	applyPromptTemplateBindings(varMap, featureSlug, params)
	policy := resolveFeaturePromptMergePolicy(featureSlug)
	if strings.Contains(sysPrompt, "{{") {
		hasUserPromptPlaceholder := strings.Contains(sysPrompt, "{{user_prompt}}")
		result := sysPrompt
		result = replacePromptTemplateVariables(result, varMap)
		if policy.appendUserPromptWhenTemplateMissesPlaceholder &&
			!hasUserPromptPlaceholder &&
			strings.TrimSpace(userPrompt) != "" {
			result += " " + userPrompt
		}
		return result
	}
	if !policy.appendBasePromptWithoutTemplate {
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
