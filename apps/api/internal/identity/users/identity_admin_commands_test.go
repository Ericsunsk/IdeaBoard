package users

import (
	"context"
	"errors"
	"reflect"
	"strconv"
	"testing"

	"ideaboard/api/internal/identity/autherrors"
	"ideaboard/api/internal/identity/principal"
	"ideaboard/api/internal/platform/database"
)

type fakeIdentityAdminUsers struct {
	target       *User
	getErr       error
	roleErrors   []error
	statusErrors []error
	deleteErrors []error
	events       *[]string
	deleteCalls  int
}

func (fake *fakeIdentityAdminUsers) GetByID(_ context.Context, _ int64) (*User, error) {
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

func TestIdentityAdminCommandsRoleOrderingAndRecovery(t *testing.T) {
	t.Run("promotion updates Auth before local role", func(t *testing.T) {
		commands, _, _, events := newIdentityAdminCommandFixture("user", 1)

		if err := commands.SetRole(context.Background(), identityAdminActor(t), 42, "admin"); err != nil {
			t.Fatalf("promote user: %v", err)
		}
		assertIdentityAdminEvents(t, events, "auth-role:admin", "local-role:admin")
	})

	t.Run("promotion rejects missing Auth identity", func(t *testing.T) {
		commands, _, auth, events := newIdentityAdminCommandFixture("user", 1)
		auth.roleErrors = []error{autherrors.ErrAuthUserNotFound}

		err := commands.SetRole(context.Background(), identityAdminActor(t), 42, "admin")
		assertIdentityAdminErrorKind(t, err, IdentityAdminAuthIdentityNotFound)
		assertIdentityAdminEvents(t, events, "auth-role:admin")
	})

	t.Run("promotion rolls Auth back when local update fails", func(t *testing.T) {
		commands, users, _, events := newIdentityAdminCommandFixture("user", 1)
		users.roleErrors = []error{errors.New("database unavailable")}

		err := commands.SetRole(context.Background(), identityAdminActor(t), 42, "admin")
		assertIdentityAdminErrorKind(t, err, IdentityAdminBusinessFailure)
		assertIdentityAdminEvents(t, events, "auth-role:admin", "local-role:admin", "auth-role:user")
	})

	t.Run("promotion retry converges after rollback failure", func(t *testing.T) {
		commands, users, auth, events := newIdentityAdminCommandFixture("user", 1)
		users.roleErrors = []error{errors.New("database unavailable"), nil}
		auth.roleErrors = []error{nil, errors.New("rollback unavailable"), nil}

		err := commands.SetRole(context.Background(), identityAdminActor(t), 42, "admin")
		assertIdentityAdminErrorKind(t, err, IdentityAdminReconciliationRequired)
		var commandErr *IdentityAdminCommandError
		if !errors.As(err, &commandErr) || commandErr.RollbackCause == nil {
			t.Fatalf("rollback failure was not retained: %#v", err)
		}
		if err := commands.SetRole(context.Background(), identityAdminActor(t), 42, "admin"); err != nil {
			t.Fatalf("retry promotion: %v", err)
		}
		assertIdentityAdminEvents(t, events,
			"auth-role:admin", "local-role:admin", "auth-role:user",
			"auth-role:admin", "local-role:admin",
		)
	})

	t.Run("demotion updates local role before Auth", func(t *testing.T) {
		commands, _, _, events := newIdentityAdminCommandFixture("admin", 1)

		if err := commands.SetRole(context.Background(), identityAdminActor(t), 42, "user"); err != nil {
			t.Fatalf("demote user: %v", err)
		}
		assertIdentityAdminEvents(t, events, "local-role:user", "auth-role:user")
	})

	t.Run("demotion stops when local update fails", func(t *testing.T) {
		commands, users, _, events := newIdentityAdminCommandFixture("admin", 1)
		users.roleErrors = []error{errors.New("database unavailable")}

		err := commands.SetRole(context.Background(), identityAdminActor(t), 42, "user")
		assertIdentityAdminErrorKind(t, err, IdentityAdminBusinessFailure)
		assertIdentityAdminEvents(t, events, "local-role:user")
	})

	t.Run("demotion retry reconciles Auth after partial failure", func(t *testing.T) {
		commands, _, auth, events := newIdentityAdminCommandFixture("admin", 1)
		auth.roleErrors = []error{autherrors.ErrAuthUserNotFound, nil}

		err := commands.SetRole(context.Background(), identityAdminActor(t), 42, "user")
		assertIdentityAdminErrorKind(t, err, IdentityAdminAuthIdentityNotFound)
		if err := commands.SetRole(context.Background(), identityAdminActor(t), 42, "user"); err != nil {
			t.Fatalf("retry demotion: %v", err)
		}
		assertIdentityAdminEvents(t, events, "local-role:user", "auth-role:user", "auth-role:user")
	})

	t.Run("demotion Auth failure requires reconciliation and retry converges", func(t *testing.T) {
		commands, _, auth, events := newIdentityAdminCommandFixture("admin", 1)
		auth.roleErrors = []error{errors.New("Auth unavailable"), nil}

		err := commands.SetRole(context.Background(), identityAdminActor(t), 42, "user")
		assertIdentityAdminErrorKind(t, err, IdentityAdminReconciliationRequired)
		if err := commands.SetRole(context.Background(), identityAdminActor(t), 42, "user"); err != nil {
			t.Fatalf("retry demotion: %v", err)
		}
		assertIdentityAdminEvents(t, events, "local-role:user", "auth-role:user", "auth-role:user")
	})
}

func TestIdentityAdminCommandsBanAndUnbanRecovery(t *testing.T) {
	t.Run("ban updates Auth before local status", func(t *testing.T) {
		commands, _, _, events := newIdentityAdminCommandFixture("user", 1)

		if err := commands.Ban(context.Background(), identityAdminActor(t), 42, IdentityAdminBanInput{Reason: "abuse", ExpiresInDays: 7}); err != nil {
			t.Fatalf("ban user: %v", err)
		}
		assertIdentityAdminEvents(t, events, "auth-ban", "local-status:0")
	})

	t.Run("ban reports Auth not found without local mutation", func(t *testing.T) {
		commands, _, auth, events := newIdentityAdminCommandFixture("user", 1)
		auth.banErrors = []error{autherrors.ErrAuthUserNotFound}

		err := commands.Ban(context.Background(), identityAdminActor(t), 42, IdentityAdminBanInput{})
		assertIdentityAdminErrorKind(t, err, IdentityAdminAuthIdentityNotFound)
		assertIdentityAdminEvents(t, events, "auth-ban")
	})

	t.Run("ban Auth failure is retryable", func(t *testing.T) {
		commands, _, auth, events := newIdentityAdminCommandFixture("user", 1)
		auth.banErrors = []error{errors.New("Auth unavailable"), nil}

		err := commands.Ban(context.Background(), identityAdminActor(t), 42, IdentityAdminBanInput{})
		assertIdentityAdminErrorKind(t, err, IdentityAdminAuthFailure)
		if err := commands.Ban(context.Background(), identityAdminActor(t), 42, IdentityAdminBanInput{}); err != nil {
			t.Fatalf("retry ban after Auth failure: %v", err)
		}
		assertIdentityAdminEvents(t, events, "auth-ban", "auth-ban", "local-status:0")
	})

	t.Run("ban compensates local failure and retry converges", func(t *testing.T) {
		commands, users, _, events := newIdentityAdminCommandFixture("user", 1)
		users.statusErrors = []error{errors.New("wallet freeze failed"), nil}

		err := commands.Ban(context.Background(), identityAdminActor(t), 42, IdentityAdminBanInput{})
		assertIdentityAdminErrorKind(t, err, IdentityAdminBusinessFailure)
		if err := commands.Ban(context.Background(), identityAdminActor(t), 42, IdentityAdminBanInput{}); err != nil {
			t.Fatalf("retry ban: %v", err)
		}
		assertIdentityAdminEvents(t, events, "auth-ban", "local-status:0", "auth-unban", "auth-ban", "local-status:0")
	})

	t.Run("ban retains rollback failure", func(t *testing.T) {
		commands, users, auth, _ := newIdentityAdminCommandFixture("user", 1)
		users.statusErrors = []error{errors.New("wallet freeze failed")}
		auth.unbanErrors = []error{errors.New("Auth unavailable")}

		err := commands.Ban(context.Background(), identityAdminActor(t), 42, IdentityAdminBanInput{})
		assertIdentityAdminErrorKind(t, err, IdentityAdminReconciliationRequired)
	})

	t.Run("unban updates Auth before local status", func(t *testing.T) {
		commands, _, _, events := newIdentityAdminCommandFixture("user", 0)

		if err := commands.Unban(context.Background(), identityAdminActor(t), 42); err != nil {
			t.Fatalf("unban user: %v", err)
		}
		assertIdentityAdminEvents(t, events, "auth-unban", "local-status:1")
	})

	t.Run("unban reports Auth not found without local mutation", func(t *testing.T) {
		commands, _, auth, events := newIdentityAdminCommandFixture("user", 0)
		auth.unbanErrors = []error{autherrors.ErrAuthUserNotFound}

		err := commands.Unban(context.Background(), identityAdminActor(t), 42)
		assertIdentityAdminErrorKind(t, err, IdentityAdminAuthIdentityNotFound)
		assertIdentityAdminEvents(t, events, "auth-unban")
	})

	t.Run("unban Auth failure is retryable", func(t *testing.T) {
		commands, _, auth, events := newIdentityAdminCommandFixture("user", 0)
		auth.unbanErrors = []error{errors.New("Auth unavailable"), nil}

		err := commands.Unban(context.Background(), identityAdminActor(t), 42)
		assertIdentityAdminErrorKind(t, err, IdentityAdminAuthFailure)
		if err := commands.Unban(context.Background(), identityAdminActor(t), 42); err != nil {
			t.Fatalf("retry unban after Auth failure: %v", err)
		}
		assertIdentityAdminEvents(t, events, "auth-unban", "auth-unban", "local-status:1")
	})

	t.Run("unban compensates local failure and retry converges", func(t *testing.T) {
		commands, users, _, events := newIdentityAdminCommandFixture("user", 0)
		users.statusErrors = []error{errors.New("wallet unfreeze failed"), nil}

		err := commands.Unban(context.Background(), identityAdminActor(t), 42)
		assertIdentityAdminErrorKind(t, err, IdentityAdminBusinessFailure)
		if err := commands.Unban(context.Background(), identityAdminActor(t), 42); err != nil {
			t.Fatalf("retry unban: %v", err)
		}
		assertIdentityAdminEvents(t, events, "auth-unban", "local-status:1", "auth-ban", "auth-unban", "local-status:1")
	})

	t.Run("unban retains rollback failure", func(t *testing.T) {
		commands, users, auth, _ := newIdentityAdminCommandFixture("user", 0)
		users.statusErrors = []error{errors.New("wallet unfreeze failed")}
		auth.banErrors = []error{errors.New("Auth unavailable")}

		err := commands.Unban(context.Background(), identityAdminActor(t), 42)
		assertIdentityAdminErrorKind(t, err, IdentityAdminReconciliationRequired)
	})
}

func TestIdentityAdminCommandsRevokeSessionsIsRetryable(t *testing.T) {
	t.Run("Auth failure can be retried", func(t *testing.T) {
		commands, _, auth, events := newIdentityAdminCommandFixture("user", 1)
		auth.revokeErrors = []error{errors.New("Auth unavailable"), nil}

		err := commands.RevokeSessions(context.Background(), identityAdminActor(t), 42)
		assertIdentityAdminErrorKind(t, err, IdentityAdminAuthFailure)
		if err := commands.RevokeSessions(context.Background(), identityAdminActor(t), 42); err != nil {
			t.Fatalf("retry revoke sessions: %v", err)
		}
		assertIdentityAdminEvents(t, events, "auth-revoke", "auth-revoke")
	})

	t.Run("Auth not found is typed", func(t *testing.T) {
		commands, _, auth, events := newIdentityAdminCommandFixture("user", 1)
		auth.revokeErrors = []error{autherrors.ErrAuthUserNotFound}

		err := commands.RevokeSessions(context.Background(), identityAdminActor(t), 42)
		assertIdentityAdminErrorKind(t, err, IdentityAdminAuthIdentityNotFound)
		assertIdentityAdminEvents(t, events, "auth-revoke")
	})
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
			err := commands.Ban(context.Background(), tt.actor(t), 42, IdentityAdminBanInput{})
			assertIdentityAdminErrorKind(t, err, tt.kind)
		})
	}
}

