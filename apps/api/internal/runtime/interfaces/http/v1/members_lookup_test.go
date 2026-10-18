package v1

import (
	"context"
	"errors"
	"fmt"
	runtimehttp "ideaboard/api/internal/runtime/interfaces/http"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"ideaboard/api/internal/identity/autherrors"
	"ideaboard/api/internal/identity/principal"
	identitydomain "ideaboard/api/internal/identity/users/domain"
	identityports "ideaboard/api/internal/identity/users/ports"
	"ideaboard/api/internal/platform/contract"
	"ideaboard/api/internal/tenancy"
	tenancypg "ideaboard/api/internal/tenancy/infrastructure/postgres"
	"ideaboard/api/internal/testdb"
	"ideaboard/api/internal/testsupport"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// fakeMemberLookup is a deterministic V1MemberLookupProvider for tests:
// normalized emails resolve to the configured public ID, unknown emails
// converge to tenancy.ErrNotFound, and errNext forces a transport error.
type fakeMemberLookup struct {
	ids     map[string]uuid.UUID
	errNext error
}

func (f *fakeMemberLookup) LookupUserPublicID(_ context.Context, normalizedEmail string) (uuid.UUID, error) {
	if f.errNext != nil {
		err := f.errNext
		f.errNext = nil
		return uuid.Nil, err
	}
	if id, ok := f.ids[strings.ToLower(strings.TrimSpace(normalizedEmail))]; ok && id != uuid.Nil {
		return id, nil
	}
	return uuid.Nil, tenancy.ErrNotFound
}

func newV1MembersLookupEngine(store tenancy.Store, dir *staticUserDirectory, lookup V1MemberLookupProvider) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(runtimehttp.ContractRequestIDMiddleware())
	router.Use(func(c *gin.Context) {
		raw := strings.TrimSpace(c.GetHeader(testsupport.HeaderTestUser))
		if raw == "" {
			c.Next()
			return
		}
		userID, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || userID <= 0 {
			c.AbortWithStatus(http.StatusBadRequest)
			return
		}
		role := strings.TrimSpace(c.GetHeader(testsupport.HeaderTestRole))
		if role == "" {
			role = "user"
		}
		id := dir.ensure(userID)
		identityPrincipal, err := principal.New(userID, id,
			fmt.Sprintf("auth-%d", userID), fmt.Sprintf("test-user-%d", userID), role)
		if err != nil || principal.Set(c, identityPrincipal) != nil {
			c.AbortWithStatus(http.StatusInternalServerError)
			return
		}
		c.Next()
	})
	v1 := router.Group(runtimehttp.ContractV1GroupPath)
	RegisterContractV1Routes(v1, runtimehttp.AuthDeps{}, ContractV1Deps{Store: store, Users: dir, MemberLookup: lookup})
	return router
}

func lookupPathFor(wsPublicID, email string) string {
	return "/api/v1/workspaces/" + wsPublicID + "/members/lookup?email=" + url.QueryEscape(email)
}

