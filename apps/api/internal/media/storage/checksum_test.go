package storage

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"strings"
	"testing"
)

func TestPrepareChecksumBodySeekable(t *testing.T) {
	data := []byte("seekable image payload")
	body, checksum, err := prepareChecksumBody(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("prepare checksum body: %v", err)
	}
	read, err := io.ReadAll(body)
	if err != nil {
		t.Fatalf("read checksum body: %v", err)
	}
	if string(read) != string(data) {
		t.Fatalf("body = %q, want %q", read, data)
	}
	want := sha256.Sum256(data)
	if got := checksum(); got != hex.EncodeToString(want[:]) {
		t.Fatalf("checksum = %q, want %q", got, hex.EncodeToString(want[:]))
	}
}

func TestPrepareChecksumBodyStreaming(t *testing.T) {
	data := "streaming image payload"
	body, checksum, err := prepareChecksumBody(strings.NewReader(data))
	if err != nil {
		t.Fatalf("prepare checksum body: %v", err)
	}
	read, err := io.ReadAll(body)
	if err != nil {
		t.Fatalf("read checksum body: %v", err)
	}
	if string(read) != data {
		t.Fatalf("body = %q, want %q", read, data)
	}
	want := sha256.Sum256([]byte(data))
	if got := checksum(); got != hex.EncodeToString(want[:]) {
		t.Fatalf("checksum = %q, want %q", got, hex.EncodeToString(want[:]))
	}
}
