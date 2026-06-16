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

	"ideaboard/api/internal/accounts"
	"ideaboard/api/internal/platform/media"
	"ideaboard/api/internal/platform/mediarefs"
	"ideaboard/api/internal/platform/outbound"
	"ideaboard/api/internal/platform/safehttp"
	"ideaboard/api/internal/workflow"
)

const (
	maxGoogleInlineImageBytes = 10 * 1024 * 1024
	maxGoogleDataURLBytes     = maxGoogleInlineImageBytes
	googleAPIVersionPath      = "/v1beta"
)

type googleGeminiRequest struct {
	Contents         []googleGeminiContent        `json:"contents"`
	GenerationConfig googleGeminiGenerationConfig `json:"generationConfig"`
}

type googleGeminiContent struct {
	Parts []googleGeminiPart `json:"parts"`
}

type googleGeminiPart struct {
	Text       string                  `json:"text,omitempty"`
	InlineData *googleGeminiInlineData `json:"inlineData,omitempty"`
}

type googleGeminiInlineData struct {
	MIMEType      string `json:"mimeType,omitempty"`
	MIMETypeSnake string `json:"mime_type,omitempty"`
	Data          string `json:"data"`
}

func (d googleGeminiInlineData) normalizedMIMEType() string {
	if strings.TrimSpace(d.MIMEType) != "" {
		return strings.TrimSpace(d.MIMEType)
	}
	if strings.TrimSpace(d.MIMETypeSnake) != "" {
		return strings.TrimSpace(d.MIMETypeSnake)
	}
	return "image/png"
}

type googleGeminiGenerationConfig struct {
	ResponseModalities []string `json:"responseModalities"`
}

type googleGeminiResponse struct {
	Candidates []googleGeminiCandidate `json:"candidates"`
}

type googleGeminiCandidate struct {
	Content googleGeminiContentResponse `json:"content"`
}

type googleGeminiContentResponse struct {
	Parts []googleGeminiPartResponse `json:"parts"`
}

type googleGeminiPartResponse struct {
	InlineData      *googleGeminiInlineData `json:"inlineData"`
	InlineDataSnake *googleGeminiInlineData `json:"inline_data"`
}

func (p googleGeminiPartResponse) imageData() *googleGeminiInlineData {
	if p.InlineData != nil {
		return p.InlineData
	}
	return p.InlineDataSnake
}

type googleImagenRequest struct {
	Instances  []googleImagenInstance `json:"instances"`
	Parameters googleImagenParameters `json:"parameters"`
}

type googleImagenInstance struct {
	Prompt string `json:"prompt"`
}

type googleImagenParameters struct {
	SampleCount int    `json:"sampleCount"`
	AspectRatio string `json:"aspectRatio,omitempty"`
}

type googleImagenResponse struct {
	Predictions []googleImagenPrediction `json:"predictions"`
}

type googleImagenPrediction struct {
	BytesBase64Encoded      string                  `json:"bytesBase64Encoded"`
	BytesBase64EncodedSnake string                  `json:"bytes_base64_encoded"`
	Image                   *googleImagenPrediction `json:"image"`
}

func (p googleImagenPrediction) base64Data() string {
	for _, value := range []string{p.BytesBase64Encoded, p.BytesBase64EncodedSnake} {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	if p.Image != nil {
		return p.Image.base64Data()
	}
	return ""
}

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

func buildGoogleModelEndpoint(baseURL, model, method string) string {
	trimmedBaseURL := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if hasGoogleAPIVersionPath(trimmedBaseURL) {
		return fmt.Sprintf("%s/models/%s:%s", trimmedBaseURL, url.PathEscape(model), method)
	}
	return fmt.Sprintf("%s%s/models/%s:%s", trimmedBaseURL, googleAPIVersionPath, url.PathEscape(model), method)
}

func hasGoogleAPIVersionPath(baseURL string) bool {
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return false
	}
	return strings.HasSuffix(strings.TrimRight(parsed.EscapedPath(), "/"), googleAPIVersionPath)
}

