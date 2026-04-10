package handler

import (
	"backend/internal/repository"
	"backend/pkg/response"

	"github.com/gin-gonic/gin"
)

type FeaturePromptHandler struct {
	configRepo *repository.SystemConfigRepository
}

func NewFeaturePromptHandler(configRepo *repository.SystemConfigRepository) *FeaturePromptHandler {
	return &FeaturePromptHandler{configRepo: configRepo}
}

// GetPrompt 获取某功能的系统提示词
func (h *FeaturePromptHandler) GetPrompt(c *gin.Context) {
	slug := c.Param("slug")
	if slug == "" {
		response.Error(c, 400, "缺少 slug 参数")
		return
	}

	prompt, err := h.configRepo.Get("feature_prompt:" + slug)
	if err != nil {
		// 不存在则返回空
		response.Success(c, gin.H{"slug": slug, "prompt": ""})
		return
	}
	response.Success(c, gin.H{"slug": slug, "prompt": prompt})
}

// SetPrompt 设置某功能的系统提示词
func (h *FeaturePromptHandler) SetPrompt(c *gin.Context) {
	slug := c.Param("slug")
	if slug == "" {
		response.Error(c, 400, "缺少 slug 参数")
		return
	}

	var req struct {
		Prompt string `json:"prompt"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, "参数错误: "+err.Error())
		return
	}

	if err := h.configRepo.Set("feature_prompt:"+slug, req.Prompt); err != nil {
		response.Error(c, 500, "保存失败: "+err.Error())
		return
	}

	response.Success(c, gin.H{"slug": slug, "prompt": req.Prompt})
}

// ListPrompts 获取所有功能的系统提示词
func (h *FeaturePromptHandler) ListPrompts(c *gin.Context) {
	prompts, err := h.configRepo.GetByPrefix("feature_prompt:")
	if err != nil {
		response.Error(c, 500, "获取失败: "+err.Error())
		return
	}
	response.Success(c, gin.H{"prompts": prompts})
}
