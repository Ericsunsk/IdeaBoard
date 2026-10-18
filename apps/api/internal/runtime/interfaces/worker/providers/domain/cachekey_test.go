package providerdomain

import "testing"

func TestBuildTextCacheKeyIsDeterministicAndScoped(t *testing.T) {
	body := []byte(`{"prompt":"cat"}`)
	first, err := BuildTextCacheKey(" render ", " model-a ", body)
	if err != nil {
		t.Fatal(err)
	}
	second, err := BuildTextCacheKey("render", "model-a", body)
	if err != nil || first != second {
		t.Fatalf("equivalent inputs = %q and %q (%v)", first, second, err)
	}
	if len(first) != len("aigw/v1/")+64 || first[:len("aigw/v1/")] != "aigw/v1/" {
		t.Fatalf("key = %q, want versioned sha256 key", first)
	}
	for _, input := range [][3]string{
		{"other-feature", "model-a", string(body)},
		{"render", "model-b", string(body)},
		{"render", "model-a", `{"prompt":"dog"}`},
	} {
		other, err := BuildTextCacheKey(input[0], input[1], []byte(input[2]))
		if err != nil {
			t.Fatal(err)
		}
		if other == first {
			t.Fatalf("scope collision for %+v", input)
		}
	}
}

func TestBuildTextCacheKeyRejectsMissingParts(t *testing.T) {
	for _, input := range [][3]string{
		{"", "model", "{}"},
		{"feature", "", "{}"},
		{"feature", "model", ""},
	} {
		if _, err := BuildTextCacheKey(input[0], input[1], []byte(input[2])); err == nil {
			t.Fatalf("accepted invalid input %+v", input)
		}
	}
}
