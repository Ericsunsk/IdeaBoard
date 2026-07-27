package prompts

import (
	"context"
	"errors"
	"strings"

	"ideaboard/api/internal/identity/principal"
	"ideaboard/api/internal/platform/jsonlimit"
	"ideaboard/api/internal/platform/response"
	"ideaboard/api/internal/platform/validation"
	"ideaboard/api/internal/workflow"

	"github.com/gin-gonic/gin"
)

const maxWorkflowFeatureRequestBytes = 64 << 10

type EligibleModelLookup interface {
	HasEligibleModel(ctx context.Context, featureSlug, model string) (bool, error)
}

type WorkflowFeatureHandler struct {
	repository *WorkflowFeatureRepository
	models     EligibleModelLookup
}

type workflowFeaturePayload struct {
	CanonicalModel        string                            `json:"canonical_model"`
	SystemPrompt          string                            `json:"system_prompt"`
	Variables             []WorkflowFeatureVariable         `json:"variables"`
	UserPromptMergePolicy *WorkflowFeaturePromptMergePolicy `json:"user_prompt_merge_policy"`
	Enabled               bool                              `json:"enabled"`
}

type workflowFeatureListResponse struct {
	WorkflowFeatures []WorkflowFeatureConfig `json:"workflow_features"`
}

func NewWorkflowFeatureHandler(repository *WorkflowFeatureRepository, models EligibleModelLookup) *WorkflowFeatureHandler {
	return &WorkflowFeatureHandler{repository: repository, models: models}
}

func (h *WorkflowFeatureHandler) Get(c *gin.Context) {
	slug, err := workflowFeatureSlugFromRequest(c)
	if err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	config, err := h.repository.GetConfig(c.Request.Context(), slug)
	if err != nil {
		response.InternalError(c, 500, "获取 Workflow Feature 配置失败", err)
		return
	}
	response.Success(c, config)
}

func (h *WorkflowFeatureHandler) Save(c *gin.Context) {
	identityPrincipal, ok := principal.RequireAdmin(c)
	if !ok {
		return
	}
	slug, err := workflowFeatureSlugFromRequest(c)
	if err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	var request workflowFeaturePayload
	if err := jsonlimit.DecodeStrictRequest(c.Writer, c.Request, maxWorkflowFeatureRequestBytes, &request); err != nil {
		response.Error(c, 400, "参数错误")
		return
	}
	request.CanonicalModel = strings.TrimSpace(request.CanonicalModel)
	if request.CanonicalModel == "" {
		response.Error(c, 400, "canonical_model 不能为空")
		return
	}
	if request.Enabled {
		if h.models == nil {
			response.InternalError(c, 500, "模型可用性校验服务未配置", errors.New("eligible model lookup is nil"))
			return
		}
		eligible, err := h.models.HasEligibleModel(c.Request.Context(), slug, request.CanonicalModel)
		if err != nil {
			response.InternalError(c, 500, "校验模型可用性失败", err)
			return
		}
		if !eligible {
			response.Error(c, 400, "当前没有支持该 Workflow Feature 和 canonical_model 的可用账号")
			return
		}
	}
	updatedBy := identityPrincipal.UserID()
	config, err := h.repository.SaveConfig(c.Request.Context(), slug, WorkflowFeatureSaveInput{
		CanonicalModel:        request.CanonicalModel,
		SystemPrompt:          request.SystemPrompt,
		Variables:             request.Variables,
		UserPromptMergePolicy: request.UserPromptMergePolicy,
		Enabled:               request.Enabled,
	}, &updatedBy)
	if err != nil {
		if validation.Is(err) {
			response.Error(c, 400, err.Error())
			return
		}
		response.InternalError(c, 500, "保存 Workflow Feature 配置失败", err)
		return
	}
	response.Success(c, config)
}

func (h *WorkflowFeatureHandler) List(c *gin.Context) {
	configs, err := h.repository.ListConfigs(c.Request.Context())
	if err != nil {
		response.InternalError(c, 500, "获取 Workflow Feature 配置失败", err)
		return
	}
	response.SuccessDTO(c, workflowFeatureListResponse{WorkflowFeatures: configs})
}

func workflowFeatureSlugFromRequest(c *gin.Context) (string, error) {
	rawSlug := c.Param("slug")
	if rawSlug == "" {
		return "", errors.New("缺少 slug 参数")
	}
	return workflow.ValidateWorkflowFeatureSlug(rawSlug)
}
