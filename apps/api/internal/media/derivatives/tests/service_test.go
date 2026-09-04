package tests

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

func TestProcessingInputProfileIsProviderNeutralAndBounded(t *testing.T) {
	profile := ProcessingInputProfile()
	if profile.Name != ProfileProcessingInput {
		t.Fatalf("profile name=%q, want %q", profile.Name, ProfileProcessingInput)
	}
	if profile.MaxSide != 2048 || profile.MaxPixels != 4*1024*1024 || profile.MaxBytes != 20*1024*1024 {
		t.Fatalf("unexpected processing input profile: %#v", profile)
	}
	profile.MaxSide = 1
	if ProcessingInputProfile().MaxSide == 1 {
		t.Fatal("processing input profile must be returned by value")
	}
}

func TestProcessingInputProfileValidatesFacts(t *testing.T) {
	profile := ProcessingInputProfile()
	for _, test := range []struct {
		name string
		w, h int
		size int64
		want bool
	}{
		{name: "valid", w: 2048, h: 2048, size: 1024, want: true},
		{name: "side", w: 2049, h: 1, size: 1024},
		{name: "pixels", w: 2048, h: 2049, size: 1024},
		{name: "bytes", w: 1, h: 1, size: 20*1024*1024 + 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := profile.ValidateInputFacts(test.w, test.h, test.size)
			if (err == nil) != test.want {
				t.Fatalf("ValidateInputFacts() err=%v, wantValid=%v", err, test.want)
			}
		})
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
