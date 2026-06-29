package httpapi

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"mime/multipart"
	"strings"
	"sync"

	"ideaboard/api/internal/boards"
	"ideaboard/api/internal/media/imageformat"
	"ideaboard/api/internal/media/storage"
	"ideaboard/api/internal/platform/response"
	"ideaboard/api/internal/platform/settings"

	"github.com/gin-gonic/gin"
)

var uploadLimiter = &uploadConcurrencyLimiter{}

type uploadConcurrencyLimiter struct {
	mu       sync.Mutex
	inFlight int
}

type uploadImageResponse struct {
	URL    string `json:"url,omitempty"`
	Avatar string `json:"avatar,omitempty"`
	Width  int    `json:"width,omitempty"`
	Height int    `json:"height,omitempty"`
	Mime   string `json:"mime,omitempty"`
	Ext    string `json:"ext,omitempty"`
}

type deleteUploadResponse struct {
	Deleted bool `json:"deleted"`
}

type uploadImageMetadata struct {
	Width  int
	Height int
	Mime   string
	Ext    string
}

func acquireUploadSlot(ctx context.Context, provider settings.MediaUploadSettingProvider) (func(), bool) {
	limit := resolveUploadMaxConcurrent(ctx, provider)
	return uploadLimiter.acquire(limit)
}

func (l *uploadConcurrencyLimiter) acquire(limit int) (func(), bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.inFlight >= limit {
		return nil, false
	}
	l.inFlight++
	return func() {
		l.mu.Lock()
		defer l.mu.Unlock()
		if l.inFlight > 0 {
			l.inFlight--
		}
	}, true
}

func resolveUploadMaxConcurrent(ctx context.Context, provider settings.MediaUploadSettingProvider) int {
	setting := settings.DefaultMediaUploadSetting()
	if provider != nil {
		nextSetting, err := provider.GetMediaUpload(ctx)
		if err != nil {
			slog.WarnContext(ctx, "runtime media upload settings read failed; using defaults", "error", err)
		} else {
			setting = nextSetting
		}
	}
	normalized, err := settings.NormalizeMediaUploadSetting(setting)
	if err != nil {
		return settings.DefaultMediaUploadSetting().UploadMaxConcurrent
	}
	return normalized.UploadMaxConcurrent
}

func uploadProfileAvatarHandler(h UploadDeps) gin.HandlerFunc {
	return func(c *gin.Context) {
		release, ok := acquireUploadSlot(c.Request.Context(), h.UploadSettings)
		if !ok {
			response.Error(c, 429, "当前上传请求过多，请稍后重试")
			return
		}
		defer release()

		file, err := c.FormFile("file")
		if err != nil {
			response.Error(c, 400, "请选择文件")
			return
		}
		ext, ok := validateUploadFileHeader(c, file, 30*1024*1024, imageformat.UploadAllowedImageExtensions())
		if !ok {
			return
		}
		userID := c.GetInt64("user_id")
		if userID == 0 {
			response.Error(c, 401, "未登录")
			return
		}
		if h.UserRepo == nil || h.StorageService == nil {
			response.InternalError(c, 500, "头像上传失败", fmt.Errorf("profile upload dependencies not configured"))
			return
		}
		user, err := h.UserRepo.GetByID(c.Request.Context(), userID)
		if err != nil {
			response.InternalError(c, 500, "头像上传失败", err)
			return
		}
		dirPrefix, err := storage.ProfileAvatarUploadDirPrefix(user.StorageUID)
		if err != nil {
			response.InternalError(c, 500, "头像上传失败", err)
			return
		}
		url, err := h.StorageService.UploadWithOptions(c.Request.Context(), file, storage.StorageStoreOptions{
			DirPrefix: dirPrefix,
		})
		if err != nil {
			response.InternalError(c, 500, "头像上传失败", err)
			return
		}
		if err := h.UserRepo.UpdateAvatarByID(c.Request.Context(), userID, url); err != nil {
			if cleanupErr := h.StorageService.DeleteProfileAvatarByURL(c.Request.Context(), user.StorageUID, url); cleanupErr != nil {
				slog.Warn("cleanup uploaded avatar failed", "user_id", userID, "url", url, "err", cleanupErr)
			}
			response.InternalError(c, 500, "头像上传失败", err)
			return
		}
		response.SuccessDTO(c, newUploadImageResponse("avatar", url, file, ext))
	}
}