func TestV1MemberLookupByEmailInMemory(t *testing.T) {
	ctx := context.Background()
	store := tenancy.NewInMemoryStoreForTests()
	dir := newStaticUserDirectory()
	const owner, admin, member, outsider = 1, 2, 3, 4
	const invitee, otherWSUser = 5, 6
	ws := v1WorkspaceFixture(t, ctx, store, "member-lookup", owner)
	store.SeedWorkspaceMember(ws.PublicID, admin, tenancy.RoleAdmin)
	store.SeedWorkspaceMember(ws.PublicID, member, tenancy.RoleMember)
	for _, id := range []int64{owner, admin, member, outsider, invitee, otherWSUser} {
		dir.ensure(id)
	}
	usr := func(userID int64) string { return tenancy.FormatUserPublicID(dir.ensure(userID)) }
	lookup := &fakeMemberLookup{ids: map[string]uuid.UUID{
		"invitee@example.com":  dir.ensure(invitee),
		"other-ws@example.com": dir.ensure(otherWSUser),
		"teammate@example.com": dir.ensure(member),
	}}
	engine := newV1MembersLookupEngine(store, dir, lookup)
	lookupPath := func(email string, userID int64) v1Call {
		return v1Call{method: http.MethodGet, path: lookupPathFor(ws.PublicID, email), userID: userID}
	}

	t.Run("exact match returns usr_", func(t *testing.T) {
		status, _, payload := doV1(t, engine, lookupPath("invitee@example.com", owner))
		if status != http.StatusOK {
			t.Fatalf("lookup = %d %v, want 200", status, payload)
		}
		if got := v1Data(t, payload).(map[string]any)["user_id"]; got != usr(invitee) {
			t.Fatalf("user_id = %v, want %s (%v)", got, usr(invitee), payload)
		}
	})

	t.Run("matching is case-insensitive and trimmed", func(t *testing.T) {
		for _, email := range []string{"INVITEE@EXAMPLE.COM", "  Invitee@Example.Com  ", "other-ws@EXAMPLE.com"} {
			status, _, payload := doV1(t, engine, lookupPath(email, admin))
			if status != http.StatusOK {
				t.Fatalf("lookup %q = %d %v, want 200", email, status, payload)
			}
		}
		if got := v1Data(t, doV1Payload(t, engine, lookupPath("  INVITEE@example.com ", owner))).(map[string]any)["user_id"]; got != usr(invitee) {
			t.Fatalf("trimmed lookup user_id = %v, want %s", got, usr(invitee))
		}
	})

	t.Run("unknown email converges to 404 member not found", func(t *testing.T) {
		status, _, payload := doV1(t, engine, lookupPath("ghost@example.com", owner))
		if status != http.StatusNotFound || v1ErrorCode(t, payload) != string(contract.CodeWorkspaceMemberNotFound) {
			t.Fatalf("unknown lookup = %d %v, want 404 workspace_member_not_found", status, payload)
		}
	})

	t.Run("invalid email renders 400 without lookup", func(t *testing.T) {
		for _, email := range []string{"", "   ", "not-an-email", "a@b", "a@.com"} {
			status, _, payload := doV1(t, engine, lookupPath(email, owner))
			if status != http.StatusBadRequest || v1ErrorCode(t, payload) != string(contract.CodeBadRequest) {
				t.Fatalf("invalid lookup %q = %d %v, want 400 bad_request", email, status, payload)
			}
		}
	})

	t.Run("member without capability gets 403, outsider stays invisible", func(t *testing.T) {
		status, _, payload := doV1(t, engine, lookupPath("invitee@example.com", member))
		if status != http.StatusForbidden || v1ErrorCode(t, payload) != string(contract.CodeWorkspaceForbidden) {
			t.Fatalf("member lookup = %d %v, want 403 workspace_forbidden", status, payload)
		}
		status, _, payload = doV1(t, engine, lookupPath("invitee@example.com", outsider))
		if status != http.StatusNotFound || v1ErrorCode(t, payload) != string(contract.CodeWorkspaceNotFound) {
			t.Fatalf("outsider lookup = %d %v, want 404 workspace_not_found", status, payload)
		}
		status, _, payload = doV1(t, engine, v1Call{method: http.MethodGet, path: lookupPathFor(ws.PublicID, "invitee@example.com")})
		if status != http.StatusUnauthorized || v1ErrorCode(t, payload) != string(contract.CodeUnauthorized) {
			t.Fatalf("anon lookup = %d %v, want 401 unauthorized", status, payload)
		}
	})

	t.Run("provider outage renders 503 without email leak", func(t *testing.T) {
		lookup.errNext = errors.New("auth down")
		status, _, payload := doV1(t, engine, lookupPath("invitee@example.com", owner))
		if status != http.StatusServiceUnavailable {
			t.Fatalf("outage lookup = %d %v, want 503", status, payload)
		}
	})

	t.Run("cross-workspace user resolves but invite keeps the original gate", func(t *testing.T) {
		status, _, payload := doV1(t, engine, lookupPath("other-ws@example.com", owner))
		if status != http.StatusOK {
			t.Fatalf("cross-ws lookup = %d %v, want 200", status, payload)
		}
		if got := v1Data(t, payload).(map[string]any)["user_id"]; got != usr(otherWSUser) {
			t.Fatalf("cross-ws user_id = %v, want %s", got, usr(otherWSUser))
		}
		membersPath := "/api/v1/workspaces/" + ws.PublicID + "/members"
		status, _, payload = doV1(t, engine, v1Call{
			method: http.MethodPost, path: membersPath, userID: owner,
			body:    fmt.Sprintf(`{"data":{"user_id":%q,"role":"member"}}`, usr(otherWSUser)),
			headers: map[string]string{contract.IdempotencyKeyHeader: "lookup-invite-other-ws"},
		})
		if status != http.StatusCreated {
			t.Fatalf("owner invite after lookup = %d %v, want 201", status, payload)
		}
		status, _, payload = doV1(t, engine, v1Call{
			method: http.MethodPost, path: membersPath, userID: member,
			body:    fmt.Sprintf(`{"data":{"user_id":%q,"role":"member"}}`, usr(invitee)),
			headers: map[string]string{contract.IdempotencyKeyHeader: "lookup-invite-by-member"},
		})
		if status != http.StatusForbidden {
			t.Fatalf("member invite = %d %v, want 403 (POST gate unchanged)", status, payload)
		}
	})
}

