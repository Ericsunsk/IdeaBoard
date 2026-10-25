package v1

import (
	"context"
	"errors"
	"fmt"
	"ideaboard/api/internal/identity/authidentity"
	"ideaboard/api/internal/identity/principal"
	"ideaboard/api/internal/identity/users/domain"
	identityports "ideaboard/api/internal/identity/users/ports"
	"ideaboard/api/internal/platform/contract"
	runtimehttp "ideaboard/api/internal/runtime/interfaces/http"
	"ideaboard/api/internal/tenancy"
	tenancypg "ideaboard/api/internal/tenancy/infrastructure/postgres"
	"ideaboard/api/internal/testdb"
	"ideaboard/api/internal/testsupport"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// ---- merged from (base members_display_test.go) ----
// fakeMemberDisplay is a deterministic V1MemberDisplayProvider for tests:
// known numeric ids resolve to the configured facts, unknown ids are
// omitted, and failNext forces a provider error to prove the handlers
// fail closed.
type fakeMemberDisplay struct {
	facts    map[int64]V1MemberDisplay
	failNext error
}

func (f *fakeMemberDisplay) GetMemberDisplay(_ context.Context, userIDs []int64) (map[int64]V1MemberDisplay, error) {
	if f.failNext != nil {
		err := f.failNext
		f.failNext = nil
		return nil, err
	}
	out := make(map[int64]V1MemberDisplay, len(userIDs))
	for _, id := range userIDs {
		if fact, ok := f.facts[id]; ok {
			out[id] = fact
		}
	}
	return out, nil
}

func newV1MembersDisplayEngine(store tenancy.Store, dir *staticUserDirectory, display V1MemberDisplayProvider) *gin.Engine {
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
	RegisterContractV1Routes(v1, runtimehttp.AuthDeps{}, ContractV1Deps{Store: store, Users: dir, MemberDisplay: display})
	return router
}

func memberListFacts(t *testing.T, payload map[string]any) map[string]V1MemberDisplay {
	t.Helper()
	items, ok := v1Data(t, payload).([]any)
	if !ok {
		t.Fatalf("member list data is not an array: %v", payload)
	}
	out := make(map[string]V1MemberDisplay, len(items))
	for _, raw := range items {
		item, ok := raw.(map[string]any)
		if !ok {
			t.Fatalf("member item is not an object: %v", raw)
		}
		userID, _ := item["user_id"].(string)
		if userID == "" {
			t.Fatalf("member item misses user_id: %v", item)
		}
		email, _ := item["email"].(string)
		avatar, _ := item["avatar"].(string)
		out[userID] = V1MemberDisplay{Email: email, Avatar: avatar}
	}
	return out
}

func TestV1MemberListDisplayInMemory(t *testing.T) {
	ctx := context.Background()
	store := tenancy.NewInMemoryStoreForTests()
	dir := newStaticUserDirectory()
	const owner, member, outsider, stranger = 1, 2, 3, 4
	ws := v1OrganizationWorkspaceFixture(t, ctx, store, "member-display", owner, member, outsider, stranger)
	store.SeedWorkspaceMember(ws.PublicID, member, tenancy.RoleMember)
	store.SeedWorkspaceMember(ws.PublicID, stranger, tenancy.RoleMember)
	for _, id := range []int64{owner, member, outsider, stranger} {
		dir.ensure(id)
	}
	usr := func(userID int64) string { return tenancy.FormatUserPublicID(dir.ensure(userID)) }
	display := &fakeMemberDisplay{facts: map[int64]V1MemberDisplay{
		owner:  {Email: "owner@example.com", Avatar: "/avatars/owner.png"},
		member: {Email: "member@example.com", Avatar: "/avatars/member.png"},
	}}
	engine := newV1MembersDisplayEngine(store, dir, display)
	membersPath := "/api/v1/workspaces/" + ws.PublicID + "/members"

	t.Run("member with facts carries them, member without omits them", func(t *testing.T) {
		status, _, payload := doV1(t, engine, v1Call{method: http.MethodGet, path: membersPath, userID: owner})
		if status != http.StatusOK {
			t.Fatalf("list = %d %v, want 200", status, payload)
		}
		got := memberListFacts(t, payload)
		if got[usr(owner)].Email != "owner@example.com" || got[usr(owner)].Avatar != "/avatars/owner.png" {
			t.Fatalf("owner facts = %+v, want email+avatar (%v)", got[usr(owner)], payload)
		}
		if got[usr(member)].Email != "member@example.com" || got[usr(member)].Avatar != "/avatars/member.png" {
			t.Fatalf("member facts = %+v, want email+avatar (%v)", got[usr(member)], payload)
		}
		if got[usr(stranger)].Email != "" || got[usr(stranger)].Avatar != "" {
			t.Fatalf("member without facts must omit the fields, got %+v (%v)", got[usr(stranger)], payload)
		}
		// The raw JSON must omit the keys (omitempty), not send empty strings.
		rawItems := v1Data(t, payload).([]any)
		for _, raw := range rawItems {
			item := raw.(map[string]any)
			if item["user_id"] == usr(stranger) {
				if _, ok := item["email"]; ok {
					t.Fatalf("member without email must not carry the key: %v", item)
				}
				if _, ok := item["avatar"]; ok {
					t.Fatalf("member without avatar must not carry the key: %v", item)
				}
			}
		}
	})

	t.Run("provider error fails closed to no facts with 200", func(t *testing.T) {
		display.failNext = errors.New("auth down")
		status, _, payload := doV1(t, engine, v1Call{method: http.MethodGet, path: membersPath, userID: member})
		if status != http.StatusOK {
			t.Fatalf("list with failing display = %d %v, want 200", status, payload)
		}
		got := memberListFacts(t, payload)
		if got[usr(owner)].Email != "" || got[usr(owner)].Avatar != "" || got[usr(member)].Email != "" {
			t.Fatalf("failing provider must omit all facts: %v", payload)
		}
	})

	t.Run("nil provider fails closed to no facts with 200", func(t *testing.T) {
		plain := newV1TestEngine(store, dir)
		status, _, payload := doV1(t, plain, v1Call{method: http.MethodGet, path: membersPath, userID: owner})
		if status != http.StatusOK {
			t.Fatalf("list without provider = %d %v, want 200", status, payload)
		}
		got := memberListFacts(t, payload)
		if got[usr(owner)].Email != "" || got[usr(owner)].Avatar != "" {
			t.Fatalf("nil provider must omit facts: %v", payload)
		}
	})

	t.Run("non-member stays invisible without fact leak", func(t *testing.T) {
		status, _, payload := doV1(t, engine, v1Call{method: http.MethodGet, path: membersPath, userID: outsider})
		if status != http.StatusNotFound || v1ErrorCode(t, payload) != string(contract.CodeWorkspaceNotFound) {
			t.Fatalf("outsider list = %d %v, want 404 workspace_not_found", status, payload)
		}
		if strings.Contains(fmt.Sprintf("%v", payload), "owner@example.com") {
			t.Fatalf("outsider response must not leak facts: %v", payload)
		}
	})

	t.Run("member writes share the list display path and unknown users stay 404", func(t *testing.T) {
		// Add carries the same facts the list would (single id through
		// the same provider and map-key assembly).
		status, _, payload := doV1(t, engine, v1Call{
			method: http.MethodPost, path: membersPath, userID: owner,
			body:    fmt.Sprintf(`{"data":{"user_id":%q,"role":"member"}}`, usr(member)),
			headers: map[string]string{contract.IdempotencyKeyHeader: "member-display-add"},
		})
		if status != http.StatusCreated && status != http.StatusOK {
			// Re-adding an existing member keeps the role with 201; either
			// way the shape must carry the shared display facts.
			t.Fatalf("add = %d %v", status, payload)
		}
		if data, ok := v1Data(t, payload).(map[string]any); ok {
			if data["email"] != "member@example.com" || data["avatar"] != "/avatars/member.png" {
				t.Fatalf("add must carry the shared display facts: %v", data)
			}
		} else {
			t.Fatalf("add data is not an object: %v", payload)
		}
		// Update resolves the same facts through the same path.
		status, _, payload = doV1(t, engine, v1Call{
			method: http.MethodPatch, path: membersPath + "/" + usr(member), userID: owner,
			body: `{"data":{"role":"admin"}}`,
		})
		if status != http.StatusOK {
			t.Fatalf("update = %d %v, want 200", status, payload)
		}
		if data, ok := v1Data(t, payload).(map[string]any); ok {
			if data["email"] != "member@example.com" || data["avatar"] != "/avatars/member.png" {
				t.Fatalf("update must carry the shared display facts: %v", data)
			}
		} else {
			t.Fatalf("update data is not an object: %v", payload)
		}
		// Unknown users stay invisible (404), never 403, on writes.
		randomUnknown := tenancy.FormatUserPublicID(uuid.New())
		status, _, payload = doV1(t, engine, v1Call{
			method: http.MethodPost, path: membersPath, userID: owner,
			body:    fmt.Sprintf(`{"data":{"user_id":%q,"role":"member"}}`, randomUnknown),
			headers: map[string]string{contract.IdempotencyKeyHeader: "member-display-unknown"},
		})
		if status != http.StatusNotFound || v1ErrorCode(t, payload) != string(contract.CodeWorkspaceMemberNotFound) {
			t.Fatalf("unknown add = %d %v, want 404 member not found", status, payload)
		}
	})
}

func TestV1MemberListDisplayPG(t *testing.T) {
	db := testdb.Open(t)
	ctx := context.Background()
	tstore := tenancypg.NewStore(db, nil)
	dir := newStaticUserDirectory()
	const owner, member, outsider = 1, 10, 8
	for _, id := range []int64{owner, member, outsider} {
		dir.ensure(id)
	}
	ws, err := tstore.CreateWorkspace(ctx, tenancy.CreateWorkspaceParams{Name: "member-display-pg", OwnerUserID: owner})
	if err != nil {
		t.Fatal(err)
	}
	configureV1WorkspaceDefaultQuota(t, db, ws)
	if _, err := tstore.AddWorkspaceMember(ctx, ws.PublicID, member, tenancy.RoleMember); err != nil {
		t.Fatal(err)
	}
	usr := func(userID int64) string { return tenancy.FormatUserPublicID(dir.ensure(userID)) }
	display := &fakeMemberDisplay{facts: map[int64]V1MemberDisplay{
		owner: {Email: "pg-owner@example.com", Avatar: "/avatars/pg-owner.png"},
	}}
	engine := newV1MembersDisplayEngine(tstore, dir, display)
	membersPath := "/api/v1/workspaces/" + ws.PublicID + "/members"

	t.Run("pg member with facts is listed, member without omits them", func(t *testing.T) {
		status, _, payload := doV1(t, engine, v1Call{method: http.MethodGet, path: membersPath, userID: owner})
		if status != http.StatusOK {
			t.Fatalf("pg list = %d %v, want 200", status, payload)
		}
		got := memberListFacts(t, payload)
		if got[usr(owner)].Email != "pg-owner@example.com" || got[usr(owner)].Avatar != "/avatars/pg-owner.png" {
			t.Fatalf("pg owner facts = %+v (%v)", got[usr(owner)], payload)
		}
		if got[usr(member)].Email != "" || got[usr(member)].Avatar != "" {
			t.Fatalf("pg member without facts must omit them, got %+v (%v)", got[usr(member)], payload)
		}
	})

	t.Run("pg provider error fails closed", func(t *testing.T) {
		display.failNext = errors.New("auth down")
		status, _, payload := doV1(t, engine, v1Call{method: http.MethodGet, path: membersPath, userID: member})
		if status != http.StatusOK {
			t.Fatalf("pg list with failing display = %d %v, want 200", status, payload)
		}
	})

	t.Run("pg non-member stays invisible", func(t *testing.T) {
		status, _, payload := doV1(t, engine, v1Call{method: http.MethodGet, path: membersPath, userID: outsider})
		if status != http.StatusNotFound || v1ErrorCode(t, payload) != string(contract.CodeWorkspaceNotFound) {
			t.Fatalf("pg outsider list = %d %v, want 404 workspace_not_found", status, payload)
		}
	})
}

// ---- merged from members_display_service_test.go ----
type stubUserRepo struct {
	users map[int64]*domain.User
	err   error
}

func (s *stubUserRepo) Create(_ context.Context, _ *domain.User) error { return nil }
func (s *stubUserRepo) GetByID(_ context.Context, id int64) (*domain.User, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.users[id], nil
}
func (s *stubUserRepo) GetByIDs(_ context.Context, ids []int64) (map[int64]*domain.User, error) {
	if s.err != nil {
		return nil, s.err
	}
	out := make(map[int64]*domain.User, len(ids))
	for _, id := range ids {
		if user, ok := s.users[id]; ok {
			out[id] = user
		}
	}
	return out, nil
}
func (s *stubUserRepo) GetByPublicID(_ context.Context, _ string) (*domain.User, error) {
	return nil, errors.New("not implemented")
}
func (s *stubUserRepo) GetByAuthUserID(_ context.Context, _ string) (*domain.User, error) {
	return nil, errors.New("not implemented")
}
func (s *stubUserRepo) GetByStorageUID(_ context.Context, _ string) (*domain.User, error) {
	return nil, errors.New("not implemented")
}
func (s *stubUserRepo) Update(_ context.Context, _ *domain.User) error { return nil }
func (s *stubUserRepo) UpdateLastLoginMetadata(_ context.Context, _, _, _ string) error {
	return nil
}
func (s *stubUserRepo) List(_ context.Context, _, _ string, _ int16, _ bool, _ int, _ int) ([]*domain.User, identityports.UserListCounts, error) {
	return nil, identityports.UserListCounts{}, errors.New("not implemented")
}
func (s *stubUserRepo) UpdateStatus(_ context.Context, _ int64, _ int16) error           { return nil }
func (s *stubUserRepo) UpdateRoleByID(_ context.Context, _ int64, _ string) error        { return nil }
func (s *stubUserRepo) UpdateAvatarByID(_ context.Context, _ int64, _ string) error      { return nil }
func (s *stubUserRepo) UpdateDisplayNameByID(_ context.Context, _ int64, _ string) error { return nil }
func (s *stubUserRepo) EnsureStorageUIDs(_ context.Context) error                        { return nil }

type stubEmailReader struct {
	identities map[string]authidentity.UserIdentity
	err        error
	calls      [][]string
}

func (s *stubEmailReader) GetUserEmails(_ context.Context, ids []string) (map[string]authidentity.UserIdentity, error) {
	s.calls = append(s.calls, append([]string(nil), ids...))
	if s.err != nil {
		return nil, s.err
	}
	out := make(map[string]authidentity.UserIdentity, len(ids))
	for _, id := range ids {
		if identity, ok := s.identities[id]; ok {
			out[id] = identity
		}
	}
	return out, nil
}

func strPtr(value string) *string { return &value }

func TestMemberDisplayServiceJoinsRepoToAuthEmailsAndAvatars(t *testing.T) {
	repo := &stubUserRepo{users: map[int64]*domain.User{
		1: {ID: 1, PublicID: uuid.New(), AuthUserID: strPtr("auth-1"), StorageUID: "AVTR01", Avatar: "av1.png"},
		2: {ID: 2, PublicID: uuid.New(), AuthUserID: strPtr("auth-2"), StorageUID: "AVTR02", Avatar: "https://cdn.example/a.png"},
		3: {ID: 3, PublicID: uuid.New()},
	}}
	reader := &stubEmailReader{identities: map[string]authidentity.UserIdentity{
		"auth-1": {Email: "  Alice@Example.com  ", Providers: []string{"email"}},
	}}
	svc := NewMemberDisplayService(repo, reader)
	got, err := svc.GetMemberDisplay(context.Background(), []int64{1, 2, 3, 1, -5, 0})
	if err != nil {
		t.Fatalf("GetMemberDisplay = %v, want nil", err)
	}
	if got[1].Email != "Alice@Example.com" {
		t.Fatalf("user 1 email = %q, want trimmed Alice@Example.com", got[1].Email)
	}
	if want := runtimehttp.V1PublicAvatarURLForRow("AVTR01", "av1.png"); got[1].Avatar != want {
		t.Fatalf("user 1 avatar = %q, want %q", got[1].Avatar, want)
	}
	if _, ok := got[2]; !ok || got[2].Avatar != "https://cdn.example/a.png" {
		t.Fatalf("user 2 external avatar must pass through: %v", got)
	}
	if _, ok := got[2]; ok && got[2].Email != "" {
		t.Fatalf("user 2 without email identity must omit email: %v", got)
	}
	if _, ok := got[3]; ok {
		t.Fatalf("user 3 without AuthUserID or avatar must be omitted: %v", got)
	}
	if len(got) != 2 {
		t.Fatalf("display = %v, want exactly 2 entries", got)
	}
	if len(reader.calls) != 1 || len(reader.calls[0]) != 2 {
		t.Fatalf("email reader calls = %v, want one deduped batch of 2 auth ids", reader.calls)
	}
}

func TestMemberDisplayServiceFailClosed(t *testing.T) {
	// Nil sides resolve to empty without error.
	svc := NewMemberDisplayService(nil, nil)
	got, err := svc.GetMemberDisplay(context.Background(), []int64{1})
	if err != nil || len(got) != 0 {
		t.Fatalf("nil service = %v, %v; want empty, nil", got, err)
	}
	got, err = svc.GetMemberDisplay(context.Background(), nil)
	if err != nil || len(got) != 0 {
		t.Fatalf("empty ids = %v, %v; want empty, nil", got, err)
	}
	// A nil email reader still projects avatars (business-owned, no hop).
	repo := &stubUserRepo{users: map[int64]*domain.User{
		1: {ID: 1, PublicID: uuid.New(), AuthUserID: strPtr("auth-1"), StorageUID: "AVTR01", Avatar: "av1.png"},
	}}
	svc = NewMemberDisplayService(repo, nil)
	got, err = svc.GetMemberDisplay(context.Background(), []int64{1})
	if err != nil {
		t.Fatalf("nil reader = %v, want nil", err)
	}
	if want := runtimehttp.V1PublicAvatarURLForRow("AVTR01", "av1.png"); got[1].Avatar != want || got[1].Email != "" {
		t.Fatalf("nil reader display = %v, want avatar %q with no email", got, want)
	}
	// Repo and reader errors propagate to the caller (the HTTP layer then
	// fails closed to no facts); they must never fabricate addresses.
	repoErr := &stubUserRepo{err: errors.New("db down")}
	svc = NewMemberDisplayService(repoErr, &stubEmailReader{})
	if _, err := svc.GetMemberDisplay(context.Background(), []int64{1}); err == nil {
		t.Fatal("repo error must propagate")
	}
	svc = NewMemberDisplayService(repo, &stubEmailReader{err: errors.New("auth down")})
	if _, err := svc.GetMemberDisplay(context.Background(), []int64{1}); err == nil {
		t.Fatal("email reader error must propagate")
	}
}
