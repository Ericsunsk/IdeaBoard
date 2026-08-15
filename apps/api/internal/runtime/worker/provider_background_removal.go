package worker

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"ideaboard/api/internal/accountpool/accounts"
	"ideaboard/api/internal/accountpool/providerendpoints"
	"ideaboard/api/internal/platform/outbound"
	"ideaboard/api/internal/platform/safehttp"
)

// callBackgroundRemoval posts the canonical image-url-json-to-png contract.
// The response stays in-memory as a data URL so the existing result materializer
// can persist it with the same TaskOutput and cleanup semantics as other images.
func (c *ProviderExecutor) callBackgroundRemoval(
	ctx context.Context,
	account *accounts.ApiAccount,
	request providerRequest,
	timeout time.Duration,
) (providerCallResult, error) {
	if account == nil || strings.TrimSpace(account.ProxyURL) == "" {
		return providerCallResult{}, fmt.Errorf("background-removal endpoint 未配置")
	}
	inputURL, err := c.backgroundRemovalInputURL(request)
	if err != nil {
		return providerCallResult{}, err
	}
	endpoint, err := outbound.ValidateConfiguredEndpoint(account.ProxyURL, outbound.AllowHosts(ctx)...)
	if err != nil {
		return providerCallResult{}, err
	}
	bodyJSON, err := json.Marshal(map[string]string{"image_url": inputURL})
	if err != nil {
		return providerCallResult{}, fmt.Errorf("序列化抠图请求失败: %w", err)
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
		return providerCallResult{}, fmt.Errorf("抠图 HTTP 请求失败: %w", err)
	}
	defer resp.Body.Close()
	data, err := readLimitedBody(resp.Body)
	if err != nil {
		return providerCallResult{}, fmt.Errorf("读取抠图响应失败: %w", err)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return providerCallResult{}, newUpstreamHTTPErrorFromResponse(resp, truncateErrorBody(string(data)))
	}
	if len(data) == 0 {
		return providerCallResult{}, fmt.Errorf("抠图响应为空")
	}
	contentType := strings.TrimSpace(resp.Header.Get("Content-Type"))
	if contentType == "" || !strings.HasPrefix(strings.ToLower(contentType), "image/") {
		contentType = "image/png"
	}
	return providerCallResult{
		ImageURL:          "data:" + contentType + ";base64," + base64.StdEncoding.EncodeToString(data),
		ParsedOutputKind:  "image-results",
		ParsedOutputCount: 1,
	}, nil
}

func (c *ProviderExecutor) backgroundRemovalInputURL(request providerRequest) (string, error) {
	ref := ""
	if len(request.ImageRefs) > 0 {
		ref = request.ImageRefs[0]
	} else {
		ref = request.CanvasRef
	}
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "", fmt.Errorf("抠图请求缺少原图")
	}
	parsed, err := url.Parse(ref)
	if err != nil {
		return "", fmt.Errorf("抠图原图地址无效: %w", err)
	}
	if parsed.Scheme == "" && strings.HasPrefix(ref, "/uploads/") {
		base := c.publicOrigins.APIParsedURL()
		if base == nil {
			return "", fmt.Errorf("无法将上传图片转换为公网 URL")
		}
		parsed = base.ResolveReference(parsed)
	}
	if !strings.EqualFold(parsed.Scheme, "http") && !strings.EqualFold(parsed.Scheme, "https") {
		return "", fmt.Errorf("抠图原图仅支持 http/https URL")
	}
	if parsed.Host == "" {
		return "", fmt.Errorf("抠图原图地址无效")
	}
	return parsed.String(), nil
}

func isBackgroundRemovalProtocol(account *accounts.ApiAccount) bool {
	return account != nil && strings.TrimSpace(account.Protocol) == providerendpoints.ProtocolImageURLJSONPNG
}
