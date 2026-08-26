package workflow

import (
	"fmt"
	"math"

	"ideaboard/api/internal/workflow/paramkeys"
)

const (
	ParamKeyVariation = paramkeys.Variation
	ParamKeyExtend    = paramkeys.Extend
	ParamKeyHD        = paramkeys.HD
	ParamKeyFusion    = paramkeys.Fusion
)

type VariationsPromptContract struct {
	Strength float64
}

type OutpaintContract struct {
	TargetWidth  int
	TargetHeight int
	ImageOffsetX int
	ImageOffsetY int
}

type HDUpscaleContract struct{ Scale float64 }

func ReadVariationsPromptContract(params map[string]interface{}) (VariationsPromptContract, bool) {
	obj, ok := readWorkflowObjectParam(params, ParamKeyVariation)
	if !ok {
		return VariationsPromptContract{}, false
	}
	strength, ok := readNumberField(obj, paramkeys.Strength)
	if !ok {
		return VariationsPromptContract{}, false
	}
	return VariationsPromptContract{Strength: strength}, true
}

func (c VariationsPromptContract) PromptVariables() map[string]interface{} {
	return map[string]interface{}{
		paramkeys.PromptPlaceholderVariationStrength: formatPercentControl(c.Strength),
	}
}

func ReadOutpaintContract(params map[string]interface{}) (OutpaintContract, bool) {
	obj, ok := readWorkflowObjectParam(params, ParamKeyExtend)
	if !ok {
		return OutpaintContract{}, false
	}
	targetWidth, ok := readNumberField(obj, paramkeys.TargetWidth)
	if !ok {
		return OutpaintContract{}, false
	}
	targetHeight, ok := readNumberField(obj, paramkeys.TargetHeight)
	if !ok {
		return OutpaintContract{}, false
	}
	imageOffsetX, ok := readNumberField(obj, paramkeys.ImageOffsetX)
	if !ok {
		return OutpaintContract{}, false
	}
	imageOffsetY, ok := readNumberField(obj, paramkeys.ImageOffsetY)
	if !ok {
		return OutpaintContract{}, false
	}
	return OutpaintContract{
		TargetWidth:  int(targetWidth),
		TargetHeight: int(targetHeight),
		ImageOffsetX: int(imageOffsetX),
		ImageOffsetY: int(imageOffsetY),
	}, true
}

func ReadHDUpscaleContract(params map[string]interface{}) (HDUpscaleContract, bool) {
	obj, ok := readWorkflowObjectParam(params, ParamKeyHD)
	if !ok {
		return HDUpscaleContract{}, false
	}
	scale, ok := readNumberField(obj, paramkeys.Scale)
	if !ok {
		return HDUpscaleContract{}, false
	}
	return HDUpscaleContract{Scale: scale}, true
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
	mode, _ := readStringField(obj, paramkeys.Mode)
	weights, _ := ReadStyleFusionInputWeights(params)
	if len(weights) == 0 {
		return StyleFusionPromptContract{}, false
	}
	return StyleFusionPromptContract{Mode: mode, InputWeights: weights}, true
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
	case FeatureSlugVariations:
		return normalizeVariationsContract(params)
	case FeatureSlugOutpaint:
		return normalizeOutpaintContract(params)
	case FeatureSlugHDUpscale:
		return normalizeHDUpscaleContract(params)
	case FeatureSlugStyleFusion:
		return normalizeStyleFusionContract(params)
	default:
		return nil
	}
}

func BuildProviderPromptParams(params map[string]interface{}) map[string]interface{} {
	return defaultProviderParamsEncoder.EncodePromptParams(params)
}

func buildProviderPromptParams(params map[string]interface{}) map[string]interface{} {
	if params == nil {
		return nil
	}
	out := make(map[string]interface{}, len(params)+8)
	for key, value := range params {
		out[key] = value
	}
	if contract, ok := ReadVariationsPromptContract(params); ok {
		out[paramkeys.VariationStrength] = contract.Strength
	}
	if contract, ok := ReadStyleFusionPromptContract(params); ok {
		out[paramkeys.FusionInputWeights] = contract.InputWeights
	}
	return out
}

func normalizeVariationsContract(params map[string]interface{}) error {
	obj, err := requiredWorkflowObjectParam(params, ParamKeyVariation)
	if err != nil {
		return err
	}
	strength, ok := readNumberField(obj, paramkeys.Strength)
	if !ok {
		return fmt.Errorf("variation.strength 必须是数字")
	}
	obj[paramkeys.Strength] = clampPercent(strength)
	params[ParamKeyVariation] = obj
	return nil
}

func normalizeOutpaintContract(params map[string]interface{}) error {
	obj, err := requiredWorkflowObjectParam(params, ParamKeyExtend)
	if err != nil {
		return err
	}
	for _, key := range []string{paramkeys.TargetWidth, paramkeys.TargetHeight, paramkeys.ImageOffsetX, paramkeys.ImageOffsetY} {
		value, ok := readNumberField(obj, key)
		if !ok {
			return fmt.Errorf("extend.%s 必须是数字", key)
		}
		if math.IsNaN(value) || math.IsInf(value, 0) || math.Trunc(value) != value {
			return fmt.Errorf("extend.%s 必须是整数", key)
		}
		if (key == paramkeys.TargetWidth || key == paramkeys.TargetHeight) && value <= 0 {
			return fmt.Errorf("extend.%s 必须是正整数", key)
		}
		if (key == paramkeys.ImageOffsetX || key == paramkeys.ImageOffsetY) && value < 0 {
			return fmt.Errorf("extend.%s 不能为负数", key)
		}
		obj[key] = value
	}
	params[ParamKeyExtend] = obj
	return nil
}

func normalizeHDUpscaleContract(params map[string]interface{}) error {
	obj, err := requiredWorkflowObjectParam(params, ParamKeyHD)
	if err != nil {
		return err
	}
	scale, ok := readNumberField(obj, paramkeys.Scale)
	if !ok {
		return fmt.Errorf("hd.scale 必须是数字")
	}
	if scale < 1 || scale > 4 || math.Abs(scale*10-math.Round(scale*10)) > 1e-9 {
		return fmt.Errorf("hd.scale 必须是 1.0-4.0 且步长为 0.1")
	}
	obj[paramkeys.Scale] = math.Round(scale*10) / 10
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
	obj[paramkeys.InputWeights] = weights
	obj[paramkeys.Mode] = "weighted_reference"
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
