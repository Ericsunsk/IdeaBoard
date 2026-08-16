package worker

import (
	"context"
	"errors"
	"fmt"
	"image"
	"image/draw"
	"math"
	"strings"
	"time"

	"ideaboard/api/internal/accountpool/accounts"
	"ideaboard/api/internal/tasking/taskmedia"
	"ideaboard/api/internal/tasking/tasks"
	"ideaboard/api/internal/workflow"
)

type imageExtendContext struct {
	original   *image.NRGBA
	geometry   workflow.ImageExtendPromptContract
	allowHosts []string
}

func (c *ProviderExecutor) prepareImageExtendPipeline(ctx context.Context, account *accounts.ApiAccount, params map[string]interface{}, timeout time.Duration) (*imageExtendContext, error) {
	geometry, ok := workflow.ReadImageExtendPromptContract(params)
	if !ok {
		return nil, fmt.Errorf("image extend 缺少扩图 geometry")
	}
	originalRef := readImageExtendOriginalRef(params)
	if originalRef == "" {
		return nil, fmt.Errorf("image extend 缺少原图")
	}
	allowHosts := trustedImageDownloadHosts(ctx, account, c.publicOrigins)
	original, err := c.loadImageFromRef(ctx, originalRef, timeout, allowHosts)
	if err != nil {
		return nil, fmt.Errorf("image extend 读取原图失败: %w", err)
	}
	originalNRGBA := cloneImageToNRGBA(original)
	if originalNRGBA.Bounds().Empty() {
		return nil, fmt.Errorf("image extend 原图尺寸无效")
	}
	return &imageExtendContext{
		original:   originalNRGBA,
		geometry:   geometry,
		allowHosts: allowHosts,
	}, nil
}

func readImageExtendOriginalRef(params map[string]interface{}) string {
	if original, ok := workflow.ReadWorkflowStringParam(params, "original_image_url"); ok {
		return original
	}
	for _, ref := range collectOrderedProviderRefs(params, []string{"image_urls", "image_url"}) {
		return ref
	}
	return ""
}

func (c *ProviderExecutor) finalizeImageExtendResults(ctx context.Context, userID int64, taskID string, extend *imageExtendContext, upstreamImageRefs []string, timeout time.Duration) providerResultFinalization {
	if extend == nil {
		return newProviderTerminalFailure(fmt.Errorf("image extend 后处理上下文无效"))
	}

	imageURLs := make([]string, 0, len(upstreamImageRefs))
	outputs := make([]tasks.TaskOutput, 0, len(upstreamImageRefs))
	var firstFailure error
	for resultIndex, upstreamImageRef := range upstreamImageRefs {
		imageURL, output, err := c.finalizeImageExtendResult(ctx, userID, taskID, extend, upstreamImageRef, timeout)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return newProviderTerminalFailure(err)
			}
			if firstFailure == nil {
				firstFailure = err
			}
			continue
		}
		output.ResultIndex = resultIndex
		imageURLs = append(imageURLs, imageURL)
		outputs = append(outputs, output)
	}
	if len(imageURLs) > 0 {
		return newProviderAlreadyMaterialized(imageURLs, outputs)
	}
	if firstFailure != nil {
		return newProviderTerminalFailure(firstFailure)
	}
	return newProviderTerminalFailure(newExecutionFailureError(
		tasks.ExecutionStageFinalizing,
		tasks.FailureCategoryFinalizeError,
		fmt.Errorf("image extend 未收到可合成的上游结果"),
	))
}

