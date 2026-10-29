package searchcatalog

// V1 vision file/external-ref input bounds (issue #809 media gap
// batch): upload_ stage-then-reference ids and public external
// image URLs share the 4-ref cap with library refs.

import (
	"testing"
)

func TestValidateV1VisionQueryInputUploadsAndURLs(t *testing.T) {
	upload := "upload_ABCDEF123456"
	valid := V1VisionQueryInput{
		UserText:        "dunes",
		LibraryAssetIDs: []string{"lasset_ABCDEF123456"},
		UploadIDs:       []string{upload, upload},
		ImageURLs:       []string{"https://images.unsplash.com/photo-1", "https://cdn.example/photo-2", "https://cdn.example/photo-2"},
	}
	normalized, err := ValidateV1VisionQueryInput(valid)
	if err != nil {
		t.Fatalf("validate = %v, want nil", err)
	}
	if len(normalized.UploadIDs) != 1 || normalized.UploadIDs[0] != upload {
		t.Fatalf("upload ids = %v, want deduped [%s]", normalized.UploadIDs, upload)
	}
	if len(normalized.ImageURLs) != 2 {
		t.Fatalf("image urls = %v, want deduped pair", normalized.ImageURLs)
	}

	// Upload-only input (no text, no library refs) is valid.
	uploadOnly, err := ValidateV1VisionQueryInput(V1VisionQueryInput{UploadIDs: []string{upload}})
	if err != nil || len(uploadOnly.UploadIDs) != 1 {
		t.Fatalf("upload-only = %+v %v, want valid", uploadOnly, err)
	}

	cases := []struct {
		name  string
		input V1VisionQueryInput
	}{
		{"malformed upload id", V1VisionQueryInput{UploadIDs: []string{"nope"}}},
		{"library-shaped upload id", V1VisionQueryInput{UploadIDs: []string{"lasset_ABCDEF123456"}}},
		{"over cap combined", V1VisionQueryInput{
			LibraryAssetIDs: []string{"lasset_AAAAAAAAAAAA", "lasset_BBBBBBBBBBBB"},
			UploadIDs:       []string{"upload_CCCCCCCCCCCC", "upload_DDDDDDDDDDDD"},
			ImageURLs:       []string{"https://images.unsplash.com/x"},
		}},
		{"evil url", V1VisionQueryInput{ImageURLs: []string{"https://[::1]/x"}}},
		{"loopback url", V1VisionQueryInput{ImageURLs: []string{"https://127.0.0.1/x"}}},
		{"private url", V1VisionQueryInput{ImageURLs: []string{"https://10.0.0.8/x"}}},
		{"plain http url", V1VisionQueryInput{ImageURLs: []string{"http://images.unsplash.com/x"}}},
		{"exotic port url", V1VisionQueryInput{ImageURLs: []string{"https://images.unsplash.com:8080/x"}}},
		{"userinfo url", V1VisionQueryInput{ImageURLs: []string{"https://user@images.unsplash.com/x"}}},
		{"empty", V1VisionQueryInput{}},
		{"blank text only", V1VisionQueryInput{UserText: "   "}},
	}
	for _, tc := range cases {
		if _, err := ValidateV1VisionQueryInput(tc.input); err != ErrV1VisionInput {
			t.Fatalf("%s: err = %v, want ErrV1VisionInput", tc.name, err)
		}
	}
}

func TestIsV1VisionExternalImageURL(t *testing.T) {
	for _, good := range []string{
		"https://images.unsplash.com/photo-1?w=800",
		"https://plus.unsplash.com/photo-2",
		"https://images.pexels.com/photos/3",
		"https://IMAGES.UNSPLASH.COM/upper",
		"https://cdn.example/photo-4",
		"https://unsplash.com/photos/x",
	} {
		if !IsV1VisionExternalImageURL(good) {
			t.Fatalf("allow %q = false, want true", good)
		}
	}
	for _, bad := range []string{
		"",
		"http://images.unsplash.com/x",
		"https://images.unsplash.com:8080/x",
		"https://user@images.unsplash.com/x",
		"https://127.0.0.1/x",
		"https://10.0.0.8/x",
		"https://[::1]/x",
		"data:image/png;base64,abcd",
		"not a url",
	} {
		if IsV1VisionExternalImageURL(bad) {
			t.Fatalf("reject %q = true, want false", bad)
		}
	}
}
