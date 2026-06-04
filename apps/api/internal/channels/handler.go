package channels

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

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

type ChannelHandler struct {
	channelRepo *ChannelRepository
	appEnv      string
}

func NewChannelHandler(channelRepo *ChannelRepository, appEnv ...string) *ChannelHandler {
	env := ""
	if len(appEnv) > 0 {
		env = strings.TrimSpace(appEnv[0])
	}
	return &ChannelHandler{channelRepo: channelRepo, appEnv: env}
}

func (h *ChannelHandler) List(c *gin.Context) {
	channels, err := h.channelRepo.List(c.Request.Context())
	if err != nil {
		response.Error(c, 500, "获取渠道商列表失败")
		return
	}
	response.Success(c, gin.H{"list": channels})
}

func (h *ChannelHandler) Create(c *gin.Context) {
	var req CreateChannelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, "参数错误")
		return
	}

	apiURL, err := outbound.ValidateConfiguredEndpoint(req.ApiURL, outbound.AllowHostsFromEnv()...)
	if err != nil {
		response.Error(c, 400, "api_url 不可用，仅支持可达的公网 http(s) 地址")
		return
	}

	apiType, ok := workflow.ValidateProviderAPIType(req.Type, apiURL)
	if !ok {
		response.Error(c, 400, "接口类型不支持")
		return
	}

	channel := &Channel{
		Type:   apiType,
		Name:   req.Name,
		ApiURL: apiURL,
		Status: 1,
	}

	if err := h.channelRepo.Create(c.Request.Context(), channel); err != nil {
		response.InternalError(c, 500, "创建渠道商失败", err)
		return
	}

	response.Success(c, gin.H{"channel": channel})
}

func (h *ChannelHandler) Update(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.Error(c, 400, "无效的ID")
		return
	}

	var req UpdateChannelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, "参数错误")
		return
	}

	apiURL, err := outbound.ValidateConfiguredEndpoint(req.ApiURL, outbound.AllowHostsFromEnv()...)
	if err != nil {
		response.Error(c, 400, "api_url 不可用，仅支持可达的公网 http(s) 地址")
		return
	}

	channel, err := h.channelRepo.GetByID(c.Request.Context(), id)
	if err != nil {
		response.Error(c, 404, "渠道商不存在")
		return
	}

	apiType, ok := workflow.ValidateProviderAPIType(req.Type, apiURL)
	if !ok {
		response.Error(c, 400, "接口类型不支持")
		return
	}

	channel.Type = apiType
	channel.Name = req.Name
	channel.ApiURL = apiURL

	if err := h.channelRepo.Update(c.Request.Context(), channel); err != nil {
		response.InternalError(c, 500, "更新渠道商失败", err)
		return
	}

	response.Success(c, gin.H{"channel": channel})
}

func (h *ChannelHandler) Delete(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.Error(c, 400, "无效的ID")
		return
	}

	if err := h.channelRepo.Delete(c.Request.Context(), id); err != nil {
		response.InternalError(c, 500, "删除渠道商失败", err)
		return
	}

	response.Success(c, nil)
}

func (h *ChannelHandler) DetectModels(c *gin.Context) {
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
		response.Error(c, 400, "模型检测失败，请检查 API Key 或渠道配置")
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

	response.Success(c, gin.H{"models": models, "api_type": apiType})
}

func (h *ChannelHandler) detectModelsAllowHosts() []string {
	allowHosts := outbound.AllowHostsFromEnv()
	if h.isDevLikeEnvironment() {
		allowHosts = append(allowHosts, "localhost", "127.0.0.1", "::1")
	}
	return allowHosts
}

func (h *ChannelHandler) isDevLikeEnvironment() bool {
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
