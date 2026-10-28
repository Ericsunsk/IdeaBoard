package http

import (
	"strings"
	"testing"
)

func TestV1PublicAvatarURLForRow(t *testing.T) {
	for _, tc := range []struct {
		name, uid, avatar, want string
	}{
		{"empty", "USER01", "", ""},
		{"OAuth", "USER01", "https://cdn.example/avatar.png", "https://cdn.example/avatar.png"},
		{"lookalike host", "USER01", "https://api.dicebear.com.evil.com/avatar.svg", "https://api.dicebear.com.evil.com/avatar.svg"},
		{"public route", "USER01", "/api/v1/public/users/USER01/avatar?v=123", "/api/v1/public/users/USER01/avatar?v=123"},
		{"missing uid", "", "https://api.dicebear.com/9.x/big-ears-neutral/svg?seed=user-1", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := V1PublicAvatarURLForRow(tc.uid, tc.avatar); got != tc.want {
				t.Fatalf("avatar projection = %q, want %q", got, tc.want)
			}
		})
	}
	for _, avatar := range []string{
		"uploads/avatars/USER01/abcdef1234567890.png",
		"https://api.dicebear.com/9.x/big-ears-neutral/svg?seed=user-1",
	} {
		got := V1PublicAvatarURLForRow("USER01", avatar)
		if !strings.HasPrefix(got, "/api/v1/public/users/USER01/avatar?v=") {
			t.Fatalf("avatar projection = %q, want versioned public route", got)
		}
		if got == V1PublicAvatarURLForRow("USER01", avatar+"changed") {
			t.Fatal("avatar replacement must change the projected version")
		}
	}
}