func doV1Payload(t *testing.T, engine *gin.Engine, call v1Call) map[string]any {
	t.Helper()
	_, _, payload := doV1(t, engine, call)
	return payload
}

func TestV1MemberLookupNilProviderFailsClosed(t *testing.T) {
	ctx := context.Background()
	store := tenancy.NewInMemoryStoreForTests()
	dir := newStaticUserDirectory()
	const owner = 1
	dir.ensure(owner)
	ws := v1WorkspaceFixture(t, ctx, store, "member-lookup-nil", owner)
	plain := newV1TestEngine(store, dir)
	status, _, payload := doV1(t, plain, v1Call{
		method: http.MethodGet, path: lookupPathFor(ws.PublicID, "invitee@example.com"), userID: owner,
	})
	if status != http.StatusNotImplemented || v1ErrorCode(t, payload) != string(contract.CodeContractNotImplemented) {
		t.Fatalf("nil provider lookup = %d %v, want 501 contract_not_implemented", status, payload)
	}
}

func TestV1MemberLookupByEmailPG(t *testing.T) {
	db := testdb.Open(t)
	ctx := context.Background()
	tstore := tenancypg.NewStore(db)
	dir := newStaticUserDirectory()
	// Numeric ids must exist in the users table (workspace_memberships has
	// a FK): testdb seeds 1, 3, 8, 10, 11 among others.
	const owner, admin, member, outsider = 1, 11, 3, 8
	const invitee = 10
	for _, id := range []int64{owner, admin, member, outsider, invitee} {
		dir.ensure(id)
	}
	ws, err := tstore.CreateWorkspace(ctx, tenancy.CreateWorkspaceParams{Name: "member-lookup-pg", OwnerUserID: owner})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tstore.AddWorkspaceMember(ctx, ws.PublicID, admin, tenancy.RoleAdmin); err != nil {
		t.Fatal(err)
	}
	if _, err := tstore.AddWorkspaceMember(ctx, ws.PublicID, member, tenancy.RoleMember); err != nil {
		t.Fatal(err)
	}
	usr := func(userID int64) string { return tenancy.FormatUserPublicID(dir.ensure(userID)) }
	lookup := &fakeMemberLookup{ids: map[string]uuid.UUID{"pg-invitee@example.com": dir.ensure(invitee)}}
	engine := newV1MembersLookupEngine(tstore, dir, lookup)

	t.Run("pg exact, case-insensitive and trimmed matches", func(t *testing.T) {
		for _, email := range []string{"pg-invitee@example.com", "PG-INVITEE@EXAMPLE.COM", "  pg-invitee@example.com "} {
			status, _, payload := doV1(t, engine, v1Call{
				method: http.MethodGet, path: lookupPathFor(ws.PublicID, email), userID: owner,
			})
			if status != http.StatusOK {
				t.Fatalf("pg lookup %q = %d %v, want 200", email, status, payload)
			}
			if got := v1Data(t, payload).(map[string]any)["user_id"]; got != usr(invitee) {
				t.Fatalf("pg lookup %q user_id = %v, want %s", email, got, usr(invitee))
			}
		}
	})

	t.Run("pg unknown email converges to 404", func(t *testing.T) {
		status, _, payload := doV1(t, engine, v1Call{
			method: http.MethodGet, path: lookupPathFor(ws.PublicID, "ghost@example.com"), userID: admin,
		})
		if status != http.StatusNotFound || v1ErrorCode(t, payload) != string(contract.CodeWorkspaceMemberNotFound) {
			t.Fatalf("pg unknown lookup = %d %v, want 404 workspace_member_not_found", status, payload)
		}
	})

	t.Run("pg member gets 403 and outsider stays invisible", func(t *testing.T) {
		status, _, payload := doV1(t, engine, v1Call{
			method: http.MethodGet, path: lookupPathFor(ws.PublicID, "pg-invitee@example.com"), userID: member,
		})
		if status != http.StatusForbidden || v1ErrorCode(t, payload) != string(contract.CodeWorkspaceForbidden) {
			t.Fatalf("pg member lookup = %d %v, want 403 workspace_forbidden", status, payload)
		}
		status, _, payload = doV1(t, engine, v1Call{
			method: http.MethodGet, path: lookupPathFor(ws.PublicID, "pg-invitee@example.com"), userID: outsider,
		})
		if status != http.StatusNotFound || v1ErrorCode(t, payload) != string(contract.CodeWorkspaceNotFound) {
			t.Fatalf("pg outsider lookup = %d %v, want 404 workspace_not_found", status, payload)
		}
	})
}

