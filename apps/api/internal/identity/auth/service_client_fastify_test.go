package auth

import (
	"context"
	"errors"
	"testing"

	"ideaboard/api/internal/platform/config"
	"ideaboard/api/internal/testsupport/testenv"
)

func TestServiceClientAgainstFastify(t *testing.T) {
	baseURL := testenv.GetOrSkip(t, "AUTH_FASTIFY_CONTRACT_URL", "AUTH_FASTIFY_CONTRACT_URL is only set by the Auth contract harness")
	internalKey := testenv.Require(t, "AUTH_FASTIFY_CONTRACT_INTERNAL_KEY", "AUTH_FASTIFY_CONTRACT_INTERNAL_KEY is required by the Auth contract harness")

	client := NewServiceClient(config.AuthServiceConfig{
		BaseURL:        baseURL,
		InternalAPIKey: internalKey,
		TimeoutSeconds: 5,
	})
	ctx := context.Background()

	session, err := client.ValidateSession(ctx, "better-auth.session_token=contract-session")
	if err != nil {
		t.Fatalf("ValidateSession against Fastify: %v", err)
	}
	if session.AuthUserID != "auth-contract-user" || session.DisplayName != "Alice" || session.Avatar != "https://lh3.googleusercontent.com/avatar" || !session.ClaimIdentityVerified {
		t.Fatalf("ValidateSession response = %+v, want trusted contract identity", session)
	}

	user, err := client.EnsureUser(ctx, "alice@example.com", "Alice", "secret1234", false, "admin")
	if err != nil {
		t.Fatalf("EnsureUser against Fastify: %v", err)
	}
	if user.AuthUserID != "auth-contract-user" || user.Role != "admin" || user.Existing {
		t.Fatalf("EnsureUser response = %+v, want newly provisioned admin identity", user)
	}
	const authUserID = "auth-contract-user"
	emails, err := client.GetUserEmails(ctx, []string{authUserID})
	if err != nil {
		t.Fatalf("GetUserEmails against Fastify: %v", err)
	}
	if emails[authUserID].Email != "alice@example.com" || len(emails[authUserID].Providers) != 1 || emails[authUserID].Providers[0] != "email" {
		t.Fatalf("GetUserEmails response = %+v, want email identity", emails)
	}

	assertFastifyMutationSuccess(t, "DeleteUser", func() error {
		return client.DeleteUser(ctx, authUserID)
	})
	assertFastifyMutationSuccess(t, "SetUserRole", func() error {
		return client.SetUserRole(ctx, authUserID, "admin")
	})
	assertFastifyMutationSuccess(t, "BanUser", func() error {
		return client.BanUser(ctx, authUserID, " policy violation ", 3600)
	})
	assertFastifyMutationSuccess(t, "UnbanUser", func() error {
		return client.UnbanUser(ctx, authUserID)
	})
	assertFastifyMutationSuccess(t, "RevokeUserSessions", func() error {
		return client.RevokeUserSessions(ctx, authUserID)
	})

	if err := client.DeleteUser(ctx, "missing-auth-user"); !errors.Is(err, ErrAuthUserNotFound) {
		t.Fatalf("DeleteUser not-found error = %v, want ErrAuthUserNotFound", err)
	}
}

func assertFastifyMutationSuccess(t *testing.T, operation string, call func() error) {
	t.Helper()
	if err := call(); err != nil {
		t.Fatalf("%s against Fastify: %v", operation, err)
	}
}
