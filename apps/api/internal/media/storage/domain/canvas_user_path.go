package domain

import (
	"fmt"
	"net/url"
	"strings"

	"ideaboard/api/internal/identity/users/domain"
)

func NormalizeCanvasStorageUserSegment(storageUID string) (string, error) {
	normalized := strings.TrimSpace(storageUID)
	if !domain.IsValidStorageUID(normalized) {
		return "", fmt.Errorf(msgNormalizeCanvasStorageUserSegment)
	}
	escaped := url.PathEscape(normalized)
	if escaped == "" {
		return "", fmt.Errorf(msgNormalizeCanvasStorageUserSegment)
	}
	return escaped, nil
}

func NormalizeCanvasBoardSegment(boardID string) (string, error) {
	normalized := strings.TrimSpace(boardID)
	if normalized == "" {
		return "", fmt.Errorf(msgNormalizeCanvasBoardSegment)
	}
	escaped := url.PathEscape(normalized)
	if escaped == "" {
		return "", fmt.Errorf(msgNormalizeCanvasBoardSegment)
	}
	return escaped, nil
}
