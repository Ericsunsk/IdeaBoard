package storage

import (
	"fmt"
	"path"
	"strings"
)

const (
	uploadObjectRoot = "uploads"
)

type UploadResourceKind string

const (
	UploadResourceKindCanvas        UploadResourceKind = "canvas"
	UploadResourceKindProfileAvatar UploadResourceKind = "profile_avatar"
	UploadResourceKindTaskOutput    UploadResourceKind = "task_output"
	UploadResourceKindLibrary       UploadResourceKind = "library"
	UploadResourceKindSiteAsset     UploadResourceKind = "site_asset"
)

type UploadCanvasResource struct {
	OwnerSegment string
	BoardSegment string
	RelativePath string
}

type UploadProfileAvatarResource struct {
	OwnerSegment string
	RelativePath string
}

type UploadTaskOutputResource struct {
	UserSegment  string
	TaskSegment  string
	RelativePath string
}

type UploadLibraryResource struct {
	UserSegment  string
	AssetSegment string
	RelativePath string
}

type UploadSiteAssetResource struct {
	RelativePath string
}

type UploadResource struct {
	Kind          UploadResourceKind
	ObjectKey     string
	Canvas        *UploadCanvasResource
	ProfileAvatar *UploadProfileAvatarResource
	TaskOutput    *UploadTaskOutputResource
	Library       *UploadLibraryResource
	SiteAsset     *UploadSiteAssetResource
}

func CanvasContentUploadDirPrefix(storageUID string, boardID string) (string, error) {
	ownerSegment, err := NormalizeCanvasStorageUserSegment(storageUID)
	if err != nil {
		return "", err
	}
	boardSegment, err := NormalizeCanvasBoardSegment(boardID)
	if err != nil {
		return "", err
	}
	return path.Join("canvas", ownerSegment, boardSegment, "content"), nil
}

func CanvasUserUploadObjectPrefix(storageUID string) (string, error) {
	ownerSegment, err := NormalizeCanvasStorageUserSegment(storageUID)
	if err != nil {
		return "", err
	}
	return path.Join(uploadObjectRoot, "canvas", ownerSegment) + "/", nil
}

func CanvasBoardUploadObjectPrefix(storageUID string, boardID string) (string, error) {
	ownerSegment, err := NormalizeCanvasStorageUserSegment(storageUID)
	if err != nil {
		return "", err
	}
	boardSegment, err := NormalizeCanvasBoardSegment(boardID)
	if err != nil {
		return "", err
	}
	return path.Join(uploadObjectRoot, "canvas", ownerSegment, boardSegment) + "/", nil
}

func ProfileAvatarUploadDirPrefix(storageUID string) (string, error) {
	ownerSegment, err := NormalizeCanvasStorageUserSegment(storageUID)
	if err != nil {
		return "", err
	}
	return path.Join("profile", ownerSegment, "avatar"), nil
}

func ProfileAvatarUploadObjectPrefix(storageUID string) (string, error) {
	ownerSegment, err := NormalizeCanvasStorageUserSegment(storageUID)
	if err != nil {
		return "", err
	}
	return path.Join(uploadObjectRoot, "profile", ownerSegment, "avatar") + "/", nil
}

func TaskOutputUserUploadObjectPrefix(userID int64) (string, error) {
	if userID <= 0 {
		return "", fmt.Errorf("用户ID无效")
	}
	return path.Join(uploadObjectRoot, "tasks", fmt.Sprintf("%d", userID)) + "/", nil
}

func TaskOutputUploadDirPrefix(userID int64, taskID string) (string, error) {
	if userID <= 0 {
		return "", fmt.Errorf("用户ID无效")
	}
	taskSegment, err := NormalizeCanvasBoardSegment(taskID)
	if err != nil {
		return "", err
	}
	return path.Join("tasks", fmt.Sprintf("%d", userID), taskSegment), nil
}

func LibraryAssetUploadDirPrefix(userID, libraryAssetID int64) (string, error) {
	if userID <= 0 || libraryAssetID <= 0 {
		return "", fmt.Errorf("素材库媒体标识无效")
	}
	return path.Join("library", fmt.Sprintf("%d", userID), fmt.Sprintf("%d", libraryAssetID)), nil
}

func LibraryAssetUploadObjectPrefix(userID int64) (string, error) {
	if userID <= 0 {
		return "", fmt.Errorf("用户ID无效")
	}
	return path.Join(uploadObjectRoot, "library", fmt.Sprintf("%d", userID)) + "/", nil
}

func SiteAssetUploadDirPrefix() string {
	return path.Join("site", "assets")
}

func userUploadObjectPrefixes(userID int64, storageUID string) ([]string, error) {
	canvasPrefix, err := CanvasUserUploadObjectPrefix(storageUID)
	if err != nil {
		return nil, err
	}
	profileAvatarPrefix, err := ProfileAvatarUploadObjectPrefix(storageUID)
	if err != nil {
		return nil, err
	}
	taskOutputPrefix, err := TaskOutputUserUploadObjectPrefix(userID)
	if err != nil {
		return nil, err
	}
	libraryPrefix, err := LibraryAssetUploadObjectPrefix(userID)
	if err != nil {
		return nil, err
	}
	return []string{canvasPrefix, profileAvatarPrefix, taskOutputPrefix, libraryPrefix}, nil
}

