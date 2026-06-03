package worker

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"api/internal/accounts"
	"api/internal/platform/media"
	"api/internal/platform/mediarefs"
	"api/internal/platform/outbound"
	"api/internal/platform/safehttp"
	"api/internal/workflow"
)

const (
	maxGoogleDataURLBytes = 10 * 1024 * 1024
)

func (c *ProviderExecutor) callGoogleAI(ctx context.Context, account *accounts.ApiAccount, prompt string, timeout time.Duration, params map[string]interface{}, resolvedModel string) (providerCallResult, error) {
	req := newProviderRequest(prompt, 0, 0, params, resolvedModel)
	return c.callGoogleAIWithRequest(ctx, account, req, timeout)
}

func (c *ProviderExecutor) callGoogleAIWithRequest(ctx context.Context, account *accounts.ApiAccount, providerReq providerRequest, timeout time.Duration) (providerCallResult, error) {
	baseURL := "https://generativelanguage.googleapis.com"
	if strings.TrimSpace(account.ProxyURL) != "" {
		baseURL = strings.TrimRight(strings.TrimSpace(account.ProxyURL), "/")
	}
	validatedBaseURL, validateErr := outbound.ValidateConfiguredEndpoint(baseURL, outbound.AllowHostsFromEnv()...)
	if validateErr != nil {
		return providerCallResult{}, validateErr
	}
	baseURL = validatedBaseURL

	requestedModel, useImagen := resolveGoogleModel(providerReq.Params, providerReq.Model)
	if useImagen {
		if len(providerReq.MediaRefs.GoogleImages) > 0 {
			return providerCallResult{}, errors.New("google imagen 当前不支持图像参考输入，请选择 gemini 图像模型")
		}
		return c.callGoogleImagenPredict(ctx, account, baseURL, providerReq, timeout, requestedModel)
	}
	return c.callGoogleGeminiGenerateContent(ctx, account, baseURL, providerReq, timeout, requestedModel)
}

func resolveGoogleModel(params map[string]interface{}, resolvedModel string) (model string, useImagen bool) {
	if fromParams, ok := workflow.ReadWorkflowStringParam(params, "model"); ok && strings.TrimSpace(fromParams) != "" {
		model = normalizeGoogleModelName(fromParams)
	} else if strings.TrimSpace(resolvedModel) != "" {
		model = normalizeGoogleModelName(resolvedModel)
	}
	return model, isGoogleImagenModel(model)
}

func normalizeGoogleModelName(model string) string {
	trimmed := strings.TrimSpace(model)
	return strings.TrimPrefix(trimmed, "models/")
}

func isGoogleImagenModel(model string) bool {
	normalized := strings.ToLower(normalizeGoogleModelName(model))
	return strings.HasPrefix(normalized, "imagen-")
}

func (c *ProviderExecutor) callGoogleGeminiGenerateContent(ctx context.Context, account *accounts.ApiAccount, baseURL string, providerReq providerRequest, timeout time.Duration, requestedModel string) (providerCallResult, error) {
	model := strings.TrimSpace(requestedModel)

	parts := []map[string]interface{}{{"text": providerReq.Prompt}}
	inlineParts, err := collectGoogleInlineImageParts(ctx, providerReq, timeout)
	if err != nil {
		return providerCallResult{}, err
	}
	parts = append(parts, inlineParts...)

	reqBody := map[string]interface{}{
		"contents": []map[string]interface{}{
			{"parts": parts},
		},
		"generationConfig": map[string]interface{}{
			"responseModalities": []string{"IMAGE", "TEXT"},
		},
	}

	endpoint := fmt.Sprintf("%s/v1beta/models/%s:generateContent", strings.TrimRight(baseURL, "/"), url.PathEscape(model))
	bodyJSON, _ := json.Marshal(reqBody)
	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewBuffer(bodyJSON))
	if err != nil {
		return providerCallResult{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-goog-api-key", account.ApiKey)

	slog.Info("Google Gemini request", "endpoint", redactLogURL(endpoint), "model", model, "inline_images", len(inlineParts))
	resp, err := safehttp.DoRequestWithAllowHosts(ctx, req, timeout, outbound.AllowHostsFromEnv()...)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return providerCallResult{}, err
		}
		return providerCallResult{}, fmt.Errorf("HTTP 请求失败: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := readLimitedBody(resp.Body)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return providerCallResult{}, err
		}
		return providerCallResult{}, fmt.Errorf("读取 API 响应失败: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return providerCallResult{}, newUpstreamHTTPError(resp.StatusCode, truncateErrorBody(string(respBody)))
	}

	imageURLs, extractErr := extractGoogleGeminiImageResponses(respBody)
	if extractErr != nil {
		return providerCallResult{}, extractErr
	}
	return providerCallResult{
		ImageURL:  imageURLs[0],
		ImageURLs: imageURLs,
		Model:     model,
	}, nil
}

