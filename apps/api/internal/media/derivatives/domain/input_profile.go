package domain

import "fmt"

// InputProfile describes the bounded image contract used when an image is
// sent to an external processing endpoint. It intentionally contains no
// provider or model names: endpoint implementations may be replaced without
// changing the media contract.
type InputProfile struct {
	Name      string
	MaxSide   int
	MaxPixels int64
	MaxBytes  int64
}

const (
	// ProfileProcessingInput is the conservative profile shared by image
	// processing endpoints. A derivative rendered to this profile is safe for
	// providers with tighter limits while remaining useful for higher-capacity
	// providers.
	ProfileProcessingInput   = "processing-input"
	ProcessingInputMaxSide   = 2048
	ProcessingInputMaxPixels = 4 * 1024 * 1024
	ProcessingInputMaxBytes  = 20 * 1024 * 1024
)

var InputProfiles = map[string]InputProfile{
	ProfileProcessingInput: {
		Name:      ProfileProcessingInput,
		MaxSide:   ProcessingInputMaxSide,
		MaxPixels: ProcessingInputMaxPixels,
		MaxBytes:  ProcessingInputMaxBytes,
	},
}

func ProcessingInputProfile() InputProfile {
	return InputProfiles[ProfileProcessingInput]
}

func (p InputProfile) ValidateInputFacts(width, height int, sizeBytes int64) error {
	if p.MaxSide <= 0 || p.MaxPixels <= 0 || p.MaxBytes <= 0 {
		return fmt.Errorf("图片处理输入规格未配置")
	}
	if width <= 0 || height <= 0 {
		return fmt.Errorf("图片尺寸无效: %dx%d", width, height)
	}
	if width > p.MaxSide || height > p.MaxSide {
		return fmt.Errorf("图片尺寸超过处理输入限制: %dx%d (最大边长 %d)", width, height, p.MaxSide)
	}
	if int64(width)*int64(height) > p.MaxPixels {
		return fmt.Errorf("图片总像素超过处理输入限制: %dx%d (最大像素 %d)", width, height, p.MaxPixels)
	}
	if sizeBytes > p.MaxBytes {
		return fmt.Errorf("图片体积超过处理输入限制: %d 字节 (最大 %d 字节)", sizeBytes, p.MaxBytes)
	}
	return nil
}
