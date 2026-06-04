package accounts

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"ideaboard/api/internal/platform/jsonlimit"
	"ideaboard/api/internal/platform/mediarefs"
	"ideaboard/api/internal/platform/outbound"
	"ideaboard/api/internal/storage"
)

func normalizeSubmitTaskMediaParams(params map[string]interface{}) {
	if params == nil {
		return
	}
	imageURLs := mediarefs.CollectStringList(params["image_urls"])
	if len(imageURLs) > 0 {
		params["image_urls"] = imageURLs
	} else {
		delete(params, "image_urls")
	}
	referenceImageURLs := mediarefs.CollectStringList(params["reference_image_urls"])
	if len(referenceImageURLs) > 0 {
		params["reference_image_urls"] = referenceImageURLs
	} else {
		delete(params, "reference_image_urls")
	}
}

func validateSubmitTaskPrompt(prompt string, maxBytes int, label string) error {
	if len([]byte(prompt)) > maxBytes {
		return fmt.Errorf("%s长度不能超过 %dKB", label, maxBytes/1024)
	}
	return nil
}

func validateSubmitTaskImageURLs(imageURLs []string) error {
	return validateSubmitTaskImageURLsWithRefValidator(imageURLs, validateSubmitTaskMediaRef)
}

func (s *AccountService) validateSubmitTaskImageURLs(ctx context.Context, userID int64, imageURLs []string) error {
	return validateSubmitTaskImageURLsWithRefValidator(imageURLs, func(raw string) error {
		return s.validateSubmitTaskMediaRef(ctx, userID, raw)
	})
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

func validateSubmitTaskMediaParams(params map[string]interface{}) error {
	return validateSubmitTaskMediaParamsWithRefValidator(params, validateSubmitTaskMediaRef)
}

func (s *AccountService) validateSubmitTaskMediaParams(ctx context.Context, userID int64, params map[string]interface{}) error {
	return validateSubmitTaskMediaParamsWithRefValidator(params, func(raw string) error {
		return s.validateSubmitTaskMediaRef(ctx, userID, raw)
	})
}

func validateSubmitTaskMediaParamsWithRefValidator(params map[string]interface{}, validateRef func(string) error) error {
	if params == nil {
		return nil
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

func validateSubmitTaskMediaRef(raw string) error {
	return mediarefs.ValidateMediaRef(raw, outbound.AllowHostsFromEnv(), maxSubmitTaskMediaDataURLBytes)
}

func (s *AccountService) validateSubmitTaskMediaRef(ctx context.Context, userID int64, raw string) error {
	if err := validateSubmitTaskMediaRef(raw); err != nil {
		return err
	}
	trimmed := strings.TrimSpace(raw)
	if !strings.HasPrefix(trimmed, "/uploads/") {
		return nil
	}
	resource, err := storage.ParseUploadResource(trimmed)
	if err != nil {
		return err
	}
	if resource.Kind != storage.UploadResourceKindWorkspace {
		return fmt.Errorf("仅支持工作区上传资源")
	}
	if s.uploadBoards == nil || s.uploadOwners == nil {
		return fmt.Errorf("无法校验工作区上传资源权限")
	}
	if err := storage.AuthorizeUploadResourceRead(
		ctx,
		userID,
		resource,
		s.uploadBoards,
		s.uploadOwners,
	); err != nil {
		return fmt.Errorf("无权使用该工作区上传资源")
	}
	return nil
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
