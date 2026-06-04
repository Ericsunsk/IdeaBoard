package httpapi

import (
	"errors"
	"fmt"
	"log/slog"
	"mime/multipart"
	"os"
	"strconv"
	"strings"
	"sync"

	"ideaboard/api/internal/boards"
	"ideaboard/api/internal/platform/media"
	"ideaboard/api/internal/platform/response"
	"ideaboard/api/internal/storage"

	"github.com/gin-gonic/gin"
)

const defaultUploadMaxConcurrent = 8

var (
	uploadLimiterOnce sync.Once
	uploadLimiter     chan struct{}
)

func acquireUploadSlot() (func(), bool) {
	uploadLimiterOnce.Do(func() {
		limit := defaultUploadMaxConcurrent
		if raw := strings.TrimSpace(os.Getenv("UPLOAD_MAX_CONCURRENT")); raw != "" {
			if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
				limit = parsed
			}
		}
		uploadLimiter = make(chan struct{}, limit)
	})
	select {
	case uploadLimiter <- struct{}{}:
		return func() { <-uploadLimiter }, true
	default:
		return nil, false
	}
}

func uploadProfileAvatarHandler(h UploadDeps) gin.HandlerFunc {
	return func(c *gin.Context) {
		release, ok := acquireUploadSlot()
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
		ext, ok := validateUploadFileHeader(c, file, 30*1024*1024, media.UploadAllowedImageExtensions())
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
		response.Success(c, uploadImageResponsePayload("avatar", url, file, ext))
	}
}

func uploadBoardAssetHandler(h UploadDeps) gin.HandlerFunc {
	return func(c *gin.Context) {
		release, ok := acquireUploadSlot()
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
		ext, ok := validateUploadFileHeader(c, file, 30*1024*1024, media.UploadAllowedImageExtensions())
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
		response.Success(c, uploadImageResponsePayload("url", url, file, ext))
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
		response.Success(c, gin.H{"deleted": true})
	}
}

func adminUploadHandler(storageService *storage.StorageService) gin.HandlerFunc {
	return func(c *gin.Context) {
		release, ok := acquireUploadSlot()
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
		ext, ok := validateUploadFileHeader(c, file, 50*1024*1024, media.UploadAllowedImageExtensions())
		if !ok {
			return
		}

		if storageService == nil {
			response.InternalError(c, 500, "上传失败", fmt.Errorf("storage service not configured"))
			return
		}
		url, err := storageService.Upload(c.Request.Context(), file)
		if err != nil {
			response.InternalError(c, 500, "上传失败", err)
			return
		}
		response.Success(c, uploadImageResponsePayload("url", url, file, ext))
	}
}

func uploadImageResponsePayload(urlKey string, url string, file *multipart.FileHeader, ext string) gin.H {
	payload := gin.H{urlKey: url}
	if meta, ok := decodeUploadImageMetadata(file, ext); ok {
		for k, v := range meta {
			payload[k] = v
		}
	}
	return payload
}
