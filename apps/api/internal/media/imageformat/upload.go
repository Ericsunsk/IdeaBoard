package imageformat

import (
	"fmt"
	"image"
	"io"
	"mime/multipart"
)

// InspectFileHeader determines the image format from the uploaded bytes and
// validates its dimensions. The filename extension is deliberately ignored:
// multipart metadata is user-controlled and may be stale after content
// negotiation or a client-side rename.
func InspectFileHeader(fh *multipart.FileHeader) (Facts, error) {
	if fh == nil {
		return Facts{}, fmt.Errorf("文件内容不合法")
	}
	f, err := fh.Open()
	if err != nil {
		return Facts{}, fmt.Errorf("读取文件失败: %w", err)
	}
	defer f.Close()
	facts, err := InspectDetected(f)
	if err != nil {
		return Facts{}, err
	}
	// Go does not currently ship an AVIF pixel decoder in this service. Its
	// container and dimension validation remains authoritative until a native
	// decoder is introduced; all other accepted upload formats are fully decoded.
	if facts.Extension != ".avif" {
		if _, err := f.Seek(0, io.SeekStart); err != nil {
			return Facts{}, fmt.Errorf("读取图片失败: %w", err)
		}
		if _, _, err := image.Decode(f); err != nil {
			return Facts{}, fmt.Errorf("图片内容无法完整解码: %w", err)
		}
	}
	return facts, nil
}

// InspectDetected detects the canonical format from the source bytes before
// running the normal signature and image-config validation.
func InspectDetected(source io.ReadSeeker) (Facts, error) {
	if source == nil {
		return Facts{}, fmt.Errorf("文件内容不合法")
	}
	if _, err := source.Seek(0, io.SeekStart); err != nil {
		return Facts{}, fmt.Errorf("读取图片失败: %w", err)
	}
	header := make([]byte, 64*1024)
	n, readErr := io.ReadFull(source, header)
	if readErr != nil && readErr != io.EOF && readErr != io.ErrUnexpectedEOF {
		return Facts{}, fmt.Errorf("读取图片失败: %w", readErr)
	}
	ext := DetectImageExtensionFromBytes(header[:n])
	if ext == "" {
		return Facts{}, fmt.Errorf("文件内容不合法")
	}
	if _, err := source.Seek(0, io.SeekStart); err != nil {
		return Facts{}, fmt.Errorf("读取图片失败: %w", err)
	}
	return Inspect(source, ext)
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

// ValidateImageReader verifies the image signature from a stream without
// requiring callers to materialize the whole file in memory.
func ValidateImageReader(r io.Reader, ext string) error {
	normalizedExt, ok := NormalizeImageExtension(ext)
	if !ok {
		return fmt.Errorf("不支持的图片格式: %s", ext)
	}
	if r == nil {
		return fmt.Errorf("文件内容不合法")
	}

	signatures := imageSignaturesByExtension(normalizedExt)
	required := requiredImageHeaderLength(signatures)
	buf := make([]byte, required)
	n, err := io.ReadFull(r, buf)
	if err != nil || n < required {
		return fmt.Errorf("文件内容不合法")
	}

	return validateImageMagic(buf, normalizedExt)
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
