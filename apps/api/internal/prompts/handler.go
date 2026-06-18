package prompts

import (
	"ideaboard/api/internal/platform/response"
	"ideaboard/api/internal/workflow"

	"github.com/gin-gonic/gin"
)

type FeaturePromptHandler struct {
	promptRepo *FeaturePromptRepository
}

type featurePromptPayload struct {
	Prompt    *string                 `json:"prompt"`
	Enabled   *bool                   `json:"enabled"`
	Variables []FeaturePromptVariable `json:"variables"`
}

type featurePromptListResponse struct {
	Prompts []FeaturePromptConfig `json:"prompts"`
}

type featurePromptRevisionListResponse struct {
	Revisions []FeaturePromptRevision `json:"revisions"`
}

func NewFeaturePromptHandler(promptRepo *FeaturePromptRepository) *FeaturePromptHandler {
	return &FeaturePromptHandler{promptRepo: promptRepo}
}

// GetPrompt 获取某功能的系统提示词
func (h *FeaturePromptHandler) GetPrompt(c *gin.Context) {
	rawSlug := c.Param("slug")
	if rawSlug == "" {
		response.Error(c, 400, "缺少 slug 参数")
		return
	}
	slug, err := workflow.ValidateWorkflowFeatureSlug(rawSlug)
	if err != nil {
		response.Error(c, 400, err.Error())
		return
	}

	config, err := h.promptRepo.GetPromptConfig(c.Request.Context(), slug)
	if err != nil {
		response.InternalError(c, 500, "获取失败", err)
		return
	}
	response.Success(c, config)
}

// SetPrompt 设置某功能的系统提示词
func (h *FeaturePromptHandler) SetPrompt(c *gin.Context) {
	rawSlug := c.Param("slug")
	if rawSlug == "" {
		response.Error(c, 400, "缺少 slug 参数")
		return
	}
	slug, err := workflow.ValidateWorkflowFeatureSlug(rawSlug)
	if err != nil {
		response.Error(c, 400, err.Error())
		return
	}

	var req featurePromptPayload
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, "参数错误")
		return
	}
	if req.Prompt == nil {
		response.Error(c, 400, "缺少 prompt 参数")
		return
	}

	current, err := h.promptRepo.GetPromptConfig(c.Request.Context(), slug)
	if err != nil {
		response.InternalError(c, 500, "读取当前配置失败", err)
		return
	}
	enabled := current.Enabled
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	variables := req.Variables
	if variables == nil {
		variables = current.Variables
	}

	config, err := h.promptRepo.SetPromptConfig(c.Request.Context(), slug, FeaturePromptSaveInput{
		Prompt:    *req.Prompt,
		Enabled:   enabled,
		Variables: variables,
	}, promptUpdatedBy(c))
	if err != nil {
		response.InternalError(c, 500, "保存失败", err)
		return
	}

	response.Success(c, config)
}

// ListPrompts 获取所有功能的系统提示词
func (h *FeaturePromptHandler) ListPrompts(c *gin.Context) {
	prompts, err := h.promptRepo.ListPromptConfigs(c.Request.Context())
	if err != nil {
		response.InternalError(c, 500, "获取失败", err)
		return
	}
	response.SuccessDTO(c, featurePromptListResponse{Prompts: prompts})
}

func (h *FeaturePromptHandler) ListRevisions(c *gin.Context) {
	rawSlug := c.Param("slug")
	if rawSlug == "" {
		response.Error(c, 400, "缺少 slug 参数")
		return
	}
	slug, err := workflow.ValidateWorkflowFeatureSlug(rawSlug)
	if err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	revisions, err := h.promptRepo.ListPromptRevisions(c.Request.Context(), slug)
	if err != nil {
		response.InternalError(c, 500, "获取版本失败", err)
		return
	}
	response.SuccessDTO(c, featurePromptRevisionListResponse{Revisions: revisions})
}

func promptUpdatedBy(c *gin.Context) *int64 {
	value, ok := c.Get("user_id")
	if !ok {
		return nil
	}
	switch typed := value.(type) {
	case int64:
		return &typed
	case int:
		converted := int64(typed)
		return &converted
	default:
		return nil
	}
}
