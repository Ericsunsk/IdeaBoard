package trust

import (
	"errors"
	"fmt"
	"strings"
)

const CanonicalEmailIdentityKind = "canonical_email"

var ErrUntrustedIdentity = errors.New("untrusted identity")

// SessionIdentity is the normalized identity envelope exchanged at the Auth boundary.
// Callers should obtain it through ValidateSessionIdentity before using it for authorization.
type SessionIdentity struct {
	AuthUserID            string `json:"auth_user_id"`
	Email                 string `json:"email"`
	Username              string `json:"username"`
	ClaimIdentity         string `json:"claim_identity"`
	ClaimIdentityKind     string `json:"claim_identity_kind"`
	ClaimIdentityVerified bool   `json:"claim_identity_verified"`
}

// ProvisioningIdentity is a trusted session identity with the business role needed by lifecycle provisioning.
type ProvisioningIdentity struct {
	AuthUserID            string `json:"auth_user_id"`
	Email                 string `json:"email"`
	Username              string `json:"username"`
	ClaimIdentity         string `json:"claim_identity"`
	ClaimIdentityKind     string `json:"claim_identity_kind"`
	ClaimIdentityVerified bool   `json:"claim_identity_verified"`
	Role                  string `json:"role"`
	EnsureActive          bool   `json:"-"`
}

func ValidateSessionIdentity(input SessionIdentity) (SessionIdentity, error) {
	input.AuthUserID = strings.TrimSpace(input.AuthUserID)
	input.Email = normalizeCanonicalEmail(input.Email)
	input.Username = strings.TrimSpace(input.Username)
	input.ClaimIdentity = normalizeCanonicalEmail(input.ClaimIdentity)
	input.ClaimIdentityKind = strings.TrimSpace(input.ClaimIdentityKind)

	if input.AuthUserID == "" {
		return SessionIdentity{}, fmt.Errorf("%w: auth user id is required", ErrUntrustedIdentity)
	}
	if input.Username == "" {
		return SessionIdentity{}, fmt.Errorf("%w: username is required", ErrUntrustedIdentity)
	}
	if !isCanonicalEmail(input.Email) || input.ClaimIdentity != input.Email {
		return SessionIdentity{}, fmt.Errorf("%w: canonical email claim is invalid", ErrUntrustedIdentity)
	}
	if input.ClaimIdentityKind != CanonicalEmailIdentityKind || !input.ClaimIdentityVerified {
		return SessionIdentity{}, fmt.Errorf("%w: canonical email claim is not verified", ErrUntrustedIdentity)
	}
	return input, nil
}

func ProvisioningIdentityFromSession(input SessionIdentity, role string) (ProvisioningIdentity, error) {
	validated, err := ValidateSessionIdentity(input)
	if err != nil {
		return ProvisioningIdentity{}, err
	}
	return NormalizeProvisioningIdentity(ProvisioningIdentity{
		AuthUserID:            validated.AuthUserID,
		Email:                 validated.Email,
		Username:              validated.Username,
		ClaimIdentity:         validated.ClaimIdentity,
		ClaimIdentityKind:     validated.ClaimIdentityKind,
		ClaimIdentityVerified: validated.ClaimIdentityVerified,
		Role:                  role,
	})
}

func NormalizeProvisioningIdentity(input ProvisioningIdentity) (ProvisioningIdentity, error) {
	email := strings.TrimSpace(input.Email)
	if email == "" {
		email = input.ClaimIdentity
	}
	session, err := ValidateSessionIdentity(SessionIdentity{
		AuthUserID:            input.AuthUserID,
		Email:                 email,
		Username:              input.Username,
		ClaimIdentity:         input.ClaimIdentity,
		ClaimIdentityKind:     input.ClaimIdentityKind,
		ClaimIdentityVerified: input.ClaimIdentityVerified,
	})
	if err != nil {
		return ProvisioningIdentity{}, err
	}

	role := strings.ToLower(strings.TrimSpace(input.Role))
	if role == "" {
		role = "user"
	}
	if role != "user" && role != "admin" {
		return ProvisioningIdentity{}, fmt.Errorf("%w: role must be user or admin", ErrUntrustedIdentity)
	}

	return ProvisioningIdentity{
		AuthUserID:            session.AuthUserID,
		Email:                 session.Email,
		Username:              session.Username,
		ClaimIdentity:         session.ClaimIdentity,
		ClaimIdentityKind:     session.ClaimIdentityKind,
		ClaimIdentityVerified: session.ClaimIdentityVerified,
		Role:                  role,
		EnsureActive:          input.EnsureActive,
	}, nil
}

func normalizeCanonicalEmail(raw string) string {
	return strings.ToLower(strings.TrimSpace(raw))
}

func isCanonicalEmail(email string) bool {
	at := strings.LastIndexByte(email, '@')
	if email == "" || len(email) > 320 || at <= 0 || at == len(email)-1 || strings.ContainsAny(email, " \t\r\n") {
		return false
	}
	local, domain := email[:at], email[at+1:]
	return len(local) <= 64 && strings.Contains(domain, ".") && !strings.HasPrefix(domain, ".") && !strings.HasSuffix(domain, ".")
}
