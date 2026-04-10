package handler

import (
	"backend/internal/model"
	"backend/internal/service"
	"backend/pkg/response"

	"github.com/gin-gonic/gin"
)

type KodoHandler struct {
	kodoService *service.KodoService
}

func NewKodoHandler(kodoService *service.KodoService) *KodoHandler {
	return &KodoHandler{kodoService: kodoService}
}

// GetConfig 获取七牛云配置
func (h *KodoHandler) GetConfig(c *gin.Context) {
	cfg, err := h.kodoService.GetConfig()
	if err != nil {
		response.Error(c, 500, "获取配置失败")
		return
	}
	masked := *cfg
	if len(masked.SecretKey) > 6 {
		masked.SecretKey = masked.SecretKey[:3] + "***" + masked.SecretKey[len(masked.SecretKey)-3:]
	}
	response.Success(c, gin.H{"config": masked})
}

// SaveConfig 保存七牛云配置
func (h *KodoHandler) SaveConfig(c *gin.Context) {
	var req model.KodoConfig
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, "参数错误")
		return
	}

	// SecretKey 为空或含 mask 时保留原值
	if req.SecretKey == "" || containsMask(req.SecretKey) {
		old, _ := h.kodoService.GetConfig()
		if old != nil && old.SecretKey != "" {
			req.SecretKey = old.SecretKey
		}
	}

	if err := h.kodoService.SaveConfig(&req); err != nil {
		response.Error(c, 500, "保存配置失败")
		return
	}
	response.Success(c, gin.H{"message": "保存成功"})
}

// TestConnection 测试连接
func (h *KodoHandler) TestConnection(c *gin.Context) {
	err := h.kodoService.TestConnection()
	if err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	response.Success(c, gin.H{"message": "连接成功"})
}

// GetBucketInfo 获取 Bucket 信息（自动抓取域名等）
func (h *KodoHandler) GetBucketInfo(c *gin.Context) {
	info, err := h.kodoService.GetBucketInfo()
	if err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	response.Success(c, gin.H{"info": info})
}

func containsMask(s string) bool {
	for _, c := range s {
		if c == '*' {
			return true
		}
	}
	return false
}
