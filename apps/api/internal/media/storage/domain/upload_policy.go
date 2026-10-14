package domain

import (
	"errors"
	"fmt"
	"path"
	"strings"
)

const (
	UploadObjectRoot = "uploads"
	// WorkspaceUploadCanonicalDir scopes new workspace upload sessions,
	// media assets and derivatives under the canonical root:
	// uploads/ws/<workspace>/<upload>/<file>.
	WorkspaceUploadCanonicalDir = "ws"
)

func TaskOutputUserUploadObjectPrefix(userID int64) (string, error) {
	if userID <= 0 {
		return "", errors.New(msgTaskOutputUserUploadObjectPrefix)
	}
	return path.Join(UploadObjectRoot, "tasks", fmt.Sprintf("%d", userID)) + "/", nil
}

func LibraryAssetUploadObjectPrefix(userID int64) (string, error) {
	if userID <= 0 {
		return "", errors.New(msgTaskOutputUserUploadObjectPrefix)
	}
	return path.Join(UploadObjectRoot, "library", fmt.Sprintf("%d", userID)) + "/", nil
}

func UserUploadObjectPrefixes(userID int64, storageUID string) ([]string, error) {
	avatarPrefix, err := AvatarUploadObjectPrefix(storageUID)
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
	return []string{avatarPrefix, taskOutputPrefix, libraryPrefix}, nil
}

func NormalizeUploadObjectKey(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", errors.New(msgNormalizeUploadObjectKeyEmpty)
	}
	cleaned := path.Clean("/" + strings.TrimPrefix(trimmed, "/"))
	if !strings.HasPrefix(cleaned, "/"+UploadObjectRoot+"/") {
		return "", errors.New(msgNormalizeUploadObjectKey)
	}
	return strings.TrimPrefix(cleaned, "/"), nil
}

// WorkspaceUploadCanonicalPrefix is the canonical object prefix for
// workspace slot keys: uploads/ws/.
func WorkspaceUploadCanonicalPrefix() string {
	return UploadObjectRoot + "/" + WorkspaceUploadCanonicalDir + "/"
}

func NormalizeUploadObjectPrefix(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", errors.New(msgNormalizeUploadObjectPrefix)
	}
	normalized, err := NormalizeUploadObjectKey(strings.TrimSuffix(trimmed, "/"))
	if err != nil {
		return "", err
	}
	return normalized + "/", nil
}
