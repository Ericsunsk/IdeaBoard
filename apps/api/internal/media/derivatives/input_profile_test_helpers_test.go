package derivatives

import "fmt"

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
	if sizeBytes < 0 || sizeBytes > p.MaxBytes {
		return fmt.Errorf("图片大小超过处理输入限制: %d bytes (最大 %d)", sizeBytes, p.MaxBytes)
	}
	return nil
}
