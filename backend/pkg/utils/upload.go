package utils

import (
	"fmt"
	"mime/multipart"
)

type imageSignature struct {
	offset int
	bytes  []byte
}

// Magic number signatures for allowed image types.
var imageMagic = map[string][]imageSignature{
	".jpg":  {{bytes: []byte{0xFF, 0xD8, 0xFF}}},
	".jpeg": {{bytes: []byte{0xFF, 0xD8, 0xFF}}},
	".png":  {{bytes: []byte{0x89, 0x50, 0x4E, 0x47}}},
	".gif":  {{bytes: []byte{0x47, 0x49, 0x46}}},
	".webp": {
		{bytes: []byte{0x52, 0x49, 0x46, 0x46}},
		{offset: 8, bytes: []byte{0x57, 0x45, 0x42, 0x50}},
	},
}

// ValidateImageContent reads the first bytes of an uploaded file and verifies
// the magic number matches the claimed extension. This prevents uploading
// disguised files (e.g. an HTML file renamed to .png).
func ValidateImageContent(fh *multipart.FileHeader, ext string) error {
	signatures, ok := imageMagic[ext]
	if !ok {
		return fmt.Errorf("不支持的图片格式: %s", ext)
	}

	f, err := fh.Open()
	if err != nil {
		return fmt.Errorf("读取文件失败")
	}
	defer f.Close()

	buf := make([]byte, requiredImageHeaderLength(signatures))
	n, err := f.Read(buf)
	if err != nil || n < len(buf) {
		return fmt.Errorf("文件内容不合法")
	}

	return validateImageMagic(buf, ext)
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
	signatures, ok := imageMagic[ext]
	if !ok {
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
