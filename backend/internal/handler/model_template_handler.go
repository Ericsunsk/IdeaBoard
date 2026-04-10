package handler

import (
	"backend/internal/model"
	"backend/internal/repository"
	"backend/pkg/response"

	"github.com/gin-gonic/gin"
)

type ModelTemplateHandler struct {
	repo *repository.ModelTemplateRepository
}

func NewModelTemplateHandler(repo *repository.ModelTemplateRepository) *ModelTemplateHandler {
	return &ModelTemplateHandler{repo: repo}
}

// ListEnabled 获取已上架的模特模板（用户端）
func (h *ModelTemplateHandler) ListEnabled(c *gin.Context) {
	list, err := h.repo.ListEnabled()
	if err != nil {
		response.Error(c, 500, "获取模特列表失败")
		return
	}
	response.Success(c, gin.H{"list": list})
}

// List 管理端列表
func (h *ModelTemplateHandler) List(c *gin.Context) {
	page, pageSize := parsePageQuery(c, 100)
	list, total, err := h.repo.List(page, pageSize)
	if err != nil {
		response.Error(c, 500, "获取列表失败")
		return
	}
	response.Success(c, gin.H{"total": total, "list": list})
}

// Create 创建
func (h *ModelTemplateHandler) Create(c *gin.Context) {
	var req model.CreateModelTemplateRequest
	if !bindJSON(c, &req) {
		return
	}
	tpl := &model.ModelTemplate{
		Name: req.Name, ImageURL: req.ImageURL, Category: req.Category,
		SortOrder: req.SortOrder, IsEnabled: req.IsEnabled,
	}
	if err := h.repo.Create(tpl); err != nil {
		response.Error(c, 500, "创建失败: "+err.Error())
		return
	}
	response.Success(c, gin.H{"model": tpl})
}

// Update 更新
func (h *ModelTemplateHandler) Update(c *gin.Context) {
	id, ok := parseIDParam(c)
	if !ok {
		return
	}
	var req model.UpdateModelTemplateRequest
	if !bindJSON(c, &req) {
		return
	}
	tpl, err := h.repo.GetByID(id)
	if err != nil {
		response.Error(c, 404, "未找到该模特")
		return
	}
	tpl.Name = req.Name
	tpl.ImageURL = req.ImageURL
	tpl.Category = req.Category
	tpl.SortOrder = req.SortOrder
	tpl.IsEnabled = req.IsEnabled
	if err := h.repo.Update(tpl); err != nil {
		response.Error(c, 500, "更新失败: "+err.Error())
		return
	}
	response.Success(c, nil)
}

// Delete 删除
func (h *ModelTemplateHandler) Delete(c *gin.Context) {
	id, ok := parseIDParam(c)
	if !ok {
		return
	}
	if err := h.repo.Delete(id); err != nil {
		response.Error(c, 500, "删除失败: "+err.Error())
		return
	}
	response.Success(c, nil)
}
