package workflow

import "testing"

func TestNormalizeOutputResolution(t *testing.T) {
	tests := []struct {
		raw  string
		want string
		ok   bool
	}{
		{raw: "", want: OutputResolution1K, ok: true},
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
