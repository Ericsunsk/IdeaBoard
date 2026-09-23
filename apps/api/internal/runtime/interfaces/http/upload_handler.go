package httpapi

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"mime/multipart"
	"net/http"
	"strings"
	"sync"

	"ideaboard/api/internal/boards"
	dbsqlc "ideaboard/api/internal/db/sqlc"
	"ideaboard/api/internal/identity/principal"
	"ideaboard/api/internal/media/imageformat"
	"ideaboard/api/internal/media/ingestion"
	"ideaboard/api/internal/media/storage"
	"ideaboard/api/internal/media/storage/domain"
	"ideaboard/api/internal/media/storage/ports"
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
	URL     string `json:"url,omitempty"`
	Avatar  string `json:"avatar,omitempty"`
	AssetID string `json:"asset_id,omitempty"`
	Width   int    `json:"width,omitempty"`
	Height  int    `json:"height,omitempty"`
	Mime    string `json:"mime,omitempty"`
	Ext     string `json:"ext,omitempty"`
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
			slog.WarnContext(ctx, "runtime media upload settings read failed; using defaults", "err", err)
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
		_, ok = validateUploadFileHeader(c, file, 30*1024*1024, imageformat.UploadAllowedImageExtensions())
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
		dirPrefix, err := domain.ProfileAvatarUploadDirPrefix(user.StorageUID)
		if err != nil {
			response.InternalError(c, 500, "头像上传失败", err)
			return
		}
		stored, err := h.StorageService.UploadRefWithOptions(c.Request.Context(), file, ports.StorageStoreOptions{
			DirPrefix: dirPrefix,
		})
		if err != nil {
			response.InternalError(c, 500, "头像上传失败", err)
			return
		}
		if err := h.UserRepo.UpdateAvatarByID(c.Request.Context(), userID, stored.URL); err != nil {
			if cleanupErr := h.StorageService.DeleteProfileAvatarByURL(c.Request.Context(), user.StorageUID, stored.URL); cleanupErr != nil {
				slog.Warn("cleanup uploaded avatar failed", "user_id", userID, "url", stored.URL, "err", cleanupErr)
			}
			response.InternalError(c, 500, "头像上传失败", err)
			return
		}
		response.SuccessDTO(c, newUploadImageResponseFromStored("avatar", stored))
	}
}

// profileAvatarContentHandler keeps uploaded avatars behind the authenticated
// same-origin API even when the backing storage profile is S3-compatible.
func profileAvatarContentHandler(h UploadDeps) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, ok := requireAuthenticatedUserID(c)
		if !ok {
			return
		}
		if h.UserRepo == nil || h.StorageService == nil {
			response.InternalError(c, http.StatusInternalServerError, "读取头像失败", fmt.Errorf("profile avatar dependencies not configured"))
			return
		}
		user, err := h.UserRepo.GetByID(c.Request.Context(), userID)
		if err != nil || strings.TrimSpace(user.Avatar) == "" {
			response.Error(c, http.StatusNotFound, "头像不存在")
			return
		}
		object, err := h.StorageService.ReadUploadResourceByURL(c.Request.Context(), user.Avatar, 30*1024*1024)
		if err != nil {
			response.Error(c, http.StatusNotFound, "头像不存在")
			return
		}
		c.Header("Cache-Control", "private, max-age=3600")
		c.Header("Content-Disposition", "inline")
		c.Data(http.StatusOK, object.ContentType, object.Data)
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

		file, ok := parseBoardUploadFile(c)
		if !ok {
			return
		}
		userID, ok := requireAuthenticatedUserID(c)
		if !ok {
			return
		}
		board, storeOpts, ok := resolveBoardUploadTarget(c, h, userID)
		if !ok {
			return
		}
		prepared, ok := prepareBoardUpload(c, h, file, storeOpts)
		if !ok {
			return
		}
		boardAsset, found, ok := findExistingBoardAsset(c, h, board.OwnerUserID, prepared)
		if !ok {
			return
		}
		if found {
			response.SuccessDTO(c, newUploadImageResponseFromAsset(boardAsset))
			return
		}
		payload, ok := persistBoardAsset(c, h, board, file, storeOpts, prepared)
		if ok {
			response.SuccessDTO(c, payload)
		}
	}
}

func parseBoardUploadFile(c *gin.Context) (*multipart.FileHeader, bool) {
	file, err := c.FormFile("file")
	if err != nil {
		response.Error(c, 400, "请选择文件")
		return nil, false
	}
	if _, ok := validateUploadFileHeader(c, file, 30*1024*1024, imageformat.UploadAllowedImageExtensions()); !ok {
		return nil, false
	}
	return file, true
}

