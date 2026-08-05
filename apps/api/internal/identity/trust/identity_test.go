package trust

import (
	"errors"
	"testing"
)

func TestValidateSessionIdentityNormalizesTrustedEnvelope(t *testing.T) {
	got, err := ValidateSessionIdentity(SessionIdentity{
		AuthUserID:            " auth-1 ",
		Email:                 " Alice@Example.com ",
		Username:              " alice ",
		ClaimIdentity:         " alice@example.com ",
		ClaimIdentityKind:     " canonical_email ",
		ClaimIdentityVerified: true,
	})
	if err != nil {
		t.Fatalf("ValidateSessionIdentity returned error: %v", err)
	}
	if got.AuthUserID != "auth-1" || got.Email != "alice@example.com" || got.Username != "alice" || got.ClaimIdentity != "alice@example.com" || got.ClaimIdentityKind != CanonicalEmailIdentityKind {
		t.Fatalf("normalized identity = %+v", got)
	}
}

func TestValidateSessionIdentityFailsClosed(t *testing.T) {
	tests := []SessionIdentity{
		{Email: "alice@example.com", Username: "alice", ClaimIdentity: "alice@example.com", ClaimIdentityKind: CanonicalEmailIdentityKind, ClaimIdentityVerified: true},
		{AuthUserID: "auth-1", Email: "alice@example.com", Username: "alice", ClaimIdentity: "attacker@example.com", ClaimIdentityKind: CanonicalEmailIdentityKind, ClaimIdentityVerified: true},
		{AuthUserID: "auth-1", Email: "alice@example.com", Username: "alice", ClaimIdentity: "alice@example.com", ClaimIdentityKind: "untrusted", ClaimIdentityVerified: false},
		{AuthUserID: "auth-1", Email: "not-an-email", Username: "alice", ClaimIdentity: "not-an-email", ClaimIdentityKind: CanonicalEmailIdentityKind, ClaimIdentityVerified: true},
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
		Username:              "alice",
		ClaimIdentity:         "ALICE@example.com",
		ClaimIdentityKind:     CanonicalEmailIdentityKind,
		ClaimIdentityVerified: true,
		EnsureActive:          true,
	})
	if err != nil {
		t.Fatalf("NormalizeProvisioningIdentity returned error: %v", err)
	}
	if got.Role != "user" || !got.EnsureActive || got.ClaimIdentity != "alice@example.com" {
		t.Fatalf("normalized provisioning identity = %+v", got)
	}
}

func TestNormalizeProvisioningIdentityRejectsEmailClaimMismatch(t *testing.T) {
	_, err := NormalizeProvisioningIdentity(ProvisioningIdentity{
		AuthUserID:            "auth-1",
		Email:                 "alice@example.com",
		Username:              "alice",
		ClaimIdentity:         "other@example.com",
		ClaimIdentityKind:     CanonicalEmailIdentityKind,
		ClaimIdentityVerified: true,
		Role:                  "admin",
	})
	if !errors.Is(err, ErrUntrustedIdentity) {
		t.Fatalf("error = %v, want ErrUntrustedIdentity", err)
	}
}
