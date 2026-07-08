package prompts

import (
	"errors"
	"net/http"

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

type featurePromptRollbackPayload struct {
	RevisionID int64 `json:"revision_id"`
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
	config, err := h.promptRepo.SetPromptConfig(c.Request.Context(), slug, saveInput, promptUpdatedBy(c))
	if err != nil {
		if gateErr, ok := isPromptEvalGateError(err); ok {
			c.JSON(http.StatusConflict, response.Response{
				Code: http.StatusConflict,
				Msg:  "Prompt eval 未通过，发布已阻止",
				Data: gateErr.Report,
			})
			return
		}
		response.InternalError(c, 500, "保存失败", err)
		return
	}

	response.Success(c, config)
}

func (h *FeaturePromptHandler) RollbackPrompt(c *gin.Context) {
	slug, err := featurePromptSlugFromRequest(c)
	if err != nil {
		response.Error(c, 400, err.Error())
		return
	}

	var req featurePromptRollbackPayload
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, "参数错误")
		return
	}
	if req.RevisionID <= 0 {
		response.Error(c, 400, "缺少 revision_id 参数")
		return
	}

	config, err := h.promptRepo.RollbackPromptConfig(c.Request.Context(), slug, req.RevisionID)
	if err != nil {
		switch {
		case errors.Is(err, ErrPromptRevisionNotFound):
			response.Error(c, 404, "回滚目标版本不存在")
		case errors.Is(err, ErrPromptRollbackTargetCurrent):
			response.Error(c, 409, "目标版本已是当前版本")
		default:
			response.InternalError(c, 500, "回滚失败", err)
		}
		return
	}

	response.Success(c, config)
}

func (h *FeaturePromptHandler) EvaluatePrompt(c *gin.Context) {
	slug, err := featurePromptSlugFromRequest(c)
	if err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	if !promptEvalSupportedForFeature(slug) {
		response.Error(c, 404, "该功能暂未配置 Prompt eval")
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
	report, err := h.promptRepo.EvaluatePromptDraft(c.Request.Context(), slug, saveInput, promptUpdatedBy(c))
	if err != nil {
		if err == ErrPromptEvalUnsupported {
			response.Error(c, 404, "该功能暂未配置 Prompt eval")
			return
		}
		response.InternalError(c, 500, "Eval 失败", err)
		return
	}
	response.Success(c, report)
}

func (h *FeaturePromptHandler) GetLatestPromptEvalReport(c *gin.Context) {
	slug, err := featurePromptSlugFromRequest(c)
	if err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	report, err := h.promptRepo.GetLatestPromptEvalReport(c.Request.Context(), slug)
	if err != nil {
		response.InternalError(c, 500, "获取 Eval 结果失败", err)
		return
	}
	response.Success(c, report)
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