func resolveBoardUploadTarget(c *gin.Context, h UploadDeps, userID int64) (*boards.Board, ports.StorageStoreOptions, bool) {
	if h.BoardService == nil || h.UserRepo == nil || h.StorageService == nil || h.MediaAssets == nil {
		response.InternalError(c, 500, "上传失败", fmt.Errorf("board upload dependencies not configured"))
		return nil, ports.StorageStoreOptions{}, false
	}
	boardID := strings.TrimSpace(c.Param("board_id"))
	board, _, err := h.BoardService.ResolveBoardWriteAccess(c.Request.Context(), userID, boardID)
	if err != nil {
		switch {
		case errors.Is(err, boards.ErrInvalidInput):
			response.Error(c, 400, "缺少画布ID")
		case errors.Is(err, boards.ErrNotFound):
			response.Error(c, 404, "画布不存在")
		case errors.Is(err, boards.ErrPermissionDenied):
			response.Error(c, 403, "无权编辑该画布")
		default:
			response.InternalError(c, 500, "上传失败", err)
		}
		return nil, ports.StorageStoreOptions{}, false
	}
	owner, err := h.UserRepo.GetByID(c.Request.Context(), board.OwnerUserID)
	if err != nil {
		response.InternalError(c, 500, "上传失败", err)
		return nil, ports.StorageStoreOptions{}, false
	}
	dirPrefix, err := domain.CanvasContentUploadDirPrefix(owner.StorageUID, board.BoardID)
	if err != nil {
		response.InternalError(c, 500, "上传失败", err)
		return nil, ports.StorageStoreOptions{}, false
	}
	return board, ports.StorageStoreOptions{DirPrefix: dirPrefix}, true
}

func prepareBoardUpload(c *gin.Context, h UploadDeps, file *multipart.FileHeader, storeOpts ports.StorageStoreOptions) (*ports.StoredUploadRef, bool) {
	// Inspect bytes before planning the Media Asset so the object key and
	// metadata use the canonical format rather than the client filename.
	prepared, err := h.StorageService.PrepareUploadRefFromFile(c.Request.Context(), file, storeOpts)
	if err != nil {
		response.InternalError(c, 500, "上传失败", err)
		return nil, false
	}
	return prepared, true
}

func findExistingBoardAsset(c *gin.Context, h UploadDeps, ownerUserID int64, prepared *ports.StoredUploadRef) (dbsqlc.MediaAsset, bool, bool) {
	if prepared.ChecksumSHA256 == "" {
		return dbsqlc.MediaAsset{}, false, true
	}
	// Reuse an existing ready asset for identical bytes owned by this user.
	// This keeps duplicate uploads from creating another physical object while
	// preserving the existing board snapshot contract (asset ID + stable URL).
	existing, found, err := h.MediaAssets.FindReadyOwnedMediaAssetByChecksum(c.Request.Context(), ownerUserID, prepared.StorageProfile, prepared.ChecksumSHA256)
	if err != nil {
		response.InternalError(c, 500, "上传失败", err)
		return dbsqlc.MediaAsset{}, false, false
	}
	if !found {
		return dbsqlc.MediaAsset{}, false, true
	}
	return confirmDedupedBoardAsset(c, h.StorageService, h.MediaAssets, ownerUserID, existing)
}

// dedupeObjectProber is the narrow storage capability the dedupe liveness
// gate needs. *storage.Service satisfies it implicitly; tests substitute fakes.
type dedupeObjectProber interface {
	ReadObjectAtProfile(ctx context.Context, profileID, objectKey string, maxBytes int64) (*ports.StoredUploadObject, error)
}

// dedupeAssetRetirer releases the checksum slot held by a dead deduped row.
type dedupeAssetRetirer interface {
	RetireMissingOwnedMediaAsset(ctx context.Context, ownerUserID, assetID int64) error
}

type dedupeDerivativeLocator interface {
	LocateStoredDerivative(ctx context.Context, lookup ports.StoredDerivativeLookup) (ports.StoredUploadLocation, bool, error)
}

