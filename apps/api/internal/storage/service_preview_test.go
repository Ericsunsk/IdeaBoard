package storage

import "testing"

func TestAppendKodoThumbnailQuery(t *testing.T) {
	tests := []struct {
		name       string
		raw        string
		accessHost string
		want       string
	}{
		{
			name:       "appends thumbnail query for matching host",
			raw:        "https://img.example.com/uploads/2026/04/24/a.png",
			accessHost: "img.example.com",
			want:       "https://img.example.com/uploads/2026/04/24/a.png?imageView2/2/w/640/interlace/1/q/75",
		},
		{
			name:       "keeps existing imageView2 query",
			raw:        "https://img.example.com/a.png?imageView2/2/w/300",
			accessHost: "img.example.com",
			want:       "https://img.example.com/a.png?imageView2/2/w/300",
		},
		{
			name:       "skips tokenized query",
			raw:        "https://img.example.com/a.png?token=abc",
			accessHost: "img.example.com",
			want:       "https://img.example.com/a.png?token=abc",
		},
		{
			name:       "skips signed private kodo url",
			raw:        "https://img.example.com/a.png?e=1735689600&token=abc:def",
			accessHost: "img.example.com",
			want:       "https://img.example.com/a.png?e=1735689600&token=abc:def",
		},
		{
			name:       "appends after normal query without signature",
			raw:        "https://img.example.com/a.png?foo=bar",
			accessHost: "img.example.com",
			want:       "https://img.example.com/a.png?foo=bar&imageView2/2/w/640/interlace/1/q/75",
		},
		{
			name:       "skips non-matching host",
			raw:        "https://cdn.other.com/a.png",
			accessHost: "img.example.com",
			want:       "https://cdn.other.com/a.png",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := appendKodoThumbnailQuery(tt.raw, tt.accessHost)
			if got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestExtractHost(t *testing.T) {
	if got := extractHost("https://img.example.com/path"); got != "img.example.com" {
		t.Fatalf("unexpected host: %q", got)
	}
	if got := extractHost("img.example.com/path"); got != "img.example.com" {
		t.Fatalf("unexpected host without scheme: %q", got)
	}
}

func TestPreviewURLTransformerCache(t *testing.T) {
	profile := &StorageProfile{
		Provider: StorageProviderKodo,
		Config: map[string]interface{}{
			"access_domain":  "https://img.example.com",
			"thumbnail_mode": true,
		},
	}
	calls := 0
	svc := &StorageService{
		previewDirty: true,
		previewResolver: func() (*StorageProfile, error) {
			calls++
			return profile, nil
		},
	}

	first := svc.PreviewURLTransformer()
	second := svc.PreviewURLTransformer()

	if calls != 1 {
		t.Fatalf("resolver calls = %d, want 1", calls)
	}
	if first == nil || second == nil {
		t.Fatalf("preview transformer should not be nil")
	}

	got := first("https://img.example.com/a.png")
	want := "https://img.example.com/a.png?imageView2/2/w/640/interlace/1/q/75"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestPreviewURLTransformerCacheInvalidation(t *testing.T) {
	profileA := &StorageProfile{
		Provider: StorageProviderKodo,
		Config: map[string]interface{}{
			"access_domain":  "https://img.example.com",
			"thumbnail_mode": true,
		},
	}
	profileB := &StorageProfile{
		Provider: StorageProviderKodo,
		Config: map[string]interface{}{
			"access_domain":  "https://img2.example.com",
			"thumbnail_mode": true,
		},
	}

	current := profileA
	calls := 0
	svc := &StorageService{
		previewDirty: true,
		previewResolver: func() (*StorageProfile, error) {
			calls++
			return current, nil
		},
	}

	before := svc.PreviewURLTransformer()
	gotBefore := before("https://img.example.com/a.png")
	wantBefore := "https://img.example.com/a.png?imageView2/2/w/640/interlace/1/q/75"
	if gotBefore != wantBefore {
		t.Fatalf("before invalidate got %q, want %q", gotBefore, wantBefore)
	}

	current = profileB
	svc.invalidatePreviewURLTransformerCache()

	after := svc.PreviewURLTransformer()
	gotAfter := after("https://img2.example.com/b.png")
	wantAfter := "https://img2.example.com/b.png?imageView2/2/w/640/interlace/1/q/75"
	if gotAfter != wantAfter {
		t.Fatalf("after invalidate got %q, want %q", gotAfter, wantAfter)
	}
	if calls != 2 {
		t.Fatalf("resolver calls = %d, want 2", calls)
	}
}
