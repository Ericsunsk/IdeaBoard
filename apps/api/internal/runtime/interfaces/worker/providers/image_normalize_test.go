package providers

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"strings"
	"testing"
)

func TestNormalizeOpenAIEditMultipartImageAcceptsSmallPNG(t *testing.T) {
	data := buildTinyPNGBytes(t)

	filename, mimeType, normalized, err := normalizeOpenAIEditMultipartImage("input.png", "image/png", data)
	if err != nil {
		t.Fatalf("normalizeOpenAIEditMultipartImage: %v", err)
	}
	if filename != "input.png" {
		t.Fatalf("unexpected filename: %q", filename)
	}
	if mimeType != "image/png" {
		t.Fatalf("unexpected mime type: %q", mimeType)
	}
	if !bytes.Equal(normalized, data) {
		t.Fatal("expected png bytes to pass through")
	}
}

func TestNormalizeOpenAIEditMultipartImageRejectsHugePNGHeaderBeforeFullDecode(t *testing.T) {
	data := buildConfigOnlyPNGBytes(20000, 20000)

	_, _, _, err := normalizeOpenAIEditMultipartImage("huge.png", "image/png", data)
	if err == nil {
		t.Fatal("expected huge image dimensions to be rejected")
	}
	if !strings.Contains(err.Error(), "图片尺寸超过限制") {
		t.Fatalf("expected dimension-limit error, got %v", err)
	}
}

func buildConfigOnlyPNGBytes(width, height uint32) []byte {
	var buf bytes.Buffer
	buf.Write([]byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a})

	ihdr := make([]byte, 13)
	binary.BigEndian.PutUint32(ihdr[0:4], width)
	binary.BigEndian.PutUint32(ihdr[4:8], height)
	ihdr[8] = 8
	ihdr[9] = 2
	writeConfigOnlyPNGChunk(&buf, "IHDR", ihdr)
	return buf.Bytes()
}

func writeConfigOnlyPNGChunk(buf *bytes.Buffer, name string, payload []byte) {
	var length [4]byte
	binary.BigEndian.PutUint32(length[:], uint32(len(payload)))
	buf.Write(length[:])
	buf.WriteString(name)
	buf.Write(payload)

	crc := crc32.NewIEEE()
	_, _ = crc.Write([]byte(name))
	_, _ = crc.Write(payload)
	var checksum [4]byte
	binary.BigEndian.PutUint32(checksum[:], crc.Sum32())
	buf.Write(checksum[:])
}
