package mediarefs

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"path"
	"strings"

	"api/internal/platform/media"
	"api/internal/platform/outbound"
)

var stringKeys = []string{"image_url", "original_image_url", "mask_image_url", "model_url", "pose_url"}
var listKeys = []string{"image_urls", "mask_image_urls"}

type DataImage struct {
	MIMEType string
	Ext      string
	Data     []byte
}

func CanonicalStringKeys() []string {
	return append([]string(nil), stringKeys...)
}

func CanonicalListKeys() []string {
	return append([]string(nil), listKeys...)
}

func ValidateUploadPath(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if !strings.HasPrefix(trimmed, "/uploads/") {
		return "", fmt.Errorf("仅支持 /uploads/ 相对路径")
	}
	cleaned := path.Clean(trimmed)
	if !strings.HasPrefix(cleaned, "/uploads/") {
		return "", fmt.Errorf("仅支持 /uploads/ 相对路径")
	}
	return cleaned, nil
}

func ParseDataImageURL(raw string, maxDecodedBytes int) (*DataImage, error) {
	trimmed := strings.TrimSpace(raw)
	lower := strings.ToLower(trimmed)
	if !strings.HasPrefix(lower, "data:image/") {
		return nil, fmt.Errorf("仅支持 data:image")
	}
	comma := strings.Index(trimmed, ",")
	if comma <= 5 {
		return nil, fmt.Errorf("data URL 格式错误")
	}
	header := trimmed[5:comma]
	payload := trimmed[comma+1:]
	parts := strings.Split(header, ";")
	mimeType := strings.ToLower(strings.TrimSpace(parts[0]))
	if !strings.HasPrefix(mimeType, "image/") {
		return nil, fmt.Errorf("data URL 格式错误")
	}
	isBase64 := false
	for _, part := range parts[1:] {
		if strings.EqualFold(strings.TrimSpace(part), "base64") {
			isBase64 = true
			break
		}
	}
	if !isBase64 {
		return nil, fmt.Errorf("data URL 格式错误")
	}
	decoded, err := base64.StdEncoding.DecodeString(payload)
	if err != nil {
		return nil, fmt.Errorf("data URL 格式错误")
	}
	if maxDecodedBytes > 0 && len(decoded) > maxDecodedBytes {
		return nil, fmt.Errorf("data URL 超过大小限制")
	}
	ext := imageExtensionFromMIME(mimeType)
	if ext == "" {
		detected := strings.ToLower(strings.TrimSpace(http.DetectContentType(decoded)))
		ext, _ = media.NormalizeImageExtension(path.Ext("x." + strings.TrimPrefix(detected, "image/")))
	}
	if ext == "" {
		ext = ".png"
	}
	return &DataImage{
		MIMEType: mimeType,
		Ext:      ext,
		Data:     decoded,
	}, nil
}

func ValidateMediaRef(raw string, allowHosts []string, maxDataURLBytes int) error {
	ref := strings.TrimSpace(raw)
	if ref == "" {
		return fmt.Errorf("地址不能为空")
	}
	lower := strings.ToLower(ref)
	if strings.HasPrefix(lower, "//") {
		return fmt.Errorf("不支持协议相对地址")
	}
	if strings.HasPrefix(lower, "data:image/") {
		if maxDataURLBytes > 0 && len(ref) >= maxDataURLBytes {
			return fmt.Errorf("data URL 超过大小限制")
		}
		_, err := ParseDataImageURL(ref, maxDataURLBytes)
		return err
	}
	if strings.HasPrefix(ref, "/") {
		_, err := ValidateUploadPath(ref)
		return err
	}
	if strings.Contains(ref, "://") {
		_, err := outbound.ValidateMediaHTTPURL(ref, allowHosts...)
		return err
	}
	return fmt.Errorf("仅支持 http(s)、/uploads/ 或 data:image")
}

func ForEachParamMediaRef(params map[string]interface{}, onRef func(key string, ref string) error) error {
	if params == nil {
		return nil
	}
	for _, key := range stringKeys {
		raw, ok := params[key]
		if !ok {
			continue
		}
		value, ok := raw.(string)
		if !ok {
			return fmt.Errorf("%s 必须是字符串", key)
		}
		if err := onRef(key, value); err != nil {
			return err
		}
	}
	for _, key := range listKeys {
		raw, ok := params[key]
		if !ok {
			continue
		}
		switch values := raw.(type) {
		case []string:
			for _, value := range values {
				if err := onRef(key, value); err != nil {
					return err
				}
			}
		case []interface{}:
			for _, item := range values {
				value, ok := item.(string)
				if !ok {
					return fmt.Errorf("%s 必须是字符串数组", key)
				}
				if err := onRef(key, value); err != nil {
					return err
				}
			}
		default:
			return fmt.Errorf("%s 必须是字符串数组", key)
		}
	}
	return nil
}

func CollectStringList(raw interface{}) []string {
	switch values := raw.(type) {
	case []string:
		out := make([]string, 0, len(values))
		for _, value := range values {
			if trimmed := strings.TrimSpace(value); trimmed != "" {
				out = append(out, trimmed)
			}
		}
		return out
	case []interface{}:
		out := make([]string, 0, len(values))
		for _, item := range values {
			value, ok := item.(string)
			if !ok {
				continue
			}
			if trimmed := strings.TrimSpace(value); trimmed != "" {
				out = append(out, trimmed)
			}
		}
		return out
	default:
		return nil
	}
}

func imageExtensionFromMIME(mimeType string) string {
	switch strings.ToLower(strings.TrimSpace(mimeType)) {
	case "image/jpeg", "image/jpg":
		return ".jpg"
	case "image/png":
		return ".png"
	case "image/gif":
		return ".gif"
	case "image/webp":
		return ".webp"
	default:
		return ""
	}
}
