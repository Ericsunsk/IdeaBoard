package media

import (
	"bytes"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"

	_ "golang.org/x/image/bmp"
	_ "golang.org/x/image/webp"
)

const (
	MaxImageDimensionPixels = 16384
	MaxImageTotalPixels     = 64 * 1024 * 1024
)

func ImageDimensions(data []byte) (width int, height int, err error) {
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return 0, 0, fmt.Errorf("读取图片尺寸失败: %w", err)
	}
	if cfg.Width <= 0 || cfg.Height <= 0 {
		return 0, 0, fmt.Errorf("图片尺寸无效")
	}
	if format == "" {
		return 0, 0, fmt.Errorf("图片格式无效")
	}
	return cfg.Width, cfg.Height, nil
}

func ValidateImageDimensions(data []byte) error {
	width, height, err := ImageDimensions(data)
	if err != nil {
		return err
	}
	if width > MaxImageDimensionPixels || height > MaxImageDimensionPixels {
		return fmt.Errorf("图片尺寸超过限制: %dx%d", width, height)
	}
	pixels := int64(width) * int64(height)
	if pixels > MaxImageTotalPixels {
		return fmt.Errorf("图片总像素超过限制: %dx%d", width, height)
	}
	return nil
}
