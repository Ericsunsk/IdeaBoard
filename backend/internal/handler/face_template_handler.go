package handler

import (
	"backend/internal/model"
	"backend/internal/repository"
	"backend/pkg/response"

	"github.com/gin-gonic/gin"
)

type FaceTemplateHandler struct {
	repo *repository.FaceTemplateRepository
}

func NewFaceTemplateHandler(repo *repository.FaceTemplateRepository) *FaceTemplateHandler {
	return &FaceTemplateHandler{repo: repo}
}

// ============ 用户端接口 ============

// ListEnabled 获取已上架的人脸模板
func (h *FaceTemplateHandler) ListEnabled(c *gin.Context) {
	list, err := h.repo.ListEnabled()
	if err != nil {
		response.Error(c, 500, "获取人脸列表失败")
		return
	}
	response.Success(c, gin.H{"list": list})
}

// ============ 管理端接口 ============

// List 获取人脸模板列表
func (h *FaceTemplateHandler) List(c *gin.Context) {
	page, pageSize := parsePageQuery(c, 100)
	list, total, err := h.repo.List(page, pageSize)
	if err != nil {
		response.Error(c, 500, "获取列表失败")
		return
	}
	response.Success(c, gin.H{"total": total, "list": list})
}

// Create 创建人脸模板
func (h *FaceTemplateHandler) Create(c *gin.Context) {
	var req model.CreateFaceTemplateRequest
	if !bindJSON(c, &req) {
		return
	}
	tpl := &model.FaceTemplate{
		Name:      req.Name,
		ImageURL:  req.ImageURL,
		Category:  req.Category,
		SortOrder: req.SortOrder,
		IsEnabled: req.IsEnabled,
	}
	if err := h.repo.Create(tpl); err != nil {
		response.Error(c, 500, "创建失败: "+err.Error())
		return
	}
	response.Success(c, gin.H{"face": tpl})
}

// Update 更新人脸模板
func (h *FaceTemplateHandler) Update(c *gin.Context) {
	id, ok := parseIDParam(c)
	if !ok {
		return
	}
	var req model.UpdateFaceTemplateRequest
	if !bindJSON(c, &req) {
		return
	}
	tpl, err := h.repo.GetByID(id)
	if err != nil {
		response.Error(c, 404, "未找到该人脸")
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

// Delete 删除人脸模板
func (h *FaceTemplateHandler) Delete(c *gin.Context) {
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
