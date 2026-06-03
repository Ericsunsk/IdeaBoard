package prompts

import (
	"fmt"
	"math"
	"strings"

	"api/internal/workflow"
)

var viewpointTemplateLabels = map[string]string{
	"top_front":      "正面平视",
	"top_side":       "微侧展示",
	"top_45":         "45° 侧视",
	"top_back":       "背面展示",
	"btm_front":      "正面立体",
	"btm_side":       "微侧立体",
	"btm_half_side":  "半侧动态",
	"btm_back":       "背面立体",
	"set_front":      "正面展示",
	"set_45_side":    "45° 微侧",
	"set_side_stand": "侧面静立",
	"set_back":       "背面展示",
}

var viewpointLightingTemplateLabels = map[string]string{
	"warm_white": "高级暖白",
	"afternoon":  "午后暖阳",
	"hard_light": "质感硬光",
}

func DescribeViewpointTemplateValue(value string) string {
	return describeTemplateLabel(value, viewpointTemplateLabels)
}

func DescribeViewpointLightingTemplateValue(value string) string {
	return describeTemplateLabel(value, viewpointLightingTemplateLabels)
}

func describeTemplateLabel(value string, labels map[string]string) string {
	normalized := strings.TrimSpace(value)
	if normalized == "" {
		return ""
	}
	if label, ok := labels[normalized]; ok {
		return label
	}
	return normalized
}

type promptTemplateBinding struct {
	placeholder string
	paramKey    string
	reader      func(map[string]interface{}, string) (interface{}, bool)
	formatter   func(interface{}) (string, bool)
}

var featurePromptBindings = map[string][]promptTemplateBinding{
	workflow.FeatureSlugStyleVariation: {
		stringPromptBinding("{{strength}}", "variation_strength"),
	},
	workflow.FeatureSlugStyleFusion: {
		{
			placeholder: "{{image_weights}}",
			paramKey:    "image_weights",
			formatter:   formatImageWeightsTemplateValue,
		},
	},
	workflow.FeatureSlugImageExtend: {
		stringPromptBinding("{{extend_top}}", "extend_top"),
		stringPromptBinding("{{extend_right}}", "extend_right"),
		stringPromptBinding("{{extend_bottom}}", "extend_bottom"),
		stringPromptBinding("{{extend_left}}", "extend_left"),
	},
	workflow.FeatureSlugHDUpscale: {
		workflowPromptBinding("{{upscale_mode}}", "upscale_mode"),
		stringPromptBinding("{{upscale_factor}}", "upscale_factor"),
		stringPromptBinding("{{output_format}}", "output_format"),
		stringPromptBinding("{{output_quality}}", "output_quality"),
	},
	workflow.FeatureSlugVirtual3D: {
		{
			placeholder: "{{viewpoint}}",
			paramKey:    "viewpoint",
			formatter:   viewpointPromptValue,
		},
		{
			placeholder: "{{lighting}}",
			paramKey:    "lighting",
			formatter:   lightingPromptValue,
		},
	},
}

func stringPromptBinding(placeholder string, paramKey string) promptTemplateBinding {
	return promptTemplateBinding{
		placeholder: placeholder,
		paramKey:    paramKey,
		formatter:   stringPromptValue,
	}
}

func workflowPromptBinding(placeholder string, paramKey string) promptTemplateBinding {
	binding := stringPromptBinding(placeholder, paramKey)
	binding.reader = workflow.ReadWorkflowParam
	return binding
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

func stringPromptValue(value interface{}) (string, bool) {
	return fmt.Sprintf("%v", value), true
}

func viewpointPromptValue(value interface{}) (string, bool) {
	return DescribeViewpointTemplateValue(fmt.Sprintf("%v", value)), true
}

func lightingPromptValue(value interface{}) (string, bool) {
	return DescribeViewpointLightingTemplateValue(fmt.Sprintf("%v", value)), true
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
	if strings.Contains(sysPrompt, "{{") {
		hasUserPromptPlaceholder := strings.Contains(sysPrompt, "{{user_prompt}}")
		result := sysPrompt
		for k, v := range varMap {
			result = strings.ReplaceAll(result, k, v)
		}
		if featureSlug != workflow.FeatureSlugStyleVariation &&
			!hasUserPromptPlaceholder &&
			strings.TrimSpace(userPrompt) != "" {
			result += " " + userPrompt
		}
		if featureSlug == workflow.FeatureSlugStyleVariation {
			return result
		}
		return result
	}
	if featureSlug == workflow.FeatureSlugStyleVariation {
		return sysPrompt
	}
	return sysPrompt + " " + basePrompt
}

func formatImageWeightsTemplateValue(raw interface{}) (string, bool) {
	switch values := raw.(type) {
	case []float64:
		return formatImageWeightFloats(values)
	case []int:
		weights := make([]float64, 0, len(values))
		for _, value := range values {
			weights = append(weights, float64(value))
		}
		return formatImageWeightFloats(weights)
	case []interface{}:
		weights := make([]float64, 0, len(values))
		for _, value := range values {
			if weight, ok := readImageWeightNumber(value); ok {
				weights = append(weights, weight)
			}
		}
		return formatImageWeightFloats(weights)
	case string:
		trimmed := strings.TrimSpace(values)
		return trimmed, trimmed != ""
	default:
		return "", false
	}
}

func readImageWeightNumber(value interface{}) (float64, bool) {
	var weight float64
	switch typed := value.(type) {
	case float64:
		weight = typed
	case float32:
		weight = float64(typed)
	case int:
		weight = float64(typed)
	case int8:
		weight = float64(typed)
	case int16:
		weight = float64(typed)
	case int32:
		weight = float64(typed)
	case int64:
		weight = float64(typed)
	case uint:
		weight = float64(typed)
	case uint8:
		weight = float64(typed)
	case uint16:
		weight = float64(typed)
	case uint32:
		weight = float64(typed)
	case uint64:
		weight = float64(typed)
	default:
		return 0, false
	}
	return weight, !math.IsNaN(weight) && !math.IsInf(weight, 0)
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
