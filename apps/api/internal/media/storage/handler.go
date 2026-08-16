package storage

import (
	"errors"

	"ideaboard/api/internal/platform/response"

	"github.com/gin-gonic/gin"
)

type StorageHandler struct {
	storageService *StorageService
}

type storageProvidersDTO struct {
	Providers []StorageProviderMeta `json:"providers"`
}

type storageProfilesDTO struct {
	Profiles []StorageProfile `json:"profiles"`
}

type storageProfileWithMessageDTO struct {
	Profile *StorageProfile `json:"profile"`
	Message string          `json:"message"`
}

type storageMessageDTO struct {
	Message string `json:"message"`
}

type storageBucketInfoDTO struct {
	Info map[string]interface{} `json:"info"`
}

func NewStorageHandler(storageService *StorageService) *StorageHandler {
	return &StorageHandler{storageService: storageService}
}

func (h *StorageHandler) ListProviders(c *gin.Context) {
	response.SuccessDTO(c, storageProvidersDTO{Providers: h.storageService.ListProviderMeta()})
}

func (h *StorageHandler) ListProfiles(c *gin.Context) {
	profiles, err := h.storageService.ListProfiles(c.Request.Context())
	if err != nil {
		response.Error(c, 500, "获取存储列表失败")
		return
	}
	response.SuccessDTO(c, storageProfilesDTO{Profiles: profiles})
}

func (h *StorageHandler) CreateProfile(c *gin.Context) {
	var req StorageProfile
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, "参数错误")
		return
	}

	profile, err := h.saveProfile(c, &req)
	if err != nil {
		writeStorageCommandError(c, "保存失败，请检查配置或稍后重试", err)
		return
	}
	response.SuccessDTO(c, storageProfileWithMessageDTO{
		Profile: profile,
		Message: storageSaveProfileMessage(c),
	})
}

func (h *StorageHandler) UpdateProfile(c *gin.Context) {
	var req StorageProfile
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, "参数错误")
		return
	}
	req.ID = c.Param("id")

	profile, err := h.saveProfile(c, &req)
	if err != nil {
		writeStorageCommandError(c, "保存失败，请检查配置或稍后重试", err)
		return
	}
	response.SuccessDTO(c, storageProfileWithMessageDTO{
		Profile: profile,
		Message: storageSaveProfileMessage(c),
	})
}

func (h *StorageHandler) DeleteProfile(c *gin.Context) {
	if err := h.storageService.DeleteProfile(c.Request.Context(), c.Param("id")); err != nil {
		response.InternalError(c, 400, "删除失败，请检查配置或稍后重试", err)
		return
	}
	response.SuccessDTO(c, storageMessageDTO{Message: "删除成功"})
}

func (h *StorageHandler) ApplyProfile(c *gin.Context) {
	profile, err := h.storageService.ApplyProfile(c.Request.Context(), c.Param("id"))
	if err != nil {
		writeStorageCommandError(c, "设为当前失败，请检查配置或稍后重试", err)
		return
	}
	response.SuccessDTO(c, storageProfileWithMessageDTO{
		Profile: profile,
		Message: "已设为当前存储",
	})
}

func (h *StorageHandler) TestProfile(c *gin.Context) {
	if err := h.storageService.TestProfile(c.Request.Context(), c.Param("id")); err != nil {
		writeStorageCommandError(c, "连接测试失败，请检查配置或稍后重试", err)
		return
	}
	response.SuccessDTO(c, storageMessageDTO{Message: "连接成功"})
}

func (h *StorageHandler) GetBucketInfo(c *gin.Context) {
	info, err := h.storageService.GetBucketInfo(c.Request.Context(), c.Param("id"))
	if err != nil {
		writeStorageCommandError(c, "获取 Bucket 信息失败，请检查配置或稍后重试", err)
		return
	}
	response.SuccessDTO(c, storageBucketInfoDTO{Info: info})
}

func writeStorageCommandError(c *gin.Context, fallback string, err error) {
	var capabilityErr *StorageCapabilityError
	if errors.As(err, &capabilityErr) {
		response.Error(c, 400, capabilityErr.Error())
		return
	}
	var probeErr *StorageProbeError
	if errors.As(err, &probeErr) {
		response.Error(c, 400, probeErr.UserMessage())
		return
	}
	response.InternalError(c, 400, fallback, err)
}

func (h *StorageHandler) saveProfile(c *gin.Context, req *StorageProfile) (*StorageProfile, error) {
	if storageShouldApplyProfile(c) {
		return h.storageService.SaveAndApplyProfile(c.Request.Context(), req)
	}
	return h.storageService.SaveProfile(c.Request.Context(), req)
}

func storageShouldApplyProfile(c *gin.Context) bool {
	return c.Query("apply") == "true" || c.Query("apply") == "1"
}

func storageSaveProfileMessage(c *gin.Context) string {
	if storageShouldApplyProfile(c) {
		return "已保存并设为当前存储"
	}
	return "保存成功"
}
