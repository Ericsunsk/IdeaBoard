package prompts

import (
	"errors"

	"ideaboard/api/internal/identity/principal"
	"ideaboard/api/internal/platform/errorrender"
	"ideaboard/api/internal/platform/jsonlimit"
	"ideaboard/api/internal/platform/response"
	"ideaboard/api/internal/workflow"

	"github.com/gin-gonic/gin"
)

const maxFeaturePromptRequestBytes = 64 << 10

type FeaturePromptHandler struct {
	reader FeaturePromptReader
	writer FeaturePromptWriter
}

type featurePromptPayload struct {
	Prompt *string `json:"prompt"`
}

type featurePromptListResponse struct {
	Prompts []FeaturePromptConfig `json:"prompts"`
}

func NewFeaturePromptHandler(store FeaturePromptStore) *FeaturePromptHandler {
	return &FeaturePromptHandler{reader: store, writer: store}
}

func (h *FeaturePromptHandler) Get(c *gin.Context) {
	slug, err := featurePromptSlugFromRequest(c)
	if err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	config, err := h.reader.GetConfig(c.Request.Context(), slug)
	if err != nil {
		if errors.Is(err, ErrFeaturePromptUnsupported) {
			response.Error(c, 400, err.Error())
			return
		}
		response.InternalError(c, 500, errorrender.MsgPromptsGet, err)
		return
	}
	response.Success(c, config)
}

func (h *FeaturePromptHandler) List(c *gin.Context) {
	configs, err := h.reader.ListConfigs(c.Request.Context())
	if err != nil {
		response.InternalError(c, 500, errorrender.MsgPromptsGet, err)
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
		response.Error(c, 400, errorrender.MsgSave)
		return
	}
	updatedBy := identityPrincipal.UserID()
	config, err := h.writer.SaveConfig(c.Request.Context(), slug, *req.Prompt, &updatedBy)
	if err != nil {
		if errors.Is(err, ErrFeaturePromptUnsupported) {
			response.Error(c, 400, err.Error())
			return
		}
		response.InternalError(c, 500, errorrender.MsgSave2, err)
		return
	}
	response.Success(c, config)
}

func featurePromptSlugFromRequest(c *gin.Context) (string, error) {
	rawSlug := c.Param("slug")
	if rawSlug == "" {
		return "", errors.New(errorrender.MsgFeaturePromptSlugFromRequest)
	}
	// Reuses the canonical ParseAIImageFeatureSlug; the ValidateWorkflow*
	// alias is never called directly here.
	slug, ok := workflow.ParseAIImageFeatureSlug(rawSlug)
	if !ok {
		// Preserve the legacy ValidateWorkflowFeatureSlug message shape.
		if _, err := workflow.ValidateWorkflowFeatureSlug(rawSlug); err != nil {
			return "", err
		}
		return "", errors.New(errorrender.MsgFeaturePromptSlugFromRequest)
	}
	return slug, nil
}
