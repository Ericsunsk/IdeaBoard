package invitations

import (
	"context"
	"errors"
	"testing"

	"ideaboard/api/internal/notifications"
	"ideaboard/api/internal/tenancy"
	tenancyevents "ideaboard/api/internal/tenancy/events"
)

type testIdentityReader struct {
	emails     map[int64]string
	users      map[string]int64
	emailErr   error
	findErr    error
	beforeRead func(context.Context)
}

func (r *testIdentityReader) Email(ctx context.Context, userID int64) (string, error) {
	if r.beforeRead != nil {
		r.beforeRead(ctx)
	}
	return r.emails[userID], r.emailErr
}

func (r *testIdentityReader) FindUser(ctx context.Context, email string) (int64, error) {
	if r.beforeRead != nil {
		r.beforeRead(ctx)
	}
	return r.users[email], r.findErr
}

type identityTransaction struct{}

type identityFixture struct {
	service *Service
	reader  *testIdentityReader
	store   *tenancy.InMemoryStore
	scope   tenancy.InviteScope
	scopeID string
}

func newIdentityFixture(t *testing.T, scope tenancy.InviteScope) identityFixture {
	t.Helper()
	store := tenancy.NewInMemoryStoreForTests()
	reader := &testIdentityReader{
		emails: map[int64]string{2: "member@example.com"},
		users:  map[string]int64{"member@example.com": 2},
		beforeRead: func(ctx context.Context) {
			if ctx.Value(identityTransaction{}) != nil {
				t.Fatal("remote identity lookup held the invitation transaction")
			}
		},
	}
	f := identityFixture{store: store, scope: scope, reader: reader}
	var err error
	if scope == tenancy.InviteScopeOrganization {
		var org *tenancy.Organization
		org, err = store.CreateOrganization(t.Context(), 1, "Recipient test")
		if org != nil {
			f.scopeID = org.PublicID
		}
	} else {
		var workspace *tenancy.Workspace
		workspace, err = store.CreateWorkspace(t.Context(), tenancy.CreateWorkspaceParams{Name: "Recipient test", OwnerUserID: 1})
		if workspace != nil {
			f.scopeID = workspace.PublicID
		}
	}
	if err != nil {
		t.Fatal(err)
	}
	f.service = &Service{
		Store: store, Identities: reader,
		Events: tenancyevents.NewCollaborationEventDispatcher(notifications.NewMemoryStore(), testMailSender{}),
		Transaction: func(ctx context.Context, fn func(context.Context) error) error {
			return fn(context.WithValue(ctx, identityTransaction{}, true))
		},
	}
	return f
}

func (f identityFixture) create(t *testing.T) (ref, token string, bound *int64) {
	t.Helper()
	if f.scope == tenancy.InviteScopeOrganization {
		invite, token, err := f.service.CreateOrganizationMemberInvite(t.Context(), f.scopeID, 1, "member@example.com", tenancy.RoleMember)
		if err != nil {
			t.Fatal(err)
		}
		return invite.PublicID, token, invite.InviteeUserID
	}
	invite, token, err := f.service.CreateWorkspaceMemberInvite(t.Context(), f.scopeID, 1, "member@example.com", tenancy.RoleMember)
	if err != nil {
		t.Fatal(err)
	}
	return invite.PublicID, token, invite.InviteeUserID
}

func (f identityFixture) decide(ctx context.Context, ref string, actorID int64, decision string) error {
	if f.scope == tenancy.InviteScopeOrganization {
		if decision == "accept" {
			_, _, err := f.service.AcceptOrganizationInvite(ctx, ref, actorID)
			return err
		}
		_, err := f.service.DeclineOrganizationInvite(ctx, ref, actorID)
		return err
	}
	if decision == "accept" {
		_, _, err := f.service.AcceptWorkspaceInvite(ctx, ref, actorID)
		return err
	}
	_, err := f.service.DeclineWorkspaceInvite(ctx, ref, actorID)
	return err
}

