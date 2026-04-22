package handler

import (
	"backend/internal/model"
	"backend/internal/service"
	"backend/pkg/response"

	"github.com/gin-gonic/gin"
)

type StorageHandler struct {
	storageService *service.StorageService
}

func NewStorageHandler(storageService *service.StorageService) *StorageHandler {
	return &StorageHandler{storageService: storageService}
}

func (h *StorageHandler) ListProviders(c *gin.Context) {
	response.Success(c, gin.H{"providers": h.storageService.ListProviderMeta()})
}

func (h *StorageHandler) ListProfiles(c *gin.Context) {
	profiles, err := h.storageService.ListProfiles()
	if err != nil {
		response.Error(c, 500, "获取存储列表失败")
		return
	}
	response.Success(c, gin.H{"profiles": profiles})
}

func (h *StorageHandler) CreateProfile(c *gin.Context) {
	var req model.StorageProfile
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, "参数错误")
		return
	}

	profile, err := h.storageService.SaveProfile(&req)
	if err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	response.Success(c, gin.H{"profile": profile, "message": "保存成功"})
}

func (h *StorageHandler) UpdateProfile(c *gin.Context) {
	var req model.StorageProfile
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, "参数错误")
		return
	}
	req.ID = c.Param("id")

	profile, err := h.storageService.SaveProfile(&req)
	if err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	response.Success(c, gin.H{"profile": profile, "message": "保存成功"})
}

func (h *StorageHandler) DeleteProfile(c *gin.Context) {
	if err := h.storageService.DeleteProfile(c.Param("id")); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	response.Success(c, gin.H{"message": "删除成功"})
}

func (h *StorageHandler) ApplyProfile(c *gin.Context) {
	profile, err := h.storageService.ApplyProfile(c.Param("id"))
	if err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	response.Success(c, gin.H{"profile": profile, "message": "已设为当前存储"})
}

func (h *StorageHandler) TestProfile(c *gin.Context) {
	if err := h.storageService.TestProfile(c.Param("id")); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	response.Success(c, gin.H{"message": "连接成功"})
}

func (h *StorageHandler) GetBucketInfo(c *gin.Context) {
	info, err := h.storageService.GetBucketInfo(c.Param("id"))
	if err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	response.Success(c, gin.H{"info": info})
}
