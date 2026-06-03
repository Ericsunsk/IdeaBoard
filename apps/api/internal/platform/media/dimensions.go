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

func ValidateImageDimensions(data []byte) error {
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("读取图片尺寸失败: %w", err)
	}
	if cfg.Width <= 0 || cfg.Height <= 0 {
		return fmt.Errorf("图片尺寸无效")
	}
	if cfg.Width > MaxImageDimensionPixels || cfg.Height > MaxImageDimensionPixels {
		return fmt.Errorf("图片尺寸超过限制: %dx%d", cfg.Width, cfg.Height)
	}
	pixels := int64(cfg.Width) * int64(cfg.Height)
	if pixels > MaxImageTotalPixels {
		return fmt.Errorf("图片总像素超过限制: %dx%d", cfg.Width, cfg.Height)
	}
	if format == "" {
		return fmt.Errorf("图片格式无效")
	}
	return nil
}
