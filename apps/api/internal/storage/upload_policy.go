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
	UploadResourceKindWorkspace     UploadResourceKind = "workspace"
	UploadResourceKindProfileAvatar UploadResourceKind = "profile_avatar"
	UploadResourceKindAuthenticated UploadResourceKind = "authenticated"
)

type UploadWorkspaceResource struct {
	OwnerSegment string
	BoardSegment string
	RelativePath string
}

type UploadProfileAvatarResource struct {
	OwnerSegment string
	RelativePath string
}

type UploadResource struct {
	Kind          UploadResourceKind
	ObjectKey     string
	Workspace     *UploadWorkspaceResource
	ProfileAvatar *UploadProfileAvatarResource
}

func WorkspaceContentUploadDirPrefix(storageUID string, boardID string) (string, error) {
	ownerSegment, err := NormalizeWorkspaceStorageUserSegment(storageUID)
	if err != nil {
		return "", err
	}
	boardSegment, err := NormalizeWorkspaceBoardSegment(boardID)
	if err != nil {
		return "", err
	}
	return path.Join("workspace", ownerSegment, boardSegment, "content"), nil
}

func WorkspaceUserUploadObjectPrefix(storageUID string) (string, error) {
	ownerSegment, err := NormalizeWorkspaceStorageUserSegment(storageUID)
	if err != nil {
		return "", err
	}
	return path.Join(uploadObjectRoot, "workspace", ownerSegment) + "/", nil
}

func WorkspaceBoardUploadObjectPrefix(storageUID string, boardID string) (string, error) {
	ownerSegment, err := NormalizeWorkspaceStorageUserSegment(storageUID)
	if err != nil {
		return "", err
	}
	boardSegment, err := NormalizeWorkspaceBoardSegment(boardID)
	if err != nil {
		return "", err
	}
	return path.Join(uploadObjectRoot, "workspace", ownerSegment, boardSegment) + "/", nil
}

func ProfileAvatarUploadDirPrefix(storageUID string) (string, error) {
	ownerSegment, err := NormalizeWorkspaceStorageUserSegment(storageUID)
	if err != nil {
		return "", err
	}
	return path.Join("profile", ownerSegment, "avatar"), nil
}

func ProfileAvatarUploadObjectPrefix(storageUID string) (string, error) {
	ownerSegment, err := NormalizeWorkspaceStorageUserSegment(storageUID)
	if err != nil {
		return "", err
	}
	return path.Join(uploadObjectRoot, "profile", ownerSegment, "avatar") + "/", nil
}

func ParseUploadResource(rawPath string) (UploadResource, error) {
	objectKey, err := normalizeUploadObjectKey(rawPath)
	if err != nil {
		return UploadResource{}, err
	}

	resource := UploadResource{
		Kind:      UploadResourceKindAuthenticated,
		ObjectKey: objectKey,
	}

	trimmed := strings.TrimPrefix(objectKey, uploadObjectRoot+"/")
	parts := strings.Split(trimmed, "/")
	if len(parts) < 1 || parts[0] == "" {
		return UploadResource{}, fmt.Errorf("文件路径不能为空")
	}

	if parts[0] == "workspace" && len(parts) >= 4 && parts[1] != "" && parts[2] != "" && strings.Join(parts[3:], "") != "" {
		resource.Kind = UploadResourceKindWorkspace
		resource.Workspace = &UploadWorkspaceResource{
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

	return resource, nil
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
