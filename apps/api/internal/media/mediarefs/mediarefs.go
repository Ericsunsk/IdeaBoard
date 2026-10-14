package mediarefs

import (
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"ideaboard/api/internal/media/imageformat"
)

var stringKeys = []string{"image_url", "mask_image_url"}
var listKeys = []string{"image_urls", "mask_image_urls", "reference_image_urls"}

type DataImage struct {
	MIMEType string
	Ext      string
	Data     []byte
}

func ParseDataImageURL(raw string, maxDecodedBytes int) (*DataImage, error) {
	trimmed := strings.TrimSpace(raw)
	lower := strings.ToLower(trimmed)
	if !strings.HasPrefix(lower, "data:image/") {
		return nil, errors.New(msgParseDataImageURL)
	}
	comma := strings.Index(trimmed, ",")
	if comma <= 5 {
		return nil, errors.New(msgParseDataImageURL2)
	}
	header := trimmed[5:comma]
	payload := trimmed[comma+1:]
	parts := strings.Split(header, ";")
	mimeType := strings.ToLower(strings.TrimSpace(parts[0]))
	if !strings.HasPrefix(mimeType, "image/") {
		return nil, errors.New(msgParseDataImageURL2)
	}
	isBase64 := false
	for _, part := range parts[1:] {
		if strings.EqualFold(strings.TrimSpace(part), "base64") {
			isBase64 = true
			break
		}
	}
	if !isBase64 {
		return nil, errors.New(msgParseDataImageURL2)
	}
	decoded, err := base64.StdEncoding.DecodeString(payload)
	if err != nil {
		return nil, errors.New(msgParseDataImageURL2)
	}
	if maxDecodedBytes > 0 && len(decoded) > maxDecodedBytes {
		return nil, errors.New(msgParseDataImageURL3)
	}
	ext := imageExtensionFromMIME(mimeType)
	if ext == "" {
		return nil, errors.New(msgParseDataImageURL4)
	}
	return &DataImage{
		MIMEType: mimeType,
		Ext:      ext,
		Data:     decoded,
	}, nil
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
