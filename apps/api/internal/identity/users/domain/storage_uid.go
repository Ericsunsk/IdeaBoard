package domain

import (
	"crypto/rand"
	"fmt"
	"regexp"
	"strings"
)

const storageUIDLength = 6

const storageUIDAlphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

var storageUIDPattern = regexp.MustCompile(`^[A-Z0-9]{6}$`)

func NewStorageUID() (string, error) {
	randomBytes := make([]byte, storageUIDLength)
	if _, err := rand.Read(randomBytes); err != nil {
		return "", fmt.Errorf("生成用户存储标识失败: %w", err)
	}
	buf := make([]byte, storageUIDLength)
	for i, b := range randomBytes {
		buf[i] = storageUIDAlphabet[int(b)%len(storageUIDAlphabet)]
	}
	return string(buf), nil
}

func IsValidStorageUID(raw string) bool {
	return storageUIDPattern.MatchString(strings.TrimSpace(raw))
}
