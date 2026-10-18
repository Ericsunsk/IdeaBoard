package workflow

import (
	"errors"
	"fmt"
	"ideaboard/api/internal/media/mediarefs"
	"ideaboard/api/internal/platform/errorrender"
	"ideaboard/api/internal/workflow/paramkeys"
)

type GenerationPrimitive string

const (
	GenerationPrimitiveTextToImage  GenerationPrimitive = "text-to-image"
	GenerationPrimitiveImageToImage GenerationPrimitive = "image-to-image"
)

func ParseGenerationPrimitive(raw string) (GenerationPrimitive, error) {
	primitive := GenerationPrimitive(raw)
	switch primitive {
	case GenerationPrimitiveTextToImage, GenerationPrimitiveImageToImage:
		return primitive, nil
	default:
		return "", fmt.Errorf("generation primitive %q is invalid", raw)
	}
}

type RequestedOutputCount uint8

const (
	MinRequestedOutputCount = 1
	MaxRequestedOutputCount = 4
)

func ParseRequestedOutputCount(value int) (RequestedOutputCount, error) {
	if value < MinRequestedOutputCount || value > MaxRequestedOutputCount {
		return 0, fmt.Errorf("requested output count must be between %d and %d", MinRequestedOutputCount, MaxRequestedOutputCount)
	}
	return RequestedOutputCount(value), nil
}

func (count RequestedOutputCount) Int() int { return int(count) }

func (count RequestedOutputCount) Validate() error {
	_, err := ParseRequestedOutputCount(count.Int())
	return err
}

// merged from internal/workflow/generation_submit_contract.go (structural consolidation; symbols unchanged).
const MaxGenerationReferenceImages = 4

// ValidateGenerationSubmitContract keeps execution mode derived from the
// ordered reference set. Callers cannot override the compiler primitive.
func ValidateGenerationSubmitContract(params map[string]interface{}) error {
	if params == nil {
		return nil
	}
	for _, key := range []string{"mode", "primitive"} {
		if _, exists := params[key]; exists {
			return fmt.Errorf(errorrender.MsgValidateGenerationSubmitContract, key)
		}
	}
	for _, key := range []string{paramkeys.ReferenceImageURLs, paramkeys.MaskImageURL, paramkeys.UseMask} {
		if _, exists := params[key]; exists {
			return errors.New(errorrender.MsgValidateGenerationSubmitContract2)
		}
	}
	imageURLs := mediarefs.CollectStringList(params[paramkeys.ImageURLs])
	if len(imageURLs) > MaxGenerationReferenceImages {
		return fmt.Errorf(errorrender.MsgValidateGenerationSubmitContract3, MaxGenerationReferenceImages)
	}
	return nil
}
