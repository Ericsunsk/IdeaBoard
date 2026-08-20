package library

import (
	"encoding/base64"
	"encoding/json"
	"strings"
)

func EncodeCursor(cursor Cursor) (string, error) {
	if cursor.ID <= 0 || cursor.CreatedAt.IsZero() {
		return "", ErrInvalidCursor
	}
	raw, err := json.Marshal(cursor)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func DecodeCursor(raw string) (*Cursor, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	decoded, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return nil, ErrInvalidCursor
	}
	var cursor Cursor
	if err := json.Unmarshal(decoded, &cursor); err != nil || cursor.ID <= 0 || cursor.CreatedAt.IsZero() {
		return nil, ErrInvalidCursor
	}
	return &cursor, nil
}