// stubLookupAuthReader is a deterministic V1MemberAuthLookupReader:
// known emails resolve to the configured Auth id, unknown emails report
// autherrors.ErrAuthUserNotFound, and err forces a transport failure.
type stubLookupAuthReader struct {
	ids map[string]string
	err error
}

func (s *stubLookupAuthReader) LookupUserByEmail(_ context.Context, email string) (string, error) {
	if s.err != nil {
		return "", s.err
	}
	if id, ok := s.ids[strings.ToLower(strings.TrimSpace(email))]; ok {
		return id, nil
	}
	return "", autherrors.ErrAuthUserNotFound
}

// stubLookupUserRepo adapts a static auth-id map to the users repository
// seam the provider consumes (GetByAuthUserID only).
type stubLookupUserRepo struct {
	identityports.UserRepository
	byAuth map[string]*identitydomain.User
}

func (s *stubLookupUserRepo) GetByAuthUserID(_ context.Context, authUserID string) (*identitydomain.User, error) {
	if user, ok := s.byAuth[strings.TrimSpace(authUserID)]; ok && user != nil {
		return user, nil
	}
	return nil, errors.New("sql: no rows in result set")
}

func TestMemberLookupServiceJoinsAuthToBusinessIdentity(t *testing.T) {
	publicID := uuid.New()
	authID := "auth-invitee"
	repo := &stubLookupUserRepo{byAuth: map[string]*identitydomain.User{
		authID: {ID: 7, PublicID: publicID, AuthUserID: &authID},
	}}
	reader := &stubLookupAuthReader{ids: map[string]string{"invitee@example.com": authID}}
	svc := NewMemberLookupService(repo, reader)

	got, err := svc.LookupUserPublicID(context.Background(), "  Invitee@Example.COM ")
	if err != nil || got != publicID {
		t.Fatalf("LookupUserPublicID = %v, %v; want %v, nil", got, err, publicID)
	}
}

func TestMemberLookupServiceFailClosed(t *testing.T) {
	publicID := uuid.New()
	authID := "auth-invitee"
	repo := &stubLookupUserRepo{byAuth: map[string]*identitydomain.User{
		authID: {ID: 7, PublicID: publicID, AuthUserID: &authID},
	}}

	t.Run("unknown auth identity converges to not found", func(t *testing.T) {
		svc := NewMemberLookupService(repo, &stubLookupAuthReader{})
		if _, err := svc.LookupUserPublicID(context.Background(), "ghost@example.com"); !errors.Is(err, tenancy.ErrNotFound) {
			t.Fatalf("unknown lookup err = %v, want tenancy.ErrNotFound", err)
		}
	})

	t.Run("missing business row converges to not found", func(t *testing.T) {
		svc := NewMemberLookupService(repo, &stubLookupAuthReader{ids: map[string]string{
			"provision-gap@example.com": "auth-never-provisioned",
		}})
		if _, err := svc.LookupUserPublicID(context.Background(), "provision-gap@example.com"); !errors.Is(err, tenancy.ErrNotFound) {
			t.Fatalf("provision-gap lookup err = %v, want tenancy.ErrNotFound", err)
		}
	})

	t.Run("transport failures surface for 503", func(t *testing.T) {
		boom := errors.New("auth down")
		svc := NewMemberLookupService(repo, &stubLookupAuthReader{err: boom})
		if _, err := svc.LookupUserPublicID(context.Background(), "invitee@example.com"); !errors.Is(err, boom) {
			t.Fatalf("outage lookup err = %v, want the transport error", err)
		}
	})

	t.Run("nil seams and invalid input converge to not found", func(t *testing.T) {
		if _, err := NewMemberLookupService(nil, nil).LookupUserPublicID(context.Background(), "invitee@example.com"); !errors.Is(err, tenancy.ErrNotFound) {
			t.Fatalf("nil-seam lookup err = %v, want tenancy.ErrNotFound", err)
		}
		svc := NewMemberLookupService(repo, &stubLookupAuthReader{})
		if _, err := svc.LookupUserPublicID(context.Background(), "not-an-email"); !errors.Is(err, tenancy.ErrNotFound) {
			t.Fatalf("invalid-email lookup err = %v, want tenancy.ErrNotFound", err)
		}
	})
}
