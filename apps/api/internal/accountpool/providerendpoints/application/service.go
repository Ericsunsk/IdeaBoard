package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"ideaboard/api/internal/accountpool/providerendpoints/domain"
	"ideaboard/api/internal/accountpool/providerendpoints/ports"
	"ideaboard/api/internal/platform/outbound"
	"ideaboard/api/internal/platform/safehttp"
	"ideaboard/api/internal/workflow"

	"github.com/gin-gonic/gin"
)

const (
	DetectModelsTimeout        = 15 * time.Second
	MaxDetectResponseSize      = 1 << 20 // 1MB
	MaxDetectedModels          = 100
	googleCloudCountTokensBody = `{"contents":[{"role":"user","parts":[{"text":"test"}]}]}`
)

var GoogleCloudImageModelCandidates = []string{
	"gemini-3.1-flash-lite-image",
	"gemini-3.1-flash-image",
	"gemini-2.5-flash-image",
}

type CreateEndpointCommand struct {
	APIType    string
	Protocol   string
	Capability string
	Name       string
	BaseURL    string
	APIKey     string
	Status     int16
}

type UpdateEndpointCommand struct {
	APIType    string
	Protocol   string
	Capability string
	Name       string
	BaseURL    string
	APIKey     string
	Status     int16
}

type DetectModelsCommand struct {
	BaseURL  string
	APIKey   string
	APIType  string
	Protocol string
}

type ModelDetectionError struct {
	PublicMessage string
}

func (e *ModelDetectionError) Error() string {
	return e.PublicMessage
}

type Service struct {
	repo   ports.Repository
	appEnv string
}

func NewService(repo ports.Repository, appEnv ...string) *Service {
	env := ""
	if len(appEnv) > 0 {
		env = strings.TrimSpace(appEnv[0])
	}
	return &Service{repo: repo, appEnv: env}
}

func (s *Service) List(ctx context.Context) ([]*domain.Endpoint, error) {
	return s.repo.List(ctx)
}

func (s *Service) GetByID(ctx context.Context, id int64) (*domain.Endpoint, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *Service) Create(ctx context.Context, cmd CreateEndpointCommand) (*domain.Endpoint, error) {
	if !domain.IsValidStatus(cmd.Status) {
		return nil, domain.ErrInvalidStatus
	}
	protocol, capabilities, err := domain.NormalizeProtocolCapabilities(cmd.Protocol, domain.RequestedCapabilities(cmd.Capability))
	if err != nil {
		return nil, err
	}
	if domain.IsManagedImageEndpointProtocol(protocol) && strings.TrimSpace(cmd.APIKey) == "" {
		return nil, domain.ErrAPIKeyRequired
	}

	baseURL, err := outbound.ValidateConfiguredEndpoint(cmd.BaseURL, outbound.AllowHosts(ctx)...)
	if err != nil {
		return nil, domain.ErrInvalidBaseURL
	}

	apiType, ok := workflow.ValidateProviderAPIType(cmd.APIType, baseURL)
	if !ok {
		return nil, domain.ErrUnsupportedAPIType
	}
	if err := domain.ValidateProtocolAPIType(protocol, apiType); err != nil {
		return nil, err
	}

	endpoint := &domain.Endpoint{
		APIType:      apiType,
		Protocol:     protocol,
		Capabilities: capabilities,
		Name:         strings.TrimSpace(cmd.Name),
		BaseURL:      baseURL,
		APIKey:       cmd.APIKey,
		Status:       cmd.Status,
	}

	if err := s.repo.Create(ctx, endpoint); err != nil {
		return nil, err
	}
	return endpoint, nil
}

