package providers

import (
	"testing"
)

func TestParseSizeFromParams(t *testing.T) {
	tests := []struct {
		name    string
		params  map[string]interface{}
		wantW   int
		wantH   int
	}{
		{
			name:   "default 1:1",
			params: map[string]interface{}{},
			wantW:  1024,
			wantH:  1024,
		},
		{
			name:   "1:1 explicit",
			params: map[string]interface{}{"ratio": "1:1"},
			wantW:  1024,
			wantH:  1024,
		},
		{
			name:   "4:3 standard",
			params: map[string]interface{}{"ratio": "4:3"},
			wantW:  1024,
			wantH:  768,
		},
		{
			name:   "3:4 standard aligned to 64 without 1365",
			params: map[string]interface{}{"ratio": "3:4"},
			wantW:  768,
			wantH:  1024,
		},
		{
			name:   "16:9 standard",
			params: map[string]interface{}{"ratio": "16:9"},
			wantW:  1024,
			wantH:  576,
		},
		{
			name:   "9:16 standard",
			params: map[string]interface{}{"ratio": "9:16"},
			wantW:  576,
			wantH:  1024,
		},
		{
			name:   "4:3 with 2k quality",
			params: map[string]interface{}{"ratio": "4:3", "quality": "2k"},
			wantW:  2048,
			wantH:  1536,
		},
		{
			name:   "3:4 with 4k quality",
			params: map[string]interface{}{"ratio": "3:4", "quality": "4k"},
			wantW:  3072,
			wantH:  4096,
		},
		{
			name:   "custom size override",
			params: map[string]interface{}{"custom_width": 800, "custom_height": 600, "quality": "2k"},
			wantW:  1600,
			wantH:  1200,
		},
		{
			name:   "invalid ratio fallback to 1:1",
			params: map[string]interface{}{"ratio": "invalid"},
			wantW:  1024,
			wantH:  1024,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotW, gotH := parseSizeFromParams(tt.params)
			if gotW != tt.wantW || gotH != tt.wantH {
				t.Errorf("parseSizeFromParams() = (%d, %d), want (%d, %d)", gotW, gotH, tt.wantW, tt.wantH)
			}
			if tt.params["custom_width"] == nil && (gotW%64 != 0 || gotH%64 != 0) {
				t.Errorf("parseSizeFromParams() result (%d, %d) is not aligned to 64", gotW, gotH)
			}
		})
	}
}
