package workflow

import "testing"

func TestNormalizeOutputResolution(t *testing.T) {
	tests := []struct {
		raw  string
		want string
		ok   bool
	}{
		{raw: "", want: OutputResolution1K, ok: true},
		{raw: "default", want: OutputResolutionDefault, ok: true},
		{raw: " 2K ", want: OutputResolution2K, ok: true},
		{raw: "4k", want: OutputResolution4K, ok: true},
		{raw: "8k", ok: false},
	}

	for _, test := range tests {
		got, ok := NormalizeOutputResolution(test.raw)
		if got != test.want || ok != test.ok {
			t.Fatalf("NormalizeOutputResolution(%q) = (%q, %v), want (%q, %v)", test.raw, got, ok, test.want, test.ok)
		}
	}
}

func TestSupportsOutputResolutionUsesExclusiveModelCapabilities(t *testing.T) {
	tests := []struct {
		name                   string
		resolution             string
		supports2K, supports4K bool
		want                   bool
	}{
		{name: "default uses baseline model", resolution: OutputResolutionDefault, want: true},
		{name: "1k uses baseline model", resolution: OutputResolution1K, want: true},
		{name: "baseline model rejects 2k", resolution: OutputResolution2K, want: false},
		{name: "baseline model rejects 4k", resolution: OutputResolution4K, want: false},
		{name: "2k model only handles 2k", resolution: OutputResolution1K, supports2K: true, want: false},
		{name: "2k model handles 2k", resolution: OutputResolution2K, supports2K: true, want: true},
		{name: "2k model rejects 4k", resolution: OutputResolution4K, supports2K: true, want: false},
		{name: "4k model only handles 4k", resolution: OutputResolution1K, supports4K: true, want: false},
		{name: "4k model handles 4k", resolution: OutputResolution4K, supports4K: true, want: true},
		{name: "4k model rejects 2k", resolution: OutputResolution2K, supports4K: true, want: false},
		{name: "dual resolution model handles 2k", resolution: OutputResolution2K, supports2K: true, supports4K: true, want: true},
		{name: "dual resolution model handles 4k", resolution: OutputResolution4K, supports2K: true, supports4K: true, want: true},
		{name: "dual resolution model rejects baseline", resolution: OutputResolution1K, supports2K: true, supports4K: true, want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := SupportsOutputResolution(test.resolution, test.supports2K, test.supports4K); got != test.want {
				t.Fatalf("SupportsOutputResolution(%q, %v, %v) = %v, want %v", test.resolution, test.supports2K, test.supports4K, got, test.want)
			}
		})
	}
}
