package assets

import (
	"context"

	"ideaboard/api/internal/platform/response"

	"github.com/gin-gonic/gin"
)

type LineDraftModeStore interface {
	Create(ctx context.Context, mode *LineDraftMode) error
	GetByID(ctx context.Context, id int64) (*LineDraftMode, error)
	List(ctx context.Context) ([]*LineDraftMode, error)
	ListEnabled(ctx context.Context) ([]*LineDraftMode, error)
	Update(ctx context.Context, mode *LineDraftMode) error
	Delete(ctx context.Context, id int64) error
}

type LineDraftHandler struct {
	repo LineDraftModeStore
}

type lineDraftModeUserView struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	CoverImage string `json:"cover_image"`
}

type lineDraftModeUserListResponse struct {
	List []lineDraftModeUserView `json:"list"`
}

type lineDraftModeListResponse struct {
	List []*LineDraftMode `json:"list"`
}

type lineDraftModeResponse struct {
	Mode *LineDraftMode `json:"mode"`
}

func NewLineDraftHandler(repo LineDraftModeStore) *LineDraftHandler {
	return &LineDraftHandler{repo: repo}
}

// ============ 用户端接口 ============

// ListEnabled 获取已启用的线稿模式
func (h *LineDraftHandler) ListEnabled(c *gin.Context) {
	modes, err := h.repo.ListEnabled(c.Request.Context())
	if err != nil {
		response.Error(c, 500, "获取线稿模式失败")
		return
	}
	list := make([]lineDraftModeUserView, 0, len(modes))
	for _, m := range modes {
		list = append(list, lineDraftModeUserView{ID: m.ID, Name: m.Name, CoverImage: m.CoverImage})
	}
	response.SuccessDTO(c, lineDraftModeUserListResponse{List: list})
}

// ============ 管理端接口 ============

// List 获取所有线稿模式
func (h *LineDraftHandler) List(c *gin.Context) {
	modes, err := h.repo.List(c.Request.Context())
	if err != nil {
		response.Error(c, 500, "获取列表失败")
		return
	}
	response.SuccessDTO(c, lineDraftModeListResponse{List: modes})
}

// Create 创建线稿模式
func (h *LineDraftHandler) Create(c *gin.Context) {
	var req CreateLineDraftModeRequest
	if !bindJSON(c, &req) {
		return
	}
	mode := &LineDraftMode{
		Name:       req.Name,
		CoverImage: req.CoverImage,
		Prompt:     req.Prompt,
		SortOrder:  req.SortOrder,
		IsEnabled:  req.IsEnabled,
	}
	if err := h.repo.Create(c.Request.Context(), mode); err != nil {
		response.InternalError(c, 500, "创建失败", err)
		return
	}
	response.SuccessDTO(c, lineDraftModeResponse{Mode: mode})
}

// Update 更新线稿模式
func (h *LineDraftHandler) Update(c *gin.Context) {
	id, ok := parseIDParam(c)
	if !ok {
		return
	}
	var req UpdateLineDraftModeRequest
	if !bindJSON(c, &req) {
		return
	}
	mode, err := h.repo.GetByID(c.Request.Context(), id)
	if err != nil {
		response.Error(c, 404, "模式不存在")
		return
	}
	mode.Name = req.Name
	mode.CoverImage = req.CoverImage
	mode.Prompt = req.Prompt
	mode.SortOrder = req.SortOrder
	mode.IsEnabled = req.IsEnabled
	if err := h.repo.Update(c.Request.Context(), mode); err != nil {
		response.InternalError(c, 500, "更新失败", err)
		return
	}
	response.Success(c, nil)
}

// Delete 删除线稿模式
func (h *LineDraftHandler) Delete(c *gin.Context) {
	id, ok := parseIDParam(c)
	if !ok {
		return
	}
	if err := h.repo.Delete(c.Request.Context(), id); err != nil {
		response.InternalError(c, 500, "删除失败", err)
		return
	}
	response.Success(c, nil)
}