func newIdentityAdminCommandFixture(role string, status int16) (IdentityAdminCommands, *fakeIdentityAdminUsers, *fakeIdentityAdminAuth, *[]string) {
	authUserID := "auth-user-42"
	events := &[]string{}
	users := &fakeIdentityAdminUsers{
		target: &User{ID: 42, Username: "target", AuthUserID: &authUserID, Role: role, Status: status},
		events: events,
	}
	auth := &fakeIdentityAdminAuth{events: events}
	return newIdentityAdminCommands(users, auth), users, auth, events
}

func identityAdminActor(t *testing.T) principal.Principal {
	t.Helper()
	actor, err := principal.New(99, "auth-admin-99", "admin", principal.RoleAdmin.String())
	if err != nil {
		t.Fatalf("create admin principal: %v", err)
	}
	return actor
}

func identityUserActor(t *testing.T) principal.Principal {
	t.Helper()
	actor, err := principal.New(98, "auth-user-98", "user", principal.RoleUser.String())
	if err != nil {
		t.Fatalf("create user principal: %v", err)
	}
	return actor
}

func identitySelfAdminActor(t *testing.T) principal.Principal {
	t.Helper()
	actor, err := principal.New(42, "auth-admin-42", "self-admin", principal.RoleAdmin.String())
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
	if !reflect.DeepEqual(*got, want) {
		t.Fatalf("events = %v, want %v", *got, want)
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
