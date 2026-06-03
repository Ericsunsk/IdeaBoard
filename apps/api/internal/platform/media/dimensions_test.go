package media

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"
)

func TestValidateImageDimensionsAcceptsSmallPNG(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 2, 3))
	img.Set(1, 2, color.NRGBA{R: 10, G: 20, B: 30, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}

	if err := ValidateImageDimensions(buf.Bytes()); err != nil {
		t.Fatalf("expected small png to pass, got %v", err)
	}
}

func TestValidateImageDimensionsRejectsHugePNGHeaderBeforeFullDecode(t *testing.T) {
	data := buildPNGConfigOnly(20000, 20000)

	err := ValidateImageDimensions(data)
	if err == nil {
		t.Fatal("expected huge png dimensions to be rejected")
	}
	if !strings.Contains(err.Error(), "图片尺寸超过限制") {
		t.Fatalf("expected dimension-limit error, got %v", err)
	}
}

func buildPNGConfigOnly(width, height uint32) []byte {
	var buf bytes.Buffer
	buf.Write([]byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a})

	ihdr := make([]byte, 13)
	binary.BigEndian.PutUint32(ihdr[0:4], width)
	binary.BigEndian.PutUint32(ihdr[4:8], height)
	ihdr[8] = 8
	ihdr[9] = 2
	writePNGChunk(&buf, "IHDR", ihdr)
	return buf.Bytes()
}

func writePNGChunk(buf *bytes.Buffer, name string, payload []byte) {
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
