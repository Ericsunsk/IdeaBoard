package workflow

import (
	"errors"
	"fmt"
	"ideaboard/api/internal/platform/errorrender"
	"math"

	"ideaboard/api/internal/workflow/paramkeys"
)

const (
	ParamKeyVariation = paramkeys.Variation
	ParamKeyOutpaint  = paramkeys.Outpaint
	ParamKeyUpscale   = paramkeys.Upscale
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

type UpscaleContract struct{ Scale float64 }

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
	obj, ok := readWorkflowObjectParam(params, ParamKeyOutpaint)
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

func ReadUpscaleContract(params map[string]interface{}) (UpscaleContract, bool) {
	obj, ok := readWorkflowObjectParam(params, ParamKeyUpscale)
	if !ok {
		return UpscaleContract{}, false
	}
	scale, ok := readNumberField(obj, paramkeys.Scale)
	if !ok {
		return UpscaleContract{}, false
	}
	return UpscaleContract{Scale: scale}, true
}

type FusionPromptContract struct {
	Mode         string
	InputWeights []float64
}

func ReadFusionPromptContract(params map[string]interface{}) (FusionPromptContract, bool) {
	obj, ok := readWorkflowObjectParam(params, ParamKeyFusion)
	if !ok {
		return FusionPromptContract{}, false
	}
	mode, _ := readStringField(obj, paramkeys.Mode)
	weights, _ := ReadFusionInputWeights(params)
	if len(weights) == 0 {
		return FusionPromptContract{}, false
	}
	return FusionPromptContract{Mode: mode, InputWeights: weights}, true
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
	case FeatureSlugUpscale:
		return normalizeUpscaleContract(params)
	case FeatureSlugFusion:
		return normalizeFusionContract(params)
	default:
		return nil
	}
}

func normalizeVariationsContract(params map[string]interface{}) error {
	obj, err := requiredWorkflowObjectParam(params, ParamKeyVariation)
	if err != nil {
		return err
	}
	strength, ok := readNumberField(obj, paramkeys.Strength)
	if !ok {
		return errors.New(errorrender.MsgNormalizeVariationsContract)
	}
	obj[paramkeys.Strength] = clampPercent(strength)
	params[ParamKeyVariation] = obj
	return nil
}

func normalizeOutpaintContract(params map[string]interface{}) error {
	obj, err := requiredWorkflowObjectParam(params, ParamKeyOutpaint)
	if err != nil {
		return err
	}
	for _, key := range []string{paramkeys.TargetWidth, paramkeys.TargetHeight, paramkeys.ImageOffsetX, paramkeys.ImageOffsetY} {
		value, ok := readNumberField(obj, key)
		if !ok {
			return fmt.Errorf(errorrender.MsgNormalizeOutpaintContract, key)
		}
		if math.IsNaN(value) || math.IsInf(value, 0) || math.Trunc(value) != value {
			return fmt.Errorf(errorrender.MsgNormalizeOutpaintContract2, key)
		}
		if (key == paramkeys.TargetWidth || key == paramkeys.TargetHeight) && value <= 0 {
			return fmt.Errorf(errorrender.MsgNormalizeOutpaintContract3, key)
		}
		if (key == paramkeys.ImageOffsetX || key == paramkeys.ImageOffsetY) && value < 0 {
			return fmt.Errorf(errorrender.MsgNormalizeOutpaintContract4, key)
		}
		obj[key] = value
	}
	params[ParamKeyOutpaint] = obj
	return nil
}

func normalizeUpscaleContract(params map[string]interface{}) error {
	obj, err := requiredWorkflowObjectParam(params, ParamKeyUpscale)
	if err != nil {
		return err
	}
	scale, ok := readNumberField(obj, paramkeys.Scale)
	if !ok {
		return errors.New(errorrender.MsgNormalizeUpscaleContract)
	}
	if scale < 1 || scale > 4 || math.Abs(scale*10-math.Round(scale*10)) > 1e-9 {
		return errors.New(errorrender.MsgNormalizeUpscaleContract2)
	}
	obj[paramkeys.Scale] = math.Round(scale*10) / 10
	params[ParamKeyUpscale] = obj
	return nil
}

func normalizeFusionContract(params map[string]interface{}) error {
	obj, err := requiredWorkflowObjectParam(params, ParamKeyFusion)
	if err != nil {
		return err
	}
	weights, ok := ReadFusionInputWeights(params)
	if !ok || len(weights) == 0 {
		return errors.New(errorrender.MsgNormalizeFusionContract)
	}
	obj[paramkeys.InputWeights] = weights
	obj[paramkeys.Mode] = "weighted_reference"
	params[ParamKeyFusion] = obj
	return nil
}

func requiredWorkflowObjectParam(params map[string]interface{}, key string) (map[string]interface{}, error) {
	obj, ok := readWorkflowObjectParam(params, key)
	if !ok {
		return nil, fmt.Errorf(errorrender.MsgRequiredWorkflowObjectParam, key)
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