func ParseUploadResource(rawPath string) (UploadResource, error) {
	objectKey, err := normalizeUploadObjectKey(rawPath)
	if err != nil {
		return UploadResource{}, err
	}

	trimmed := strings.TrimPrefix(objectKey, uploadObjectRoot+"/")
	resource, err := parseUploadResourceParts(strings.Split(trimmed, "/"))
	if err != nil {
		return UploadResource{}, err
	}
	resource.ObjectKey = objectKey
	return resource, nil
}

func parseUploadResourceParts(parts []string) (UploadResource, error) {
	if len(parts) < 1 || parts[0] == "" {
		return UploadResource{}, fmt.Errorf("文件路径不能为空")
	}

	var resource UploadResource
	var ok bool
	switch parts[0] {
	case "canvas":
		resource, ok = parseCanvasUploadResource(parts)
	case "profile":
		resource, ok = parseProfileAvatarUploadResource(parts)
	case "tasks":
		resource, ok = parseTaskOutputUploadResource(parts)
	case "library":
		resource, ok = parseLibraryUploadResource(parts)
	case "site":
		resource, ok = parseSiteAssetUploadResource(parts)
	}
	if !ok {
		return UploadResource{}, fmt.Errorf("不支持的上传资源路径")
	}
	return resource, nil
}

func parseCanvasUploadResource(parts []string) (UploadResource, bool) {
	relativePath, ok := uploadRelativePath(parts, 3)
	if len(parts) < 4 || parts[1] == "" || parts[2] == "" || !ok {
		return UploadResource{}, false
	}
	return UploadResource{
		Kind: UploadResourceKindCanvas,
		Canvas: &UploadCanvasResource{
			OwnerSegment: parts[1],
			BoardSegment: parts[2],
			RelativePath: relativePath,
		},
	}, true
}

func parseProfileAvatarUploadResource(parts []string) (UploadResource, bool) {
	relativePath, ok := uploadRelativePath(parts, 3)
	if len(parts) < 4 || parts[1] == "" || parts[2] != "avatar" || !ok {
		return UploadResource{}, false
	}
	return UploadResource{
		Kind: UploadResourceKindProfileAvatar,
		ProfileAvatar: &UploadProfileAvatarResource{
			OwnerSegment: parts[1],
			RelativePath: relativePath,
		},
	}, true
}

func parseTaskOutputUploadResource(parts []string) (UploadResource, bool) {
	relativePath, ok := uploadRelativePath(parts, 3)
	if len(parts) < 4 || parts[1] == "" || parts[2] == "" || !ok {
		return UploadResource{}, false
	}
	return UploadResource{
		Kind: UploadResourceKindTaskOutput,
		TaskOutput: &UploadTaskOutputResource{
			UserSegment:  parts[1],
			TaskSegment:  parts[2],
			RelativePath: relativePath,
		},
	}, true
}

func parseLibraryUploadResource(parts []string) (UploadResource, bool) {
	relativePath, ok := uploadRelativePath(parts, 3)
	if len(parts) < 4 || parts[1] == "" || parts[2] == "" || !ok {
		return UploadResource{}, false
	}
	return UploadResource{
		Kind: UploadResourceKindLibrary,
		Library: &UploadLibraryResource{
			UserSegment:  parts[1],
			AssetSegment: parts[2],
			RelativePath: relativePath,
		},
	}, true
}

func parseSiteAssetUploadResource(parts []string) (UploadResource, bool) {
	relativePath, ok := uploadRelativePath(parts, 2)
	if len(parts) < 3 || parts[1] != "assets" || !ok {
		return UploadResource{}, false
	}
	return UploadResource{
		Kind: UploadResourceKindSiteAsset,
		SiteAsset: &UploadSiteAssetResource{
			RelativePath: relativePath,
		},
	}, true
}

func uploadRelativePath(parts []string, start int) (string, bool) {
	if len(parts) <= start {
		return "", false
	}
	if strings.Join(parts[start:], "") == "" {
		return "", false
	}
	return strings.Join(parts[start:], "/"), true
}

func normalizeUploadObjectKey(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", fmt.Errorf("文件路径不能为空")
	}
	cleaned := path.Clean("/" + strings.TrimPrefix(trimmed, "/"))
	if !strings.HasPrefix(cleaned, "/"+uploadObjectRoot+"/") {
		return "", fmt.Errorf("仅允许删除 uploads 目录下的文件")
	}
	return strings.TrimPrefix(cleaned, "/"), nil
}

func normalizeUploadObjectPrefix(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", fmt.Errorf("文件前缀不能为空")
	}
	normalized, err := normalizeUploadObjectKey(strings.TrimSuffix(trimmed, "/"))
	if err != nil {
		return "", err
	}
	return normalized + "/", nil
}
