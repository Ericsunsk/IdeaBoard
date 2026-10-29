package domain

import (
	"crypto/rand"
	"fmt"
	"ideaboard/api/internal/platform/errorrender"
	"math/big"
	"regexp"
	"strings"
)

const storageUIDLength = 6

const storageUIDAlphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

var storageUIDPattern = regexp.MustCompile(`^[A-Z0-9]{6}$`)

func NewStorageUID() (string, error) {
	max := big.NewInt(int64(len(storageUIDAlphabet)))
	buf := make([]byte, storageUIDLength)
	for i := 0; i < storageUIDLength; i++ {
		n, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", fmt.Errorf(errorrender.MsgNewStorageUID, err)
		}
		buf[i] = storageUIDAlphabet[n.Int64()]
	}
	return string(buf), nil
}

func IsValidStorageUID(raw string) bool {
	return storageUIDPattern.MatchString(strings.TrimSpace(raw))
}
