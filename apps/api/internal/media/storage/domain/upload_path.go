package domain

import (
	"fmt"
	"net/url"
	"strings"

	"ideaboard/api/internal/identity/users/domain"
)

func NormalizeStorageUserSegment(storageUID string) (string, error) {
	normalized := strings.TrimSpace(storageUID)
	if !domain.IsValidStorageUID(normalized) {
		return "", fmt.Errorf(msgNormalizeStorageUserSegment)
	}
	escaped := url.PathEscape(normalized)
	if escaped == "" {
		return "", fmt.Errorf(msgNormalizeStorageUserSegment)
	}
	return escaped, nil
}

func NormalizeUploadSegment(raw string) (string, error) {
	normalized := strings.TrimSpace(raw)
	if normalized == "" {
		return "", fmt.Errorf(msgNormalizeUploadSegment)
	}
	escaped := url.PathEscape(normalized)
	if escaped == "" {
		return "", fmt.Errorf(msgNormalizeUploadSegment)
	}
	return escaped, nil
}
