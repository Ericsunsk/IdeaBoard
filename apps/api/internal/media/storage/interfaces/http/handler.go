package http

import (
	"context"
	"errors"

	"ideaboard/api/internal/media/storage/domain"
	"ideaboard/api/internal/platform/response"

	"github.com/gin-gonic/gin"
)

type StorageProfilesService interface {
	ListProviderMeta() []domain.StorageProviderMeta
	ListProfiles(ctx context.Context) ([]domain.StorageProfile, error)
	SaveProfile(ctx context.Context, profile *domain.StorageProfile) (*domain.StorageProfile, error)
	SaveAndApplyProfile(ctx context.Context, profile *domain.StorageProfile) (*domain.StorageProfile, error)
	DeleteProfile(ctx context.Context, id string) error
	ApplyProfile(ctx context.Context, id string) (*domain.StorageProfile, error)
	TestProfile(ctx context.Context, id string) error
	GetBucketInfo(ctx context.Context, id string) (map[string]interface{}, error)
}

type Handler struct {
	service StorageProfilesService
}

func NewHandler(service StorageProfilesService) *Handler {
	return &Handler{service: service}
}

func (h *Handler) ListProviders(c *gin.Context) {
	if h.service == nil {
		writeStorageUnavailable(c)
		return
	}
	response.SuccessDTO(c, storageProvidersResponse{Providers: h.service.ListProviderMeta()})
}

func (h *Handler) ListProfiles(c *gin.Context) {
	if h.service == nil {
		writeStorageUnavailable(c)
		return
	}
	profiles, err := h.service.ListProfiles(c.Request.Context())
	if err != nil {
		writeStorageListError(c, err)
		return
	}
	response.SuccessDTO(c, storageProfilesResponse{Profiles: profiles})
}

func (h *Handler) CreateProfile(c *gin.Context) {
	if h.service == nil {
		writeStorageUnavailable(c)
		return
	}
	var req domain.StorageProfile
	if err := c.ShouldBindJSON(&req); err != nil {
		writeStorageInputError(c)
		return
	}

	profile, err := h.saveProfile(c, &req)
	if err != nil {
		writeStorageCommandError(c, msgCreateProfile, err)
		return
	}
	response.SuccessDTO(c, storageProfileWithMessageResponse{
		Profile: profile,
		Message: storageSaveProfileMessage(c),
	})
}

func (h *Handler) UpdateProfile(c *gin.Context) {
	if h.service == nil {
		writeStorageUnavailable(c)
		return
	}
	var req domain.StorageProfile
	if err := c.ShouldBindJSON(&req); err != nil {
		writeStorageInputError(c)
		return
	}
	req.ID = c.Param("id")

	profile, err := h.saveProfile(c, &req)
	if err != nil {
		writeStorageCommandError(c, msgCreateProfile, err)
		return
	}
	response.SuccessDTO(c, storageProfileWithMessageResponse{
		Profile: profile,
		Message: storageSaveProfileMessage(c),
	})
}

func (h *Handler) DeleteProfile(c *gin.Context) {
	if h.service == nil {
		writeStorageUnavailable(c)
		return
	}
	if err := h.service.DeleteProfile(c.Request.Context(), c.Param("id")); err != nil {
		writeStorageCommandError(c, msgDeleteProfile, err)
		return
	}
	response.SuccessDTO(c, storageMessageResponse{Message: msgDeleteProfile2})
}

func (h *Handler) ApplyProfile(c *gin.Context) {
	if h.service == nil {
		writeStorageUnavailable(c)
		return
	}
	profile, err := h.service.ApplyProfile(c.Request.Context(), c.Param("id"))
	if err != nil {
		writeStorageCommandError(c, msgApplyProfile, err)
		return
	}
	response.SuccessDTO(c, storageProfileWithMessageResponse{
		Profile: profile,
		Message: msgApplyProfile2,
	})
}

func (h *Handler) TestProfile(c *gin.Context) {
	if h.service == nil {
		writeStorageUnavailable(c)
		return
	}
	if err := h.service.TestProfile(c.Request.Context(), c.Param("id")); err != nil {
		writeStorageCommandError(c, msgTestProfile, err)
		return
	}
	response.SuccessDTO(c, storageMessageResponse{Message: msgTestProfile2})
}

func (h *Handler) GetBucketInfo(c *gin.Context) {
	if h.service == nil {
		writeStorageUnavailable(c)
		return
	}
	info, err := h.service.GetBucketInfo(c.Request.Context(), c.Param("id"))
	if err != nil {
		writeStorageCommandError(c, msgGetBucketInfo, err)
		return
	}
	response.SuccessDTO(c, storageBucketInfoResponse{Info: info})
}

type storageProvidersResponse struct {
	Providers []domain.StorageProviderMeta `json:"providers"`
}

type storageProfilesResponse struct {
	Profiles []domain.StorageProfile `json:"profiles"`
}

type storageProfileWithMessageResponse struct {
	Profile *domain.StorageProfile `json:"profile"`
	Message string                 `json:"message"`
}

type storageMessageResponse struct {
	Message string `json:"message"`
}

type storageBucketInfoResponse struct {
	Info map[string]interface{} `json:"info"`
}

type userMessageError interface {
	UserMessage() string
}

func (h *Handler) saveProfile(c *gin.Context, req *domain.StorageProfile) (*domain.StorageProfile, error) {
	if h.service == nil {
		return nil, errors.New(msgSaveProfile)
	}
	if storageShouldApplyProfile(c) {
		return h.service.SaveAndApplyProfile(c.Request.Context(), req)
	}
	return h.service.SaveProfile(c.Request.Context(), req)
}

func storageShouldApplyProfile(c *gin.Context) bool {
	return c.Query("apply") == "true" || c.Query("apply") == "1"
}

func storageSaveProfileMessage(c *gin.Context) string {
	if storageShouldApplyProfile(c) {
		return msgStorageSaveProfileMessage
	}
	return msgStorageSaveProfileMessage2
}