func TestInvitationRecipientIdentity(t *testing.T) {
	unavailable := errors.New("Auth unavailable")
	for _, scope := range []tenancy.InviteScope{tenancy.InviteScopeWorkspace, tenancy.InviteScopeOrganization} {
		for _, decision := range []string{"accept", "decline"} {
			for _, tc := range []struct {
				name       string
				bound      bool
				email      string
				emailErr   error
				actorID    int64
				wantDenied bool
			}{
				{name: "verified email without user binding", email: " MEMBER@EXAMPLE.COM ", actorID: 2},
				{name: "bound identity during Auth outage", bound: true, emailErr: unavailable, actorID: 2},
				{name: "unbound identity during Auth outage", emailErr: unavailable, actorID: 2, wantDenied: true},
				{name: "trusted email mismatch despite matching binding", bound: true, email: "other@example.com", actorID: 2, wantDenied: true},
				{name: "binding mismatch despite matching email", bound: true, email: "member@example.com", actorID: 3, wantDenied: true},
			} {
				t.Run(string(scope)+"/"+decision+"/"+tc.name, func(t *testing.T) {
					f := newIdentityFixture(t, scope)
					if !tc.bound {
						f.reader.users = nil
					}
					ref, token, binding := f.create(t)
					if (binding != nil) != tc.bound {
						t.Fatalf("wrong recipient binding: %v", binding)
					}
					f.reader.emails = map[int64]string{tc.actorID: tc.email}
					f.reader.emailErr = tc.emailErr
					// Workspace exercises public token redemption as well as
					// in-app references; Organization remains in-app only.
					if scope == tenancy.InviteScopeWorkspace && decision == "accept" {
						ref = token
					}
					err := f.decide(t.Context(), ref, tc.actorID, decision)
					if tc.wantDenied {
						if !errors.Is(err, tenancy.ErrNotFound) {
							t.Fatalf("untrusted recipient was not hidden: %v", err)
						}
					} else if err != nil {
						t.Fatalf("trusted recipient rejected: %v", err)
					}
				})
			}
		}
	}
}

func TestInvitationRecipientLookupAndPendingLists(t *testing.T) {
	for _, scope := range []tenancy.InviteScope{tenancy.InviteScopeWorkspace, tenancy.InviteScopeOrganization} {
		for _, tc := range []struct {
			name     string
			known    bool
			findErr  error
			emailErr error
		}{
			{name: "registered user", known: true},
			{name: "unknown user"},
			{name: "lookup unavailable", findErr: errors.New("Auth unavailable")},
			{name: "bound list during Auth outage", known: true, emailErr: errors.New("Auth unavailable")},
			{name: "unbound list during Auth outage", emailErr: errors.New("Auth unavailable")},
		} {
			t.Run(string(scope)+"/"+tc.name, func(t *testing.T) {
				f := newIdentityFixture(t, scope)
				if !tc.known {
					f.reader.users = nil
				}
				f.reader.findErr = tc.findErr
				ref, _, binding := f.create(t)
				if tc.known && (binding == nil || *binding != 2) || !tc.known && binding != nil {
					t.Fatalf("incorrect target resolution: %v", binding)
				}
				f.reader.emailErr = tc.emailErr
				wantCount := 1
				if tc.emailErr != nil && !tc.known {
					wantCount = 0
				}
				// Lists use trusted email or the stored user binding. An Auth
				// outage cannot expose email-addressed invitations to anyone.
				if scope == tenancy.InviteScopeOrganization {
					items, err := f.service.ListPendingOrganizationInvitesForUser(t.Context(), 2)
					if err != nil || len(items) != wantCount || wantCount > 0 && items[0].PublicID != ref {
						t.Fatalf("pending organization invitations: %+v %v", items, err)
					}
				} else {
					items, err := f.service.ListPendingWorkspaceInvitesForUser(t.Context(), 2)
					if err != nil || len(items) != wantCount || wantCount > 0 && items[0].PublicID != ref {
						t.Fatalf("pending workspace invitations: %+v %v", items, err)
					}
				}
			})
		}
	}
}

func TestInvitationUnknownReferenceDoesNotEnterDecisionTransaction(t *testing.T) {
	for _, scope := range []tenancy.InviteScope{tenancy.InviteScopeWorkspace, tenancy.InviteScopeOrganization} {
		t.Run(string(scope), func(t *testing.T) {
			f := newIdentityFixture(t, scope)
			f.service.Transaction = func(context.Context, func(context.Context) error) error {
				t.Fatal("unknown invitation entered a decision transaction")
				return nil
			}
			ref := tenancy.WorkspaceInvitePrefix + "missing"
			if scope == tenancy.InviteScopeOrganization {
				ref = tenancy.OrganizationInvitePrefix + "missing"
			}
			if err := f.decide(t.Context(), ref, 2, "accept"); !errors.Is(err, tenancy.ErrNotFound) {
				t.Fatalf("unknown invite not hidden: %v", err)
			}
		})
	}
}
