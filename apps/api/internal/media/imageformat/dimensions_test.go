package imageformat

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/color"
	"image/gif"
	"image/png"
	"strings"
	"testing"
)

func TestInspectBytesAcceptsSmallPNG(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 2, 3))
	img.Set(1, 2, color.NRGBA{R: 10, G: 20, B: 30, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}

	facts, err := InspectBytes(buf.Bytes(), ".png")
	if err != nil {
		t.Fatalf("expected small png to pass, got %v", err)
	}
	if facts.Width != 2 || facts.Height != 3 || facts.Extension != ".png" || facts.MIMEType != "image/png" || facts.Animated {
		t.Fatalf("unexpected facts: %+v", facts)
	}
}

func TestInspectBytesAcceptsAVIF(t *testing.T) {
	facts, err := InspectBytes(decodeTestAVIF(t), ".avif")
	if err != nil {
		t.Fatalf("expected avif to pass, got %v", err)
	}
	if facts.Width != 640 || facts.Height != 480 || facts.Extension != ".avif" || facts.MIMEType != "image/avif" || facts.Animated {
		t.Fatalf("unexpected facts: %+v", facts)
	}
}

func TestInspectBytesRejectsHugePNGHeaderBeforeFullDecode(t *testing.T) {
	data := buildPNGConfigOnly(20000, 20000)

	_, err := InspectBytes(data, ".png")
	if err == nil {
		t.Fatal("expected huge png dimensions to be rejected")
	}
	if !strings.Contains(err.Error(), "图片尺寸超过限制") {
		t.Fatalf("expected dimension-limit error, got %v", err)
	}
}

func TestInspectCompleteChecksDimensionsBeforeDecodingBody(t *testing.T) {
	_, err := InspectCompleteDetectedBytes(buildPNGConfigOnly(20000, 20000))
	if err == nil || !strings.Contains(err.Error(), "图片尺寸超过限制") {
		t.Fatalf("expected dimension-limit error before body decode, got %v", err)
	}
	if _, err := InspectCompleteDetectedBytes(buildPNGConfigOnly(1, 1)); err == nil {
		t.Fatal("accepted a complete header without image body")
	}
}

func TestInspectRejectsCorruptImageAndResetsReader(t *testing.T) {
	reader := bytes.NewReader([]byte("not an image"))
	_, err := Inspect(reader, ".png")
	if err == nil {
		t.Fatal("expected corrupt image to be rejected")
	}
	position, seekErr := reader.Seek(0, 1)
	if seekErr != nil {
		t.Fatalf("read position: %v", seekErr)
	}
	if position != 0 {
		t.Fatalf("expected corrupt-reader reset to start, got %d", position)
	}
}

func TestInspectDetectsAnimatedGIFAndResetsReader(t *testing.T) {
	palette := color.Palette{color.Black, color.White}
	animation := &gif.GIF{
		Image: []*image.Paletted{
			image.NewPaletted(image.Rect(0, 0, 2, 2), palette),
			image.NewPaletted(image.Rect(0, 0, 2, 2), palette),
		},
		Delay: []int{1, 1},
	}
	var encoded bytes.Buffer
	if err := gif.EncodeAll(&encoded, animation); err != nil {
		t.Fatalf("encode gif: %v", err)
	}
	reader := bytes.NewReader(encoded.Bytes())
	facts, err := Inspect(reader, ".gif")
	if err != nil {
		t.Fatalf("inspect gif: %v", err)
	}
	if !facts.Animated || facts.Width != 2 || facts.Height != 2 {
		t.Fatalf("unexpected facts: %+v", facts)
	}
	position, err := reader.Seek(0, 1)
	if err != nil {
		t.Fatalf("read position: %v", err)
	}
	if position != 0 {
		t.Fatalf("expected reader reset to start, got %d", position)
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
