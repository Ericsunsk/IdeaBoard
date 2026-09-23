package task

import (
	"fmt"
	"strings"

	"ideaboard/api/internal/media/mediarefs"
	"ideaboard/api/internal/workflow/paramkeys"
)

var unsupportedSubmitTaskParamKeys = []string{
	paramkeys.ImageURL,
	paramkeys.MaskImageURLs,
	paramkeys.VariationStrength,
	paramkeys.FusionInputWeights,
	paramkeys.UpscaleMode,
	paramkeys.UpscaleFactor,
	paramkeys.OutputFormat,
	paramkeys.OutputQuality,
	paramkeys.SampleCount,
	paramkeys.ImageCount,
}

func normalizeSubmitTaskMediaParams(params map[string]interface{}) {
	if params == nil {
		return
	}
	imageURLs := mediarefs.CollectStringList(params[paramkeys.ImageURLs])
	if len(imageURLs) > 0 {
		params[paramkeys.ImageURLs] = imageURLs
	} else {
		delete(params, paramkeys.ImageURLs)
	}
	referenceImageURLs := mediarefs.CollectStringList(params[paramkeys.ReferenceImageURLs])
	if len(referenceImageURLs) > 0 {
		params[paramkeys.ReferenceImageURLs] = referenceImageURLs
	} else {
		delete(params, paramkeys.ReferenceImageURLs)
	}
}

func validateSubmitTaskPrompt(prompt string, maxBytes int, label string) error {
	if len([]byte(prompt)) > maxBytes {
		return fmt.Errorf("%s长度不能超过 %dKB", label, maxBytes/1024)
	}
	return nil
}

func validateSubmitTaskImageURLsWithRefValidator(imageURLs []string, validateRef func(string) error) error {
	if len(imageURLs) > maxSubmitTaskImageURLs {
		return fmt.Errorf("图片数量不能超过 %d 张", maxSubmitTaskImageURLs)
	}
	for _, raw := range imageURLs {
		url := strings.TrimSpace(raw)
		if url == "" {
			return fmt.Errorf("图片地址不能为空")
		}
		if len([]byte(url)) > maxSubmitTaskImageURLBytes {
			return fmt.Errorf("图片地址长度不能超过 %d 字符", maxSubmitTaskImageURLBytes)
		}
		if err := validateRef(url); err != nil {
			return err
		}
	}
	return nil
}

func validateSubmitTaskMediaParamsWithRefValidator(params map[string]interface{}, validateRef func(string) error) error {
	if params == nil {
		return nil
	}
	if err := validateSubmitTaskContractKeys(params); err != nil {
		return err
	}
	for _, key := range mediarefs.CanonicalListKeys() {
		values := mediarefs.CollectStringList(params[key])
		if len(values) > maxSubmitTaskImageURLs {
			return fmt.Errorf("%s 数量不能超过 %d 张", key, maxSubmitTaskImageURLs)
		}
	}
	return mediarefs.ForEachParamMediaRef(params, func(key string, ref string) error {
		if err := validateRef(ref); err != nil {
			return fmt.Errorf("%s 无效: %w", key, err)
		}
		return nil
	})
}

func validateSubmitTaskContractKeys(params map[string]interface{}) error {
	for _, key := range unsupportedSubmitTaskParamKeys {
		if _, ok := params[key]; ok {
			return fmt.Errorf("任务参数必须使用结构化字段")
		}
	}
	return nil
}
