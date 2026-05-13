package accounts

import (
	"encoding/json"
	"fmt"
	"strings"

	"api/internal/platform/jsonlimit"
	"api/internal/platform/mediarefs"
	"api/internal/platform/outbound"
)

func validateSubmitTaskPrompt(prompt string, maxBytes int, label string) error {
	if len([]byte(prompt)) > maxBytes {
		return fmt.Errorf("%s长度不能超过 %dKB", label, maxBytes/1024)
	}
	return nil
}

func validateSubmitTaskImageURLs(imageURLs []string) error {
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
		if err := validateSubmitTaskMediaRef(url); err != nil {
			return err
		}
	}
	return nil
}

func validateSubmitTaskMediaParams(params map[string]interface{}) error {
	if params == nil {
		return nil
	}
	return mediarefs.ForEachParamMediaRef(params, func(key string, ref string) error {
		if err := validateSubmitTaskMediaRef(ref); err != nil {
			return fmt.Errorf("%s 无效: %w", key, err)
		}
		return nil
	})
}

func validateSubmitTaskMediaRef(raw string) error {
	return mediarefs.ValidateMediaRef(raw, outbound.AllowHostsFromEnv(), maxSubmitTaskMediaDataURLBytes)
}

func marshalSubmitTaskParams(params map[string]interface{}) (string, error) {
	if params == nil {
		return "", nil
	}
	if jsonlimit.ValueDepth(params) > maxSubmitTaskParamsDepth {
		return "", fmt.Errorf("任务参数层级过深")
	}
	payload, err := json.Marshal(params)
	if err != nil {
		return "", fmt.Errorf("任务参数格式错误")
	}
	if len(payload) > maxSubmitTaskParamsBytes {
		return "", fmt.Errorf("任务参数不能超过 %dMB", maxSubmitTaskParamsBytes/1024/1024)
	}
	return string(payload), nil
}