func (c *ProviderExecutor) finalizeImageExtendResult(ctx context.Context, userID int64, taskID string, extend *imageExtendContext, upstreamImageRef string, timeout time.Duration) (string, tasks.TaskOutput, error) {
	if extend == nil || extend.original == nil {
		return "", tasks.TaskOutput{}, fmt.Errorf("image extend 后处理上下文无效")
	}
	upstream, err := c.loadImageFromRef(ctx, upstreamImageRef, timeout, extend.allowHosts)
	if err != nil {
		return "", tasks.TaskOutput{}, newExecutionFailureError(
			tasks.ExecutionStageFinalizing,
			tasks.FailureCategoryFinalizeError,
			fmt.Errorf("image extend 读取上游结果失败: %w", err),
		)
	}
	composed, err := composeImageExtendResult(extend.original, upstream, extend.geometry)
	if err != nil {
		return "", tasks.TaskOutput{}, newExecutionFailureError(
			tasks.ExecutionStageFinalizing,
			tasks.FailureCategoryFinalizeError,
			fmt.Errorf("image extend 合成最终画布失败: %w", err),
		)
	}
	data, err := encodeImagePNG(composed)
	if err != nil {
		return "", tasks.TaskOutput{}, newExecutionFailureError(
			tasks.ExecutionStageFinalizing,
			tasks.FailureCategoryFinalizeError,
			fmt.Errorf("image extend 编码最终画布失败: %w", err),
		)
	}
	if c.resultMaterializer == nil {
		return "", tasks.TaskOutput{}, newExecutionFailureError(
			tasks.ExecutionStageFinalizing,
			tasks.FailureCategoryStorageError,
			fmt.Errorf("image extend 持久化失败: 存储服务未初始化"),
		)
	}
	filename := fmt.Sprintf("image-extend-%d.png", time.Now().UnixNano())
	materialized, err := c.resultMaterializer.MaterializeBytesResult(ctx, userID, taskID, filename, upstreamImageRef, data, taskmedia.ResultMaterializeOptions{})
	if err != nil {
		return "", tasks.TaskOutput{}, newExecutionFailureError(
			tasks.ExecutionStageFinalizing,
			tasks.FailureCategoryStorageError,
			fmt.Errorf("image extend 持久化失败: %w", err),
		)
	}
	if len(materialized.Items) != 1 || strings.TrimSpace(materialized.Items[0].ImageURL) == "" {
		return "", tasks.TaskOutput{}, newExecutionFailureError(
			tasks.ExecutionStageFinalizing,
			tasks.FailureCategoryStorageError,
			fmt.Errorf("image extend 持久化返回无效结果"),
		)
	}
	return materialized.Items[0].ImageURL, materialized.Items[0].Output, nil
}

func composeImageExtendResult(original *image.NRGBA, generated image.Image, geometry workflow.ImageExtendPromptContract) (*image.NRGBA, error) {
	if original == nil || generated == nil {
		return nil, fmt.Errorf("图像为空")
	}
	output := cloneImageToNRGBA(generated)
	if output.Bounds().Empty() {
		return nil, fmt.Errorf("上游结果尺寸无效")
	}
	originalRect, err := imageExtendOriginalRect(output.Bounds(), geometry)
	if err != nil {
		return nil, err
	}
	scaledOriginal := scaleImageToNRGBA(original, originalRect.Dx(), originalRect.Dy())
	draw.Draw(output, originalRect, scaledOriginal, scaledOriginal.Bounds().Min, draw.Src)
	return output, nil
}

func imageExtendOriginalRect(outputBounds image.Rectangle, geometry workflow.ImageExtendPromptContract) (image.Rectangle, error) {
	if outputBounds.Empty() {
		return image.Rectangle{}, fmt.Errorf("输出尺寸无效")
	}
	widthTotal := 100 + geometry.LeftPercent + geometry.RightPercent
	heightTotal := 100 + geometry.TopPercent + geometry.BottomPercent
	if widthTotal <= 100 && heightTotal <= 100 {
		return image.Rectangle{}, fmt.Errorf("扩图 geometry 无效")
	}
	width := max(1, int(math.Round(float64(outputBounds.Dx())*100/widthTotal)))
	height := max(1, int(math.Round(float64(outputBounds.Dy())*100/heightTotal)))
	left := int(math.Round(float64(outputBounds.Dx()) * geometry.LeftPercent / widthTotal))
	top := int(math.Round(float64(outputBounds.Dy()) * geometry.TopPercent / heightTotal))
	left = min(max(0, left), outputBounds.Dx()-1)
	top = min(max(0, top), outputBounds.Dy()-1)
	width = min(width, outputBounds.Dx()-left)
	height = min(height, outputBounds.Dy()-top)
	if width <= 0 || height <= 0 {
		return image.Rectangle{}, fmt.Errorf("扩图原图区域无效")
	}
	return image.Rect(left, top, left+width, top+height), nil
}