// confirmDedupedBoardAsset verifies that a checksum-deduped asset and its
// provider-neutral processing-input are still usable before serving it. Either
// object can disappear while the rows survive (e.g. wiped with a deleted
// board), and serving such a row wedges clients on derivatives that can never
// materialize. A verifiably missing object retires the dead row — freeing the
// checksum slot — so this upload falls through to a fresh persist with the
// bytes already in hand.
// Transient storage errors fail open to preserve existing behavior.
func confirmDedupedBoardAsset(c *gin.Context, prober dedupeObjectProber, retirer dedupeAssetRetirer, ownerUserID int64, existing dbsqlc.MediaAsset) (dbsqlc.MediaAsset, bool, bool) {
	if prober == nil {
		return existing, true, true
	}
	ctx := c.Request.Context()
	probeErr := func() error {
		probe, err := prober.ReadObjectAtProfile(ctx, existing.StorageProfileID, existing.StorageKey, 1)
		if err != nil {
			return err
		}
		if probe == nil || len(probe.Data) == 0 {
			return domain.ErrStoredObjectNotFound
		}
		locator, ok := retirer.(dedupeDerivativeLocator)
		if !ok {
			return nil
		}
		location, found, err := locator.LocateStoredDerivative(ctx, ports.StoredDerivativeLookup{AssetID: existing.ID, Tier: "processing-input"})
		if err != nil {
			return err
		}
		if !found || strings.TrimSpace(location.StorageProfileID) == "" || strings.TrimSpace(location.StorageKey) == "" {
			return domain.ErrStoredObjectNotFound
		}
		derivative, err := prober.ReadObjectAtProfile(ctx, location.StorageProfileID, location.StorageKey, 1)
		if err != nil {
			return err
		}
		if derivative == nil || len(derivative.Data) == 0 {
			return domain.ErrStoredObjectNotFound
		}
		return nil
	}()
	if probeErr == nil {
		return existing, true, true
	}
	if !errors.Is(probeErr, domain.ErrStoredObjectNotFound) {
		slog.Warn("dedupe asset liveness probe failed", "asset_id", existing.AssetID, "err", probeErr)
		return existing, true, true
	}
	if retirer == nil {
		return existing, true, true
	}
	if err := retirer.RetireMissingOwnedMediaAsset(ctx, ownerUserID, existing.ID); err != nil {
		slog.Warn("retire missing deduped asset failed", "asset_id", existing.AssetID, "err", err)
		response.InternalError(c, 500, "上传失败", err)
		return dbsqlc.MediaAsset{}, false, false
	}
	slog.Info("retired deduped media asset with missing object", "asset_id", existing.AssetID)
	return dbsqlc.MediaAsset{}, false, true
}

func persistBoardAsset(c *gin.Context, h UploadDeps, board *boards.Board, file *multipart.FileHeader, storeOpts ports.StorageStoreOptions, prepared *ports.StoredUploadRef) (uploadImageResponse, bool) {
	handle, err := h.MediaAssets.Begin(c.Request.Context(), ingestion.Request{
		OwnerUserID: board.OwnerUserID, StorageProfile: prepared.StorageProfile,
		StorageKey: prepared.StorageKey, URL: prepared.URL,
		Origin: ingestion.BoardUploadFacts{BoardID: board.BoardID},
	})
	if err != nil {
		response.InternalError(c, 500, "上传失败", err)
		return uploadImageResponse{}, false
	}
	storeOpts.ObjectKey = prepared.StorageKey
	stored, err := h.StorageService.UploadRefAtProfile(c.Request.Context(), prepared.StorageProfile, file, storeOpts)
	if err != nil {
		_ = h.MediaAssets.Fail(context.WithoutCancel(c.Request.Context()), handle, err)
		response.InternalError(c, 500, "上传失败", err)
		return uploadImageResponse{}, false
	}
	stored.Width = prepared.Width
	stored.Height = prepared.Height
	stored.MIMEType = prepared.MIMEType
	stored.Animated = prepared.Animated
	asset, err := h.MediaAssets.GetByID(c.Request.Context(), board.OwnerUserID, handle.AssetID)
	if err != nil {
		_ = h.MediaAssets.Fail(context.WithoutCancel(c.Request.Context()), handle, err)
		_ = h.StorageService.DeleteObjectByStorageProfile(context.WithoutCancel(c.Request.Context()), stored.StorageProfile, stored.StorageKey)
		response.InternalError(c, 500, "上传失败", err)
		return uploadImageResponse{}, false
	}
	// Browser-visible board media must always use the authenticated
	// same-origin delivery route, regardless of whether the backing profile
	// is local or S3-compatible. Keep provider URLs server-side only.
	stored.URL = mediaAssetOriginalDeliveryURL(asset.AssetID)
	if err := h.MediaAssets.Complete(c.Request.Context(), handle, stored.IngestionCompletion("")); err != nil {
		return convergeBoardAssetCompletionFailure(c, h, board.OwnerUserID, handle, stored, err)
	}
	payload := newUploadImageResponseFromStored("url", stored)
	payload.AssetID = asset.AssetID
	return payload, true
}

