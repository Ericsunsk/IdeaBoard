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
	identityports "ideaboard/api/internal/identity/users/ports"
	"ideaboard/api/internal/media/storage/application/authorization"
	"ideaboard/api/internal/media/storage/domain"
	storageports "ideaboard/api/internal/media/storage/ports"
	"ideaboard/api/internal/platform/response"

	"github.com/gin-gonic/gin"
)

func uploadStaticHandler(deps StaticUploadDeps) gin.HandlerFunc {
	return func(c *gin.Context) {
		resource, err := domain.ParseUploadResource("/uploads/" + strings.TrimPrefix(c.Param("filepath"), "/"))
		if err != nil {
			c.Status(http.StatusNotFound)
			return
		}

		if resource.Kind == domain.UploadResourceKindSiteAsset {
			serveLocalUploadFile(c, resource.ObjectKey)
			return
		}
		c.Header("Cache-Control", "private, no-store")
		c.Header("Vary", "Cookie")

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
		return 0, domain.ErrUploadAccessDependencyMissing
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
	resource domain.UploadResource,
) (storageports.ReadableUploadAccessResult, bool) {
	userID, ok := requireAuthenticatedUserID(c)
	if !ok {
		return storageports.ReadableUploadAccessResult{}, false
	}
	readAccess, ok := resolveStaticUploadReadAccess(c, deps, resource.Kind)
	if !ok {
		return storageports.ReadableUploadAccessResult{}, false
	}
	result, err := readAccess.AuthorizeUploadResourceRead(c.Request.Context(), userID, resource)
	if err != nil {
		writeStaticUploadReadError(c, resource.Kind, err)
		return storageports.ReadableUploadAccessResult{}, false
	}

	return result, true
}

func resolveStaticUploadReadAccess(
	c *gin.Context,
	deps StaticUploadDeps,
	kind domain.UploadResourceKind,
) (storageports.ReadableUploadAccess, bool) {
	if deps.ReadAccess != nil {
		return deps.ReadAccess, true
	}

	if kind == domain.UploadResourceKindCanvas {
		if deps.BoardService == nil || deps.Auth.UserRepo == nil {
			response.InternalError(c, http.StatusInternalServerError, "读取上传资源失败", fmt.Errorf("canvas upload dependencies not configured"))
			return nil, false
		}
	} else if deps.Auth.UserRepo == nil {
		response.InternalError(c, http.StatusInternalServerError, "读取上传资源失败", fmt.Errorf("upload dependencies not configured"))
		return nil, false
	}

	return authorization.NewReadableUploadAccess(
		staticUploadBoardAccessResolver{service: deps.BoardService},
		staticUploadOwnerStorageUIDResolver{repo: deps.Auth.UserRepo},
	), true
}

func writeStaticUploadReadError(c *gin.Context, kind domain.UploadResourceKind, err error) {
	switch kind {
	case domain.UploadResourceKindCanvas:
		writeCanvasUploadReadError(c, err)
	case domain.UploadResourceKindProfileAvatar, domain.UploadResourceKindTaskOutput:
		writeOwnedUploadReadError(c, err)
	default:
		c.Status(http.StatusNotFound)
	}
}

func writeCanvasUploadReadError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, domain.ErrUploadAccessInvalidPath):
		response.Error(c, http.StatusNotFound, "资源不存在")
	case errors.Is(err, boards.ErrInvalidBoardInput):
		response.Error(c, http.StatusNotFound, "资源不存在")
	case errors.Is(err, boards.ErrBoardNotFound):
		response.Error(c, http.StatusNotFound, "资源不存在")
	case errors.Is(err, boards.ErrBoardPermissionDenied), errors.Is(err, domain.ErrUploadAccessDenied):
		response.Error(c, http.StatusForbidden, "无权访问该资源")
	case errors.Is(err, domain.ErrUploadAccessOwnerMismatch):
		response.Error(c, http.StatusNotFound, "资源不存在")
	default:
		response.InternalError(c, http.StatusInternalServerError, "读取上传资源失败", err)
	}
}

func writeOwnedUploadReadError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, domain.ErrUploadAccessInvalidPath):
		response.Error(c, http.StatusNotFound, "资源不存在")
	case errors.Is(err, domain.ErrUploadAccessDenied), errors.Is(err, domain.ErrUploadAccessOwnerMismatch):
		response.Error(c, http.StatusForbidden, "无权访问该资源")
	default:
		response.InternalError(c, http.StatusInternalServerError, "读取上传资源失败", err)
	}
}

type staticUploadOwnerStorageUIDResolver struct {
	repo identityports.UserRepository
}

func (r staticUploadOwnerStorageUIDResolver) GetStorageUIDByUserID(ctx context.Context, userID int64) (string, error) {
	if r.repo == nil {
		return "", domain.ErrUploadAccessDependencyMissing
	}
	user, err := r.repo.GetByID(ctx, userID)
	if err != nil {
		return "", err
	}
	return user.StorageUID, nil
}