func (s *Service) Update(ctx context.Context, id int64, cmd UpdateEndpointCommand) (*domain.Endpoint, error) {
	if !domain.IsValidStatus(cmd.Status) {
		return nil, domain.ErrInvalidStatus
	}
	protocol, capabilities, err := domain.NormalizeProtocolCapabilities(cmd.Protocol, domain.RequestedCapabilities(cmd.Capability))
	if err != nil {
		return nil, err
	}
	if domain.IsManagedImageEndpointProtocol(protocol) && strings.TrimSpace(cmd.APIKey) == "" {
		return nil, domain.ErrAPIKeyRequired
	}

	baseURL, err := outbound.ValidateConfiguredEndpoint(cmd.BaseURL, outbound.AllowHosts(ctx)...)
	if err != nil {
		return nil, domain.ErrInvalidBaseURL
	}

	endpoint, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	apiType, ok := workflow.ValidateProviderAPIType(cmd.APIType, baseURL)
	if !ok {
		return nil, domain.ErrUnsupportedAPIType
	}
	if err := domain.ValidateProtocolAPIType(protocol, apiType); err != nil {
		return nil, err
	}

	endpoint.APIType = apiType
	endpoint.Protocol = protocol
	endpoint.Capabilities = capabilities
	endpoint.Name = strings.TrimSpace(cmd.Name)
	endpoint.BaseURL = baseURL
	endpoint.APIKey = cmd.APIKey
	endpoint.Status = cmd.Status

	if err := s.repo.Update(ctx, endpoint); err != nil {
		return nil, err
	}
	return endpoint, nil
}

func (s *Service) UpdateStatus(ctx context.Context, id int64, status int16) (*domain.Endpoint, error) {
	if !domain.IsValidStatus(status) {
		return nil, domain.ErrInvalidStatus
	}
	return s.repo.UpdateStatus(ctx, id, status)
}

func (s *Service) Delete(ctx context.Context, id int64) error {
	return s.repo.Delete(ctx, id)
}

func (s *Service) DetectModels(ctx context.Context, cmd DetectModelsCommand) ([]string, string, error) {
	baseURL, apiKey, apiType, validationMessage := validateModelDetectionRequest(cmd)
	if validationMessage != "" {
		return nil, "", &ModelDetectionError{PublicMessage: validationMessage}
	}
	if workflow.IsGoogleCloudAPIType(apiType) {
		models, failure := s.detectGoogleCloudImageModels(ctx, baseURL, apiKey)
		if failure != nil {
			return nil, "", failure
		}
		return models, apiType, nil
	}
	models, err := s.detectStandardModels(ctx, baseURL, apiKey, apiType)
	if err != nil {
		return nil, "", err
	}
	return models, apiType, nil
}

func validateModelDetectionRequest(cmd DetectModelsCommand) (baseURL, apiKey, apiType, publicMessage string) {
	baseURL = strings.TrimSpace(cmd.BaseURL)
	apiKey = strings.TrimSpace(cmd.APIKey)
	if baseURL == "" || apiKey == "" {
		return "", "", "", "base_url 与 api_key 不能为空"
	}
	if strings.TrimSpace(cmd.Protocol) != "" && !domain.ProtocolSupportsModelDetection(cmd.Protocol) {
		return "", "", "", "当前 Protocol 不支持模型检测"
	}
	var ok bool
	apiType, ok = workflow.ValidateProviderAPIType(cmd.APIType, baseURL)
	if !ok {
		return "", "", "", "接口类型不支持"
	}
	return baseURL, apiKey, apiType, ""
}

func (s *Service) detectStandardModels(ctx context.Context, baseURL, apiKey, apiType string) ([]string, error) {
	detectionURL, err := BuildModelDetectionURL(baseURL, apiType)
	if err != nil {
		return nil, &ModelDetectionError{PublicMessage: "Base URL 无效"}
	}

	resp, err := safehttp.DoWithAllowHosts(ctx, http.MethodGet, detectionURL, func(r *http.Request) {
		if workflow.IsGoogleNativeAPIType(apiType) {
			r.Header.Set("x-goog-api-key", apiKey)
		} else {
			r.Header.Set("Authorization", "Bearer "+apiKey)
		}
		r.Header.Set("Accept", "application/json")
	}, DetectModelsTimeout, s.DetectModelsAllowHosts(ctx)...)
	if err != nil {
		failureCategory := ProviderModelDetectionTransportFailureCategory(err)
		slog.Warn("model detection request failed", "api_type", apiType, "detection_url", detectionURL, "failure_category", failureCategory)
		if workflow.IsGoogleNativeAPIType(apiType) {
			return nil, &ModelDetectionError{PublicMessage: "模型检测失败：Google API 请求超时或网络不可达。请确认本机代理/TUN 已接管后端进程，或配置 HTTP_PROXY/HTTPS_PROXY"}
		}
		return nil, &ModelDetectionError{PublicMessage: "模型检测失败，请检查 Base URL 或网络"}
	}
	defer resp.Body.Close()

	limited := &io.LimitedReader{R: resp.Body, N: MaxDetectResponseSize + 1}
	body, err := io.ReadAll(limited)
	if err != nil {
		return nil, &ModelDetectionError{PublicMessage: "模型检测失败，响应读取异常"}
	}
	if int64(len(body)) > MaxDetectResponseSize {
		return nil, &ModelDetectionError{PublicMessage: "模型检测失败，响应过大"}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		publicMessage, reason := providerModelDetectionError(apiType, resp.StatusCode, body)
		slog.Warn("model detection upstream rejected request", "api_type", apiType, "detection_url", detectionURL, "status", resp.StatusCode, "reason", reason)
		return nil, &ModelDetectionError{PublicMessage: publicMessage}
	}

	models, err := ParseModelIDs(body)
	if err != nil {
		return nil, &ModelDetectionError{PublicMessage: "模型检测失败，返回格式不支持"}
	}

	return models, nil
}

