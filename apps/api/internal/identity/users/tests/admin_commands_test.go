package tests

// Request-path policy tests for the durable admin commands (issue #888).
// These run against in-memory fakes because they assert pre-transaction
// validation only: actor/target policy never reaches the sync store.
// Business-first mutation, intent persistence, and convergence are
// covered with real PostgreSQL transactions in durable_sync_test.go.

import (
	"context"
	"errors"
	"reflect"
	"strconv"
	"testing"

	"ideaboard/api/internal/identity/autherrors"
	"ideaboard/api/internal/identity/principal"
	"ideaboard/api/internal/platform/database"

	"github.com/google/uuid"
)

const identityAdminTargetPublicID = "00000000-0000-0000-0000-000000000042"

type fakeIdentityAdminUsers struct {
	target       *User
	getErr       error
	roleErrors   []error
	statusErrors []error
	deleteErrors []error
	events       *[]string
	deleteCalls  int
}

func (fake *fakeIdentityAdminUsers) GetByPublicID(_ context.Context, _ string) (*User, error) {
	if fake.getErr != nil {
		return nil, fake.getErr
	}
	return fake.target, nil
}

func (fake *fakeIdentityAdminUsers) UpdateRole(_ context.Context, _ int64, role string) error {
	appendIdentityAdminEvent(fake.events, "local-role:"+role)
	err := popIdentityAdminError(&fake.roleErrors)
	if err == nil {
		fake.target.Role = role
	}
	return err
}

func (fake *fakeIdentityAdminUsers) UpdateStatus(_ context.Context, _ int64, status int16) error {
	appendIdentityAdminEvent(fake.events, "local-status:"+strconv.Itoa(int(status)))
	err := popIdentityAdminError(&fake.statusErrors)
	if err == nil {
		fake.target.Status = status
	}
	return err
}

func (fake *fakeIdentityAdminUsers) DeleteUser(_ context.Context, _ int64, _ AuthUserDeleter) error {
	fake.deleteCalls++
	appendIdentityAdminEvent(fake.events, "local-delete")
	return popIdentityAdminError(&fake.deleteErrors)
}

type fakeIdentityAdminAuth struct {
	roleErrors   []error
	banErrors    []error
	unbanErrors  []error
	revokeErrors []error
	deleteErrors []error
	events       *[]string
}

func (fake *fakeIdentityAdminAuth) DeleteUser(_ context.Context, _ string) error {
	appendIdentityAdminEvent(fake.events, "auth-delete")
	return popIdentityAdminError(&fake.deleteErrors)
}

func (fake *fakeIdentityAdminAuth) SetUserRole(_ context.Context, _ string, role string) error {
	appendIdentityAdminEvent(fake.events, "auth-role:"+role)
	return popIdentityAdminError(&fake.roleErrors)
}

func (fake *fakeIdentityAdminAuth) BanUser(_ context.Context, _ string, _ string, _ int) error {
	appendIdentityAdminEvent(fake.events, "auth-ban")
	return popIdentityAdminError(&fake.banErrors)
}

func (fake *fakeIdentityAdminAuth) UnbanUser(_ context.Context, _ string) error {
	appendIdentityAdminEvent(fake.events, "auth-unban")
	return popIdentityAdminError(&fake.unbanErrors)
}

func (fake *fakeIdentityAdminAuth) RevokeUserSessions(_ context.Context, _ string) error {
	appendIdentityAdminEvent(fake.events, "auth-revoke")
	return popIdentityAdminError(&fake.revokeErrors)
}

func TestIdentityAdminCommandsRequestValidation(t *testing.T) {
	t.Run("invalid role is rejected before any write", func(t *testing.T) {
		commands, _, _, events := newIdentityAdminCommandFixture("user", 1)

		_, err := commands.SetRole(context.Background(), identityAdminActor(t), identityAdminTargetPublicID, "superadmin")
		assertIdentityAdminErrorKind(t, err, IdentityAdminInvalid)
		assertIdentityAdminEvents(t, events)
	})

	t.Run("ban rejects negative expiry before any write", func(t *testing.T) {
		commands, _, _, events := newIdentityAdminCommandFixture("user", 1)

		_, err := commands.Ban(context.Background(), identityAdminActor(t), identityAdminTargetPublicID, IdentityAdminBanInput{ExpiresInDays: -1})
		assertIdentityAdminErrorKind(t, err, IdentityAdminInvalid)
		assertIdentityAdminEvents(t, events)
	})

	t.Run("mutations require a sync store", func(t *testing.T) {
		authUserID := "auth-user-42"
		users := &fakeIdentityAdminUsers{
			target: &User{ID: 42, PublicID: uuid.MustParse(identityAdminTargetPublicID), DisplayName: "target", AuthUserID: &authUserID, Role: "user", Status: 1},
			events: &[]string{},
		}
		commands := newIdentityAdminCommands(users, &fakeIdentityAdminAuth{events: &[]string{}}, nil)

		_, err := commands.SetRole(context.Background(), identityAdminActor(t), identityAdminTargetPublicID, "admin")
		assertIdentityAdminErrorKind(t, err, IdentityAdminUnavailable)
	})
}

