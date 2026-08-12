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
	"ideaboard/api/internal/identity/principal"
	"ideaboard/api/internal/media/assets"
	"ideaboard/api/internal/media/imageformat"
	"ideaboard/api/internal/media/storage"
	"ideaboard/api/internal/platform/response"
	"ideaboard/api/internal/platform/settings"

	"github.com/gin-gonic/gin"
)

var uploadLimiter = &uploadConcurrencyLimiter{}

func requireAuthenticatedUserID(c *gin.Context) (int64, bool) {
	identityPrincipal, ok := principal.Require(c)
	if !ok {
		return 0, false
	}
	return identityPrincipal.UserID(), true
}

type uploadConcurrencyLimiter struct {
	mu       sync.Mutex
	inFlight int
}

type uploadImageResponse struct {
	URL          string `json:"url,omitempty"`
	Avatar       string `json:"avatar,omitempty"`
	MediaAssetID int64  `json:"media_asset_id,omitempty"`
	Width        int    `json:"width,omitempty"`
	Height       int    `json:"height,omitempty"`
	Mime         string `json:"mime,omitempty"`
	Ext          string `json:"ext,omitempty"`
}

type uploadImageMetadata struct {
	Width  int
	Height int
	Mime   string
	Ext    string
}

type boardUploadAssetRegistrar interface {
	RegisterBoardUpload(context.Context, assets.BoardUploadAsset) (int64, error)
}

type boardUploadObjectCleaner interface {
	DeleteObjectByStorageProfile(context.Context, string, string) error
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
		userID, ok := requireAuthenticatedUserID(c)
		if !ok {
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
		userID, ok := requireAuthenticatedUserID(c)
		if !ok {
			return
		}
		boardID := strings.TrimSpace(c.Param("board_id"))
		if h.BoardService == nil || h.UserRepo == nil || h.StorageService == nil || h.MediaAssets == nil {
			response.InternalError(c, 500, "上传失败", fmt.Errorf("board upload dependencies not configured"))
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
		owner, err := h.UserRepo.GetByID(c.Request.Context(), board.OwnerUserID)
		if err != nil {
			response.InternalError(c, 500, "上传失败", err)
			return
		}
		dirPrefix, err := storage.CanvasContentUploadDirPrefix(owner.StorageUID, board.BoardID)
		if err != nil {
			response.InternalError(c, 500, "上传失败", err)
			return
		}
		stored, err := h.StorageService.UploadRefWithOptions(c.Request.Context(), file, storage.StorageStoreOptions{
			DirPrefix: dirPrefix,
		})
		if err != nil {
			response.InternalError(c, 500, "上传失败", err)
			return
		}
		if meta, ok := decodeUploadImageMetadata(file, ext); ok {
			stored.Width = meta.Width
			stored.Height = meta.Height
			stored.MIMEType = meta.Mime
		}
		mediaAssetID, err := registerBoardUploadAsset(c.Request.Context(), h.MediaAssets, h.StorageService, assets.BoardUploadAsset{
			OwnerUserID: board.OwnerUserID,
			BoardID:     board.BoardID,
			Stored:      *stored,
		})
		if err != nil {
			response.InternalError(c, 500, "上传失败", err)
			return
		}
		payload := newUploadImageResponse("url", stored.URL, file, ext)
		payload.MediaAssetID = mediaAssetID
		response.SuccessDTO(c, payload)
	}
}

func registerBoardUploadAsset(
	ctx context.Context,
	registry boardUploadAssetRegistrar,
	cleaner boardUploadObjectCleaner,
	upload assets.BoardUploadAsset,
) (int64, error) {
	mediaAssetID, err := registry.RegisterBoardUpload(ctx, upload)
	if err == nil {
		return mediaAssetID, nil
	}
	cleanupCtx := context.WithoutCancel(ctx)
	cleanupErr := cleaner.DeleteObjectByStorageProfile(
		cleanupCtx,
		upload.Stored.StorageProfile,
		upload.Stored.StorageKey,
	)
	if cleanupErr != nil {
		slog.ErrorContext(cleanupCtx, "cleanup unregistered board upload failed",
			"board_id", upload.BoardID,
			"owner_user_id", upload.OwnerUserID,
			"storage_profile_id", upload.Stored.StorageProfile,
			"storage_key", upload.Stored.StorageKey,
			"registration_error", err,
			"cleanup_error", cleanupErr,
		)
	}
	return 0, err
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
