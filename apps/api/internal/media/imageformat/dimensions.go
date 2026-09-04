package imageformat

import (
	"bytes"
	"fmt"
	"image"
	"image/gif"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"

	_ "golang.org/x/image/bmp"
	_ "golang.org/x/image/webp"
)

const (
	MaxImageDimensionPixels = 16384
	MaxImageTotalPixels     = 64 * 1024 * 1024
)

type Facts struct {
	Extension string
	MIMEType  string
	Width     int
	Height    int
	Animated  bool
}

func InspectBytes(data []byte, ext string) (Facts, error) {
	return Inspect(bytes.NewReader(data), ext)
}

func InspectDetectedBytes(data []byte) (Facts, error) {
	ext := DetectImageExtensionFromBytes(data)
	if ext == "" {
		return Facts{}, fmt.Errorf("文件内容不合法")
	}
	return InspectBytes(data, ext)
}

// Inspect validates and describes an image from a seekable source without
// forcing file-backed callers to allocate a second full-file byte slice.
func Inspect(source io.ReadSeeker, ext string) (facts Facts, err error) {
	normalizedExt, ok := NormalizeImageExtension(ext)
	if !ok {
		return Facts{}, fmt.Errorf("不支持的图片格式: %s", ext)
	}
	if source == nil {
		return Facts{}, fmt.Errorf("文件内容不合法")
	}
	defer func() {
		if _, seekErr := source.Seek(0, io.SeekStart); err == nil && seekErr != nil {
			err = fmt.Errorf("重置图片读取位置失败: %w", seekErr)
		}
	}()

	if _, err = source.Seek(0, io.SeekStart); err != nil {
		return Facts{}, fmt.Errorf("读取图片失败: %w", err)
	}
	if err = ValidateImageReader(source, normalizedExt); err != nil {
		return Facts{}, err
	}
	cfg, format, err := decodeImageConfig(source)
	if err != nil {
		return Facts{}, err
	}
	if err := validateImageDimensions(cfg, format); err != nil {
		return Facts{}, err
	}
	animated, err := inspectGIFAnimation(source, normalizedExt)
	if err != nil {
		return Facts{}, err
	}

	return Facts{
		Extension: normalizedExt,
		MIMEType:  ImageMIMEFromExtension(normalizedExt),
		Width:     cfg.Width,
		Height:    cfg.Height,
		Animated:  animated,
	}, nil
}

func decodeImageConfig(source io.ReadSeeker) (image.Config, string, error) {
	if _, err := source.Seek(0, io.SeekStart); err != nil {
		return image.Config{}, "", fmt.Errorf("读取图片失败: %w", err)
	}
	cfg, format, err := image.DecodeConfig(source)
	if err != nil {
		return image.Config{}, "", fmt.Errorf("读取图片尺寸失败: %w", err)
	}
	return cfg, format, nil
}

func validateImageDimensions(cfg image.Config, format string) error {
	if cfg.Width <= 0 || cfg.Height <= 0 || format == "" {
		return fmt.Errorf("图片尺寸或格式无效")
	}
	if cfg.Width > MaxImageDimensionPixels || cfg.Height > MaxImageDimensionPixels {
		return fmt.Errorf("图片尺寸超过限制: %dx%d", cfg.Width, cfg.Height)
	}
	if int64(cfg.Width)*int64(cfg.Height) > MaxImageTotalPixels {
		return fmt.Errorf("图片总像素超过限制: %dx%d", cfg.Width, cfg.Height)
	}
	return nil
}

func inspectGIFAnimation(source io.ReadSeeker, ext string) (bool, error) {
	if ext != ".gif" {
		return false, nil
	}
	if _, err := source.Seek(0, io.SeekStart); err != nil {
		return false, fmt.Errorf("读取 GIF 动画信息失败: %w", err)
	}
	decoded, err := gif.DecodeAll(source)
	if err != nil {
		return false, fmt.Errorf("读取 GIF 动画信息失败: %w", err)
	}
	return len(decoded.Image) > 1, nil
}