func convergeBoardAssetCompletionFailure(c *gin.Context, h UploadDeps, ownerUserID int64, handle ingestion.Handle, stored *ports.StoredUploadRef, completionErr error) (uploadImageResponse, bool) {
	// A concurrent identical upload may have committed first and won the
	// checksum unique index. Re-read the canonical asset before surfacing an
	// error so the loser still succeeds without retaining its object.
	if existing, found, lookupErr := h.MediaAssets.FindReadyOwnedMediaAssetByChecksum(c.Request.Context(), ownerUserID, stored.StorageProfile, stored.ChecksumSHA256); lookupErr == nil && found {
		// The lookup may return this very handle's asset when Complete
		// committed the row before failing (e.g. a deferred post-commit
		// step). That object is live and owned by the returned asset:
		// deleting it would destroy a healthy upload.
		if existing.ID == handle.AssetID {
			return newUploadImageResponseFromAsset(existing), true
		}
		cleanupCtx := context.WithoutCancel(c.Request.Context())
		_ = h.StorageService.DeleteObjectByStorageProfile(cleanupCtx, stored.StorageProfile, stored.StorageKey)
		_ = h.MediaAssets.Fail(cleanupCtx, handle, completionErr)
		return newUploadImageResponseFromAsset(existing), true
	}
	_ = h.MediaAssets.Fail(context.WithoutCancel(c.Request.Context()), handle, completionErr)
	_ = h.StorageService.DeleteObjectByStorageProfile(context.WithoutCancel(c.Request.Context()), stored.StorageProfile, stored.StorageKey)
	response.InternalError(c, 500, "上传失败", completionErr)
	return uploadImageResponse{}, false
}

func newUploadImageResponseFromStored(urlKey string, stored *ports.StoredUploadRef) uploadImageResponse {
	payload := uploadImageResponse{}
	if stored == nil {
		return payload
	}
	if urlKey == "avatar" {
		payload.Avatar = profileAvatarDeliveryURL
	} else {
		payload.URL = stored.URL
	}
	payload.Width = stored.Width
	payload.Height = stored.Height
	payload.Mime = stored.MIMEType
	if ext := imageformat.ImageExtensionFromMIME(stored.MIMEType); ext != "" {
		payload.Ext = strings.TrimPrefix(ext, ".")
	}
	return payload
}

func newUploadImageResponseFromAsset(asset dbsqlc.MediaAsset) uploadImageResponse {
	payload := uploadImageResponse{
		URL:     mediaAssetOriginalDeliveryURL(asset.AssetID),
		AssetID: asset.AssetID,
		Width:   int(asset.Width),
		Height:  int(asset.Height),
		Mime:    asset.MimeType,
	}
	if ext := imageformat.ImageExtensionFromMIME(asset.MimeType); ext != "" {
		payload.Ext = strings.TrimPrefix(ext, ".")
	}
	return payload
}

const profileAvatarDeliveryURL = "/api/user/profile/avatar/content"

func mediaAssetOriginalDeliveryURL(assetID string) string {
	if strings.TrimSpace(assetID) == "" {
		return ""
	}
	return fmt.Sprintf("/api/user/media-assets/%s/original/content", assetID)
}

func adminUploadHandler(storageService *storage.Service, uploadSettings settings.MediaUploadSettingProvider) gin.HandlerFunc {
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
		_, ok = validateUploadFileHeader(c, file, 50*1024*1024, imageformat.UploadAllowedImageExtensions())
		if !ok {
			return
		}

		if storageService == nil {
			response.InternalError(c, 500, "上传失败", fmt.Errorf("storage service not configured"))
			return
		}
		stored, err := storageService.UploadRefWithOptions(c.Request.Context(), file, ports.StorageStoreOptions{
			DirPrefix: domain.SiteAssetUploadDirPrefix(),
		})
		if err != nil {
			response.InternalError(c, 500, "上传失败", err)
			return
		}
		response.SuccessDTO(c, newUploadImageResponseFromStored("url", stored))
	}
}

// validateUploadFileHeader centralises upload validation:
// 1. File size limit.
// 2. Format detection and image-config validation from actual bytes.
// 3. Detected-format allow-list enforcement (prevents metadata spoofing).
// On failure it writes an error response to c and returns ("", false).
func validateUploadFileHeader(c *gin.Context, fh *multipart.FileHeader, maxBytes int64, allowedExts map[string]bool) (ext string, ok bool) {
	if fh == nil {
		response.Error(c, 400, "请选择文件")
		return "", false
	}
	if fh.Size > maxBytes {
		response.Error(c, 400, fmt.Sprintf("文件大小不能超过 %dMB", maxBytes/1024/1024))
		return "", false
	}
	facts, err := imageformat.InspectFileHeader(fh)
	if err != nil || !allowedExts[facts.Extension] {
		if err != nil {
			slog.Warn("upload file inspect failed", "err", err)
			response.Error(c, 400, "文件内容不合法")
		} else {
			response.Error(c, 400, "仅支持 "+imageformat.SupportedUploadImageLabel()+" 格式")
		}
		return "", false
	}
	ext = facts.Extension
	return ext, true
}
