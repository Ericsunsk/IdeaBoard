package domain

import (
	"fmt"
	"path"
	"strings"
)

const (
	UploadObjectRoot = "uploads"
	// WorkspaceUploadObjectRoot scopes workspace upload sessions, media
	// assets and derivatives (see workspacemedia key builders). Both roots
	// stay anchored under the storage root; traversal outside them is
	// still rejected below.
	WorkspaceUploadObjectRoot = "ws-uploads"
)

type UploadResourceKind string

const (
	UploadResourceKindProfileAvatar UploadResourceKind = "profile_avatar"
	UploadResourceKindTaskOutput    UploadResourceKind = "task_output"
	UploadResourceKindLibrary       UploadResourceKind = "library"
	UploadResourceKindSiteAsset     UploadResourceKind = "site_asset"
)

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
	ProfileAvatar *UploadProfileAvatarResource
	TaskOutput    *UploadTaskOutputResource
	Library       *UploadLibraryResource
	SiteAsset     *UploadSiteAssetResource
}

func ProfileAvatarUploadDirPrefix(storageUID string) (string, error) {
	ownerSegment, err := NormalizeStorageUserSegment(storageUID)
	if err != nil {
		return "", err
	}
	return path.Join("profile", ownerSegment, "avatar"), nil
}

func ProfileAvatarUploadObjectPrefix(storageUID string) (string, error) {
	ownerSegment, err := NormalizeStorageUserSegment(storageUID)
	if err != nil {
		return "", err
	}
	return path.Join(UploadObjectRoot, "profile", ownerSegment, "avatar") + "/", nil
}

func TaskOutputUserUploadObjectPrefix(userID int64) (string, error) {
	if userID <= 0 {
		return "", fmt.Errorf(msgTaskOutputUserUploadObjectPrefix)
	}
	return path.Join(UploadObjectRoot, "tasks", fmt.Sprintf("%d", userID)) + "/", nil
}

func TaskOutputUploadDirPrefix(userID int64, taskID string) (string, error) {
	if userID <= 0 {
		return "", fmt.Errorf(msgTaskOutputUserUploadObjectPrefix)
	}
	taskSegment, err := NormalizeUploadSegment(taskID)
	if err != nil {
		return "", err
	}
	return path.Join("tasks", fmt.Sprintf("%d", userID), taskSegment), nil
}

func LibraryAssetUploadObjectPrefix(userID int64) (string, error) {
	if userID <= 0 {
		return "", fmt.Errorf(msgTaskOutputUserUploadObjectPrefix)
	}
	return path.Join(UploadObjectRoot, "library", fmt.Sprintf("%d", userID)) + "/", nil
}

func SiteAssetUploadDirPrefix() string {
	return path.Join("site", "assets")
}

func UserUploadObjectPrefixes(userID int64, storageUID string) ([]string, error) {
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
	return []string{profileAvatarPrefix, taskOutputPrefix, libraryPrefix}, nil
}

func ParseUploadResource(rawPath string) (UploadResource, error) {
	objectKey, err := NormalizeUploadObjectKey(rawPath)
	if err != nil {
		return UploadResource{}, err
	}

	trimmed := strings.TrimPrefix(objectKey, UploadObjectRoot+"/")
	resource, err := parseUploadResourceParts(strings.Split(trimmed, "/"))
	if err != nil {
		return UploadResource{}, err
	}
	resource.ObjectKey = objectKey
	return resource, nil
}

func parseUploadResourceParts(parts []string) (UploadResource, error) {
	if len(parts) < 1 || parts[0] == "" {
		return UploadResource{}, fmt.Errorf(msgParseUploadResourceParts)
	}

	var resource UploadResource
	var ok bool
	switch parts[0] {
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
		return UploadResource{}, fmt.Errorf(msgParseUploadResourceParts2)
	}
	return resource, nil
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

func NormalizeUploadObjectKey(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", fmt.Errorf(msgParseUploadResourceParts)
	}
	cleaned := path.Clean("/" + strings.TrimPrefix(trimmed, "/"))
	if !strings.HasPrefix(cleaned, "/"+UploadObjectRoot+"/") &&
		!strings.HasPrefix(cleaned, "/"+WorkspaceUploadObjectRoot+"/") {
		return "", fmt.Errorf(msgNormalizeUploadObjectKey)
	}
	return strings.TrimPrefix(cleaned, "/"), nil
}

func NormalizeUploadObjectPrefix(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", fmt.Errorf(msgNormalizeUploadObjectPrefix)
	}
	normalized, err := NormalizeUploadObjectKey(strings.TrimSuffix(trimmed, "/"))
	if err != nil {
		return "", err
	}
	return normalized + "/", nil
}
