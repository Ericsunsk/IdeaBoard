package providerendpoints

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"ideaboard/api/internal/platform/database"
	"ideaboard/api/internal/platform/outbound"
	"ideaboard/api/internal/platform/response"
	"ideaboard/api/internal/platform/safehttp"
	"ideaboard/api/internal/workflow"

	"github.com/gin-gonic/gin"
)

const (
	// Google Cloud requests can traverse a local TUN/proxy route whose first
	// DNS/TLS connection regularly exceeds five seconds on macOS.
	detectModelsTimeout        = 15 * time.Second
	maxDetectResponseSize      = 1 << 20 // 1MB
	maxDetectedModels          = 100
	googleCloudCountTokensBody = `{"contents":[{"role":"user","parts":[{"text":"test"}]}]}`
)

var googleCloudImageModelCandidates = []string{
	"gemini-3.1-flash-lite-image",
	"gemini-3.1-flash-image",
	"gemini-2.5-flash-image",
}

type providerEndpointListResponse struct {
	List []*ProviderEndpoint `json:"list"`
}

type providerEndpointResponse struct {
	ProviderEndpoint *ProviderEndpoint `json:"provider_endpoint"`
}

type detectedModelsResponse struct {
	Models  []string `json:"models"`
	APIType string   `json:"api_type"`
}

type googleAPIErrorResponse struct {
	Error struct {
		Status  string                 `json:"status"`
		Details []googleAPIErrorDetail `json:"details"`
	} `json:"error"`
}

type googleAPIErrorDetail struct {
	Reason   string            `json:"reason"`
	Metadata map[string]string `json:"metadata"`
}

type googleCloudModelProbeResult struct {
	model        string
	detectionURL string
	statusCode   int
	body         []byte
	err          error
}

type ProviderEndpointHandler struct {
	endpointRepo *ProviderEndpointRepository
	appEnv       string
}

func NewProviderEndpointHandler(endpointRepo *ProviderEndpointRepository, appEnv ...string) *ProviderEndpointHandler {
	env := ""
	if len(appEnv) > 0 {
		env = strings.TrimSpace(appEnv[0])
	}
	return &ProviderEndpointHandler{endpointRepo: endpointRepo, appEnv: env}
}

func (h *ProviderEndpointHandler) List(c *gin.Context) {
	endpoints, err := h.endpointRepo.List(c.Request.Context())
	if err != nil {
		response.Error(c, 500, "获取 ProviderEndpoint 列表失败")
		return
	}
	response.SuccessDTO(c, providerEndpointListResponse{List: endpoints})
}

func (h *ProviderEndpointHandler) Create(c *gin.Context) {
	var req CreateProviderEndpointRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, "参数错误")
		return
	}
	if !isValidProviderEndpointStatus(req.Status) {
		response.Error(c, 400, "status 只支持 0 或 1")
		return
	}

	baseURL, err := outbound.ValidateConfiguredEndpoint(req.BaseURL, outbound.AllowHosts(c.Request.Context())...)
	if err != nil {
		response.Error(c, 400, "base_url 不可用，仅支持可达的公网 http(s) 地址")
		return
	}

	apiType, ok := workflow.ValidateProviderAPIType(req.APIType, baseURL)
	if !ok {
		response.Error(c, 400, "接口类型不支持")
		return
	}

	endpoint := &ProviderEndpoint{
		APIType: apiType,
		Name:    req.Name,
		BaseURL: baseURL,
		Status:  req.Status,
	}

	if err := h.endpointRepo.Create(c.Request.Context(), endpoint); err != nil {
		if respondProviderEndpointConflict(c, err) {
			return
		}
		response.InternalError(c, 500, "创建 ProviderEndpoint 失败", err)
		return
	}

	response.SuccessDTO(c, providerEndpointResponse{ProviderEndpoint: endpoint})
}

