package domain

import (
	"crypto/rand"
	"fmt"
	"ideaboard/api/internal/platform/errorrender"
	"regexp"
	"strings"
)

const storageUIDLength = 6

const storageUIDAlphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

var storageUIDPattern = regexp.MustCompile(`^[A-Z0-9]{6}$`)

func NewStorageUID() (string, error) {
	randomBytes := make([]byte, storageUIDLength)
	if _, err := rand.Read(randomBytes); err != nil {
		return "", fmt.Errorf(errorrender.MsgNewStorageUID, err)
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
