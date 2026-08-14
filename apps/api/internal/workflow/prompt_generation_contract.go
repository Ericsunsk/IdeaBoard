package workflow

import (
	"fmt"

	"ideaboard/api/internal/media/mediarefs"
	"ideaboard/api/internal/workflow/paramkeys"
)

const MaxPromptGenerationReferenceImages = 4

// ValidatePromptGenerationSubmitContract keeps execution mode derived from
// the ordered reference set. Callers cannot override the compiler primitive.
func ValidatePromptGenerationSubmitContract(params map[string]interface{}) error {
	if params == nil {
		return nil
	}
	for _, key := range []string{"mode", "primitive"} {
		if _, exists := params[key]; exists {
			return fmt.Errorf("prompt-generation 不接受显式 %s，生成模式由参考图推导", key)
		}
	}
	for _, key := range []string{
		paramkeys.ReferenceImageURLs,
		paramkeys.OriginalImageURL,
		paramkeys.MaskImageURL,
		paramkeys.UseMask,
	} {
		if _, exists := params[key]; exists {
			return fmt.Errorf("prompt-generation 参考图必须只使用 image_urls")
		}
	}
	imageURLs := mediarefs.CollectStringList(params[paramkeys.ImageURLs])
	if len(imageURLs) > MaxPromptGenerationReferenceImages {
		return fmt.Errorf("prompt-generation 最多支持 %d 张参考图", MaxPromptGenerationReferenceImages)
	}
	return nil
}