type googleCloudModelProbeResult struct {
	model        string
	detectionURL string
	statusCode   int
	body         []byte
	err          error
}

func (s *Service) detectGoogleCloudImageModels(ctx context.Context, baseURL, apiKey string) ([]string, *ModelDetectionError) {
	probeCtx, cancel := context.WithTimeout(ctx, DetectModelsTimeout)
	defer cancel()

	results := make(chan googleCloudModelProbeResult, len(GoogleCloudImageModelCandidates))
	allowHosts := s.DetectModelsAllowHosts(ctx)
	for _, model := range GoogleCloudImageModelCandidates {
		detectionURL, err := BuildGoogleCloudModelProbeURL(baseURL, model)
		if err != nil {
			return nil, &ModelDetectionError{PublicMessage: "Base URL 无效"}
		}
		go func(model, detectionURL string) {
			result := probeGoogleCloudImageModel(probeCtx, detectionURL, model, apiKey, allowHosts)
			results <- result
		}(model, detectionURL)
	}

	resultsByModel := make(map[string]googleCloudModelProbeResult, len(GoogleCloudImageModelCandidates))
	for range GoogleCloudImageModelCandidates {
		result := <-results
		resultsByModel[result.model] = result
	}

	models := make([]string, 0, len(GoogleCloudImageModelCandidates))
	for _, model := range GoogleCloudImageModelCandidates {
		result := resultsByModel[model]
		if result.err == nil && result.statusCode >= 200 && result.statusCode < 300 {
			models = append(models, model)
		}
	}
	if len(models) > 0 {
		return models, nil
	}

	return nil, evaluateProbeFailure(resultsByModel)
}

func evaluateProbeFailure(resultsByModel map[string]googleCloudModelProbeResult) *ModelDetectionError {
	for _, model := range GoogleCloudImageModelCandidates {
		result := resultsByModel[model]
		if result.err != nil {
			failureCategory := ProviderModelDetectionTransportFailureCategory(result.err)
			slog.Warn("Google Cloud model probe failed", "model", model, "detection_url", result.detectionURL, "failure_category", failureCategory)
			return &ModelDetectionError{PublicMessage: "模型检测失败：Google API 请求超时或网络不可达。请确认本机代理/TUN 已接管后端进程，或配置 HTTP_PROXY/HTTPS_PROXY"}
		}
		if result.statusCode == http.StatusNotFound {
			continue
		}
		publicMessage, reason := providerModelDetectionError(workflow.ProviderGoogleCloud, result.statusCode, result.body)
		slog.Warn("Google Cloud model probe rejected", "model", model, "detection_url", result.detectionURL, "status", result.statusCode, "reason", reason)
		return &ModelDetectionError{PublicMessage: publicMessage}
	}

	return &ModelDetectionError{PublicMessage: "模型检测失败：当前 Google Cloud 项目没有可用的 Gemini 图像模型"}
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
	}, DetectModelsTimeout, allowHosts...)
	if err != nil {
		result.err = err
		return result
	}
	defer resp.Body.Close()

	result.statusCode = resp.StatusCode
	limited := &io.LimitedReader{R: resp.Body, N: MaxDetectResponseSize + 1}
	result.body, result.err = io.ReadAll(limited)
	if result.err == nil && int64(len(result.body)) > MaxDetectResponseSize {
		result.err = fmt.Errorf("response exceeds size limit")
	}
	return result
}

