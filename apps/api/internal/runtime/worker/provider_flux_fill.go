package worker

import (
	"context"
	"fmt"
	"math"
	"net/url"
	"strings"
	"time"

	"ideaboard/api/internal/accountpool/accounts"
	"ideaboard/api/internal/workflow"
)

const defaultFluxFillSystemPrompt = "自然补全图像内容，并保持原图的主体、风格、光照与透视一致"

func (c *ProviderExecutor) callFluxFill(ctx context.Context, account *accounts.ApiAccount, request providerRequest, operation workflow.RuntimeOperation, timeout time.Duration) (providerCallResult, error) {
	imageURL, err := c.imageTransformInputURL(request, "图像填充")
	if err != nil {
		return providerCallResult{}, err
	}
	baseURL, err := url.Parse(strings.TrimSpace(account.ProxyURL))
	if err != nil {
		return providerCallResult{}, fmt.Errorf("图像填充 endpoint 无效: %w", err)
	}
	systemPrompt := strings.TrimSpace(request.SystemPrompt)
	if systemPrompt == "" {
		systemPrompt = defaultFluxFillSystemPrompt
	}
	payload := map[string]interface{}{
		"image_url":     imageURL,
		"system_prompt": systemPrompt,
		"prompt":        strings.TrimSpace(request.UserPrompt),
		"seed":          request.Seed,
	}
	var route string
	switch operation {
	case workflow.RuntimeOperationLocalInpaint:
		if len(request.MaskRefs) != 1 {
			return providerCallResult{}, fmt.Errorf("图像填充局部重绘缺少蒙版")
		}
		maskRequest := request
		maskRequest.ImageRefs = request.MaskRefs
		maskURL, maskErr := c.imageTransformInputURL(maskRequest, "图像填充蒙版")
		if maskErr != nil {
			return providerCallResult{}, maskErr
		}
		payload["mask_url"] = maskURL
		route = "/inpaint"
	case workflow.RuntimeOperationImageExtend:
		width, height, ok := parseFluxFillSize(request.Size)
		if !ok {
			return providerCallResult{}, fmt.Errorf("图像填充扩图缺少目标尺寸")
		}
		if request.OriginalWidth <= 0 || request.OriginalHeight <= 0 || request.OpenAI.ExtendLeft == nil || request.OpenAI.ExtendTop == nil {
			return providerCallResult{}, fmt.Errorf("图像填充扩图缺少原图尺寸或扩展参数")
		}
		offsetX := int(math.Round(float64(request.OriginalWidth) * *request.OpenAI.ExtendLeft / 100))
		offsetY := int(math.Round(float64(request.OriginalHeight) * *request.OpenAI.ExtendTop / 100))
		if offsetX < 0 || offsetY < 0 || offsetX+request.OriginalWidth > width || offsetY+request.OriginalHeight > height {
			return providerCallResult{}, fmt.Errorf("图像填充扩图 geometry 无效")
		}
		payload["custom_width"], payload["custom_height"] = width, height
		payload["image_offset_x"], payload["image_offset_y"] = offsetX, offsetY
		route = "/outpaint"
	default:
		return providerCallResult{}, fmt.Errorf("图像填充不支持操作 %q", operation)
	}
	baseURL.Path = strings.TrimRight(baseURL.Path, "/") + route
	return c.callImageURLJSONImage(ctx, &accounts.ApiAccount{ProxyURL: baseURL.String(), ApiKey: account.ApiKey}, payload, timeout, "图像填充")
}

func parseFluxFillSize(value string) (int, int, bool) {
	var width, height int
	if _, err := fmt.Sscanf(strings.TrimSpace(value), "%dx%d", &width, &height); err != nil || width < 8 || height < 8 {
		return 0, 0, false
	}
	return width, height, true
}
