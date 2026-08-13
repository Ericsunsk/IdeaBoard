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
	"strings"
	"time"

	"ideaboard/api/internal/accountpool/accounts"
	"ideaboard/api/internal/media/imageformat"
	"ideaboard/api/internal/media/mediarefs"
	"ideaboard/api/internal/platform/outbound"
	"ideaboard/api/internal/platform/safehttp"
	"ideaboard/api/internal/workflow"
)

const (
	maxGoogleInlineImageBytes = 10 * 1024 * 1024
	maxGoogleDataURLBytes     = maxGoogleInlineImageBytes
)

type googleAPIContract struct {
	DefaultBaseURL  string
	VersionPath     string
	ModelCollection string
	SupportsImagen  bool
}

func googleContract(apiType string) googleAPIContract {
	if workflow.IsGoogleCloudAPIType(apiType) {
		return googleAPIContract{
			DefaultBaseURL:  "https://aiplatform.googleapis.com",
			VersionPath:     "/v1",
			ModelCollection: "/v1/publishers/google/models",
		}
	}
	return googleAPIContract{
		DefaultBaseURL:  "https://generativelanguage.googleapis.com",
		VersionPath:     "/v1beta",
		ModelCollection: "/v1beta/models",
		SupportsImagen:  true,
	}
}

type googleGeminiRequest struct {
	Contents         []googleGeminiContent        `json:"contents"`
	GenerationConfig googleGeminiGenerationConfig `json:"generationConfig"`
}

