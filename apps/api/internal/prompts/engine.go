package prompts

import (
	"encoding/json"
	"fmt"
	"strings"

	"api/internal/workflow"
)

const (
	fabricOnBodyFeatureSlug = workflow.FeatureSlugFabricOnBody
	styleFusionFeatureSlug  = workflow.FeatureSlugStyleFusion
	canvasExtendFeatureSlug = workflow.FeatureSlugCanvasExtend
	hdUpscaleFeatureSlug    = workflow.FeatureSlugHDUpscale
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

var wearTileModeTemplateLabels = map[string]string{
	"seamless": "平铺重复",
	"single":   "单张贴合",
}

func DescribeViewpointTemplateValue(value string) string {
	return describeTemplateLabel(value, viewpointTemplateLabels)
}

func DescribeViewpointLightingTemplateValue(value string) string {
	return describeTemplateLabel(value, viewpointLightingTemplateLabels)
}

func DescribeWearTileModeTemplateValue(value string) string {
	return describeTemplateLabel(value, wearTileModeTemplateLabels)
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

func parseBoolLike(v interface{}) (bool, bool) {
	switch b := v.(type) {
	case bool:
		return b, true
	case float64:
		return b != 0, true
	case float32:
		return b != 0, true
	case int:
		return b != 0, true
	case int8:
		return b != 0, true
	case int16:
		return b != 0, true
	case int32:
		return b != 0, true
	case int64:
		return b != 0, true
	case uint:
		return b != 0, true
	case uint8:
		return b != 0, true
	case uint16:
		return b != 0, true
	case uint32:
		return b != 0, true
	case uint64:
		return b != 0, true
	case string:
		s := strings.TrimSpace(strings.ToLower(b))
		if s == "" {
			return false, false
		}
		switch s {
		case "1", "true", "yes", "y", "on", "enabled":
			return true, true
		case "0", "false", "no", "n", "off", "disabled":
			return false, true
		}
		return false, false
	default:
		return false, false
	}
}

func hasNonEmptyString(v interface{}) bool {
	s, ok := v.(string)
	return ok && strings.TrimSpace(s) != ""
}

func hasNonEmptyStringList(v interface{}) bool {
	switch arr := v.(type) {
	case []string:
		for _, item := range arr {
			if strings.TrimSpace(item) != "" {
				return true
			}
		}
	case []interface{}:
		for _, item := range arr {
			if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
				return true
			}
		}
	}
	return false
}

func readMaskEnabledFlag(params map[string]interface{}) (bool, bool) {
	if params == nil {
		return false, false
	}
	if raw, ok := workflow.ReadWorkflowParam(params, "use_mask"); ok {
		if enabled, parsed := parseBoolLike(raw); parsed {
			return enabled, true
		}
	}
	return false, false
}

func IsMaskedHDUpscaleRequest(params map[string]interface{}) bool {
	if params == nil {
		return false
	}

	if enabled, parsed := readMaskEnabledFlag(params); parsed && enabled {
		return true
	}

	if rawMaskURL, ok := workflow.ReadWorkflowParam(params, "mask_image_url"); ok && hasNonEmptyString(rawMaskURL) {
		return true
	}
	if rawMaskURLs, ok := workflow.ReadWorkflowParam(params, "mask_image_urls"); ok && hasNonEmptyStringList(rawMaskURLs) {
		return true
	}

	return hasNonEmptyString(params["original_image_url"]) && hasNonEmptyStringList(params["image_urls"])
}

func IsMaskedFabricOnBodyRequest(params map[string]interface{}) bool {
	if params == nil {
		return false
	}
	if enabled, parsed := readMaskEnabledFlag(params); parsed && enabled {
		return true
	}
	if rawMaskURL, ok := workflow.ReadWorkflowParam(params, "mask_image_url"); ok && hasNonEmptyString(rawMaskURL) {
		return true
	}
	if rawMaskURLs, ok := workflow.ReadWorkflowParam(params, "mask_image_urls"); ok && hasNonEmptyStringList(rawMaskURLs) {
		return true
	}
	return false
}

func applyFabricOnBodyOptionalMaskTemplateDefaults(
	featureSlug string,
	params map[string]interface{},
	varMap map[string]string,
) {
	if featureSlug != fabricOnBodyFeatureSlug || IsMaskedFabricOnBodyRequest(params) {
		return
	}
	varMap["{{mask_image_url}}"] = ""
	varMap["{{use_mask}}"] = "false"
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
	if params != nil {
		if v, ok := params["color_type"]; ok {
			varMap["{{color_type}}"] = fmt.Sprintf("%v", v)
		}
		if v, ok := params["colors"]; ok {
			switch arr := v.(type) {
			case []interface{}:
				strs := make([]string, 0, len(arr))
				for _, item := range arr {
					strs = append(strs, fmt.Sprintf("%v", item))
				}
				varMap["{{colors}}"] = strings.Join(strs, ", ")
			default:
				varMap["{{colors}}"] = fmt.Sprintf("%v", v)
			}
		}
		if v, ok := params["category"]; ok {
			varMap["{{category}}"] = fmt.Sprintf("%v", v)
		}
		if v, ok := params["variation_strength"]; ok {
			varMap["{{strength}}"] = fmt.Sprintf("%v", v)
		}
		if v, ok := params["blend_ratio"]; ok {
			varMap["{{blend_ratio}}"] = fmt.Sprintf("%v", v)
		}
		if v, ok := params["extend_top"]; ok {
			varMap["{{extend_top}}"] = fmt.Sprintf("%v", v)
		}
		if v, ok := params["extend_right"]; ok {
			varMap["{{extend_right}}"] = fmt.Sprintf("%v", v)
		}
		if v, ok := params["extend_bottom"]; ok {
			varMap["{{extend_bottom}}"] = fmt.Sprintf("%v", v)
		}
		if v, ok := params["extend_left"]; ok {
			varMap["{{extend_left}}"] = fmt.Sprintf("%v", v)
		}
		if v, ok := params["image_a_weight"]; ok {
			varMap["{{image_a_weight}}"] = fmt.Sprintf("%v", v)
		}
		if v, ok := params["image_b_weight"]; ok {
			varMap["{{image_b_weight}}"] = fmt.Sprintf("%v", v)
		}
		if v, ok := params["target_color"]; ok {
			varMap["{{target_color}}"] = fmt.Sprintf("%v", v)
		}
		if v, ok := params["fabric_rotation"]; ok {
			varMap["{{fabric_rotation}}"] = fmt.Sprintf("%v", v)
		}
		if v, ok := params["fabric_opacity"]; ok {
			varMap["{{fabric_opacity}}"] = fmt.Sprintf("%v", v)
		}
		if v, ok := params["fabric_scale"]; ok {
			varMap["{{fabric_scale}}"] = fmt.Sprintf("%v", v)
		}
		if v, ok := params["tile_mode"]; ok {
			varMap["{{tile_mode}}"] = DescribeWearTileModeTemplateValue(fmt.Sprintf("%v", v))
		}
		if v, ok := params["original_image_url"]; ok {
			varMap["{{original_image_url}}"] = fmt.Sprintf("%v", v)
		}
		if v, ok := workflow.ReadWorkflowParam(params, "mask_image_url"); ok {
			varMap["{{mask_image_url}}"] = fmt.Sprintf("%v", v)
		}
		if enabled, parsed := readMaskEnabledFlag(params); parsed {
			varMap["{{use_mask}}"] = fmt.Sprintf("%t", enabled)
		}
		if v, ok := workflow.ReadWorkflowParam(params, "upscale_mode"); ok {
			varMap["{{upscale_mode}}"] = fmt.Sprintf("%v", v)
		}
		if v, ok := params["upscale_factor"]; ok {
			varMap["{{upscale_factor}}"] = fmt.Sprintf("%v", v)
		}
		if v, ok := params["output_format"]; ok {
			varMap["{{output_format}}"] = fmt.Sprintf("%v", v)
		}
		if v, ok := params["output_quality"]; ok {
			varMap["{{output_quality}}"] = fmt.Sprintf("%v", v)
		}
		if v, ok := params["pattern_mode"]; ok {
			varMap["{{pattern_mode}}"] = fmt.Sprintf("%v", v)
		}
		if v, ok := params["angle_spec"]; ok {
			if encoded, err := json.Marshal(v); err == nil {
				varMap["{{angle_spec}}"] = string(encoded)
			}
		}
		if v, ok := params["background"]; ok {
			varMap["{{background}}"] = fmt.Sprintf("%v", v)
		}
		if v, ok := params["viewpoint"]; ok {
			varMap["{{viewpoint}}"] = DescribeViewpointTemplateValue(fmt.Sprintf("%v", v))
		}
		if v, ok := params["lighting"]; ok {
			varMap["{{lighting}}"] = DescribeViewpointLightingTemplateValue(fmt.Sprintf("%v", v))
		}
		if v, ok := params["model_url"]; ok {
			varMap["{{model_url}}"] = fmt.Sprintf("%v", v)
		}
		if v, ok := params["pose_url"]; ok {
			varMap["{{pose_url}}"] = fmt.Sprintf("%v", v)
		}
		if v, ok := params["pose_strength"]; ok {
			varMap["{{pose_strength}}"] = fmt.Sprintf("%v", v)
		}
		if v, ok := params["model_strength"]; ok {
			varMap["{{model_strength}}"] = fmt.Sprintf("%v", v)
		}
	}
	applyFabricOnBodyOptionalMaskTemplateDefaults(featureSlug, params, varMap)

	if strings.Contains(sysPrompt, "{{") {
		result := sysPrompt
		for k, v := range varMap {
			result = strings.ReplaceAll(result, k, v)
		}
		if featureSlug == workflow.FeatureSlugStyleInnovate {
			return result
		}
		return result
	}
	if featureSlug == workflow.FeatureSlugStyleInnovate {
		return sysPrompt
	}
	return sysPrompt + " " + basePrompt
}
