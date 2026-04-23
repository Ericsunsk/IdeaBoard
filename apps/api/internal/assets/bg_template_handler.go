package assets

import "github.com/gin-gonic/gin"

type BgTemplateHandler struct {
	repo *BgTemplateRepository
}

func NewBgTemplateHandler(repo *BgTemplateRepository) *BgTemplateHandler {
	return &BgTemplateHandler{repo: repo}
}

// ============ 用户端接口 ============

// ListEnabled 获取已上架的背景模板
func (h *BgTemplateHandler) ListEnabled(c *gin.Context) {
	handleListEnabled(c, func() (any, error) {
		return h.repo.ListEnabled()
	}, "获取背景列表失败")
}

// ============ 管理端接口 ============

// List 获取背景模板列表
func (h *BgTemplateHandler) List(c *gin.Context) {
	handlePagedList(c, 100, func(page, pageSize int) (any, int64, error) {
		return h.repo.List(page, pageSize)
	})
}

// Create 创建背景模板
func (h *BgTemplateHandler) Create(c *gin.Context) {
	handleCreate[CreateBgTemplateRequest, BgTemplate](
		c,
		func(req *CreateBgTemplateRequest) *BgTemplate {
			return &BgTemplate{
				Name:      req.Name,
				ImageURL:  req.ImageURL,
				Category:  req.Category,
				SortOrder: req.SortOrder,
				IsEnabled: req.IsEnabled,
			}
		},
		h.repo.Create,
		"bg",
	)
}

// Update 更新背景模板
func (h *BgTemplateHandler) Update(c *gin.Context) {
	handleUpdate[UpdateBgTemplateRequest, BgTemplate](
		c,
		h.repo.GetByID,
		func(tpl *BgTemplate, req *UpdateBgTemplateRequest) {
			tpl.Name = req.Name
			tpl.ImageURL = req.ImageURL
			tpl.Category = req.Category
			tpl.SortOrder = req.SortOrder
			tpl.IsEnabled = req.IsEnabled
		},
		h.repo.Update,
		"未找到该背景",
	)
}

// Delete 删除背景模板
func (h *BgTemplateHandler) Delete(c *gin.Context) {
	handleDelete(c, h.repo.Delete)
}
