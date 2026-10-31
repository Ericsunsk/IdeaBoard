package domain

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
)

const (
	prefixAsset             = "asset_"
	prefixBoard             = "board_"
	idempotencyMaxKeyLength = 128
)

func isValidPublicID(prefix, id string) bool {
	if prefix == "" || !strings.HasPrefix(id, prefix) {
		return false
	}
	suffix := strings.TrimPrefix(id, prefix)
	if suffix == "" {
		return false
	}
	for _, r := range suffix {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
		default:
			return false
		}
	}
	return true
}

func isAssetID(id string) bool {
	return isValidPublicID(prefixAsset, id)
}

func isBoardID(id string) bool {
	return isValidPublicID(prefixBoard, id)
}

func mintExecutionToken() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("domain: mint execution token: %w", err)
	}
	return "exec_" + hex.EncodeToString(b), nil
}
