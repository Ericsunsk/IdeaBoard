package workflow

import (
	"errors"
	"fmt"

	"ideaboard/api/internal/media/mediarefs"
	"ideaboard/api/internal/workflow/paramkeys"
)

const MaxGenerationReferenceImages = 4

// ValidateGenerationSubmitContract keeps execution mode derived from the
// ordered reference set. Callers cannot override the compiler primitive.
func ValidateGenerationSubmitContract(params map[string]interface{}) error {
	if params == nil {
		return nil
	}
	for _, key := range []string{"mode", "primitive"} {
		if _, exists := params[key]; exists {
			return fmt.Errorf(msgValidateGenerationSubmitContract, key)
		}
	}
	for _, key := range []string{paramkeys.ReferenceImageURLs, paramkeys.MaskImageURL, paramkeys.UseMask} {
		if _, exists := params[key]; exists {
			return errors.New(msgValidateGenerationSubmitContract2)
		}
	}
	imageURLs := mediarefs.CollectStringList(params[paramkeys.ImageURLs])
	if len(imageURLs) > MaxGenerationReferenceImages {
		return fmt.Errorf(msgValidateGenerationSubmitContract3, MaxGenerationReferenceImages)
	}
	return nil
}
