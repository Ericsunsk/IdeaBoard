package worker

import (
	"api/internal/platform/media"
	"bytes"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	"image/png"
	"mime"
	"path"
	"strings"

	_ "golang.org/x/image/bmp"
	_ "golang.org/x/image/webp"
)

func normalizeOpenAIEditMultipartImage(filename, mimeType string, data []byte) (string, string, []byte, error) {
	if err := media.ValidateImageDimensions(data); err != nil {
		return "", "", nil, err
	}
	normalizedMimeType := normalizeImageMIMEType(mimeType, data)
	switch normalizedMimeType {
	case "image/png", "image/jpeg":
		return ensureFilenameHasImageExt(filename, normalizedMimeType), normalizedMimeType, data, nil
	default:
		decoded, _, err := image.Decode(bytes.NewReader(data))
		if err != nil {
			return "", "", nil, fmt.Errorf("解码图像失败: %w", err)
		}
		var encoded bytes.Buffer
		if err := png.Encode(&encoded, decoded); err != nil {
			return "", "", nil, fmt.Errorf("编码 PNG 失败: %w", err)
		}
		return ensureFilenameHasImageExt(filename, "image/png"), "image/png", encoded.Bytes(), nil
	}
}

func normalizeImageMIMEType(mimeType string, data []byte) string {
	if parsed, _, err := mime.ParseMediaType(strings.TrimSpace(mimeType)); err == nil && strings.HasPrefix(parsed, "image/") {
		if normalized, ok := media.NormalizeImageMIME(parsed); ok {
			return normalized
		}
	}
	if ext := media.DetectImageExtensionFromBytes(data); ext != "" {
		if normalized := media.ImageMIMEFromExtension(ext); normalized != "" {
			return normalized
		}
	}
	return "image/png"
}

func ensureFilenameHasImageExt(filename string, mimeType string) string {
	base := strings.TrimSpace(path.Base(filename))
	if base == "" || base == "." || base == "/" {
		base = "image"
	}
	ext := ".png"
	if mimeType == "image/jpeg" {
		ext = ".jpg"
	}
	if strings.EqualFold(path.Ext(base), ext) {
		return base
	}
	return strings.TrimSuffix(base, path.Ext(base)) + ext
}
