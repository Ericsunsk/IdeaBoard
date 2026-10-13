package domain

import (
	"errors"
	"net/url"
	"strings"

	"ideaboard/api/internal/identity/users/domain"
)

func NormalizeStorageUserSegment(storageUID string) (string, error) {
	normalized := strings.TrimSpace(storageUID)
	if !domain.IsValidStorageUID(normalized) {
		return "", errors.New(msgNormalizeStorageUserSegment)
	}
	escaped := url.PathEscape(normalized)
	if escaped == "" {
		return "", errors.New(msgNormalizeStorageUserSegment)
	}
	return escaped, nil
}

func NormalizeUploadSegment(raw string) (string, error) {
	normalized := strings.TrimSpace(raw)
	if normalized == "" {
		return "", errors.New(msgNormalizeUploadSegment)
	}
	escaped := url.PathEscape(normalized)
	if escaped == "" {
		return "", errors.New(msgNormalizeUploadSegment)
	}
	return escaped, nil
}
