package assets

import (
			"api/internal/platform/response"

	"github.com/gin-gonic/gin"
)

type BgTemplateHandler struct {
	repo *BgTemplateRepository
}

func NewBgTemplateHandler(repo *BgTemplateRepository) *BgTemplateHandler {
	return &BgTemplateHandler{repo: repo}
}

// ============ 用户端接口 ============

// ListEnabled 获取已上架的背景模板
func (h *BgTemplateHandler) ListEnabled(c *gin.Context) {
	list, err := h.repo.ListEnabled()
	if err != nil {
		response.Error(c, 500, "获取背景列表失败")
		return
	}
	response.Success(c, gin.H{"list": list})
}

// ============ 管理端接口 ============

// List 获取背景模板列表
func (h *BgTemplateHandler) List(c *gin.Context) {
	page, pageSize := parsePageQuery(c, 100)
	list, total, err := h.repo.List(page, pageSize)
	if err != nil {
		response.Error(c, 500, "获取列表失败")
		return
	}
	response.Success(c, gin.H{"total": total, "list": list})
}

// Create 创建背景模板
func (h *BgTemplateHandler) Create(c *gin.Context) {
	var req CreateBgTemplateRequest
	if !bindJSON(c, &req) {
		return
	}
	tpl := &BgTemplate{
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
	response.Success(c, gin.H{"bg": tpl})
}

// Update 更新背景模板
func (h *BgTemplateHandler) Update(c *gin.Context) {
	id, ok := parseIDParam(c)
	if !ok {
		return
	}
	var req UpdateBgTemplateRequest
	if !bindJSON(c, &req) {
		return
	}
	tpl, err := h.repo.GetByID(id)
	if err != nil {
		response.Error(c, 404, "未找到该背景")
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

// Delete 删除背景模板
func (h *BgTemplateHandler) Delete(c *gin.Context) {
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