func (c *ProviderExecutor) callGoogleGeminiGenerateContent(ctx context.Context, account *accounts.ApiAccount, baseURL string, providerReq providerRequest, timeout time.Duration, requestedModel string) (providerCallResult, error) {
	model := strings.TrimSpace(requestedModel)

	parts := []googleGeminiPart{{Text: providerReq.Prompt}}
	inlineParts, err := c.collectGoogleInlineImageParts(ctx, providerReq, timeout)
	if err != nil {
		return providerCallResult{}, err
	}
	parts = append(parts, inlineParts...)

	reqBody := googleGeminiRequest{
		Contents: []googleGeminiContent{
			{Parts: parts},
		},
		GenerationConfig: googleGeminiGenerationConfig{
			ResponseModalities: []string{"IMAGE", "TEXT"},
		},
	}

	endpoint := buildGoogleModelEndpoint(baseURL, model, "generateContent")
	bodyJSON, marshalErr := json.Marshal(reqBody)
	if marshalErr != nil {
		return providerCallResult{}, fmt.Errorf("序列化 Google Gemini 请求失败: %w", marshalErr)
	}
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

func (c *ProviderExecutor) collectGoogleInlineImageParts(ctx context.Context, providerReq providerRequest, timeout time.Duration) ([]googleGeminiPart, error) {
	refs := providerReq.MediaRefs.GoogleImages
	if len(refs) == 0 {
		return nil, nil
	}

	parts := make([]googleGeminiPart, 0, len(refs))
	for _, ref := range refs {
		mimeType, data, err := c.resolveGoogleImageBytes(ctx, ref, timeout)
		if err != nil {
			return nil, err
		}
		parts = append(parts, googleGeminiPart{
			InlineData: &googleGeminiInlineData{
				MIMEType: mimeType,
				Data:     base64.StdEncoding.EncodeToString(data),
			},
		})
	}
	return parts, nil
}

func collectGoogleImageRefs(params map[string]interface{}) []string {
	return mediarefs.CollectUniqueParamRefs(params, googleImageInputRefKeys)
}

func (c *ProviderExecutor) resolveGoogleImageBytes(ctx context.Context, ref string, timeout time.Duration) (string, []byte, error) {
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

	if obj, ok, err := c.readRelativeUploadObject(ctx, ref, maxGoogleInlineImageBytes); ok {
		if err != nil {
			return "", nil, fmt.Errorf("读取上传图像失败: %w", err)
		}
		if err := media.ValidateImageDimensions(obj.Data); err != nil {
			return "", nil, err
		}
		mimeType := strings.TrimSpace(obj.ContentType)
		if !strings.HasPrefix(strings.ToLower(mimeType), "image/") {
			mimeType = inferLocalAssetMimeType(obj.Filename, obj.Data)
		}
		return mimeType, obj.Data, nil
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
		body, readErr := readLimitedBody(resp.Body)
		if readErr != nil {
			slog.Warn("read error response body failed", "err", readErr)
		}
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
	var respData googleGeminiResponse
	if err := json.Unmarshal(respBody, &respData); err != nil {
		return nil, fmt.Errorf("解析响应失败: %v", err)
	}

	var urls []string
	for _, candidate := range respData.Candidates {
		for _, part := range candidate.Content.Parts {
			inline := part.imageData()
			if inline == nil {
				continue
			}
			data := inline.Data
			if strings.TrimSpace(data) == "" {
				continue
			}
			urls = append(urls, "data:"+inline.normalizedMIMEType()+";base64,"+strings.TrimSpace(data))
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
	reqBody := googleImagenRequest{
		Instances: []googleImagenInstance{
			{Prompt: providerReq.Prompt},
		},
		Parameters: googleImagenParameters{
			SampleCount: sampleCount,
		},
	}
	if ratio, ok := parseGoogleAspectRatio(providerReq.Params); ok {
		reqBody.Parameters.AspectRatio = ratio
	}

	endpoint := buildGoogleModelEndpoint(baseURL, model, "predict")
	bodyJSON, marshalErr := json.Marshal(reqBody)
	if marshalErr != nil {
		return providerCallResult{}, fmt.Errorf("序列化 Google Imagen 请求失败: %w", marshalErr)
	}
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
	var respData googleImagenResponse
	if err := json.Unmarshal(respBody, &respData); err != nil {
		return nil, fmt.Errorf("解析响应失败: %v", err)
	}

	urls := make([]string, 0, len(respData.Predictions))
	for _, prediction := range respData.Predictions {
		if base64Data := prediction.base64Data(); strings.TrimSpace(base64Data) != "" {
			urls = append(urls, "data:image/png;base64,"+strings.TrimSpace(base64Data))
		}
	}
	if len(urls) == 0 {
		return nil, errors.New("google imagen 未返回图片数据")
	}
	return urls, nil
}
