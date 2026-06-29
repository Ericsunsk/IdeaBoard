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
	UploadResourceKindCanvas     UploadResourceKind = "canvas"
	UploadResourceKindProfileAvatar UploadResourceKind = "profile_avatar"
	UploadResourceKindTaskOutput    UploadResourceKind = "task_output"
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

type UploadSiteAssetResource struct {
	RelativePath string
}

type UploadResource struct {
	Kind          UploadResourceKind
	ObjectKey     string
	Canvas     *UploadCanvasResource
	ProfileAvatar *UploadProfileAvatarResource
	TaskOutput    *UploadTaskOutputResource
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

func TaskOutputUploadObjectPrefix(userID int64, taskID string) (string, error) {
	dirPrefix, err := TaskOutputUploadDirPrefix(userID, taskID)
	if err != nil {
		return "", err
	}
	return path.Join(uploadObjectRoot, dirPrefix) + "/", nil
}

func SiteAssetUploadDirPrefix() string {
	return path.Join("site", "assets")
}

func SiteAssetUploadObjectPrefix() string {
	return path.Join(uploadObjectRoot, SiteAssetUploadDirPrefix()) + "/"
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
	return []string{canvasPrefix, profileAvatarPrefix, taskOutputPrefix}, nil
}

func ParseUploadResource(rawPath string) (UploadResource, error) {
	objectKey, err := normalizeUploadObjectKey(rawPath)
	if err != nil {
		return UploadResource{}, err
	}

	resource := UploadResource{
		ObjectKey: objectKey,
	}

	trimmed := strings.TrimPrefix(objectKey, uploadObjectRoot+"/")
	parts := strings.Split(trimmed, "/")
	if len(parts) < 1 || parts[0] == "" {
		return UploadResource{}, fmt.Errorf("文件路径不能为空")
	}

	if parts[0] == "canvas" && len(parts) >= 4 && parts[1] != "" && parts[2] != "" && strings.Join(parts[3:], "") != "" {
		resource.Kind = UploadResourceKindCanvas
		resource.Canvas = &UploadCanvasResource{
			OwnerSegment: parts[1],
			BoardSegment: parts[2],
			RelativePath: strings.Join(parts[3:], "/"),
		}
		return resource, nil
	}

	if parts[0] == "profile" && len(parts) >= 4 && parts[1] != "" && parts[2] == "avatar" && strings.Join(parts[3:], "") != "" {
		resource.Kind = UploadResourceKindProfileAvatar
		resource.ProfileAvatar = &UploadProfileAvatarResource{
			OwnerSegment: parts[1],
			RelativePath: strings.Join(parts[3:], "/"),
		}
		return resource, nil
	}

	if parts[0] == "tasks" && len(parts) >= 4 && parts[1] != "" && parts[2] != "" && strings.Join(parts[3:], "") != "" {
		resource.Kind = UploadResourceKindTaskOutput
		resource.TaskOutput = &UploadTaskOutputResource{
			UserSegment:  parts[1],
			TaskSegment:  parts[2],
			RelativePath: strings.Join(parts[3:], "/"),
		}
		return resource, nil
	}

	if parts[0] == "site" && len(parts) >= 3 && parts[1] == "assets" && strings.Join(parts[2:], "") != "" {
		resource.Kind = UploadResourceKindSiteAsset
		resource.SiteAsset = &UploadSiteAssetResource{
			RelativePath: strings.Join(parts[2:], "/"),
		}
		return resource, nil
	}

	return UploadResource{}, fmt.Errorf("不支持的上传资源路径")
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
