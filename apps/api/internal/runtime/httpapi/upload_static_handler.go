package httpapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"ideaboard/api/internal/auth"
	"ideaboard/api/internal/boards"
	"ideaboard/api/internal/platform/response"
	"ideaboard/api/internal/storage"
	"ideaboard/api/internal/users"

	"github.com/gin-gonic/gin"
)

func uploadStaticHandler(deps StaticUploadDeps) gin.HandlerFunc {
	return func(c *gin.Context) {
		resource, err := storage.ParseUploadResource("/uploads/" + strings.TrimPrefix(c.Param("filepath"), "/"))
		if err != nil {
			c.Status(http.StatusNotFound)
			return
		}

		if !authorizeUploadRead(c, deps.Auth) {
			return
		}

		switch resource.Kind {
		case storage.UploadResourceKindWorkspace:
			if resource.Workspace == nil || !authorizeWorkspaceUploadRead(c, deps, *resource.Workspace) {
				return
			}
		case storage.UploadResourceKindProfileAvatar:
		case storage.UploadResourceKindTaskOutput:
			if !authorizeOwnedUploadRead(c, deps, resource) {
				return
			}
		default:
			c.Status(http.StatusNotFound)
			return
		}

		localPath := filepath.FromSlash(resource.ObjectKey)
		info, err := os.Stat(localPath)
		if err != nil || info.IsDir() {
			c.Status(http.StatusNotFound)
			return
		}
		c.File(localPath)
	}
}

func authorizeUploadRead(c *gin.Context, deps AuthDeps) bool {
	auth.AuthMiddleware(deps.AuthServiceClient, deps.UserRepo)(c)
	if c.IsAborted() {
		return false
	}

	userID := c.GetInt64("user_id")
	if userID == 0 {
		response.Error(c, http.StatusUnauthorized, "未登录")
		return false
	}
	return true
}

type staticUploadBoardAccessResolver struct {
	service *boards.BoardService
}

func (r staticUploadBoardAccessResolver) ResolveUploadBoardAccess(ctx context.Context, userID int64, boardID string) (int64, error) {
	board, _, err := r.service.ResolveBoardAccess(ctx, userID, boardID)
	if err != nil {
		return 0, err
	}
	return board.UserID, nil
}

func authorizeWorkspaceUploadRead(c *gin.Context, deps StaticUploadDeps, uploadPath storage.UploadWorkspaceResource) bool {
	userID := c.GetInt64("user_id")
	if deps.BoardService == nil || deps.Auth.UserRepo == nil {
		response.InternalError(c, http.StatusInternalServerError, "读取上传资源失败", fmt.Errorf("workspace upload dependencies not configured"))
		return false
	}
	resource := storage.UploadResource{
		Kind: storage.UploadResourceKindWorkspace,
		Workspace: &storage.UploadWorkspaceResource{
			OwnerSegment: uploadPath.OwnerSegment,
			BoardSegment: uploadPath.BoardSegment,
			RelativePath: uploadPath.RelativePath,
		},
	}
	err := storage.AuthorizeUploadResourceRead(
		c.Request.Context(),
		userID,
		resource,
		staticUploadBoardAccessResolver{service: deps.BoardService},
		staticUploadOwnerStorageUIDResolver{repo: deps.Auth.UserRepo},
	)
	if err != nil {
		switch {
		case errors.Is(err, boards.ErrInvalidBoardInput):
			response.Error(c, http.StatusNotFound, "资源不存在")
		case errors.Is(err, boards.ErrBoardNotFound):
			response.Error(c, http.StatusNotFound, "资源不存在")
		case errors.Is(err, boards.ErrBoardPermissionDenied):
			response.Error(c, http.StatusForbidden, "无权访问该资源")
		case errors.Is(err, storage.ErrUploadAccessOwnerMismatch):
			response.Error(c, http.StatusNotFound, "资源不存在")
		default:
			response.InternalError(c, http.StatusInternalServerError, "读取上传资源失败", err)
		}
		return false
	}

	return true
}

func authorizeOwnedUploadRead(c *gin.Context, deps StaticUploadDeps, resource storage.UploadResource) bool {
	userID := c.GetInt64("user_id")
	if deps.Auth.UserRepo == nil {
		response.InternalError(c, http.StatusInternalServerError, "读取上传资源失败", fmt.Errorf("upload dependencies not configured"))
		return false
	}
	err := storage.AuthorizeUploadResourceRead(
		c.Request.Context(),
		userID,
		resource,
		staticUploadBoardAccessResolver{service: deps.BoardService},
		staticUploadOwnerStorageUIDResolver{repo: deps.Auth.UserRepo},
	)
	if err == nil {
		return true
	}
	switch {
	case errors.Is(err, storage.ErrUploadAccessInvalidPath):
		response.Error(c, http.StatusNotFound, "资源不存在")
	case errors.Is(err, storage.ErrUploadAccessDenied), errors.Is(err, storage.ErrUploadAccessOwnerMismatch):
		response.Error(c, http.StatusForbidden, "无权访问该资源")
	default:
		response.InternalError(c, http.StatusInternalServerError, "读取上传资源失败", err)
	}
	return false
}

type staticUploadOwnerStorageUIDResolver struct {
	repo *users.UserRepository
}

func (r staticUploadOwnerStorageUIDResolver) GetStorageUIDByUserID(ctx context.Context, userID int64) (string, error) {
	user, err := r.repo.GetByID(ctx, userID)
	if err != nil {
		return "", err
	}
	return user.StorageUID, nil
}
