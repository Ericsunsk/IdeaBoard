package derivatives

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"
)

func TestProcessingTierIsSizedForEditorWork(t *testing.T) {
	spec, ok := TierSpecs[TierProcessing]
	if !ok {
		t.Fatal("processing tier is not configured")
	}
	if spec.MaxSide < 2048 || spec.MaxSide > 2560 {
		t.Fatalf("processing max side=%d, want 2048..2560", spec.MaxSide)
	}
}

func TestRenderKeepsAspectRatioAndCapsLongestSide(t *testing.T) {
	source := image.NewRGBA(image.Rect(0, 0, 800, 400))
	for y := 0; y < 400; y++ {
		for x := 0; x < 800; x++ {
			source.SetRGBA(x, y, color.RGBA{R: 220, G: 120, B: 40, A: 255})
		}
	}
	var input bytes.Buffer
	if err := png.Encode(&input, source); err != nil {
		t.Fatal(err)
	}
	encoded, width, height, format, err := render(input.Bytes(), 256)
	if err != nil {
		t.Fatal(err)
	}
	if width != 256 || height != 128 || format != FormatJPEG {
		t.Fatalf("rendered=%dx%d format=%s, want 256x128 jpeg", width, height, format)
	}
	if len(encoded) == 0 {
		t.Fatal("rendered image is empty")
	}
}

func TestRenderPreservesTransparencyAsPNG(t *testing.T) {
	source := image.NewNRGBA(image.Rect(0, 0, 32, 32))
	source.SetNRGBA(10, 10, color.NRGBA{R: 255, A: 100})
	var input bytes.Buffer
	if err := png.Encode(&input, source); err != nil {
		t.Fatal(err)
	}
	_, _, _, format, err := render(input.Bytes(), 64)
	if err != nil {
		t.Fatal(err)
	}
	if format != FormatPNG {
		t.Fatalf("format=%s, want png", format)
	}
}