func (h *ProviderEndpointHandler) Update(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.Error(c, 400, "无效的ID")
		return
	}

	var req UpdateProviderEndpointRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, "参数错误")
		return
	}
	if !isValidProviderEndpointStatus(req.Status) {
		response.Error(c, 400, "status 只支持 0 或 1")
		return
	}

	baseURL, err := outbound.ValidateConfiguredEndpoint(req.BaseURL, outbound.AllowHosts(c.Request.Context())...)
	if err != nil {
		response.Error(c, 400, "base_url 不可用，仅支持可达的公网 http(s) 地址")
		return
	}

	endpoint, err := h.endpointRepo.GetByID(c.Request.Context(), id)
	if err != nil {
		response.Error(c, 404, "ProviderEndpoint 不存在")
		return
	}

	apiType, ok := workflow.ValidateProviderAPIType(req.APIType, baseURL)
	if !ok {
		response.Error(c, 400, "接口类型不支持")
		return
	}

	endpoint.APIType = apiType
	endpoint.Name = req.Name
	endpoint.BaseURL = baseURL
	endpoint.Status = req.Status

	if err := h.endpointRepo.Update(c.Request.Context(), endpoint); err != nil {
		if database.IsNotFound(err) {
			response.Error(c, 404, "ProviderEndpoint 不存在")
			return
		}
		if respondProviderEndpointConflict(c, err) {
			return
		}
		response.InternalError(c, 500, "更新 ProviderEndpoint 失败", err)
		return
	}

	response.SuccessDTO(c, providerEndpointResponse{ProviderEndpoint: endpoint})
}

func (h *ProviderEndpointHandler) Delete(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.Error(c, 400, "无效的ID")
		return
	}

	if err := h.endpointRepo.Delete(c.Request.Context(), id); err != nil {
		if database.IsNotFound(err) {
			response.Error(c, 404, "ProviderEndpoint 不存在")
			return
		}
		if errors.Is(err, ErrProviderEndpointInUse) {
			response.Error(c, 409, "ProviderEndpoint 已绑定 APIKey，不能删除")
			return
		}
		response.InternalError(c, 500, "删除 ProviderEndpoint 失败", err)
		return
	}

	response.Success(c, nil)
}

func (h *ProviderEndpointHandler) DetectModels(c *gin.Context) {
	var req DetectModelsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, "参数错误")
		return
	}

	baseURL := strings.TrimSpace(req.BaseURL)
	apiKey := strings.TrimSpace(req.APIKey)
	if baseURL == "" || apiKey == "" {
		response.Error(c, 400, "base_url 与 api_key 不能为空")
		return
	}

	apiType, ok := workflow.ValidateProviderAPIType(req.APIType, baseURL)
	if !ok {
		response.Error(c, 400, "接口类型不支持")
		return
	}
	if workflow.IsGoogleCloudAPIType(apiType) {
		models, failure := h.detectGoogleCloudImageModels(c.Request.Context(), baseURL, apiKey)
		if failure != nil {
			response.Error(c, 400, failure.publicMessage)
			return
		}
		response.SuccessDTO(c, detectedModelsResponse{Models: models, APIType: apiType})
		return
	}

	detectionURL, err := buildModelDetectionURL(baseURL, apiType)
	if err != nil {
		response.Error(c, 400, "Base URL 无效")
		return
	}

	resp, err := safehttp.DoWithAllowHosts(c.Request.Context(), http.MethodGet, detectionURL, func(r *http.Request) {
		if workflow.IsGoogleNativeAPIType(apiType) {
			r.Header.Set("x-goog-api-key", apiKey)
		} else {
			r.Header.Set("Authorization", "Bearer "+apiKey)
		}
		r.Header.Set("Accept", "application/json")
	}, detectModelsTimeout, h.detectModelsAllowHosts(c.Request.Context())...)
	if err != nil {
		failureCategory := providerModelDetectionTransportFailureCategory(err)
		slog.Warn("model detection request failed", "api_type", apiType, "detection_url", detectionURL, "failure_category", failureCategory)
		if workflow.IsGoogleNativeAPIType(apiType) {
			response.Error(c, 400, "模型检测失败：Google API 请求超时或网络不可达。请确认本机代理/TUN 已接管后端进程，或配置 HTTP_PROXY/HTTPS_PROXY")
			return
		}
		response.Error(c, 400, "模型检测失败，请检查 Base URL 或网络")
		return
	}
	defer resp.Body.Close()

	limited := &io.LimitedReader{R: resp.Body, N: maxDetectResponseSize + 1}
	body, err := io.ReadAll(limited)
	if err != nil {
		response.Error(c, 400, "模型检测失败，响应读取异常")
		return
	}
	if int64(len(body)) > maxDetectResponseSize {
		response.Error(c, 400, "模型检测失败，响应过大")
		return
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		publicMessage, reason := providerModelDetectionError(apiType, resp.StatusCode, body)
		slog.Warn("model detection upstream rejected request", "api_type", apiType, "detection_url", detectionURL, "status", resp.StatusCode, "reason", reason)
		response.Error(c, 400, publicMessage)
		return
	}

	models, err := parseModelIDs(body)
	if err != nil {
		response.Error(c, 400, "模型检测失败，返回格式不支持")
		return
	}

	response.SuccessDTO(c, detectedModelsResponse{Models: models, APIType: apiType})
}

