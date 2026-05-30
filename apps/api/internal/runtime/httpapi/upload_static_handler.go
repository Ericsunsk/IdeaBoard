package httpapi

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"api/internal/auth"
	"api/internal/boards"
	"api/internal/platform/response"
	"api/internal/storage"

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

		if resource.Kind == storage.UploadResourceKindWorkspace && resource.Workspace != nil {
			if !authorizeWorkspaceUploadRead(c, deps, *resource.Workspace) {
				return
			}
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

	userID := c.GetUint64("user_id")
	if userID == 0 {
		response.Error(c, http.StatusUnauthorized, "未登录")
		return false
	}
	return true
}

func authorizeWorkspaceUploadRead(c *gin.Context, deps StaticUploadDeps, uploadPath storage.UploadWorkspaceResource) bool {
	userID := c.GetUint64("user_id")
	if deps.BoardService == nil || deps.Auth.UserRepo == nil {
		response.InternalError(c, http.StatusInternalServerError, "读取上传资源失败", fmt.Errorf("workspace upload dependencies not configured"))
		return false
	}

	board, _, err := deps.BoardService.ResolveBoardAccess(c.Request.Context(), userID, uploadPath.BoardSegment)
	if err != nil {
		switch {
		case errors.Is(err, boards.ErrInvalidBoardInput):
			response.Error(c, http.StatusNotFound, "资源不存在")
		case errors.Is(err, boards.ErrBoardNotFound):
			response.Error(c, http.StatusNotFound, "资源不存在")
		case errors.Is(err, boards.ErrBoardPermissionDenied):
			response.Error(c, http.StatusForbidden, "无权访问该资源")
		default:
			response.InternalError(c, http.StatusInternalServerError, "读取上传资源失败", err)
		}
		return false
	}

	owner, err := deps.Auth.UserRepo.GetByID(c.Request.Context(), board.UserID)
	if err != nil {
		response.InternalError(c, http.StatusInternalServerError, "读取上传资源失败", err)
		return false
	}
	ownerSegment, err := storage.NormalizeWorkspaceStorageUserSegment(owner.StorageUID)
	if err != nil {
		response.InternalError(c, http.StatusInternalServerError, "读取上传资源失败", err)
		return false
	}
	if ownerSegment != uploadPath.OwnerSegment {
		response.Error(c, http.StatusNotFound, "资源不存在")
		return false
	}

	return true
}