type googleGeminiContent struct {
	Role  string             `json:"role,omitempty"`
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

type googleProviderParams struct {
	Model       string
	UseImagen   bool
	ImageRefs   []string
	SampleCount int
	AspectRatio string
}

func readGoogleProviderParams(providerReq providerRequest) googleProviderParams {
	model := resolveGoogleProviderModel(providerReq.Params, providerReq.Model)
	return googleProviderParams{
		Model:       model,
		UseImagen:   isGoogleImagenModel(model),
		ImageRefs:   providerReq.MediaRefs.GoogleImages,
		SampleCount: providerReq.OutputCount,
		AspectRatio: parseGoogleAspectRatioValue(providerReq.Params),
	}
}

func resolveGoogleProviderModel(params map[string]interface{}, resolvedModel string) string {
	if strings.TrimSpace(resolvedModel) != "" {
		return normalizeGoogleModelName(resolvedModel)
	}
	if fromParams, ok := workflow.ReadWorkflowStringParam(params, "model"); ok && strings.TrimSpace(fromParams) != "" {
		return normalizeGoogleModelName(fromParams)
	}
	return ""
}

func (c *ProviderExecutor) callGoogleAIWithRequest(ctx context.Context, account *accounts.ApiAccount, providerReq providerRequest, timeout time.Duration) (providerCallResult, error) {
	return c.callGoogleProviderWithRequest(ctx, account, workflow.ProviderGoogleAI, providerReq, timeout)
}

func (c *ProviderExecutor) callGoogleProviderWithRequest(ctx context.Context, account *accounts.ApiAccount, apiType string, providerReq providerRequest, timeout time.Duration) (providerCallResult, error) {
	contract := googleContract(apiType)
	baseURL := contract.DefaultBaseURL
	if strings.TrimSpace(account.ProxyURL) != "" {
		baseURL = strings.TrimRight(strings.TrimSpace(account.ProxyURL), "/")
	}
	validatedBaseURL, validateErr := outbound.ValidateConfiguredEndpoint(baseURL, outbound.AllowHosts(ctx)...)
	if validateErr != nil {
		return providerCallResult{}, validateErr
	}
	baseURL = validatedBaseURL

	googleParams := readGoogleProviderParams(providerReq)
	if googleParams.UseImagen {
		if !contract.SupportsImagen {
			return providerCallResult{}, errors.New("API Key 模式仅支持 Gemini 模型，请选择 Gemini 图像模型")
		}
		if len(googleParams.ImageRefs) > 0 {
			return providerCallResult{}, errors.New("google imagen 当前不支持图像参考输入，请选择 gemini 图像模型")
		}
		return c.callGoogleImagenPredict(ctx, account, baseURL, providerReq, timeout, googleParams)
	}
	return c.callGoogleGeminiGenerateContentForAPIType(ctx, account, apiType, baseURL, providerReq, timeout, googleParams)
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
	return buildGoogleProviderModelEndpoint(baseURL, workflow.ProviderGoogleAI, model, method)
}

func buildGoogleProviderModelEndpoint(baseURL, apiType, model, method string) string {
	collectionURL := buildGoogleModelCollectionURL(baseURL, apiType)
	return fmt.Sprintf("%s/%s:%s", collectionURL, url.PathEscape(model), method)
}

func buildGoogleModelCollectionURL(baseURL, apiType string) string {
	trimmedBaseURL := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	contract := googleContract(apiType)
	parsed, err := url.Parse(trimmedBaseURL)
	if err != nil {
		return trimmedBaseURL + contract.ModelCollection
	}
	basePath := strings.TrimRight(parsed.EscapedPath(), "/")
	if strings.HasSuffix(basePath, contract.ModelCollection) {
		return trimmedBaseURL
	}
	if strings.HasSuffix(basePath, contract.VersionPath) {
		return trimmedBaseURL + strings.TrimPrefix(contract.ModelCollection, contract.VersionPath)
	}
	return trimmedBaseURL + contract.ModelCollection
}

func (c *ProviderExecutor) callGoogleGeminiGenerateContent(ctx context.Context, account *accounts.ApiAccount, baseURL string, providerReq providerRequest, timeout time.Duration, googleParams googleProviderParams) (providerCallResult, error) {
	return c.callGoogleGeminiGenerateContentForAPIType(ctx, account, workflow.ProviderGoogleAI, baseURL, providerReq, timeout, googleParams)
}

func (c *ProviderExecutor) callGoogleGeminiGenerateContentForAPIType(ctx context.Context, account *accounts.ApiAccount, apiType, baseURL string, providerReq providerRequest, timeout time.Duration, googleParams googleProviderParams) (providerCallResult, error) {
	model := strings.TrimSpace(googleParams.Model)

	parts := []googleGeminiPart{{Text: providerReq.Prompt}}
	inlineParts, err := c.collectGoogleInlineImageParts(ctx, googleParams.ImageRefs, timeout)
	if err != nil {
		return providerCallResult{}, err
	}
	parts = append(parts, inlineParts...)

	reqBody := googleGeminiRequest{
		Contents: []googleGeminiContent{
			{Role: "user", Parts: parts},
		},
		GenerationConfig: googleGeminiGenerationConfig{
			ResponseModalities: []string{"TEXT", "IMAGE"},
		},
	}

	endpoint := buildGoogleProviderModelEndpoint(baseURL, apiType, model, "generateContent")
	bodyJSON, marshalErr := json.Marshal(reqBody)
	if marshalErr != nil {
		return providerCallResult{}, fmt.Errorf("序列化 Google Gemini 请求失败: %w", marshalErr)
	}
	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewBuffer(bodyJSON))
	if err != nil {
		return providerCallResult{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	setGoogleAPIKey(req, apiType, account.ApiKey)

	slog.Info("Google Gemini request", "endpoint", redactLogURL(endpoint), "model", model, "inline_images", len(inlineParts))
	resp, err := safehttp.DoRequestWithAllowHosts(ctx, req, timeout, outbound.AllowHosts(ctx)...)
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
		return providerCallResult{}, newUpstreamHTTPErrorFromResponse(resp, truncateErrorBody(string(respBody)))
	}

	imageURLs, extractErr := extractGoogleGeminiImageResponses(respBody)
	if extractErr != nil {
		return providerCallResult{
			Model:            model,
			RawOutputPreview: providerRawOutputPreview(respBody),
			ParsedOutputKind: "image-results",
		}, newProviderOutputParseError(extractErr)
	}
	return providerCallResult{
		ImageURL:          imageURLs[0],
		ImageURLs:         imageURLs,
		Model:             model,
		RawOutputPreview:  providerRawOutputPreview(respBody),
		ParsedOutputKind:  "image-results",
		ParsedOutputCount: len(imageURLs),
	}, nil
}

func setGoogleAPIKey(req *http.Request, apiType, apiKey string) {
	if workflow.IsGoogleCloudAPIType(apiType) {
		query := req.URL.Query()
		query.Set("key", apiKey)
		req.URL.RawQuery = query.Encode()
		return
	}
	req.Header.Set("x-goog-api-key", apiKey)
}

func (c *ProviderExecutor) collectGoogleInlineImageParts(ctx context.Context, refs []string, timeout time.Duration) ([]googleGeminiPart, error) {
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

func (c *ProviderExecutor) resolveGoogleImageBytes(ctx context.Context, ref string, timeout time.Duration) (string, []byte, error) {
	ref = strings.TrimSpace(ref)
	if strings.HasPrefix(ref, "data:") {
		parsed, err := mediarefs.ParseDataImageURL(ref, maxGoogleDataURLBytes)
		if err != nil {
			return "", nil, fmt.Errorf("解析 data URL 失败: %w", err)
		}
		if err := imageformat.ValidateImageDimensions(parsed.Data); err != nil {
			return "", nil, err
		}
		return parsed.MIMEType, parsed.Data, nil
	}

	if obj, ok, err := c.readRelativeUploadObject(ctx, ref, maxGoogleInlineImageBytes); ok {
		if err != nil {
			return "", nil, fmt.Errorf("读取上传图像失败: %w", err)
		}
		if err := imageformat.ValidateImageDimensions(obj.Data); err != nil {
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
	allowHosts := trustedRuntimeAssetHosts(c.publicOrigins)
	if len(allowHosts) == 0 {
		return "", nil, fmt.Errorf("远程图像输入仅支持可信资产域名")
	}
	resp, err := safehttp.GetWithAllowHosts(ctx, ref, timeout, allowHosts...)
	if err != nil {
		return "", nil, fmt.Errorf("拉取图像失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, readErr := readLimitedImageRef(resp.Body)
		if readErr != nil {
			slog.Warn("read error response body failed", "err", readErr)
		}
		return "", nil, fmt.Errorf("拉取图像返回 %d: %s", resp.StatusCode, truncateErrorBody(string(body)))
	}
	body, err := readLimitedImageRef(resp.Body)
	if err != nil {
		return "", nil, err
	}
	if len(body) == 0 {
		return "", nil, fmt.Errorf("拉取图像为空: %q", ref)
	}
	if err := imageformat.ValidateImageDimensions(body); err != nil {
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

func (c *ProviderExecutor) callGoogleImagenPredict(ctx context.Context, account *accounts.ApiAccount, baseURL string, providerReq providerRequest, timeout time.Duration, googleParams googleProviderParams) (providerCallResult, error) {
	model := strings.TrimSpace(googleParams.Model)
	reqBody := googleImagenRequest{
		Instances: []googleImagenInstance{
			{Prompt: providerReq.Prompt},
		},
		Parameters: googleImagenParameters{
			SampleCount: googleParams.SampleCount,
		},
	}
	if googleParams.AspectRatio != "" {
		reqBody.Parameters.AspectRatio = googleParams.AspectRatio
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

	slog.Info("Google Imagen request", "endpoint", redactLogURL(endpoint), "model", model, "sample_count", googleParams.SampleCount)
	resp, err := safehttp.DoRequestWithAllowHosts(ctx, req, timeout, outbound.AllowHosts(ctx)...)
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
		return providerCallResult{}, newUpstreamHTTPErrorFromResponse(resp, truncateErrorBody(string(respBody)))
	}

	imageURLs, err := extractGoogleImagenResponses(respBody)
	if err != nil {
		return providerCallResult{
			Model:            model,
			RawOutputPreview: providerRawOutputPreview(respBody),
			ParsedOutputKind: "image-results",
		}, newProviderOutputParseError(err)
	}
	return providerCallResult{
		ImageURL:          imageURLs[0],
		ImageURLs:         imageURLs,
		Model:             model,
		RawOutputPreview:  providerRawOutputPreview(respBody),
		ParsedOutputKind:  "image-results",
		ParsedOutputCount: len(imageURLs),
	}, nil
}

func parseGoogleAspectRatioValue(params map[string]interface{}) string {
	if params == nil {
		return ""
	}
	raw, ok := workflow.ReadWorkflowStringParam(params, "ratio")
	if !ok {
		return ""
	}
	ratio := strings.TrimSpace(raw)
	switch ratio {
	case "1:1", "16:9", "9:16", "3:4", "4:3":
		return ratio
	default:
		return ""
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
