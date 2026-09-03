package checkout

import (
	"strings"
	"testing"
	"time"
)

func TestNormalizeReturnBaseURL(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
		ok   bool
	}{
		{name: "https trims slash", raw: "https://app.example.com///", want: "https://app.example.com", ok: true},
		{name: "loopback http", raw: "http://localhost:5173/", want: "http://localhost:5173", ok: true},
		{name: "reject query", raw: "https://app.example.com/?next=1"},
		{name: "reject public http", raw: "http://app.example.com"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := NormalizeReturnBaseURL(test.raw)
			if test.ok {
				if err != nil || got != test.want {
					t.Fatalf("NormalizeReturnBaseURL() = %q, %v; want %q", got, err, test.want)
				}
				return
			}
			if err == nil {
				t.Fatalf("NormalizeReturnBaseURL() = %q, nil; want error", got)
			}
		})
	}
}

func TestReturnURLAndNewPublicOrderNumber(t *testing.T) {
	returnURL := ReturnURL("https://app.example.com", "token value", "success")
	if !strings.Contains(returnURL, "payment_return=token+value") || !strings.HasSuffix(returnURL, "checkout_return=success") {
		t.Fatalf("ReturnURL() = %q", returnURL)
	}
	number, err := NewPublicOrderNumber(time.Date(2026, 9, 3, 0, 0, 0, 0, time.UTC))
	if err != nil || !strings.HasPrefix(number, "IBP-20260903-") || len(number) != len("IBP-20260903-")+12 {
		t.Fatalf("NewPublicOrderNumber() = %q, %v", number, err)
	}
}
