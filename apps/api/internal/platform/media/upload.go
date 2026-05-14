package media

import (
	"fmt"
	"mime/multipart"
)

// ValidateImageContent reads the first bytes of an uploaded file and verifies
// the magic number matches the claimed extension. This prevents uploading
// disguised files (e.g. an HTML file renamed to .png).
func ValidateImageContent(fh *multipart.FileHeader, ext string) error {
	normalizedExt, ok := NormalizeImageExtension(ext)
	if !ok {
		return fmt.Errorf("不支持的图片格式: %s", ext)
	}

	f, err := fh.Open()
	if err != nil {
		return fmt.Errorf("读取文件失败")
	}
	defer f.Close()

	signatures := imageSignaturesByExtension(normalizedExt)
	buf := make([]byte, requiredImageHeaderLength(signatures))
	n, err := f.Read(buf)
	if err != nil || n < len(buf) {
		return fmt.Errorf("文件内容不合法")
	}

	return validateImageMagic(buf, normalizedExt)
}

// ValidateImageBytes verifies in-memory bytes match the allowed image extension.
func ValidateImageBytes(data []byte, ext string) error {
	normalizedExt, ok := NormalizeImageExtension(ext)
	if !ok {
		return fmt.Errorf("不支持的图片格式: %s", ext)
	}

	signatures := imageSignaturesByExtension(normalizedExt)
	required := requiredImageHeaderLength(signatures)
	if len(data) < required {
		return fmt.Errorf("文件内容不合法")
	}

	return validateImageMagic(data[:required], normalizedExt)
}

func requiredImageHeaderLength(signatures []imageSignature) int {
	maxLen := 0
	for _, sig := range signatures {
		if sig.offset+len(sig.bytes) > maxLen {
			maxLen = sig.offset + len(sig.bytes)
		}
	}
	return maxLen
}

func validateImageMagic(buf []byte, ext string) error {
	signatures := imageSignaturesByExtension(ext)
	if len(signatures) == 0 {
		return fmt.Errorf("不支持的图片格式: %s", ext)
	}
	for _, sig := range signatures {
		if len(buf) < sig.offset+len(sig.bytes) {
			return fmt.Errorf("文件内容不合法")
		}
		for i, b := range sig.bytes {
			if buf[sig.offset+i] != b {
				return fmt.Errorf("文件内容与扩展名不匹配")
			}
		}
	}
	return nil
}
