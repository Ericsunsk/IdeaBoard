package providers

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	accounts "ideaboard/api/internal/accountpool/accounts/domain"
	"ideaboard/api/internal/workflow"
)

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
		return providerCallResult{}, fmt.Errorf("图像填充缺少 System Prompt")
	}
	payload := map[string]interface{}{
		"image_url":     imageURL,
		"system_prompt": systemPrompt,
		"prompt":        strings.TrimSpace(request.UserPrompt),
		"seed":          request.Seed,
	}
	var route string
	switch operation {
	case workflow.RuntimeOperationInpaint:
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
	case workflow.RuntimeOperationOutpaint:
		if request.Outpaint == nil || request.OriginalWidth <= 0 || request.OriginalHeight <= 0 {
			return providerCallResult{}, fmt.Errorf("图像填充扩图缺少原图尺寸或扩展参数")
		}
		width, height := request.Outpaint.TargetWidth, request.Outpaint.TargetHeight
		offsetX, offsetY := request.Outpaint.ImageOffsetX, request.Outpaint.ImageOffsetY
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
	result, err := c.callImageURLJSONImage(ctx, &accounts.ApiAccount{ProxyURL: baseURL.String(), ApiKey: account.ApiKey}, payload, timeout, "图像填充")
	return result, markProviderInvocationOutcomeUnknown(err)
}
