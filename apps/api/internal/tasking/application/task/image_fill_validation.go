package task

import (
	"fmt"
	"math"

	"ideaboard/api/internal/platform/validation"
	tasks "ideaboard/api/internal/tasking/domain"
	"ideaboard/api/internal/workflow"
)

const (
	imageFillDimensionAlignment = int64(8)
	imageFillMaxEdgePixels      = int64(4096)
	imageFillMaxPixels          = int64(2048 * 2048)
)

func validateImageFillDimensions(featureSlug string, params *tasks.WorkflowTaskParams) error {
	if !isImageFillFeature(featureSlug) {
		return nil
	}
	if params == nil || params.Controls == nil {
		return validation.New(msgValidateImageFillDimensions)
	}

	originalWidth, originalHeight, err := validateImageFillSize(
		msgValidateImageFillDimensions2,
		params.Controls.OriginalWidth,
		params.Controls.OriginalHeight,
	)
	if err != nil {
		return err
	}
	switch featureSlug {
	case workflow.FeatureSlugOutpaint:
		return validateOutpaintDimensions(params.Outpaint, originalWidth, originalHeight)
	case workflow.FeatureSlugInpaint:
		return validateInpaintDimensions(params.Controls, originalWidth, originalHeight)
	}
	return nil
}

func isImageFillFeature(featureSlug string) bool {
	return featureSlug == workflow.FeatureSlugOutpaint || featureSlug == workflow.FeatureSlugInpaint
}

func validateOutpaintDimensions(outpaint *tasks.OutpaintParams, originalWidth, originalHeight int64) error {
	if outpaint == nil {
		return validation.New(msgValidateOutpaintDimensions)
	}
	targetWidth, targetHeight, err := validateImageFillSize(
		msgValidateOutpaintDimensions2,
		float64(outpaint.TargetWidth),
		float64(outpaint.TargetHeight),
	)
	if err != nil {
		return err
	}
	if targetWidth < originalWidth || targetHeight < originalHeight {
		return validation.New(msgValidateOutpaintDimensions3)
	}
	if !isValidOutpaintOffset(outpaint, targetWidth, targetHeight, originalWidth, originalHeight) {
		return validation.New(msgValidateOutpaintDimensions4)
	}
	if targetWidth == originalWidth && targetHeight == originalHeight {
		return validation.New(msgValidateOutpaintDimensions5)
	}
	return nil
}

func isValidOutpaintOffset(outpaint *tasks.OutpaintParams, targetWidth, targetHeight, originalWidth, originalHeight int64) bool {
	offsetX := int64(outpaint.ImageOffsetX)
	offsetY := int64(outpaint.ImageOffsetY)
	return offsetX >= 0 && offsetY >= 0 &&
		offsetX <= targetWidth-originalWidth &&
		offsetY <= targetHeight-originalHeight
}

func validateInpaintDimensions(controls *tasks.GenerationControls, originalWidth, originalHeight int64) error {
	targetWidth, targetHeight, err := validateImageFillSize(
		msgValidateOutpaintDimensions2,
		controls.CustomWidth,
		controls.CustomHeight,
	)
	if err != nil {
		return err
	}
	if targetWidth != originalWidth || targetHeight != originalHeight {
		return validation.New(msgValidateInpaintDimensions)
	}
	return nil
}

func validateImageFillSize(label string, widthValue, heightValue float64) (int64, int64, error) {
	if !isPositiveInteger(widthValue) || !isPositiveInteger(heightValue) {
		return 0, 0, validation.New(fmt.Sprintf(msgValidateImageFillSize, label))
	}
	if widthValue > float64(imageFillMaxEdgePixels) || heightValue > float64(imageFillMaxEdgePixels) {
		return 0, 0, validation.New(fmt.Sprintf(msgValidateImageFillSize2, label))
	}

	width := int64(widthValue)
	height := int64(heightValue)
	alignedWidth := alignImageFillDimension(width)
	alignedHeight := alignImageFillDimension(height)
	if alignedWidth > imageFillMaxEdgePixels || alignedHeight > imageFillMaxEdgePixels || alignedWidth*alignedHeight > imageFillMaxPixels {
		return 0, 0, validation.New(fmt.Sprintf(msgValidateImageFillSize2, label))
	}
	return width, height, nil
}

func isPositiveInteger(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) && value > 0 && math.Trunc(value) == value
}

func alignImageFillDimension(value int64) int64 {
	return (value + imageFillDimensionAlignment - 1) / imageFillDimensionAlignment * imageFillDimensionAlignment
}
