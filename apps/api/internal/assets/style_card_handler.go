package assets

import (
	"strconv"

			"api/internal/platform/response"

	"github.com/gin-gonic/gin"
)

type StyleCardHandler struct {
	cardService *StyleCardService
}

func NewStyleCardHandler(cardService *StyleCardService) *StyleCardHandler {
	return &StyleCardHandler{cardService: cardService}
}

// ============ 用户端接口（只读） ============

// ListEnabled 获取已上架的风格卡片列表（用户可见）
func (h *StyleCardHandler) ListEnabled(c *gin.Context) {
	cards, err := h.cardService.ListEnabled()
	if err != nil {
		response.Error(c, 500, "获取风格列表失败")
		return
	}
	response.Success(c, gin.H{"list": cards})
}

// ============ 管理端接口（CRUD） ============

// List 获取风格卡片列表
func (h *StyleCardHandler) List(c *gin.Context) {
	var req StyleCardListRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.Error(c, 400, "参数错误")
		return
	}
	cards, total, err := h.cardService.List(&req)
	if err != nil {
		response.Error(c, 500, "获取列表失败")
		return
	}
	response.Success(c, gin.H{"total": total, "list": cards})
}

// Create 创建风格卡片
func (h *StyleCardHandler) Create(c *gin.Context) {
	var req CreateStyleCardRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, "参数错误: "+err.Error())
		return
	}
	card, err := h.cardService.Create(&req)
	if err != nil {
		response.Error(c, 500, "创建失败: "+err.Error())
		return
	}
	response.Success(c, gin.H{"card": card})
}

// Update 更新风格卡片
func (h *StyleCardHandler) Update(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil || id == 0 {
		response.Error(c, 400, "无效的卡片ID")
		return
	}
	var req UpdateStyleCardRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, "参数错误: "+err.Error())
		return
	}
	if err := h.cardService.Update(id, &req); err != nil {
		response.Error(c, 500, "更新失败: "+err.Error())
		return
	}
	response.Success(c, nil)
}

// Delete 删除风格卡片
func (h *StyleCardHandler) Delete(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil || id == 0 {
		response.Error(c, 400, "无效的卡片ID")
		return
	}
	if err := h.cardService.Delete(id); err != nil {
		response.Error(c, 500, "删除失败: "+err.Error())
		return
	}
	response.Success(c, nil)
}
