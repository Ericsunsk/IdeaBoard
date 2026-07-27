package workflow

import (
	"fmt"
	"strings"

	"ideaboard/api/internal/tasking/tasks"
	"ideaboard/api/internal/workflow/paramkeys"
)

type PatternGenInputContract struct {
	PromptRequired bool
	Ratios         []string
}

type PatternGenOutputContract struct {
	Kind     string
	MinCount int
}

type PatternGenNodeIOContract struct {
	Feature string
	Input   PatternGenInputContract
	Output  PatternGenOutputContract
}

var PatternGenIOContract = PatternGenNodeIOContract{
	Feature: FeatureSlugPatternGen,
	Input: PatternGenInputContract{
		PromptRequired: true,
		Ratios:         []string{"auto", "1:1", "3:4", "4:3", "9:16", "16:9"},
	},
	Output: PatternGenOutputContract{
		Kind:     "image-results",
		MinCount: 1,
	},
}

var patternGenSubmitParamKeys = map[string]struct{}{
	paramkeys.Feature: {},
	paramkeys.Ratio:   {},
}

var patternGenMediaParamKeys = map[string]struct{}{
	paramkeys.ImageURL:           {},
	paramkeys.ImageURLs:          {},
	paramkeys.ReferenceImageURLs: {},
	paramkeys.OriginalImageURL:   {},
	paramkeys.MaskImageURL:       {},
	paramkeys.MaskImageURLs:      {},
	paramkeys.UseMask:            {},
}

var patternGenImageEditParamKeys = map[string]struct{}{
	paramkeys.Media:              {},
	paramkeys.Variation:          {},
	paramkeys.Extend:             {},
	paramkeys.HD:                 {},
	paramkeys.Fusion:             {},
	paramkeys.VariationStrength:  {},
	paramkeys.ExtendTop:          {},
	paramkeys.ExtendRight:        {},
	paramkeys.ExtendBottom:       {},
	paramkeys.ExtendLeft:         {},
	paramkeys.ClarityStrength:    {},
	paramkeys.FusionInputWeights: {},
}

func ValidatePatternGenSubmitContract(prompt string, params map[string]interface{}) error {
	if strings.TrimSpace(prompt) == "" {
		return fmt.Errorf("请输入 Prompt 后再生成图案")
	}
	for key, value := range params {
		if err := validatePatternGenSubmitParam(key, value); err != nil {
			return err
		}
	}
	return nil
}

func validatePatternGenSubmitParam(key string, value interface{}) error {
	if _, ok := patternGenSubmitParamKeys[key]; !ok {
		if _, mediaKey := patternGenMediaParamKeys[key]; mediaKey {
			return fmt.Errorf("图案生成不需要输入图片，请移除图片输入后重试")
		}
		if _, editKey := patternGenImageEditParamKeys[key]; editKey {
			return fmt.Errorf("图案生成暂不支持图像编辑参数")
		}
		return nil
	}
	switch key {
	case paramkeys.Feature:
		feature, ok := value.(string)
		if !ok || strings.TrimSpace(feature) != FeatureSlugPatternGen {
			return fmt.Errorf("图案生成任务必须使用 pattern-gen workflow")
		}
	case paramkeys.Ratio:
		ratio, ok := value.(string)
		if !ok {
			return fmt.Errorf("图案生成参数 ratio 必须是字符串")
		}
		if err := validatePatternGenRatio(ratio); err != nil {
			return err
		}
	}
	return nil
}

func ValidatePatternGenTaskContract(prompt string, params *tasks.WorkflowTaskParams) error {
	if strings.TrimSpace(prompt) == "" {
		return fmt.Errorf("请输入 Prompt 后再生成图案")
	}
	if params == nil {
		return fmt.Errorf("图案生成任务参数缺失")
	}
	if feature := strings.TrimSpace(params.Feature); feature != "" && feature != FeatureSlugPatternGen {
		return fmt.Errorf("图案生成任务必须使用 pattern-gen workflow")
	}
	if params.Media != nil {
		return fmt.Errorf("图案生成不需要输入图片，请移除图片输入后重试")
	}
	if params.Variation != nil || params.Extend != nil || params.HD != nil || params.Fusion != nil {
		return fmt.Errorf("图案生成暂不支持图像编辑参数")
	}
	return validatePatternGenControls(params.Controls)
}

func ValidatePatternGenOutputContract(kind string, imageURLs []string) error {
	if strings.TrimSpace(kind) != PatternGenIOContract.Output.Kind {
		return fmt.Errorf("图案生成输出不符合 workflow contract: 输出类型必须是 %s", PatternGenIOContract.Output.Kind)
	}
	validCount := 0
	for _, imageURL := range imageURLs {
		if strings.TrimSpace(imageURL) != "" {
			validCount++
		}
	}
	if validCount < PatternGenIOContract.Output.MinCount {
		return fmt.Errorf("图案生成输出不符合 workflow contract: 至少需要 %d 张图片", PatternGenIOContract.Output.MinCount)
	}
	return nil
}

func validatePatternGenControls(controls *tasks.GenerationControls) error {
	if controls == nil {
		return nil
	}
	if ratio := strings.TrimSpace(controls.Ratio); ratio != "" {
		return validatePatternGenRatio(ratio)
	}
	return nil
}

func validatePatternGenRatio(raw string) error {
	ratio := strings.TrimSpace(raw)
	for _, allowed := range PatternGenIOContract.Input.Ratios {
		if ratio == allowed {
			return nil
		}
	}
	return fmt.Errorf("图案生成尺寸比例仅支持 %s", strings.Join(PatternGenIOContract.Input.Ratios, "、"))
}