type providerModelDetectionFailure struct {
	publicMessage string
}

func (h *ProviderEndpointHandler) detectGoogleCloudImageModels(ctx context.Context, baseURL, apiKey string) ([]string, *providerModelDetectionFailure) {
	probeCtx, cancel := context.WithTimeout(ctx, detectModelsTimeout)
	defer cancel()

	results := make(chan googleCloudModelProbeResult, len(googleCloudImageModelCandidates))
	allowHosts := h.detectModelsAllowHosts(ctx)
	for _, model := range googleCloudImageModelCandidates {
		detectionURL, err := buildGoogleCloudModelProbeURL(baseURL, model)
		if err != nil {
			return nil, &providerModelDetectionFailure{publicMessage: "Base URL 无效"}
		}
		go func(model, detectionURL string) {
			result := probeGoogleCloudImageModel(probeCtx, detectionURL, model, apiKey, allowHosts)
			results <- result
		}(model, detectionURL)
	}

	resultsByModel := make(map[string]googleCloudModelProbeResult, len(googleCloudImageModelCandidates))
	for range googleCloudImageModelCandidates {
		result := <-results
		resultsByModel[result.model] = result
	}

	models := make([]string, 0, len(googleCloudImageModelCandidates))
	for _, model := range googleCloudImageModelCandidates {
		result := resultsByModel[model]
		if result.err == nil && result.statusCode >= 200 && result.statusCode < 300 {
			models = append(models, model)
		}
	}
	if len(models) > 0 {
		return models, nil
	}

	for _, model := range googleCloudImageModelCandidates {
		result := resultsByModel[model]
		if result.err != nil {
			failureCategory := providerModelDetectionTransportFailureCategory(result.err)
			slog.Warn("Google Cloud model probe failed", "model", model, "detection_url", result.detectionURL, "failure_category", failureCategory)
			return nil, &providerModelDetectionFailure{publicMessage: "模型检测失败：Google API 请求超时或网络不可达。请确认本机代理/TUN 已接管后端进程，或配置 HTTP_PROXY/HTTPS_PROXY"}
		}
		if result.statusCode == http.StatusNotFound {
			continue
		}
		publicMessage, reason := providerModelDetectionError(workflow.ProviderGoogleCloud, result.statusCode, result.body)
		slog.Warn("Google Cloud model probe rejected", "model", model, "detection_url", result.detectionURL, "status", result.statusCode, "reason", reason)
		return nil, &providerModelDetectionFailure{publicMessage: publicMessage}
	}

	return nil, &providerModelDetectionFailure{publicMessage: "模型检测失败：当前 Google Cloud 项目没有可用的 Gemini 图像模型"}
}

func probeGoogleCloudImageModel(ctx context.Context, detectionURL, model, apiKey string, allowHosts []string) googleCloudModelProbeResult {
	result := googleCloudModelProbeResult{model: model, detectionURL: detectionURL}
	resp, err := safehttp.DoWithAllowHosts(ctx, http.MethodPost, detectionURL, func(r *http.Request) {
		query := r.URL.Query()
		query.Set("key", apiKey)
		r.URL.RawQuery = query.Encode()
		r.Header.Set("Accept", "application/json")
		r.Header.Set("Content-Type", "application/json")
		r.Body = io.NopCloser(strings.NewReader(googleCloudCountTokensBody))
		r.ContentLength = int64(len(googleCloudCountTokensBody))
	}, detectModelsTimeout, allowHosts...)
	if err != nil {
		result.err = err
		return result
	}
	defer resp.Body.Close()

	result.statusCode = resp.StatusCode
	limited := &io.LimitedReader{R: resp.Body, N: maxDetectResponseSize + 1}
	result.body, result.err = io.ReadAll(limited)
	if result.err == nil && int64(len(result.body)) > maxDetectResponseSize {
		result.err = fmt.Errorf("response exceeds size limit")
	}
	return result
}