func uploadBoardAssetHandler(h UploadDeps) gin.HandlerFunc {
	return func(c *gin.Context) {
		release, ok := acquireUploadSlot(c.Request.Context(), h.UploadSettings)
		if !ok {
			response.Error(c, 429, "当前上传请求过多，请稍后重试")
			return
		}
		defer release()

		file, err := c.FormFile("file")
		if err != nil {
			response.Error(c, 400, "请选择文件")
			return
		}
		ext, ok := validateUploadFileHeader(c, file, 30*1024*1024, imageformat.UploadAllowedImageExtensions())
		if !ok {
			return
		}
		userID := c.GetInt64("user_id")
		if userID == 0 {
			response.Error(c, 401, "未登录")
			return
		}
		boardID := strings.TrimSpace(c.Param("board_id"))
		if h.BoardService == nil {
			response.InternalError(c, 500, "上传失败", fmt.Errorf("board service not configured"))
			return
		}
		board, _, err := h.BoardService.ResolveBoardWriteAccess(c.Request.Context(), userID, boardID)
		if err != nil {
			switch {
			case errors.Is(err, boards.ErrInvalidBoardInput):
				response.Error(c, 400, "缺少画布ID")
			case errors.Is(err, boards.ErrBoardNotFound):
				response.Error(c, 404, "画布不存在")
			case errors.Is(err, boards.ErrBoardPermissionDenied):
				response.Error(c, 403, "无权编辑该画布")
			default:
				response.InternalError(c, 500, "上传失败", err)
			}
			return
		}
		owner, err := h.UserRepo.GetByID(c.Request.Context(), board.UserID)
		if err != nil {
			response.InternalError(c, 500, "上传失败", err)
			return
		}
		dirPrefix, err := storage.WorkspaceContentUploadDirPrefix(owner.StorageUID, board.BoardID)
		if err != nil {
			response.Error(c, 400, err.Error())
			return
		}
		url, err := h.StorageService.UploadWithOptions(c.Request.Context(), file, storage.StorageStoreOptions{
			DirPrefix: dirPrefix,
		})
		if err != nil {
			response.InternalError(c, 500, "上传失败", err)
			return
		}
		response.SuccessDTO(c, newUploadImageResponse("url", url, file, ext))
	}
}

func deleteBoardUploadHandler(h UploadDeps) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			URL string `json:"url"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			response.Error(c, 400, "请求参数错误")
			return
		}
		userID := c.GetInt64("user_id")
		if userID == 0 {
			response.Error(c, 401, "未登录")
			return
		}
		if h.BoardService == nil {
			response.InternalError(c, 500, "删除失败", fmt.Errorf("board service not configured"))
			return
		}
		boardID := strings.TrimSpace(c.Param("board_id"))
		board, _, err := h.BoardService.ResolveBoardWriteAccess(c.Request.Context(), userID, boardID)
		if err != nil {
			switch {
			case errors.Is(err, boards.ErrInvalidBoardInput):
				response.Error(c, 400, "缺少画布ID")
			case errors.Is(err, boards.ErrBoardNotFound):
				response.Error(c, 404, "画布不存在")
			case errors.Is(err, boards.ErrBoardPermissionDenied):
				response.Error(c, 403, "无权编辑该画布")
			default:
				response.InternalError(c, 500, "删除失败", err)
			}
			return
		}
		owner, err := h.UserRepo.GetByID(c.Request.Context(), board.UserID)
		if err != nil {
			response.InternalError(c, 500, "删除失败", err)
			return
		}
		if err := h.StorageService.DeleteWorkspaceUploadByURL(c.Request.Context(), owner.StorageUID, board.BoardID, req.URL); err != nil {
			response.Error(c, 400, err.Error())
			return
		}
		response.SuccessDTO(c, deleteUploadResponse{Deleted: true})
	}
}

func adminUploadHandler(storageService *storage.StorageService, uploadSettings settings.MediaUploadSettingProvider) gin.HandlerFunc {
	return func(c *gin.Context) {
		release, ok := acquireUploadSlot(c.Request.Context(), uploadSettings)
		if !ok {
			response.Error(c, 429, "当前上传请求过多，请稍后重试")
			return
		}
		defer release()

		file, err := c.FormFile("file")
		if err != nil {
			response.Error(c, 400, "请选择文件")
			return
		}
		ext, ok := validateUploadFileHeader(c, file, 50*1024*1024, imageformat.UploadAllowedImageExtensions())
		if !ok {
			return
		}

		if storageService == nil {
			response.InternalError(c, 500, "上传失败", fmt.Errorf("storage service not configured"))
			return
		}
		url, err := storageService.UploadWithOptions(c.Request.Context(), file, storage.StorageStoreOptions{
			DirPrefix: storage.SiteAssetUploadDirPrefix(),
		})
		if err != nil {
			response.InternalError(c, 500, "上传失败", err)
			return
		}
		response.SuccessDTO(c, newUploadImageResponse("url", url, file, ext))
	}
}

func newUploadImageResponse(urlKey string, url string, file *multipart.FileHeader, ext string) uploadImageResponse {
	payload := uploadImageResponse{}
	switch urlKey {
	case "avatar":
		payload.Avatar = url
	default:
		payload.URL = url
	}
	if meta, ok := decodeUploadImageMetadata(file, ext); ok {
		payload.Width = meta.Width
		payload.Height = meta.Height
		payload.Mime = meta.Mime
		payload.Ext = meta.Ext
	}
	return payload
}
