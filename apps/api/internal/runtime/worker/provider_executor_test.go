package worker

import (
	"testing"
	"time"

	"ideaboard/api/internal/accounts"
)

func TestResolveAccountTimeout(t *testing.T) {
	tests := []struct {
		name    string
		account *accounts.ApiAccount
		want    time.Duration
	}{
		{
			name:    "nil account fallback",
			account: nil,
			want:    120 * time.Second,
		},
		{
			name: "negative timeout fallback",
			account: &accounts.ApiAccount{
				Timeout: -10,
			},
			want: 120 * time.Second,
		},
		{
			name: "zero timeout fallback",
			account: &accounts.ApiAccount{
				Timeout: 0,
			},
			want: 120 * time.Second,
		},
		{
			name: "positive timeout keep",
			account: &accounts.ApiAccount{
				Timeout: 75,
			},
			want: 75 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resolveAccountTimeout(tt.account); got != tt.want {
				t.Fatalf("resolveAccountTimeout() = %v, want %v", got, tt.want)
			}
		})
	}
}
