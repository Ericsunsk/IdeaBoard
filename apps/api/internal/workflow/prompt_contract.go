package workflow

import (
	"fmt"
	"math"
)

const (
	ParamKeyVariation = "variation"
	ParamKeyExtend    = "extend"
	ParamKeyHD        = "hd"
	ParamKeyFusion    = "fusion"
)

type StyleVariationPromptContract struct {
	Strength float64
}

type ImageExtendPromptContract struct {
	TopPercent    float64
	RightPercent  float64
	BottomPercent float64
	LeftPercent   float64
}

type HDUpscalePromptContract struct {
	ClarityStrength float64
}

func ReadStyleVariationPromptContract(params map[string]interface{}) (StyleVariationPromptContract, bool) {
	obj, ok := readWorkflowObjectParam(params, ParamKeyVariation)
	if !ok {
		return StyleVariationPromptContract{}, false
	}
	strength, ok := readNumberField(obj, "strength")
	if !ok {
		return StyleVariationPromptContract{}, false
	}
	return StyleVariationPromptContract{Strength: strength}, true
}

func (c StyleVariationPromptContract) PromptVariables() map[string]interface{} {
	return map[string]interface{}{
		"{{variation_strength}}": formatPercentControl(c.Strength),
	}
}

func ReadImageExtendPromptContract(params map[string]interface{}) (ImageExtendPromptContract, bool) {
	obj, ok := readWorkflowObjectParam(params, ParamKeyExtend)
	if !ok {
		return ImageExtendPromptContract{}, false
	}
	top, ok := readNumberField(obj, "top_percent")
	if !ok {
		return ImageExtendPromptContract{}, false
	}
	right, ok := readNumberField(obj, "right_percent")
	if !ok {
		return ImageExtendPromptContract{}, false
	}
	bottom, ok := readNumberField(obj, "bottom_percent")
	if !ok {
		return ImageExtendPromptContract{}, false
	}
	left, ok := readNumberField(obj, "left_percent")
	if !ok {
		return ImageExtendPromptContract{}, false
	}
	return ImageExtendPromptContract{
		TopPercent:    top,
		RightPercent:  right,
		BottomPercent: bottom,
		LeftPercent:   left,
	}, true
}

func (c ImageExtendPromptContract) PromptVariables() map[string]interface{} {
	return map[string]interface{}{
		"{{extend_top_percent}}":    formatPercentControl(c.TopPercent),
		"{{extend_right_percent}}":  formatPercentControl(c.RightPercent),
		"{{extend_bottom_percent}}": formatPercentControl(c.BottomPercent),
		"{{extend_left_percent}}":   formatPercentControl(c.LeftPercent),
	}
}

func ReadHDUpscalePromptContract(params map[string]interface{}) (HDUpscalePromptContract, bool) {
	obj, ok := readWorkflowObjectParam(params, ParamKeyHD)
	if !ok {
		return HDUpscalePromptContract{}, false
	}
	clarity, ok := readNumberField(obj, "clarity_strength")
	if !ok {
		return HDUpscalePromptContract{}, false
	}
	return HDUpscalePromptContract{ClarityStrength: clarity}, true
}

func (c HDUpscalePromptContract) PromptVariables() map[string]interface{} {
	return map[string]interface{}{
		"{{clarity_strength}}": formatPercentControl(c.ClarityStrength),
	}
}

type StyleFusionPromptContract struct {
	Mode         string
	InputWeights []float64
}

func ReadStyleFusionPromptContract(params map[string]interface{}) (StyleFusionPromptContract, bool) {
	obj, ok := readWorkflowObjectParam(params, ParamKeyFusion)
	if !ok {
		return StyleFusionPromptContract{}, false
	}
	mode, _ := readStringField(obj, "mode")
	weights, _ := ReadStyleFusionInputWeights(params)
	if len(weights) == 0 {
		return StyleFusionPromptContract{}, false
	}
	return StyleFusionPromptContract{Mode: mode, InputWeights: weights}, true
}

func (c StyleFusionPromptContract) PromptVariables() map[string]interface{} {
	return map[string]interface{}{
		"{{fusion_input_weights}}": c.InputWeights,
	}
}

func readStringField(obj map[string]interface{}, key string) (string, bool) {
	raw, ok := obj[key]
	if !ok {
		return "", false
	}
	s, ok := raw.(string)
	return s, ok
}

