package taskservice

import (
	"fmt"
	"math"

	"ideaboard/api/internal/platform/validation"
	"ideaboard/api/internal/tasking/tasks"
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
		return validation.New("图像填充需要提供正整数的原图宽高和目标宽高")
	}

	originalWidth, originalHeight, err := validateImageFillSize(
		"原图",
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
		return validation.New("扩图需要提供目标尺寸和原图偏移")
	}
	targetWidth, targetHeight, err := validateImageFillSize(
		"目标图片",
		float64(outpaint.TargetWidth),
		float64(outpaint.TargetHeight),
	)
	if err != nil {
		return err
	}
	if targetWidth < originalWidth || targetHeight < originalHeight {
		return validation.New("扩图目标尺寸不能小于原图尺寸")
	}
	if !isValidOutpaintOffset(outpaint, targetWidth, targetHeight, originalWidth, originalHeight) {
		return validation.New("扩图偏移不能让原图超出目标画布")
	}
	if targetWidth == originalWidth && targetHeight == originalHeight {
		return validation.New("扩图目标尺寸必须至少扩展一个方向")
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
		"目标图片",
		controls.CustomWidth,
		controls.CustomHeight,
	)
	if err != nil {
		return err
	}
	if targetWidth != originalWidth || targetHeight != originalHeight {
		return validation.New("局部重绘的目标尺寸必须与原图尺寸一致")
	}
	return nil
}

func validateImageFillSize(label string, widthValue, heightValue float64) (int64, int64, error) {
	if !isPositiveInteger(widthValue) || !isPositiveInteger(heightValue) {
		return 0, 0, validation.New(fmt.Sprintf("%s宽高必须是正整数", label))
	}
	if widthValue > float64(imageFillMaxEdgePixels) || heightValue > float64(imageFillMaxEdgePixels) {
		return 0, 0, validation.New(fmt.Sprintf("%s超过图像填充服务的 4096 边长或约 4MP 限制", label))
	}

	width := int64(widthValue)
	height := int64(heightValue)
	alignedWidth := alignImageFillDimension(width)
	alignedHeight := alignImageFillDimension(height)
	if alignedWidth > imageFillMaxEdgePixels || alignedHeight > imageFillMaxEdgePixels || alignedWidth*alignedHeight > imageFillMaxPixels {
		return 0, 0, validation.New(fmt.Sprintf("%s超过图像填充服务的 4096 边长或约 4MP 限制", label))
	}
	return width, height, nil
}

func isPositiveInteger(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) && value > 0 && math.Trunc(value) == value
}

func alignImageFillDimension(value int64) int64 {
	return (value + imageFillDimensionAlignment - 1) / imageFillDimensionAlignment * imageFillDimensionAlignment
}
