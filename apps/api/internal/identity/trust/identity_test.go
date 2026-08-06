package trust

import (
	"errors"
	"testing"
)

func TestValidateSessionIdentityNormalizesTrustedEnvelope(t *testing.T) {
	got, err := ValidateSessionIdentity(SessionIdentity{
		AuthUserID:            " auth-1 ",
		Email:                 " Alice@Example.com ",
		DisplayName:           "  Alice 设计师  ",
		Avatar:                " https://lh3.googleusercontent.com/avatar ",
		ClaimIdentity:         " alice@example.com ",
		ClaimIdentityKind:     " canonical_email ",
		ClaimIdentityVerified: true,
	})
	if err != nil {
		t.Fatalf("ValidateSessionIdentity returned error: %v", err)
	}
	if got.AuthUserID != "auth-1" || got.Email != "alice@example.com" || got.DisplayName != "Alice 设计师" || got.Avatar != "https://lh3.googleusercontent.com/avatar" || got.ClaimIdentity != "alice@example.com" || got.ClaimIdentityKind != CanonicalEmailIdentityKind {
		t.Fatalf("normalized identity = %+v", got)
	}
}

func TestValidateSessionIdentityDropsUntrustedAvatarURL(t *testing.T) {
	got, err := ValidateSessionIdentity(SessionIdentity{
		AuthUserID: "auth-1", Email: "alice@example.com", DisplayName: "Alice",
		Avatar: "javascript:alert(1)", ClaimIdentity: "alice@example.com",
		ClaimIdentityKind: CanonicalEmailIdentityKind, ClaimIdentityVerified: true,
	})
	if err != nil {
		t.Fatalf("ValidateSessionIdentity returned error: %v", err)
	}
	if got.Avatar != "" {
		t.Fatalf("Avatar = %q, want dropped", got.Avatar)
	}
}

func TestValidateSessionIdentityFailsClosed(t *testing.T) {
	tests := []SessionIdentity{
		{Email: "alice@example.com", DisplayName: "Alice", ClaimIdentity: "alice@example.com", ClaimIdentityKind: CanonicalEmailIdentityKind, ClaimIdentityVerified: true},
		{AuthUserID: "auth-1", Email: "alice@example.com", DisplayName: "Alice", ClaimIdentity: "attacker@example.com", ClaimIdentityKind: CanonicalEmailIdentityKind, ClaimIdentityVerified: true},
		{AuthUserID: "auth-1", Email: "alice@example.com", DisplayName: "Alice", ClaimIdentity: "alice@example.com", ClaimIdentityKind: "untrusted", ClaimIdentityVerified: false},
		{AuthUserID: "auth-1", Email: "not-an-email", DisplayName: "Alice", ClaimIdentity: "not-an-email", ClaimIdentityKind: CanonicalEmailIdentityKind, ClaimIdentityVerified: true},
	}
	for index, input := range tests {
		if _, err := ValidateSessionIdentity(input); !errors.Is(err, ErrUntrustedIdentity) {
			t.Errorf("case %d error = %v, want ErrUntrustedIdentity", index, err)
		}
	}
}

func TestNormalizeProvisioningIdentityDefaultsRoleAndPreservesLifecycleFlag(t *testing.T) {
	got, err := NormalizeProvisioningIdentity(ProvisioningIdentity{
		AuthUserID:            "auth-1",
		DisplayName:           "Alice",
		ClaimIdentity:         "ALICE@example.com",
		ClaimIdentityKind:     CanonicalEmailIdentityKind,
		ClaimIdentityVerified: true,
		EnsureActive:          true,
	})
	if err != nil {
		t.Fatalf("NormalizeProvisioningIdentity returned error: %v", err)
	}
	if got.Role != "user" || !got.EnsureActive || got.DisplayName != "Alice" || got.ClaimIdentity != "alice@example.com" {
		t.Fatalf("normalized provisioning identity = %+v", got)
	}
}

func TestNormalizeProvisioningIdentityRejectsEmailClaimMismatch(t *testing.T) {
	_, err := NormalizeProvisioningIdentity(ProvisioningIdentity{
		AuthUserID:            "auth-1",
		Email:                 "alice@example.com",
		ClaimIdentity:         "other@example.com",
		ClaimIdentityKind:     CanonicalEmailIdentityKind,
		ClaimIdentityVerified: true,
		Role:                  "admin",
	})
	if !errors.Is(err, ErrUntrustedIdentity) {
		t.Fatalf("error = %v, want ErrUntrustedIdentity", err)
	}
}
