package tests

import (
	"testing"
)

// processing-input is the tier that feeds the editor and AI models, so its
// geometry has to be large enough for that work.
func TestProcessingInputTierIsSizedForEditorWork(t *testing.T) {
	spec, ok := TierSpecs[TierProcessingInput]
	if !ok {
		t.Fatal("processing-input tier is not configured")
	}
	if spec.MaxSide != ProcessingInputMaxSide {
		t.Fatalf("processing-input max side=%d, want %d", spec.MaxSide, ProcessingInputMaxSide)
	}
	if spec.MaxSide < 2048 || spec.MaxSide > 2560 {
		t.Fatalf("processing-input max side=%d, want 2048..2560", spec.MaxSide)
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
