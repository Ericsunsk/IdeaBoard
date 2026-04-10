package handler

import (
	"strconv"

	"backend/internal/model"
	"backend/internal/repository"
	"backend/pkg/response"

	"github.com/gin-gonic/gin"
)

type PoseTemplateHandler struct {
	repo *repository.PoseTemplateRepository
}

func NewPoseTemplateHandler(repo *repository.PoseTemplateRepository) *PoseTemplateHandler {
	return &PoseTemplateHandler{repo: repo}
}

// ListEnabled 获取已上架的姿势模板（用户端）
func (h *PoseTemplateHandler) ListEnabled(c *gin.Context) {
	list, err := h.repo.ListEnabled()
	if err != nil {
		response.Error(c, 500, "获取姿势列表失败")
		return
	}
	response.Success(c, gin.H{"list": list})
}

// List 管理端列表
func (h *PoseTemplateHandler) List(c *gin.Context) {
	pageStr := c.DefaultQuery("page", "1")
	pageSizeStr := c.DefaultQuery("page_size", "50")
	page, _ := strconv.Atoi(pageStr)
	pageSize, _ := strconv.Atoi(pageSizeStr)
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 50
	}
	list, total, err := h.repo.List(page, pageSize)
	if err != nil {
		response.Error(c, 500, "获取列表失败")
		return
	}
	response.Success(c, gin.H{"total": total, "list": list})
}

// Create 创建
func (h *PoseTemplateHandler) Create(c *gin.Context) {
	var req model.CreatePoseTemplateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, "参数错误: "+err.Error())
		return
	}
	tpl := &model.PoseTemplate{
		Name: req.Name, ImageURL: req.ImageURL, Category: req.Category,
		SortOrder: req.SortOrder, IsEnabled: req.IsEnabled,
	}
	if err := h.repo.Create(tpl); err != nil {
		response.Error(c, 500, "创建失败: "+err.Error())
		return
	}
	response.Success(c, gin.H{"pose": tpl})
}

// Update 更新
func (h *PoseTemplateHandler) Update(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		response.Error(c, 400, "无效的ID")
		return
	}
	var req model.UpdatePoseTemplateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, "参数错误: "+err.Error())
		return
	}
	tpl, err := h.repo.GetByID(id)
	if err != nil {
		response.Error(c, 404, "未找到该姿势")
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
func (h *PoseTemplateHandler) Delete(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		response.Error(c, 400, "无效的ID")
		return
	}
	if err := h.repo.Delete(id); err != nil {
		response.Error(c, 500, "删除失败: "+err.Error())
		return
	}
	response.Success(c, nil)
}
