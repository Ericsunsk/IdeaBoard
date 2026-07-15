package prompts

import (
	"errors"

	"ideaboard/api/internal/identity/principal"
	"ideaboard/api/internal/platform/response"
	"ideaboard/api/internal/workflow"

	"github.com/gin-gonic/gin"
)

type FeaturePromptHandler struct {
	promptRepo *FeaturePromptRepository
}

type featurePromptPayload struct {
	Prompt                *string                     `json:"prompt"`
	Enabled               *bool                       `json:"enabled"`
	Variables             []FeaturePromptVariable     `json:"variables"`
	ModelDefaults         *FeaturePromptModelDefaults `json:"model_defaults"`
	UserPromptMergePolicy *FeaturePromptMergePolicy   `json:"user_prompt_merge_policy"`
}

type featurePromptListResponse struct {
	Prompts []FeaturePromptConfig `json:"prompts"`
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
	identityPrincipal, ok := principal.RequireAdmin(c)
	if !ok {
		return
	}
	slug, err := featurePromptSlugFromRequest(c)
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

	saveInput, err := h.featurePromptSaveInput(c, slug, req)
	if err != nil {
		response.InternalError(c, 500, "读取当前配置失败", err)
		return
	}
	updatedBy := identityPrincipal.UserID()
	config, err := h.promptRepo.SetPromptConfig(c.Request.Context(), slug, saveInput, &updatedBy)
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

func featurePromptSlugFromRequest(c *gin.Context) (string, error) {
	rawSlug := c.Param("slug")
	if rawSlug == "" {
		return "", errors.New("缺少 slug 参数")
	}
	return workflow.ValidateWorkflowFeatureSlug(rawSlug)
}

func (h *FeaturePromptHandler) featurePromptSaveInput(c *gin.Context, slug string, req featurePromptPayload) (FeaturePromptSaveInput, error) {
	current, err := h.promptRepo.GetPromptConfig(c.Request.Context(), slug)
	if err != nil {
		return FeaturePromptSaveInput{}, err
	}
	enabled := current.Enabled
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	variables := req.Variables
	if variables == nil {
		variables = current.Variables
	}
	modelDefaults := current.ModelDefaults
	if req.ModelDefaults != nil {
		modelDefaults = *req.ModelDefaults
	}
	mergePolicy := req.UserPromptMergePolicy
	if mergePolicy == nil {
		mergePolicy = &current.UserPromptMergePolicy
	}
	return FeaturePromptSaveInput{
		Prompt:                *req.Prompt,
		Enabled:               enabled,
		Variables:             variables,
		ModelDefaults:         modelDefaults,
		UserPromptMergePolicy: mergePolicy,
	}, nil
}
