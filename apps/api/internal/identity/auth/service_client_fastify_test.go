package auth

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"ideaboard/api/internal/platform/config"
)

func TestServiceClientAgainstFastify(t *testing.T) {
	baseURL := strings.TrimSpace(os.Getenv("AUTH_FASTIFY_CONTRACT_URL"))
	if baseURL == "" {
		t.Skip("AUTH_FASTIFY_CONTRACT_URL is only set by the Auth contract harness")
	}
	internalKey := strings.TrimSpace(os.Getenv("AUTH_FASTIFY_CONTRACT_INTERNAL_KEY"))
	if internalKey == "" {
		t.Fatal("AUTH_FASTIFY_CONTRACT_INTERNAL_KEY is required by the Auth contract harness")
	}

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
	if session.AuthUserID != "auth-contract-user" || session.Username != "alice" || !session.ClaimIdentityVerified {
		t.Fatalf("ValidateSession response = %+v, want trusted contract identity", session)
	}

	user, err := client.EnsureUser(ctx, " alice ", "secret123", false, "admin")
	if err != nil {
		t.Fatalf("EnsureUser against Fastify: %v", err)
	}
	if user.AuthUserID != "auth-contract-user" || user.Role != "admin" || user.Existing {
		t.Fatalf("EnsureUser response = %+v, want newly provisioned admin identity", user)
	}

	const authUserID = "auth-contract-user"
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
