package prompts

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"api/internal/workflow"
)

const (
	fabricOnBodyFeatureSlug = "fabric-on-body"
	styleFusionFeatureSlug  = "style-fusion"
	canvasExtendFeatureSlug = "canvas-extend"
	hdUpscaleFeatureSlug    = "hd-upscale"
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

const (
	styleFusionHiddenConstraintGeneric  = "【执行语义】图片输入与 Image A / Image B 的来源权重是主导信息。用户 prompt 作为辅助/弱修饰信号，用于描述融合方式、质感、细节、氛围、构图；若与主导信息冲突，以图片来源与滑块权重为准。"
	styleFusionHiddenConstraintWithNum  = "【执行语义】图片输入与 Image A / Image B 的来源权重是主导信息（Image A: %s，Image B: %s）。用户 prompt 作为辅助/弱修饰信号，用于描述融合方式、质感、细节、氛围、构图；若与主导信息冲突，以图片来源与滑块权重为准。"
	canvasExtendHiddenConstraintGeneric = "【执行语义】画布延伸以扩边参数为主导，决定上、右、下、左四个方向的延伸幅度。用户 prompt 只用于描述新扩展区域的内容、纹理、背景和补全方式；若与扩边方向冲突，以扩边参数为准。"
	canvasExtendHiddenConstraintWithNum = "【执行语义】画布延伸以扩边参数为主导（上 %s，右 %s，下 %s，左 %s），决定四个方向的延伸幅度。用户 prompt 只用于描述新扩展区域的内容、纹理、背景和补全方式；若与扩边方向冲突，以扩边参数为准。"
	multiAngleHiddenConstraintGeneric   = "【执行语义】当前请求仅输出单张、单视角结果，不允许拼接多视图。职责与优先级如下：1) Image 输入（image / image_urls）提供基础画面与主体语义；2) Model 输入（model_url + model_strength）约束主体身份与关键外观一致性；3) Pose 输入（pose_url + pose_strength）约束姿态与肢体方向；4) angle_spec 决定目标视角（yaw 为水平旋转角，framing 为景别）；5) Prompt 仅作为弱修饰，用于补充风格、材质、氛围与细节。若冲突，按以上顺序执行。"
	multiAngleHiddenConstraintWithModel = "【Model 输入语义】当提供 model_url 时，必须优先保持主体身份与核心外观稳定，仅在该前提下按 angle_spec 与 Pose 约束做视角变化。"
	HDUpscaleMaskedConstraint           = "【执行语义】当前请求为局部/遮罩高清增强（非整图放大）。请以原图（original_image_url）作为全局上下文，仅对蒙版（mask_image_url / mask_image_urls / mask_url）限定区域进行高清增强，未遮罩区域保持原图主体、结构与语义一致。"
	FabricOnBodyMaskedConstraint        = "【执行语义】当前请求为 Wear 局部贴图。请以 Body 原图（original_image_url）作为基底，仅在蒙版（mask_image_url / mask_image_urls / mask_url）白色区域内应用 Texture；非蒙版区域必须保持 Body 原图的主体、轮廓、结构与细节不变。"
)

func BuildStyleFusionHiddenConstraint(params map[string]interface{}) string {
	if params == nil {
		return styleFusionHiddenConstraintGeneric
	}

	if imageAWeight, imageBWeight, ok := readStyleFusionWeights(params); ok {
		return fmt.Sprintf(
			styleFusionHiddenConstraintWithNum,
			formatWeightPercent(imageAWeight),
			formatWeightPercent(imageBWeight),
		)
	}

	return styleFusionHiddenConstraintGeneric
}

func BuildCanvasExtendHiddenConstraint(params map[string]interface{}) string {
	if params == nil {
		return canvasExtendHiddenConstraintGeneric
	}

	top, hasTop := readCanvasExtendPercent(params["extend_top"])
	right, hasRight := readCanvasExtendPercent(params["extend_right"])
	bottom, hasBottom := readCanvasExtendPercent(params["extend_bottom"])
	left, hasLeft := readCanvasExtendPercent(params["extend_left"])
	if hasTop || hasRight || hasBottom || hasLeft {
		return fmt.Sprintf(
			canvasExtendHiddenConstraintWithNum,
			formatWeightPercent(top),
			formatWeightPercent(right),
			formatWeightPercent(bottom),
			formatWeightPercent(left),
		)
	}

	return canvasExtendHiddenConstraintGeneric
}

func BuildMultiAngleHiddenConstraint(params map[string]interface{}) string {
	constraint := multiAngleHiddenConstraintGeneric
	if params == nil {
		return constraint
	}
	if hasNonEmptyString(params["model_url"]) {
		constraint += "\n" + multiAngleHiddenConstraintWithModel
		if modelURL, ok := params["model_url"].(string); ok {
			constraint += "\n" + "model_url=" + strings.TrimSpace(modelURL)
		}
	}
	if modelStrength, ok := readCanvasExtendPercent(params["model_strength"]); ok {
		constraint += "\n" + "model_strength=" + formatWeightPercent(modelStrength)
	}
	if hasNonEmptyString(params["pose_url"]) {
		if poseURL, ok := params["pose_url"].(string); ok {
			constraint += "\n" + "pose_url=" + strings.TrimSpace(poseURL)
		}
	}
	if poseStrength, ok := readCanvasExtendPercent(params["pose_strength"]); ok {
		constraint += "\n" + "pose_strength=" + formatWeightPercent(poseStrength)
	}
	rawSpec, ok := params["angle_spec"]
	if !ok {
		return constraint
	}
	spec, err := json.Marshal(rawSpec)
	if err != nil || len(spec) == 0 {
		return constraint
	}
	return constraint + "\n" + "angle_spec=" + string(spec)
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

func readStyleFusionWeights(params map[string]interface{}) (float64, float64, bool) {
	if params == nil {
		return 0, 0, false
	}

	if rawA, okA := params["image_a_weight"]; okA {
		if rawB, okB := params["image_b_weight"]; okB {
			if imageAWeight, ok := parseWeight(rawA); ok {
				if imageBWeight, ok := parseWeight(rawB); ok {
					return normalizeWeights(imageAWeight, imageBWeight)
				}
			}
		}
	}

	if blendRatio, ok := params["blend_ratio"]; ok {
		if imageAWeight, ok := parseWeight(blendRatio); ok {
			if imageAWeight >= 0 && imageAWeight <= 1 {
				return normalizeWeights(imageAWeight, 1-imageAWeight)
			}
			return normalizeWeights(imageAWeight, 100-imageAWeight)
		}
	}

	return 0, 0, false
}

func parseWeight(v interface{}) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int8:
		return float64(n), true
	case int16:
		return float64(n), true
	case int32:
		return float64(n), true
	case int64:
		return float64(n), true
	case uint:
		return float64(n), true
	case uint8:
		return float64(n), true
	case uint16:
		return float64(n), true
	case uint32:
		return float64(n), true
	case uint64:
		return float64(n), true
	case string:
		parsed, err := strconv.ParseFloat(strings.TrimSpace(n), 64)
		if err != nil {
			return 0, false
		}
		return parsed, true
	default:
		return 0, false
	}
}

func readCanvasExtendPercent(v interface{}) (float64, bool) {
	value, ok := parseWeight(v)
	if !ok {
		return 0, false
	}
	if value < 0 {
		value = 0
	}
	if value > 100 {
		value = 100
	}
	return value, true
}

func normalizeWeights(a, b float64) (float64, float64, bool) {
	if a < 0 || b < 0 {
		return 0, 0, false
	}
	total := a + b
	if total <= 0 {
		return 0, 0, false
	}
	return a / total * 100, b / total * 100, true
}

func formatWeightPercent(weight float64) string {
	value := strconv.FormatFloat(weight, 'f', 1, 64)
	value = strings.TrimSuffix(value, ".0")
	return value + "%"
}

func AppendHiddenConstraint(fullPrompt, constraint string) string {
	if strings.TrimSpace(constraint) == "" {
		return fullPrompt
	}
	if strings.TrimSpace(fullPrompt) == "" {
		return constraint
	}
	return fullPrompt + "\n\n" + constraint
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
		case "1", "true", "yes", "y", "on", "enabled", "masked", "local", "partial":
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
		return result
	}
	return sysPrompt + " " + basePrompt
}
