package providerendpoints

import (
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
	detectModelsTimeout   = 5 * time.Second
	maxDetectResponseSize = 1 << 20 // 1MB
	maxDetectedModels     = 100
)

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

	baseURL, err := outbound.ValidateConfiguredEndpoint(req.BaseURL, outbound.AllowHostsFromEnv()...)
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

	baseURL, err := outbound.ValidateConfiguredEndpoint(req.BaseURL, outbound.AllowHostsFromEnv()...)
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
	modelsURL, err := buildModelsURL(baseURL, apiType)
	if err != nil {
		response.Error(c, 400, "Base URL 无效")
		return
	}

	resp, err := safehttp.DoWithAllowHosts(c.Request.Context(), http.MethodGet, modelsURL, func(r *http.Request) {
		if workflow.IsGoogleNativeAPIType(apiType) {
			r.Header.Set("x-goog-api-key", apiKey)
		} else {
			r.Header.Set("Authorization", "Bearer "+apiKey)
		}
		r.Header.Set("Accept", "application/json")
	}, detectModelsTimeout, h.detectModelsAllowHosts()...)
	if err != nil {
		slog.Warn("model detection request failed", "api_type", apiType, "models_url", modelsURL, "err", err)
		if workflow.IsGoogleNativeAPIType(apiType) {
			response.Error(c, 400, "模型检测失败：Google API 网络不可达，请为后端配置 HTTP_PROXY/HTTPS_PROXY 后重试")
			return
		}
		response.Error(c, 400, "模型检测失败，请检查 Base URL 或网络")
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		response.Error(c, 400, "模型检测失败，请检查 API Key 或 ProviderEndpoint 配置")
		return
	}

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

	models, err := parseModelIDs(body)
	if err != nil {
		response.Error(c, 400, "模型检测失败，返回格式不支持")
		return
	}

	response.SuccessDTO(c, detectedModelsResponse{Models: models, APIType: apiType})
}

func (h *ProviderEndpointHandler) detectModelsAllowHosts() []string {
	allowHosts := outbound.AllowHostsFromEnv()
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

func buildModelsURL(rawBaseURL string, apiType ...string) (string, error) {
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
