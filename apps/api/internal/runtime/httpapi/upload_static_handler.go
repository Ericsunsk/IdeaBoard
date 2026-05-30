package httpapi

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"

	"api/internal/auth"
	"api/internal/boards"
	"api/internal/platform/response"
	"api/internal/storage"

	"github.com/gin-gonic/gin"
)

type workspaceUploadPath struct {
	ownerUID string
	boardID  string
}

func uploadStaticHandler(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		uploadPath, ok := cleanUploadStaticPath(c.Param("filepath"))
		if !ok {
			c.Status(http.StatusNotFound)
			return
		}

		if !authorizeUploadRead(c, deps) {
			return
		}

		workspacePath, isWorkspaceUpload := parseWorkspaceUploadPath(uploadPath)
		if isWorkspaceUpload {
			if !authorizeWorkspaceUploadRead(c, deps, workspacePath) {
				return
			}
		}

		localPath := filepath.FromSlash(uploadPath)
		info, err := os.Stat(localPath)
		if err != nil || info.IsDir() {
			c.Status(http.StatusNotFound)
			return
		}
		c.File(localPath)
	}
}

func cleanUploadStaticPath(raw string) (string, bool) {
	cleaned := path.Clean("/uploads/" + strings.TrimPrefix(raw, "/"))
	if !strings.HasPrefix(cleaned, "/uploads/") {
		return "", false
	}
	return strings.TrimPrefix(cleaned, "/"), true
}

func parseWorkspaceUploadPath(uploadPath string) (workspaceUploadPath, bool) {
	parts := strings.Split(uploadPath, "/")
	if len(parts) < 5 {
		return workspaceUploadPath{}, false
	}
	if parts[0] != "uploads" || parts[1] != "workspace" {
		return workspaceUploadPath{}, false
	}
	if parts[2] == "" || parts[3] == "" || strings.Join(parts[4:], "") == "" {
		return workspaceUploadPath{}, false
	}
	return workspaceUploadPath{
		ownerUID: parts[2],
		boardID:  parts[3],
	}, true
}

func authorizeUploadRead(c *gin.Context, deps Dependencies) bool {
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

func authorizeWorkspaceUploadRead(c *gin.Context, deps Dependencies, uploadPath workspaceUploadPath) bool {
	userID := c.GetUint64("user_id")
	if deps.BoardService == nil || deps.UserRepo == nil {
		response.InternalError(c, http.StatusInternalServerError, "读取上传资源失败", fmt.Errorf("workspace upload dependencies not configured"))
		return false
	}

	board, _, err := deps.BoardService.ResolveBoardAccess(userID, uploadPath.boardID)
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

	owner, err := deps.UserRepo.GetByID(board.UserID)
	if err != nil {
		response.InternalError(c, http.StatusInternalServerError, "读取上传资源失败", err)
		return false
	}
	ownerSegment, err := storage.NormalizeWorkspaceStorageUserSegment(owner.StorageUID)
	if err != nil {
		response.InternalError(c, http.StatusInternalServerError, "读取上传资源失败", err)
		return false
	}
	if ownerSegment != uploadPath.ownerUID {
		response.Error(c, http.StatusNotFound, "资源不存在")
		return false
	}

	return true
}
