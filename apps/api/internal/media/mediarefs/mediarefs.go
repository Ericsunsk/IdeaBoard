package mediarefs

import (
	"encoding/base64"
	"fmt"
	"path"
	"strings"

	"ideaboard/api/internal/media/imageformat"
	"ideaboard/api/internal/platform/outbound"
)

var stringKeys = []string{"image_url", "mask_image_url", "canvas_image_ref"}
var listKeys = []string{"image_urls", "mask_image_urls", "reference_image_urls"}

type DataImage struct {
	MIMEType string
	Ext      string
	Data     []byte
}

func CanonicalListKeys() []string {
	return append([]string(nil), listKeys...)
}

func ValidateUploadPath(raw string) (string, error) {
	cleaned, err := normalizeUploadPath(raw)
	if err != nil {
		return "", err
	}
	parts, err := uploadPathParts(cleaned)
	if err != nil {
		return "", err
	}
	if isSupportedUploadPath(parts) {
		return cleaned, nil
	}
	return "", fmt.Errorf(msgValidateUploadPath)
}

func normalizeUploadPath(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if !strings.HasPrefix(trimmed, "/uploads/") {
		return "", fmt.Errorf(msgNormalizeUploadPath)
	}
	cleaned := path.Clean(trimmed)
	if !strings.HasPrefix(cleaned, "/uploads/") {
		return "", fmt.Errorf(msgNormalizeUploadPath)
	}
	return cleaned, nil
}

func uploadPathParts(cleaned string) ([]string, error) {
	parts := strings.Split(strings.TrimPrefix(cleaned, "/uploads/"), "/")
	if len(parts) < 1 || parts[0] == "" {
		return nil, fmt.Errorf(msgUploadPathParts)
	}
	return parts, nil
}

func isSupportedUploadPath(parts []string) bool {
	if len(parts) < 4 || parts[1] == "" || strings.Join(parts[3:], "") == "" {
		return false
	}
	switch parts[0] {
	case "canvas", "tasks":
		return parts[2] != ""
	case "profile":
		return parts[2] == "avatar" || parts[2] == "vector"
	default:
		return false
	}
}

func ParseDataImageURL(raw string, maxDecodedBytes int) (*DataImage, error) {
	trimmed := strings.TrimSpace(raw)
	lower := strings.ToLower(trimmed)
	if !strings.HasPrefix(lower, "data:image/") {
		return nil, fmt.Errorf(msgParseDataImageURL)
	}
	comma := strings.Index(trimmed, ",")
	if comma <= 5 {
		return nil, fmt.Errorf(msgParseDataImageURL2)
	}
	header := trimmed[5:comma]
	payload := trimmed[comma+1:]
	parts := strings.Split(header, ";")
	mimeType := strings.ToLower(strings.TrimSpace(parts[0]))
	if !strings.HasPrefix(mimeType, "image/") {
		return nil, fmt.Errorf(msgParseDataImageURL2)
	}
	isBase64 := false
	for _, part := range parts[1:] {
		if strings.EqualFold(strings.TrimSpace(part), "base64") {
			isBase64 = true
			break
		}
	}
	if !isBase64 {
		return nil, fmt.Errorf(msgParseDataImageURL2)
	}
	decoded, err := base64.StdEncoding.DecodeString(payload)
	if err != nil {
		return nil, fmt.Errorf(msgParseDataImageURL2)
	}
	if maxDecodedBytes > 0 && len(decoded) > maxDecodedBytes {
		return nil, fmt.Errorf(msgParseDataImageURL3)
	}
	ext := imageExtensionFromMIME(mimeType)
	if ext == "" {
		return nil, fmt.Errorf(msgParseDataImageURL4)
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
		return fmt.Errorf(msgValidateMediaRef)
	}
	lower := strings.ToLower(ref)
	if strings.HasPrefix(lower, "//") {
		return fmt.Errorf(msgValidateMediaRef2)
	}
	if strings.HasPrefix(lower, "data:image/") {
		if maxDataURLBytes > 0 && len(ref) >= maxDataURLBytes {
			return fmt.Errorf(msgParseDataImageURL3)
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
	return fmt.Errorf(msgValidateMediaRef3)
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
			return fmt.Errorf(msgForEachParamMediaRef, key)
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
		if err := forEachStringList(raw, key, true, func(value string) error {
			return onRef(key, value)
		}); err != nil {
			return err
		}
	}
	return nil
}

func CollectStringList(raw interface{}) []string {
	out := make([]string, 0)
	_ = forEachStringList(raw, "", false, func(value string) error {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			out = append(out, trimmed)
		}
		return nil
	})
	if len(out) == 0 {
		return nil
	}
	return out
}

func forEachStringList(raw interface{}, key string, strict bool, onValue func(string) error) error {
	switch values := raw.(type) {
	case []string:
		for _, value := range values {
			if err := onValue(value); err != nil {
				return err
			}
		}
	case []interface{}:
		for _, item := range values {
			value, ok := item.(string)
			if !ok {
				if strict {
					return fmt.Errorf(msgForEachStringList, key)
				}
				continue
			}
			if err := onValue(value); err != nil {
				return err
			}
		}
	default:
		if strict {
			return fmt.Errorf(msgForEachStringList, key)
		}
	}
	return nil
}

func imageExtensionFromMIME(mimeType string) string {
	return imageformat.ImageExtensionFromMIME(mimeType)
}
