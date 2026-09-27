package prompts

import (
	"errors"

	"ideaboard/api/internal/identity/principal"
	"ideaboard/api/internal/platform/jsonlimit"
	"ideaboard/api/internal/platform/response"
	"ideaboard/api/internal/workflow"

	"github.com/gin-gonic/gin"
)

const maxFeaturePromptRequestBytes = 64 << 10

type FeaturePromptHandler struct {
	promptRepo *FeaturePromptRepository
}

type featurePromptPayload struct {
	Prompt *string `json:"prompt"`
}

type featurePromptListResponse struct {
	Prompts []FeaturePromptConfig `json:"prompts"`
}

func NewFeaturePromptHandler(promptRepo *FeaturePromptRepository) *FeaturePromptHandler {
	return &FeaturePromptHandler{promptRepo: promptRepo}
}

func (h *FeaturePromptHandler) Get(c *gin.Context) {
	slug, err := featurePromptSlugFromRequest(c)
	if err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	config, err := h.promptRepo.GetConfig(c.Request.Context(), slug)
	if err != nil {
		if errors.Is(err, ErrFeaturePromptUnsupported) {
			response.Error(c, 400, err.Error())
			return
		}
		response.InternalError(c, 500, "获取失败", err)
		return
	}
	response.Success(c, config)
}

func (h *FeaturePromptHandler) List(c *gin.Context) {
	configs, err := h.promptRepo.ListConfigs(c.Request.Context())
	if err != nil {
		response.InternalError(c, 500, "获取失败", err)
		return
	}
	response.SuccessDTO(c, featurePromptListResponse{Prompts: configs})
}

func (h *FeaturePromptHandler) Save(c *gin.Context) {
	// Authorization rides the outer Control Plane guard (controlProxy +
	// guard); the actor is resolved from the request principal.
	identityPrincipal, ok := principal.Require(c)
	if !ok {
		return
	}
	slug, err := featurePromptSlugFromRequest(c)
	if err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	var req featurePromptPayload
	if err := jsonlimit.DecodeStrictRequest(c.Writer, c.Request, maxFeaturePromptRequestBytes, &req); err != nil || req.Prompt == nil {
		response.Error(c, 400, "参数错误")
		return
	}
	updatedBy := identityPrincipal.UserID()
	config, err := h.promptRepo.SaveConfig(c.Request.Context(), slug, *req.Prompt, &updatedBy)
	if err != nil {
		if errors.Is(err, ErrFeaturePromptUnsupported) {
			response.Error(c, 400, err.Error())
			return
		}
		response.InternalError(c, 500, "保存失败", err)
		return
	}
	response.Success(c, config)
}

func featurePromptSlugFromRequest(c *gin.Context) (string, error) {
	rawSlug := c.Param("slug")
	if rawSlug == "" {
		return "", errors.New("缺少 slug 参数")
	}
	return workflow.ValidateWorkflowFeatureSlug(rawSlug)
}