func collectGoogleInlineImageParts(ctx context.Context, providerReq providerRequest, timeout time.Duration) ([]map[string]interface{}, error) {
	refs := providerReq.MediaRefs.GoogleImages
	if len(refs) == 0 {
		return nil, nil
	}

	parts := make([]map[string]interface{}, 0, len(refs))
	for _, ref := range refs {
		mimeType, data, err := resolveGoogleImageBytes(ctx, ref, timeout)
		if err != nil {
			return nil, err
		}
		parts = append(parts, map[string]interface{}{
			"inlineData": map[string]interface{}{
				"mimeType": mimeType,
				"data":     base64.StdEncoding.EncodeToString(data),
			},
		})
	}
	return parts, nil
}

func collectGoogleImageRefs(params map[string]interface{}) []string {
	return mediarefs.CollectUniqueParamRefs(params, googleImageInputRefKeys)
}

func resolveGoogleImageBytes(ctx context.Context, ref string, timeout time.Duration) (string, []byte, error) {
	ref = strings.TrimSpace(ref)
	if strings.HasPrefix(ref, "data:") {
		parsed, err := mediarefs.ParseDataImageURL(ref, maxGoogleDataURLBytes)
		if err != nil {
			return "", nil, fmt.Errorf("解析 data URL 失败: %w", err)
		}
		if err := media.ValidateImageDimensions(parsed.Data); err != nil {
			return "", nil, err
		}
		return parsed.MIMEType, parsed.Data, nil
	}

	parsedURL, err := url.Parse(ref)
	if err != nil || parsedURL == nil || parsedURL.Scheme == "" || parsedURL.Host == "" {
		return "", nil, fmt.Errorf("不支持的图像引用: %q", ref)
	}
	allowHosts := trustedOpenAIAssetHosts()
	if len(allowHosts) == 0 {
		return "", nil, fmt.Errorf("远程图像输入仅支持可信资产域名")
	}
	resp, err := safehttp.GetWithAllowHosts(ctx, ref, timeout, allowHosts...)
	if err != nil {
		return "", nil, fmt.Errorf("拉取图像失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := readLimitedBody(resp.Body)
		return "", nil, fmt.Errorf("拉取图像返回 %d: %s", resp.StatusCode, truncateErrorBody(string(body)))
	}
	body, err := readLimitedBody(resp.Body)
	if err != nil {
		return "", nil, err
	}
	if len(body) == 0 {
		return "", nil, fmt.Errorf("拉取图像为空: %q", ref)
	}
	if err := media.ValidateImageDimensions(body); err != nil {
		return "", nil, err
	}
	mimeType := strings.TrimSpace(resp.Header.Get("Content-Type"))
	if idx := strings.Index(mimeType, ";"); idx > 0 {
		mimeType = strings.TrimSpace(mimeType[:idx])
	}
	if !strings.HasPrefix(strings.ToLower(mimeType), "image/") {
		mimeType = "image/png"
	}
	return mimeType, body, nil
}

func extractGoogleGeminiImageResponses(respBody []byte) ([]string, error) {
	var respData map[string]interface{}
	if err := json.Unmarshal(respBody, &respData); err != nil {
		return nil, fmt.Errorf("解析响应失败: %v", err)
	}

	var urls []string
	candidates, _ := respData["candidates"].([]interface{})
	for _, candidate := range candidates {
		candidateObj, _ := candidate.(map[string]interface{})
		contentObj, _ := candidateObj["content"].(map[string]interface{})
		parts, _ := contentObj["parts"].([]interface{})
		for _, part := range parts {
			partObj, _ := part.(map[string]interface{})
			inline := partObj["inlineData"]
			if inline == nil {
				inline = partObj["inline_data"]
			}
			inlineObj, _ := inline.(map[string]interface{})
			data, _ := inlineObj["data"].(string)
			if strings.TrimSpace(data) == "" {
				continue
			}
			mimeType, _ := inlineObj["mimeType"].(string)
			if strings.TrimSpace(mimeType) == "" {
				mimeType, _ = inlineObj["mime_type"].(string)
			}
			if strings.TrimSpace(mimeType) == "" {
				mimeType = "image/png"
			}
			urls = append(urls, "data:"+strings.TrimSpace(mimeType)+";base64,"+strings.TrimSpace(data))
		}
	}

	if len(urls) == 0 {
		return nil, errors.New("google gemini 未返回图片数据，请确认模型支持图像输出")
	}
	return urls, nil
}

func (c *ProviderExecutor) callGoogleImagenPredict(ctx context.Context, account *accounts.ApiAccount, baseURL string, providerReq providerRequest, timeout time.Duration, requestedModel string) (providerCallResult, error) {
	model := strings.TrimSpace(requestedModel)
	sampleCount := parseGoogleSampleCount(providerReq.Params)
	reqBody := map[string]interface{}{
		"instances": []map[string]interface{}{
			{"prompt": providerReq.Prompt},
		},
		"parameters": map[string]interface{}{
			"sampleCount": sampleCount,
		},
	}
	if ratio, ok := parseGoogleAspectRatio(providerReq.Params); ok {
		reqBody["parameters"].(map[string]interface{})["aspectRatio"] = ratio
	}

	endpoint := fmt.Sprintf("%s/v1beta/models/%s:predict", strings.TrimRight(baseURL, "/"), url.PathEscape(model))
	bodyJSON, _ := json.Marshal(reqBody)
	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewBuffer(bodyJSON))
	if err != nil {
		return providerCallResult{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-goog-api-key", account.ApiKey)

	slog.Info("Google Imagen request", "endpoint", redactLogURL(endpoint), "model", model, "sample_count", sampleCount)
	resp, err := safehttp.DoRequestWithAllowHosts(ctx, req, timeout, outbound.AllowHostsFromEnv()...)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return providerCallResult{}, err
		}
		return providerCallResult{}, fmt.Errorf("HTTP 请求失败: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := readLimitedBody(resp.Body)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return providerCallResult{}, err
		}
		return providerCallResult{}, fmt.Errorf("读取 API 响应失败: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return providerCallResult{}, newUpstreamHTTPError(resp.StatusCode, truncateErrorBody(string(respBody)))
	}

	imageURLs, err := extractGoogleImagenResponses(respBody)
	if err != nil {
		return providerCallResult{}, err
	}
	return providerCallResult{
		ImageURL:  imageURLs[0],
		ImageURLs: imageURLs,
		Model:     model,
	}, nil
}

func parseGoogleSampleCount(params map[string]interface{}) int {
	if params == nil {
		return 1
	}
	parseCount := func(key string) (int, bool) {
		raw, ok := params[key]
		if !ok {
			return 0, false
		}
		switch typed := raw.(type) {
		case int:
			return typed, true
		case float64:
			return int(typed), true
		case float32:
			return int(typed), true
		case int64:
			return int(typed), true
		case int32:
			return int(typed), true
		case json.Number:
			n, err := strconv.Atoi(strings.TrimSpace(string(typed)))
			if err == nil {
				return n, true
			}
		case string:
			n, err := strconv.Atoi(strings.TrimSpace(typed))
			if err == nil {
				return n, true
			}
		}
		return 0, false
	}

	for _, key := range []string{"output_count", "sample_count"} {
		if count, ok := parseCount(key); ok {
			if count < 1 {
				return 1
			}
			if count > 4 {
				return 4
			}
			return count
		}
	}
	return 1
}

func parseGoogleAspectRatio(params map[string]interface{}) (string, bool) {
	if params == nil {
		return "", false
	}
	raw, ok := workflow.ReadWorkflowStringParam(params, "ratio")
	if !ok {
		return "", false
	}
	ratio := strings.TrimSpace(raw)
	switch ratio {
	case "1:1", "16:9", "9:16", "3:4", "4:3":
		return ratio, true
	default:
		return "", false
	}
}

func extractGoogleImagenResponses(respBody []byte) ([]string, error) {
	var respData map[string]interface{}
	if err := json.Unmarshal(respBody, &respData); err != nil {
		return nil, fmt.Errorf("解析响应失败: %v", err)
	}

	predictions, _ := respData["predictions"].([]interface{})
	urls := make([]string, 0, len(predictions))
	for _, prediction := range predictions {
		if base64Data := findGoogleImagenBase64(prediction); strings.TrimSpace(base64Data) != "" {
			urls = append(urls, "data:image/png;base64,"+strings.TrimSpace(base64Data))
		}
	}
	if len(urls) == 0 {
		return nil, errors.New("google imagen 未返回图片数据")
	}
	return urls, nil
}

func findGoogleImagenBase64(node interface{}) string {
	switch typed := node.(type) {
	case map[string]interface{}:
		for _, key := range []string{"bytesBase64Encoded", "bytes_base64_encoded"} {
			if text, ok := typed[key].(string); ok && strings.TrimSpace(text) != "" {
				return text
			}
		}
		for _, child := range typed {
			if found := findGoogleImagenBase64(child); found != "" {
				return found
			}
		}
	case []interface{}:
		for _, child := range typed {
			if found := findGoogleImagenBase64(child); found != "" {
				return found
			}
		}
	}
	return ""
}
