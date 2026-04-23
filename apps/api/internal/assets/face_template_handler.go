package assets

import "github.com/gin-gonic/gin"

type FaceTemplateHandler struct {
	repo *FaceTemplateRepository
}

func NewFaceTemplateHandler(repo *FaceTemplateRepository) *FaceTemplateHandler {
	return &FaceTemplateHandler{repo: repo}
}

// ============ 用户端接口 ============

// ListEnabled 获取已上架的人脸模板
func (h *FaceTemplateHandler) ListEnabled(c *gin.Context) {
	handleListEnabled(c, func() (any, error) {
		return h.repo.ListEnabled()
	}, "获取人脸列表失败")
}

// ============ 管理端接口 ============

// List 获取人脸模板列表
func (h *FaceTemplateHandler) List(c *gin.Context) {
	handlePagedList(c, 100, func(page, pageSize int) (any, int64, error) {
		return h.repo.List(page, pageSize)
	})
}

// Create 创建人脸模板
func (h *FaceTemplateHandler) Create(c *gin.Context) {
	handleCreate[CreateFaceTemplateRequest, FaceTemplate](
		c,
		func(req *CreateFaceTemplateRequest) *FaceTemplate {
			return &FaceTemplate{
				Name:      req.Name,
				ImageURL:  req.ImageURL,
				Category:  req.Category,
				SortOrder: req.SortOrder,
				IsEnabled: req.IsEnabled,
			}
		},
		h.repo.Create,
		"face",
	)
}

// Update 更新人脸模板
func (h *FaceTemplateHandler) Update(c *gin.Context) {
	handleUpdate[UpdateFaceTemplateRequest, FaceTemplate](
		c,
		h.repo.GetByID,
		func(tpl *FaceTemplate, req *UpdateFaceTemplateRequest) {
			tpl.Name = req.Name
			tpl.ImageURL = req.ImageURL
			tpl.Category = req.Category
			tpl.SortOrder = req.SortOrder
			tpl.IsEnabled = req.IsEnabled
		},
		h.repo.Update,
		"未找到该人脸",
	)
}

// Delete 删除人脸模板
func (h *FaceTemplateHandler) Delete(c *gin.Context) {
	handleDelete(c, h.repo.Delete)
}
