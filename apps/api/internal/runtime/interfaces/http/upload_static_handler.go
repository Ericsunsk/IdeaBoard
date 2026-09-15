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
		resource, err := domain.ParseUploadResource(UploadsStaticRoutePrefix + strings.TrimPrefix(c.Param("filepath"), "/"))
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
	localPath, err := jailUploadStaticPath(objectKey)
	if err != nil {
		c.Status(http.StatusNotFound)
		return
	}
	// Lstat (not Stat): a symlink inside uploads must never be followed to
	// resolve outside the jail. Uploads only contain regular files.
	info, err := os.Lstat(localPath)
	if err != nil || info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		c.Status(http.StatusNotFound)
		return
	}
	// User-controlled bytes are served same-origin. SVG/active content is
	// rejected at upload, but sandbox neuters direct-navigation script
	// execution as defense in depth. Safe for <img> embedding: subresource
	// loads ignore the image response CSP.
	c.Header("Content-Security-Policy", "sandbox")
	c.File(localPath)
}

// jailUploadStaticPath re-asserts the domain-boundary jail at the serving
// layer: objectKey must stay under the runtime uploads root after cleaning.
// Canonical cleaning lives in domain.NormalizeUploadObjectKey (single source);
// the Abs/Rel check below only guards against root relocation between the
// two calls. ParseUploadResource already enforces this; serving must not
// trust callers.
func jailUploadStaticPath(objectKey string) (string, error) {
	normalized, err := domain.NormalizeUploadObjectKey(objectKey)
	if err != nil {
		return "", err
	}
	cleaned := filepath.FromSlash(normalized)
	root := "uploads"
	if cleaned != root && !strings.HasPrefix(cleaned, root+string(filepath.Separator)) {
		return "", fmt.Errorf("upload object key escapes uploads root")
	}
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("resolve uploads root: %w", err)
	}
	fullAbs, err := filepath.Abs(cleaned)
	if err != nil {
		return "", fmt.Errorf("resolve upload path: %w", err)
	}
	relative, err := filepath.Rel(rootAbs, fullAbs)
	if err != nil {
		return "", fmt.Errorf("relativize upload path: %w", err)
	}
	if relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("upload object key escapes uploads root")
	}
	return fullAbs, nil
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
	service *boards.Service
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
	case errors.Is(err, boards.ErrInvalidInput):
		response.Error(c, http.StatusNotFound, "资源不存在")
	case errors.Is(err, boards.ErrNotFound):
		response.Error(c, http.StatusNotFound, "资源不存在")
	case errors.Is(err, boards.ErrPermissionDenied), errors.Is(err, domain.ErrUploadAccessDenied):
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
