package assets

import (
	"api/internal/platform/response"

	"github.com/gin-gonic/gin"
)

type PatternStyleHandler struct {
	repo *PatternStyleRepository
}

func NewPatternStyleHandler(repo *PatternStyleRepository) *PatternStyleHandler {
	return &PatternStyleHandler{repo: repo}
}

// ListEnabled 用户端：获取已上架的风格模型（支持 ?category= 筛选）
func (h *PatternStyleHandler) ListEnabled(c *gin.Context) {
	category := c.Query("category")
	var list []*PatternStyle
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
	handlePagedList(c, 200, func(page, pageSize int) (any, int64, error) {
		return h.repo.List(page, pageSize)
	})
}

// Create 创建
func (h *PatternStyleHandler) Create(c *gin.Context) {
	handleCreate[CreatePatternStyleRequest, PatternStyle](
		c,
		func(req *CreatePatternStyleRequest) *PatternStyle {
			return &PatternStyle{
				Name:      req.Name,
				ImageURL:  req.ImageURL,
				Category:  req.Category,
				SortOrder: req.SortOrder,
				IsEnabled: req.IsEnabled,
			}
		},
		h.repo.Create,
		"pattern_style",
	)
}

// Update 更新
func (h *PatternStyleHandler) Update(c *gin.Context) {
	handleUpdate[UpdatePatternStyleRequest, PatternStyle](
		c,
		h.repo.GetByID,
		func(ps *PatternStyle, req *UpdatePatternStyleRequest) {
			ps.Name = req.Name
			ps.ImageURL = req.ImageURL
			ps.Category = req.Category
			ps.SortOrder = req.SortOrder
			ps.IsEnabled = req.IsEnabled
		},
		h.repo.Update,
		"未找到该风格模型",
	)
}

// Delete 删除
func (h *PatternStyleHandler) Delete(c *gin.Context) {
	handleDelete(c, h.repo.Delete)
}
