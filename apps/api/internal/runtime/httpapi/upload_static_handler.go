package httpapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"ideaboard/api/internal/boards"
	"ideaboard/api/internal/identity/auth"
	"ideaboard/api/internal/identity/users"
	"ideaboard/api/internal/media/storage"
	"ideaboard/api/internal/platform/response"

	"github.com/gin-gonic/gin"
)

func uploadStaticHandler(deps StaticUploadDeps) gin.HandlerFunc {
	return func(c *gin.Context) {
		resource, err := storage.ParseUploadResource("/uploads/" + strings.TrimPrefix(c.Param("filepath"), "/"))
		if err != nil {
			c.Status(http.StatusNotFound)
			return
		}

		if resource.Kind == storage.UploadResourceKindSiteAsset {
			serveLocalUploadFile(c, resource.ObjectKey)
			return
		}

		if !authorizeUploadRead(c, deps.Auth) {
			return
		}

		result, ok := authorizeStaticUploadResourceRead(c, deps, resource)
		if !ok {
			return
		}

		serveLocalUploadFile(c, result.Resource.ObjectKey)
	}
}

func serveLocalUploadFile(c *gin.Context, objectKey string) {
	localPath := filepath.FromSlash(objectKey)
	info, err := os.Stat(localPath)
	if err != nil || info.IsDir() {
		c.Status(http.StatusNotFound)
		return
	}
	c.File(localPath)
}

func authorizeUploadRead(c *gin.Context, deps AuthDeps) bool {
	auth.AuthMiddleware(deps.AuthServiceClient, deps.UserRepo, deps.UserLifecycle)(c)
	if c.IsAborted() {
		return false
	}

	_, ok := requireAuthenticatedUserID(c)
	return ok
}

type staticUploadBoardAccessResolver struct {
	service *boards.BoardService
}

func (r staticUploadBoardAccessResolver) ResolveUploadBoardAccess(ctx context.Context, userID int64, boardID string) (int64, error) {
	if r.service == nil {
		return 0, storage.ErrUploadAccessDependencyMissing
	}
	board, _, err := r.service.ResolveBoardAccess(ctx, userID, boardID)
	if err != nil {
		return 0, err
	}
	return board.OwnerUserID, nil
}

func authorizeStaticUploadResourceRead(
	c *gin.Context,
	deps StaticUploadDeps,
	resource storage.UploadResource,
) (storage.ReadableUploadAccessResult, bool) {
	userID, ok := requireAuthenticatedUserID(c)
	if !ok {
		return storage.ReadableUploadAccessResult{}, false
	}
	readAccess, ok := resolveStaticUploadReadAccess(c, deps, resource.Kind)
	if !ok {
		return storage.ReadableUploadAccessResult{}, false
	}
	result, err := readAccess.AuthorizeUploadResourceRead(c.Request.Context(), userID, resource)
	if err != nil {
		writeStaticUploadReadError(c, resource.Kind, err)
		return storage.ReadableUploadAccessResult{}, false
	}

	return result, true
}

func resolveStaticUploadReadAccess(
	c *gin.Context,
	deps StaticUploadDeps,
	kind storage.UploadResourceKind,
) (storage.ReadableUploadAccess, bool) {
	if deps.ReadAccess != nil {
		return deps.ReadAccess, true
	}

	if kind == storage.UploadResourceKindCanvas {
		if deps.BoardService == nil || deps.Auth.UserRepo == nil {
			response.InternalError(c, http.StatusInternalServerError, "读取上传资源失败", fmt.Errorf("canvas upload dependencies not configured"))
			return nil, false
		}
	} else if deps.Auth.UserRepo == nil {
		response.InternalError(c, http.StatusInternalServerError, "读取上传资源失败", fmt.Errorf("upload dependencies not configured"))
		return nil, false
	}

	return storage.NewReadableUploadAccess(
		staticUploadBoardAccessResolver{service: deps.BoardService},
		staticUploadOwnerStorageUIDResolver{repo: deps.Auth.UserRepo},
	), true
}

func writeStaticUploadReadError(c *gin.Context, kind storage.UploadResourceKind, err error) {
	switch kind {
	case storage.UploadResourceKindCanvas:
		writeCanvasUploadReadError(c, err)
	case storage.UploadResourceKindProfileAvatar, storage.UploadResourceKindTaskOutput:
		writeOwnedUploadReadError(c, err)
	default:
		c.Status(http.StatusNotFound)
	}
}

func writeCanvasUploadReadError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, storage.ErrUploadAccessInvalidPath):
		response.Error(c, http.StatusNotFound, "资源不存在")
	case errors.Is(err, boards.ErrInvalidBoardInput):
		response.Error(c, http.StatusNotFound, "资源不存在")
	case errors.Is(err, boards.ErrBoardNotFound):
		response.Error(c, http.StatusNotFound, "资源不存在")
	case errors.Is(err, boards.ErrBoardPermissionDenied), errors.Is(err, storage.ErrUploadAccessDenied):
		response.Error(c, http.StatusForbidden, "无权访问该资源")
	case errors.Is(err, storage.ErrUploadAccessOwnerMismatch):
		response.Error(c, http.StatusNotFound, "资源不存在")
	default:
		response.InternalError(c, http.StatusInternalServerError, "读取上传资源失败", err)
	}
}

func writeOwnedUploadReadError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, storage.ErrUploadAccessInvalidPath):
		response.Error(c, http.StatusNotFound, "资源不存在")
	case errors.Is(err, storage.ErrUploadAccessDenied), errors.Is(err, storage.ErrUploadAccessOwnerMismatch):
		response.Error(c, http.StatusForbidden, "无权访问该资源")
	default:
		response.InternalError(c, http.StatusInternalServerError, "读取上传资源失败", err)
	}
}

type staticUploadOwnerStorageUIDResolver struct {
	repo *users.UserRepository
}

func (r staticUploadOwnerStorageUIDResolver) GetStorageUIDByUserID(ctx context.Context, userID int64) (string, error) {
	if r.repo == nil {
		return "", storage.ErrUploadAccessDependencyMissing
	}
	user, err := r.repo.GetByID(ctx, userID)
	if err != nil {
		return "", err
	}
	return user.StorageUID, nil
}
