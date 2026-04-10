package handler

import (
	"strconv"

	"backend/internal/model"
	"backend/internal/repository"
	"backend/pkg/response"

	"github.com/gin-gonic/gin"
)

type LineDraftHandler struct {
	repo *repository.LineDraftModeRepository
}

func NewLineDraftHandler(repo *repository.LineDraftModeRepository) *LineDraftHandler {
	return &LineDraftHandler{repo: repo}
}

// ============ 用户端接口 ============

// ListEnabled 获取已启用的线稿模式
func (h *LineDraftHandler) ListEnabled(c *gin.Context) {
	modes, err := h.repo.ListEnabled()
	if err != nil {
		response.Error(c, 500, "获取线稿模式失败")
		return
	}
	// 对用户隐藏 prompt
	type ModeView struct {
		ID         uint64 `json:"id"`
		Name       string `json:"name"`
		CoverImage string `json:"cover_image"`
	}
	var list []ModeView
	for _, m := range modes {
		list = append(list, ModeView{ID: m.ID, Name: m.Name, CoverImage: m.CoverImage})
	}
	response.Success(c, gin.H{"list": list})
}

// ============ 管理端接口 ============

// List 获取所有线稿模式
func (h *LineDraftHandler) List(c *gin.Context) {
	modes, err := h.repo.List()
	if err != nil {
		response.Error(c, 500, "获取列表失败")
		return
	}
	response.Success(c, gin.H{"list": modes})
}

// Create 创建线稿模式
func (h *LineDraftHandler) Create(c *gin.Context) {
	var req model.CreateLineDraftModeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, "参数错误: "+err.Error())
		return
	}
	mode := &model.LineDraftMode{
		Name:       req.Name,
		CoverImage: req.CoverImage,
		Prompt:     req.Prompt,
		SortOrder:  req.SortOrder,
		IsEnabled:  req.IsEnabled,
	}
	if err := h.repo.Create(mode); err != nil {
		response.Error(c, 500, "创建失败: "+err.Error())
		return
	}
	response.Success(c, gin.H{"mode": mode})
}

// Update 更新线稿模式
func (h *LineDraftHandler) Update(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		response.Error(c, 400, "无效的ID")
		return
	}
	var req model.UpdateLineDraftModeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, "参数错误: "+err.Error())
		return
	}
	mode, err := h.repo.GetByID(id)
	if err != nil {
		response.Error(c, 404, "模式不存在")
		return
	}
	mode.Name = req.Name
	mode.CoverImage = req.CoverImage
	mode.Prompt = req.Prompt
	mode.SortOrder = req.SortOrder
	mode.IsEnabled = req.IsEnabled
	if err := h.repo.Update(mode); err != nil {
		response.Error(c, 500, "更新失败: "+err.Error())
		return
	}
	response.Success(c, nil)
}

// Delete 删除线稿模式
func (h *LineDraftHandler) Delete(c *gin.Context) {
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
