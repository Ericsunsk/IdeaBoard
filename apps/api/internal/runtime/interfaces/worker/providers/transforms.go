package providers

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strings"
	"time"

	accounts "ideaboard/api/internal/accountpool/accounts/domain"
	"ideaboard/api/internal/platform/outbound"
	"ideaboard/api/internal/platform/safehttp"
	"ideaboard/api/internal/workflow"
)

// callCutout posts the image-pipeline cutout contract.
func (c *ProviderExecutor) callCutout(
	ctx context.Context,
	account *accounts.ApiAccount,
	request providerRequest,
	timeout time.Duration,
) (providerCallResult, error) {
	inputURL, err := c.imageTransformInputURL(request, "抠图")
	if err != nil {
		return providerCallResult{}, err
	}
	return c.callImageURLJSONImage(ctx, account, map[string]interface{}{"image_url": inputURL}, timeout, "抠图")
}

// callUpscale posts the dedicated Modal upscale contract. The prompt,
// output count and output resolution are intentionally absent from this
// payload: the endpoint transforms one source image into one result.
func (c *ProviderExecutor) callUpscale(
	ctx context.Context,
	account *accounts.ApiAccount,
	request providerRequest,
	timeout time.Duration,
) (providerCallResult, error) {
	if len(request.ImageRefs) == 0 || strings.TrimSpace(request.ImageRefs[0]) == "" {
		return providerCallResult{}, fmt.Errorf("高清放大请求缺少原图")
	}
	// Upscale is a single-image transform. It must not silently fall back to a
	// canvas/original reference when the ordered input list is absent.
	request.CanvasRef = ""
	inputURL, err := c.imageTransformInputURL(request, "高清放大")
	if err != nil {
		return providerCallResult{}, err
	}
	if request.Scale < 1 || request.Scale > 4 || math.Abs(request.Scale*10-math.Round(request.Scale*10)) > 1e-9 {
		return providerCallResult{}, fmt.Errorf("高清放大 scale 必须是 1.0-4.0 且步长为 0.1")
	}
	return c.callImageURLJSONImage(ctx, account, map[string]interface{}{
		"image_url":     inputURL,
		"scale":         request.Scale,
		"output_format": "png",
	}, timeout, "高清放大")
}

func (c *ProviderExecutor) callFluxFill(
	ctx context.Context,
	account *accounts.ApiAccount,
	request providerRequest,
	operation workflow.RuntimeOperation,
	timeout time.Duration,
) (providerCallResult, error) {
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

func (c *ProviderExecutor) callImageURLJSONImage(
	ctx context.Context,
	account *accounts.ApiAccount,
	payload map[string]interface{},
	timeout time.Duration,
	label string,
) (providerCallResult, error) {
	if account == nil || strings.TrimSpace(account.ProxyURL) == "" {
		return providerCallResult{}, fmt.Errorf("%s endpoint 未配置", label)
	}
	endpoint, err := outbound.ValidateConfiguredEndpoint(account.ProxyURL, outbound.AllowHosts(ctx)...)
	if err != nil {
		return providerCallResult{}, err
	}
	bodyJSON, err := json.Marshal(payload)
	if err != nil {
		return providerCallResult{}, fmt.Errorf("序列化%s请求失败: %w", label, err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(bodyJSON))
	if err != nil {
		return providerCallResult{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Api-Key", account.ApiKey)
	resp, err := safehttp.DoRequestWithAllowHosts(ctx, req, timeout, outbound.AllowHosts(ctx)...)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return providerCallResult{}, err
		}
		return providerCallResult{}, fmt.Errorf("%s HTTP 请求失败: %w", label, err)
	}
	defer resp.Body.Close()
	data, err := readLimitedBody(resp.Body)
	if err != nil {
		return providerCallResult{}, fmt.Errorf("读取%s响应失败: %w", label, err)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return providerCallResult{}, newUpstreamHTTPErrorFromResponse(resp, truncateErrorBody(string(data)))
	}
	if len(data) == 0 {
		return providerCallResult{}, fmt.Errorf("%s响应为空", label)
	}
	contentType := strings.TrimSpace(resp.Header.Get("Content-Type"))
	if contentType == "" || !strings.HasPrefix(strings.ToLower(contentType), "image/") {
		return providerCallResult{}, fmt.Errorf("%s响应 Content-Type 无效: %q", label, contentType)
	}
	return providerCallResult{
		ImageURL:          "data:" + contentType + ";base64," + base64.StdEncoding.EncodeToString(data),
		ParsedOutputKind:  "image-results",
		ParsedOutputCount: 1,
	}, nil
}

func (c *ProviderExecutor) imageTransformInputURL(request providerRequest, label string) (string, error) {
	ref := ""
	if len(request.ImageRefs) > 0 {
		ref = request.ImageRefs[0]
	} else {
		ref = request.CanvasRef
	}
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "", fmt.Errorf("%s请求缺少原图", label)
	}
	parsed, err := url.Parse(ref)
	if err != nil {
		return "", fmt.Errorf("%s原图地址无效: %w", label, err)
	}
	if parsed.Scheme == "" && strings.HasPrefix(ref, "/uploads/") {
		base := c.publicOrigins.APIParsedURL()
		if base == nil {
			return "", fmt.Errorf("无法将上传图片转换为公网 URL")
		}
		parsed = base.ResolveReference(parsed)
	}
	if !strings.EqualFold(parsed.Scheme, "https") {
		return "", fmt.Errorf("%s原图仅支持公网 HTTPS URL", label)
	}
	if parsed.Host == "" {
		return "", fmt.Errorf("%s原图地址无效", label)
	}
	return parsed.String(), nil
}
