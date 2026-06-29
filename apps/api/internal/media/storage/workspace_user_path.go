package storage

import (
	"fmt"
	"net/url"
	"strings"

	"ideaboard/api/internal/identity/users"
)

func NormalizeWorkspaceStorageUserSegment(storageUID string) (string, error) {
	normalized := strings.TrimSpace(storageUID)
	if !users.IsValidStorageUID(normalized) {
		return "", fmt.Errorf("用户存储标识无效")
	}
	escaped := url.PathEscape(normalized)
	if escaped == "" {
		return "", fmt.Errorf("用户存储标识无效")
	}
	return escaped, nil
}

func NormalizeWorkspaceBoardSegment(boardID string) (string, error) {
	normalized := strings.TrimSpace(boardID)
	if normalized == "" {
		return "", fmt.Errorf("缺少画布ID")
	}
	escaped := url.PathEscape(normalized)
	if escaped == "" {
		return "", fmt.Errorf("缺少画布ID")
	}
	return escaped, nil
}