func TestIdentityAdminCommandsRevokeSessionsIsRetryable(t *testing.T) {
	t.Run("Auth failure can be retried", func(t *testing.T) {
		commands, _, auth, events := newIdentityAdminCommandFixture("user", 1)
		auth.revokeErrors = []error{errors.New("Auth unavailable"), nil}

		_, err := commands.RevokeSessions(context.Background(), identityAdminActor(t), identityAdminTargetPublicID)
		assertIdentityAdminErrorKind(t, err, IdentityAdminAuthFailure)
		if _, err := commands.RevokeSessions(context.Background(), identityAdminActor(t), identityAdminTargetPublicID); err != nil {
			t.Fatalf("retry revoke sessions: %v", err)
		}
		assertIdentityAdminEvents(t, events, "auth-revoke", "auth-revoke")
	})

	t.Run("Auth not found is typed", func(t *testing.T) {
		commands, _, auth, events := newIdentityAdminCommandFixture("user", 1)
		auth.revokeErrors = []error{autherrors.ErrAuthUserNotFound}

		_, err := commands.RevokeSessions(context.Background(), identityAdminActor(t), identityAdminTargetPublicID)
		assertIdentityAdminErrorKind(t, err, IdentityAdminAuthIdentityNotFound)
		assertIdentityAdminEvents(t, events, "auth-revoke")
	})
}

func TestIdentityAdminCommandsDeleteReportsUnsettledPaymentConflict(t *testing.T) {
	commands, users, _, events := newIdentityAdminCommandFixture("user", 1)
	users.deleteErrors = []error{ErrUserDeletionBlockedByPaymentState}

	_, err := commands.Delete(context.Background(), identityAdminActor(t), identityAdminTargetPublicID)
	assertIdentityAdminErrorKind(t, err, IdentityAdminConflict)
	assertIdentityAdminEvents(t, events, "local-delete")
}

func TestIdentityAdminCommandsOwnTargetPolicy(t *testing.T) {
	tests := []struct {
		name   string
		actor  func(*testing.T) principal.Principal
		mutate func(*fakeIdentityAdminUsers)
		kind   IdentityAdminErrorKind
	}{
		{name: "invalid actor", actor: func(*testing.T) principal.Principal { return principal.Principal{} }, kind: IdentityAdminInvalid},
		{name: "non-admin actor", actor: identityUserActor, kind: IdentityAdminForbidden},
		{name: "self operation", actor: identitySelfAdminActor, kind: IdentityAdminInvalid},
		{name: "target not found", actor: identityAdminActor, mutate: func(users *fakeIdentityAdminUsers) { users.getErr = database.ErrNotFound }, kind: IdentityAdminTargetNotFound},
		{name: "admin target", actor: identityAdminActor, mutate: func(users *fakeIdentityAdminUsers) { users.target.Role = "admin" }, kind: IdentityAdminInvalid},
		{name: "missing Auth identity", actor: identityAdminActor, mutate: func(users *fakeIdentityAdminUsers) { users.target.AuthUserID = nil }, kind: IdentityAdminInvalid},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			commands, users, _, _ := newIdentityAdminCommandFixture("user", 1)
			if tt.mutate != nil {
				tt.mutate(users)
			}
			_, err := commands.Ban(context.Background(), tt.actor(t), identityAdminTargetPublicID, IdentityAdminBanInput{})
			assertIdentityAdminErrorKind(t, err, tt.kind)
		})
	}
}

func newIdentityAdminCommandFixture(role string, status int16) (IdentityAdminCommands, *fakeIdentityAdminUsers, *fakeIdentityAdminAuth, *[]string) {
	authUserID := "auth-user-42"
	events := &[]string{}
	users := &fakeIdentityAdminUsers{
		target: &User{ID: 42, PublicID: uuid.MustParse(identityAdminTargetPublicID), DisplayName: "target", AuthUserID: &authUserID, Role: role, Status: status},
		events: events,
	}
	auth := &fakeIdentityAdminAuth{events: events}
	return newIdentityAdminCommands(users, auth, nil), users, auth, events
}

func identityAdminActor(t *testing.T) principal.Principal {
	t.Helper()
	actor, err := principal.New(99, uuid.New(), "auth-admin-99", "Test Admin", principal.RoleAdmin.String())
	if err != nil {
		t.Fatalf("create admin principal: %v", err)
	}
	return actor
}

func identityUserActor(t *testing.T) principal.Principal {
	t.Helper()
	actor, err := principal.New(98, uuid.New(), "auth-user-98", "Test User", principal.RoleUser.String())
	if err != nil {
		t.Fatalf("create user principal: %v", err)
	}
	return actor
}

func identitySelfAdminActor(t *testing.T) principal.Principal {
	t.Helper()
	actor, err := principal.New(42, uuid.New(), "auth-admin-42", "Self Admin", principal.RoleAdmin.String())
	if err != nil {
		t.Fatalf("create self admin principal: %v", err)
	}
	return actor
}

func assertIdentityAdminErrorKind(t *testing.T, err error, want IdentityAdminErrorKind) {
	t.Helper()
	var commandErr *IdentityAdminCommandError
	if !errors.As(err, &commandErr) {
		t.Fatalf("error = %#v, want IdentityAdminCommandError kind %q", err, want)
	}
	if commandErr.Kind != want {
		t.Fatalf("error kind = %q, want %q (error: %v)", commandErr.Kind, want, err)
	}
}

func assertIdentityAdminEvents(t *testing.T, got *[]string, want ...string) {
	t.Helper()
	actual := []string{}
	if got != nil {
		actual = *got
	}
	expected := []string{}
	if want != nil {
		expected = want
	}
	if !reflect.DeepEqual(actual, expected) {
		t.Fatalf("events = %v, want %v", actual, expected)
	}
}

func appendIdentityAdminEvent(events *[]string, event string) {
	if events != nil {
		*events = append(*events, event)
	}
}

func popIdentityAdminError(errorsQueue *[]error) error {
	if len(*errorsQueue) == 0 {
		return nil
	}
	err := (*errorsQueue)[0]
	*errorsQueue = (*errorsQueue)[1:]
	return err
}