func ProviderModelDetectionTransportFailureCategory(err error) string {
	switch {
	case errors.Is(err, context.Canceled):
		return "request_canceled"
	case errors.Is(err, context.DeadlineExceeded):
		return "request_timeout"
	default:
		return "network_error"
	}
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

func providerModelDetectionError(apiType string, statusCode int, body []byte) (string, string) {
	if !workflow.IsGoogleNativeAPIType(apiType) {
		return "模型检测失败，请检查 API Key 或 ProviderEndpoint 配置", ""
	}

	var payload googleAPIErrorResponse
	if err := json.Unmarshal(body, &payload); err != nil {
		return "模型检测失败，请检查 Google API Key、权限与 ProviderEndpoint 配置", ""
	}
	reason, service := googleAPIErrorFacts(payload)
	if message, ok := googleAPIReasonMessage(apiType, reason, service); ok {
		return message, reason
	}
	return googleAPIStatusMessage(statusCode, payload.Error.Status)
}

func googleAPIErrorFacts(payload googleAPIErrorResponse) (reason, service string) {
	for _, detail := range payload.Error.Details {
		if reason == "" {
			reason = strings.ToUpper(strings.TrimSpace(detail.Reason))
		}
		if service == "" {
			service = strings.ToLower(strings.TrimSpace(detail.Metadata["service"]))
		}
	}
	return reason, service
}

func googleAPIReasonMessage(apiType, reason, service string) (string, bool) {
	switch reason {
	case "CREDENTIALS_MISSING":
		if workflow.IsGoogleCloudAPIType(apiType) {
			return "模型检测失败：请使用关联 Service Account 的 Google Cloud API Key，并授予模型调用权限", true
		}
	case "SERVICE_DISABLED":
		if service == "aiplatform.googleapis.com" || workflow.IsGoogleCloudAPIType(apiType) {
			return "模型检测失败：请在 Google Cloud 项目中启用 Gemini Enterprise Agent Platform API（原 Vertex AI，aiplatform.googleapis.com）", true
		}
		return "模型检测失败：请在 Google Cloud 项目中启用 Generative Language API", true
	case "API_KEY_INVALID", "API_KEY_NOT_FOUND":
		return "模型检测失败：Google API Key 无效或不属于当前项目", true
	case "BILLING_DISABLED", "PROJECT_BILLING_DISABLED":
		return "模型检测失败：请为 Google Cloud 项目启用 Billing", true
	case "API_KEY_SERVICE_BLOCKED", "API_KEY_IP_ADDRESS_BLOCKED", "API_KEY_HTTP_REFERRER_BLOCKED", "API_KEY_ANDROID_APP_BLOCKED", "API_KEY_IOS_APP_BLOCKED":
		return "模型检测失败：Google API Key restrictions 拒绝了当前服务或服务器", true
	}
	return "", false
}

func googleAPIStatusMessage(statusCode int, rawStatus string) (string, string) {
	status := strings.ToUpper(strings.TrimSpace(rawStatus))
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

func (s *Service) DetectModelsAllowHosts(ctx context.Context) []string {
	allowHosts := outbound.AllowHosts(ctx)
	if s.IsDevLikeEnvironment() {
		allowHosts = append(allowHosts, "localhost", "127.0.0.1", "::1")
	}
	return allowHosts
}

func (s *Service) IsDevLikeEnvironment() bool {
	env := strings.ToLower(strings.TrimSpace(s.appEnv))
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

func BuildModelDetectionURL(rawBaseURL string, apiType ...string) (string, error) {
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

func BuildGoogleCloudModelProbeURL(rawBaseURL, model string) (string, error) {
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

func ParseModelIDs(body []byte) ([]string, error) {
	var payload any
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}

	rawItems := extractRawModelItems(payload)
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
		if len(models) >= MaxDetectedModels {
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

func extractRawModelItems(payload any) []any {
	switch v := payload.(type) {
	case map[string]any:
		if data, ok := v["data"]; ok {
			if items := normalizeToSlice(data); len(items) > 0 {
				return items
			}
		}
		if models, ok := v["models"]; ok {
			return normalizeToSlice(models)
		}
	case []any:
		return v
	}
	return nil
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

func (s *Service) SetAppEnv(env string) {
	s.appEnv = env
}
