package storage

import (
	"ideaboard/api/internal/platform/response"

	"github.com/gin-gonic/gin"
)

type StorageHandler struct {
	storageService *StorageService
}

func NewStorageHandler(storageService *StorageService) *StorageHandler {
	return &StorageHandler{storageService: storageService}
}

func (h *StorageHandler) ListProviders(c *gin.Context) {
	response.Success(c, gin.H{"providers": h.storageService.ListProviderMeta()})
}

func (h *StorageHandler) ListProfiles(c *gin.Context) {
	profiles, err := h.storageService.ListProfiles(c.Request.Context())
	if err != nil {
		response.Error(c, 500, "获取存储列表失败")
		return
	}
	response.Success(c, gin.H{"profiles": profiles})
}

func (h *StorageHandler) CreateProfile(c *gin.Context) {
	var req StorageProfile
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, "参数错误")
		return
	}

	profile, err := h.storageService.SaveProfile(c.Request.Context(), &req)
	if err != nil {
		response.InternalError(c, 400, "保存失败，请检查配置或稍后重试", err)
		return
	}
	response.Success(c, gin.H{"profile": profile, "message": "保存成功"})
}

func (h *StorageHandler) UpdateProfile(c *gin.Context) {
	var req StorageProfile
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, "参数错误")
		return
	}
	req.ID = c.Param("id")

	profile, err := h.storageService.SaveProfile(c.Request.Context(), &req)
	if err != nil {
		response.InternalError(c, 400, "保存失败，请检查配置或稍后重试", err)
		return
	}
	response.Success(c, gin.H{"profile": profile, "message": "保存成功"})
}

func (h *StorageHandler) DeleteProfile(c *gin.Context) {
	if err := h.storageService.DeleteProfile(c.Request.Context(), c.Param("id")); err != nil {
		response.InternalError(c, 400, "删除失败，请检查配置或稍后重试", err)
		return
	}
	response.Success(c, gin.H{"message": "删除成功"})
}

func (h *StorageHandler) ApplyProfile(c *gin.Context) {
	profile, err := h.storageService.ApplyProfile(c.Request.Context(), c.Param("id"))
	if err != nil {
		response.InternalError(c, 400, "设为当前失败，请检查配置或稍后重试", err)
		return
	}
	response.Success(c, gin.H{"profile": profile, "message": "已设为当前存储"})
}

func (h *StorageHandler) TestProfile(c *gin.Context) {
	if err := h.storageService.TestProfile(c.Request.Context(), c.Param("id")); err != nil {
		response.InternalError(c, 400, "连接测试失败，请检查配置或稍后重试", err)
		return
	}
	response.Success(c, gin.H{"message": "连接成功"})
}

func (h *StorageHandler) GetBucketInfo(c *gin.Context) {
	info, err := h.storageService.GetBucketInfo(c.Request.Context(), c.Param("id"))
	if err != nil {
		response.InternalError(c, 400, "获取 Bucket 信息失败，请检查配置或稍后重试", err)
		return
	}
	response.Success(c, gin.H{"info": info})
}