func providerModelDetectionTransportFailureCategory(err error) string {
	switch {
	case errors.Is(err, context.Canceled):
		return "request_canceled"
	case errors.Is(err, context.DeadlineExceeded):
		return "request_timeout"
	default:
		return "network_error"
	}
}

func providerModelDetectionError(apiType string, statusCode int, body []byte) (string, string) {
	if !workflow.IsGoogleNativeAPIType(apiType) {
		return "模型检测失败，请检查 API Key 或 ProviderEndpoint 配置", ""
	}

	var payload googleAPIErrorResponse
	if err := json.Unmarshal(body, &payload); err != nil {
		return "模型检测失败，请检查 Google API Key、权限与 ProviderEndpoint 配置", ""
	}
	reason := ""
	service := ""
	for _, detail := range payload.Error.Details {
		if reason == "" {
			reason = strings.ToUpper(strings.TrimSpace(detail.Reason))
		}
		if service == "" {
			service = strings.ToLower(strings.TrimSpace(detail.Metadata["service"]))
		}
	}

	switch reason {
	case "CREDENTIALS_MISSING":
		if workflow.IsGoogleCloudAPIType(apiType) {
			return "模型检测失败：请使用关联 Service Account 的 Google Cloud API Key，并授予模型调用权限", reason
		}
	case "SERVICE_DISABLED":
		if service == "aiplatform.googleapis.com" || workflow.IsGoogleCloudAPIType(apiType) {
			return "模型检测失败：请在 Google Cloud 项目中启用 Gemini Enterprise Agent Platform API（原 Vertex AI，aiplatform.googleapis.com）", reason
		}
		return "模型检测失败：请在 Google Cloud 项目中启用 Generative Language API", reason
	case "API_KEY_INVALID", "API_KEY_NOT_FOUND":
		return "模型检测失败：Google API Key 无效或不属于当前项目", reason
	case "BILLING_DISABLED", "PROJECT_BILLING_DISABLED":
		return "模型检测失败：请为 Google Cloud 项目启用 Billing", reason
	case "API_KEY_SERVICE_BLOCKED", "API_KEY_IP_ADDRESS_BLOCKED", "API_KEY_HTTP_REFERRER_BLOCKED", "API_KEY_ANDROID_APP_BLOCKED", "API_KEY_IOS_APP_BLOCKED":
		return "模型检测失败：Google API Key restrictions 拒绝了当前服务或服务器", reason
	}

	status := strings.ToUpper(strings.TrimSpace(payload.Error.Status))
	switch status {
	case "UNAUTHENTICATED":
		return "模型检测失败：Google API Key 无效或认证失败", status
	case "PERMISSION_DENIED":
		return "模型检测失败：Google Cloud 项目权限不足，请检查 Service Account IAM 和 API Key restrictions", status
	case "RESOURCE_EXHAUSTED":
		return "模型检测失败：Google Cloud 项目配额已用尽", status
	}
	if statusCode == http.StatusTooManyRequests {
		return "模型检测失败：Google Cloud 请求过于频繁，请稍后重试", status
	}
	return "模型检测失败，请检查 Google API Key、Billing、IAM 与 ProviderEndpoint 配置", status
}

func (h *ProviderEndpointHandler) detectModelsAllowHosts(ctx context.Context) []string {
	allowHosts := outbound.AllowHosts(ctx)
	if h.isDevLikeEnvironment() {
		allowHosts = append(allowHosts, "localhost", "127.0.0.1", "::1")
	}
	return allowHosts
}

func (h *ProviderEndpointHandler) isDevLikeEnvironment() bool {
	env := strings.ToLower(strings.TrimSpace(h.appEnv))
	switch env {
	case "prod", "production", "release":
		return false
	case "dev", "development", "local":
		return true
	}

	mode := strings.ToLower(strings.TrimSpace(gin.Mode()))
	if mode == gin.DebugMode || mode == "development" {
		return true
	}
	return false
}

