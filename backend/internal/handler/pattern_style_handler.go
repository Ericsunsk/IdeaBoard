package handler

import (
	"strconv"

	"backend/internal/model"
	"backend/internal/repository"
	"backend/pkg/response"

	"github.com/gin-gonic/gin"
)

type PatternStyleHandler struct {
	repo *repository.PatternStyleRepository
}

func NewPatternStyleHandler(repo *repository.PatternStyleRepository) *PatternStyleHandler {
	return &PatternStyleHandler{repo: repo}
}

// ListEnabled 用户端：获取已上架的风格模型（支持 ?category= 筛选）
func (h *PatternStyleHandler) ListEnabled(c *gin.Context) {
	category := c.Query("category")
	var list []*model.PatternStyle
	var err error
	if category != "" {
		list, err = h.repo.ListEnabledByCategory(category)
	} else {
		list, err = h.repo.ListEnabled()
	}
	if err != nil {
		response.Error(c, 500, "获取风格模型列表失败")
		return
	}
	response.Success(c, gin.H{"list": list})
}

// List 管理端列表
func (h *PatternStyleHandler) List(c *gin.Context) {
	pageStr := c.DefaultQuery("page", "1")
	pageSizeStr := c.DefaultQuery("page_size", "50")
	page, _ := strconv.Atoi(pageStr)
	pageSize, _ := strconv.Atoi(pageSizeStr)
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 200 {
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
func (h *PatternStyleHandler) Create(c *gin.Context) {
	var req model.CreatePatternStyleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, "参数错误: "+err.Error())
		return
	}
	ps := &model.PatternStyle{
		Name: req.Name, ImageURL: req.ImageURL, Category: req.Category,
		SortOrder: req.SortOrder, IsEnabled: req.IsEnabled,
	}
	if err := h.repo.Create(ps); err != nil {
		response.Error(c, 500, "创建失败: "+err.Error())
		return
	}
	response.Success(c, gin.H{"pattern_style": ps})
}

// Update 更新
func (h *PatternStyleHandler) Update(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		response.Error(c, 400, "无效的ID")
		return
	}
	var req model.UpdatePatternStyleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, "参数错误: "+err.Error())
		return
	}
	ps, err := h.repo.GetByID(id)
	if err != nil {
		response.Error(c, 404, "未找到该风格模型")
		return
	}
	ps.Name = req.Name
	ps.ImageURL = req.ImageURL
	ps.Category = req.Category
	ps.SortOrder = req.SortOrder
	ps.IsEnabled = req.IsEnabled
	if err := h.repo.Update(ps); err != nil {
		response.Error(c, 500, "更新失败: "+err.Error())
		return
	}
	response.Success(c, nil)
}

// Delete 删除
func (h *PatternStyleHandler) Delete(c *gin.Context) {
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