func NormalizeStructuredPromptContractParams(params map[string]interface{}, featureSlug string) error {
	if params == nil {
		return nil
	}
	switch featureSlug {
	case FeatureSlugStyleVariation:
		return normalizeStyleVariationContract(params)
	case FeatureSlugImageExtend:
		return normalizeImageExtendContract(params)
	case FeatureSlugHDUpscale:
		return normalizeHDUpscaleContract(params)
	case FeatureSlugStyleFusion:
		return normalizeStyleFusionContract(params)
	default:
		return nil
	}
}

func BuildProviderPromptParams(params map[string]interface{}) map[string]interface{} {
	if params == nil {
		return nil
	}
	out := make(map[string]interface{}, len(params)+8)
	for key, value := range params {
		out[key] = value
	}
	if contract, ok := ReadStyleVariationPromptContract(params); ok {
		out["variation_strength"] = contract.Strength
	}
	if contract, ok := ReadImageExtendPromptContract(params); ok {
		out["extend_top"] = contract.TopPercent
		out["extend_right"] = contract.RightPercent
		out["extend_bottom"] = contract.BottomPercent
		out["extend_left"] = contract.LeftPercent
	}
	if contract, ok := ReadStyleFusionPromptContract(params); ok {
		out["fusion_input_weights"] = contract.InputWeights
	}
	return out
}

func normalizeStyleVariationContract(params map[string]interface{}) error {
	obj, err := requiredWorkflowObjectParam(params, ParamKeyVariation)
	if err != nil {
		return err
	}
	strength, ok := readNumberField(obj, "strength")
	if !ok {
		return fmt.Errorf("variation.strength 必须是数字")
	}
	obj["strength"] = clampPercent(strength)
	params[ParamKeyVariation] = obj
	return nil
}

func normalizeImageExtendContract(params map[string]interface{}) error {
	obj, err := requiredWorkflowObjectParam(params, ParamKeyExtend)
	if err != nil {
		return err
	}
	hasExtension := false
	for _, key := range []string{"top_percent", "right_percent", "bottom_percent", "left_percent"} {
		value, ok := readNumberField(obj, key)
		if !ok {
			return fmt.Errorf("extend.%s 必须是数字", key)
		}
		normalized := math.Round(math.Max(0, math.Min(100, value)))
		obj[key] = normalized
		if normalized > 0 {
			hasExtension = true
		}
	}
	if !hasExtension {
		return fmt.Errorf("extend 至少需要一个方向大于 0")
	}
	params[ParamKeyExtend] = obj
	return nil
}

func normalizeHDUpscaleContract(params map[string]interface{}) error {
	obj, err := requiredWorkflowObjectParam(params, ParamKeyHD)
	if err != nil {
		return err
	}
	clarity, ok := readNumberField(obj, "clarity_strength")
	if !ok {
		return fmt.Errorf("hd.clarity_strength 必须是数字")
	}
	obj["clarity_strength"] = clampPercent(clarity)
	params[ParamKeyHD] = obj
	return nil
}

func normalizeStyleFusionContract(params map[string]interface{}) error {
	obj, err := requiredWorkflowObjectParam(params, ParamKeyFusion)
	if err != nil {
		return err
	}
	weights, ok := ReadStyleFusionInputWeights(params)
	if !ok || len(weights) == 0 {
		return fmt.Errorf("fusion.input_weights 至少需要一个权重")
	}
	obj["input_weights"] = weights
	obj["mode"] = "weighted_reference"
	params[ParamKeyFusion] = obj
	return nil
}

func requiredWorkflowObjectParam(params map[string]interface{}, key string) (map[string]interface{}, error) {
	obj, ok := readWorkflowObjectParam(params, key)
	if !ok {
		return nil, fmt.Errorf("%s 参数缺失或格式错误", key)
	}
	return obj, nil
}

func readWorkflowObjectParam(params map[string]interface{}, key string) (map[string]interface{}, bool) {
	raw, ok := ReadWorkflowParam(params, key)
	if !ok {
		return nil, false
	}
	obj, ok := raw.(map[string]interface{})
	if !ok {
		return nil, false
	}
	return obj, true
}

func readNumberField(obj map[string]interface{}, key string) (float64, bool) {
	raw, ok := obj[key]
	if !ok {
		return 0, false
	}
	return readFiniteNumber(raw)
}

func clampPercent(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 100 {
		return 100
	}
	return math.Round(value)
}

func formatPercentControl(value float64) interface{} {
	rounded := math.Round(value)
	if math.Abs(rounded-value) < 0.000001 {
		return int(rounded)
	}
	return value
}