func isValidProviderEndpointStatus(status int16) bool {
	return status == 0 || status == 1
}

func respondProviderEndpointConflict(c *gin.Context, err error) bool {
	if errors.Is(err, ErrProviderEndpointNameConflict) {
		response.Error(c, 409, "ProviderEndpoint 名称已存在，请使用其他名称")
		return true
	}
	if errors.Is(err, ErrProviderEndpointURLConflict) {
		response.Error(c, 409, "同一接口类型下已存在相同 Base URL 的 ProviderEndpoint")
		return true
	}
	return false
}

func buildModelDetectionURL(rawBaseURL string, apiType ...string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(rawBaseURL))
	if err != nil {
		return "", err
	}
	if parsed.Scheme == "" || parsed.Hostname() == "" {
		return "", fmt.Errorf("invalid base url")
	}
	basePath := strings.TrimRight(parsed.Path, "/")
	if len(apiType) > 0 && workflow.IsGoogleNativeAPIType(apiType[0]) {
		if basePath == "" {
			parsed.Path = "/v1beta/models"
		} else if strings.HasSuffix(basePath, "/v1beta") {
			parsed.Path = basePath + "/models"
		} else {
			parsed.Path = basePath + "/v1beta/models"
		}
	} else {
		parsed.Path = basePath + "/models"
	}
	parsed.RawPath = ""
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String(), nil
}

func buildGoogleCloudModelProbeURL(rawBaseURL, model string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(rawBaseURL))
	if err != nil {
		return "", err
	}
	if parsed.Scheme == "" || parsed.Hostname() == "" || strings.TrimSpace(model) == "" {
		return "", fmt.Errorf("invalid Google Cloud model probe URL")
	}
	basePath := strings.TrimRight(parsed.Path, "/")
	modelAction := "/publishers/google/models/" + url.PathEscape(strings.TrimSpace(model)) + ":countTokens"
	if basePath == "" {
		parsed.Path = "/v1" + modelAction
	} else if strings.HasSuffix(basePath, "/v1") {
		parsed.Path = basePath + modelAction
	} else {
		parsed.Path = basePath + "/v1" + modelAction
	}
	parsed.RawPath = ""
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String(), nil
}

func parseModelIDs(body []byte) ([]string, error) {
	var payload any
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}

	var rawItems []any
	switch v := payload.(type) {
	case map[string]any:
		if data, ok := v["data"]; ok {
			rawItems = normalizeToSlice(data)
		}
		if len(rawItems) == 0 {
			if models, ok := v["models"]; ok {
				rawItems = normalizeToSlice(models)
			}
		}
	case []any:
		rawItems = v
	}

	if len(rawItems) == 0 {
		return nil, fmt.Errorf("no models found")
	}

	seen := make(map[string]struct{}, len(rawItems))
	models := make([]string, 0, len(rawItems))
	for _, item := range rawItems {
		modelID := extractModelID(item)
		if modelID == "" {
			continue
		}
		if _, ok := seen[modelID]; ok {
			continue
		}
		seen[modelID] = struct{}{}
		models = append(models, modelID)
		if len(models) >= maxDetectedModels {
			break
		}
	}

	if len(models) == 0 {
		return nil, fmt.Errorf("no valid model ids")
	}
	return models, nil
}

func normalizeToSlice(v any) []any {
	switch item := v.(type) {
	case []any:
		return item
	default:
		return nil
	}
}

func extractModelID(v any) string {
	switch item := v.(type) {
	case string:
		return normalizeDetectedModelID(item)
	case map[string]any:
		if rawID, ok := item["id"]; ok {
			if id, ok := rawID.(string); ok {
				return normalizeDetectedModelID(id)
			}
		}
		if rawName, ok := item["name"]; ok {
			if name, ok := rawName.(string); ok {
				return normalizeDetectedModelID(name)
			}
		}
	}
	return ""
}

func normalizeDetectedModelID(value string) string {
	trimmed := strings.TrimSpace(value)
	return strings.TrimPrefix(trimmed, "models/")
}
