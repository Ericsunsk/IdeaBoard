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
			name:       "appends after existing non-thumbnail query",
			raw:        "https://img.example.com/a.png?token=abc",
			accessHost: "img.example.com",
			want:       "https://img.example.com/a.png?token=abc&imageView2/2/w/640/interlace/1/q/75",
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
