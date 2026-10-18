package tenancy

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/google/uuid"
)

func TestUserPublicIDTranslatorRoundTrip(t *testing.T) {
	id := uuid.MustParse("550e8400-e29b-41d4-a716-446655440000")
	external := FormatUserPublicID(id)
	if external != "usr_550e8400e29b41d4a716446655440000" {
		t.Fatalf("external = %q", external)
	}
	parsed, err := ParseUserPublicID(external)
	if err != nil || parsed != id {
		t.Fatalf("parsed = %v, %v", parsed, err)
	}
	for _, bad := range []string{"", "usr_", "org_abc", "usr_not hex!!", "task_550e8400e29b41d4a716446655440000"} {
		if _, err := ParseUserPublicID(bad); err == nil {
			t.Fatalf("malformed id %q accepted", bad)
		}
	}
}

func TestPublicIDMintingShape(t *testing.T) {
	ws, err := NewWorkspacePublicID()
	if err != nil || len(ws) < 5 {
		t.Fatalf("workspace id = %q, %v", ws, err)
	}
	org, err := NewOrganizationPublicID()
	if err != nil || len(org) < 5 {
		t.Fatalf("organization id = %q, %v", org, err)
	}
	seen := map[string]struct{}{ws: {}}
	for i := 0; i < 64; i++ {
		next, err := NewWorkspacePublicID()
		if err != nil {
			t.Fatal(err)
		}
		if _, dup := seen[next]; dup {
			t.Fatal("minted duplicate public id")
		}
		seen[next] = struct{}{}
	}
}

func TestCheckMemberMutationMatrix(t *testing.T) {
	const (
		owner  = RoleOwner
		admin  = RoleAdmin
		member = RoleMember
	)
	cases := []struct {
		name          string
		actorRole     string
		actor, target int64
		current, next string
		owners        int
		remove        bool
		wantErr       error
	}{
		{name: "owner promotes member to admin", actorRole: owner, actor: 1, target: 3, current: member, next: admin, owners: 1},
		{name: "owner promotes member to owner", actorRole: owner, actor: 1, target: 3, current: member, next: owner, owners: 1},
		{name: "admin promotes member to admin forbidden", actorRole: admin, actor: 2, target: 3, current: member, next: admin, owners: 1, wantErr: ErrForbidden},
		{name: "admin demotes member to member noop", actorRole: admin, actor: 2, target: 3, current: member, next: member, owners: 1},
		{name: "member promotes forbidden", actorRole: member, actor: 3, target: 4, current: member, next: admin, owners: 1, wantErr: ErrForbidden},
		{name: "outsider promotes forbidden", actorRole: "", actor: 9, target: 3, current: member, next: admin, owners: 1, wantErr: ErrForbidden},
		{name: "last owner demote blocked", actorRole: owner, actor: 1, target: 1, current: owner, next: admin, owners: 1, wantErr: ErrLastOwner},
		{name: "second owner demote allowed", actorRole: owner, actor: 1, target: 5, current: owner, next: admin, owners: 2},
		{name: "last owner self-remove blocked", actorRole: owner, actor: 1, target: 1, current: owner, owners: 1, remove: true, wantErr: ErrLastOwner},
		{name: "member self-remove allowed", actorRole: member, actor: 3, target: 3, current: member, owners: 1, remove: true},
		{name: "admin removes member", actorRole: admin, actor: 2, target: 3, current: member, owners: 1, remove: true},
		{name: "admin removes owner forbidden", actorRole: admin, actor: 2, target: 1, current: owner, owners: 2, remove: true, wantErr: ErrForbidden},
		{name: "owner removes admin", actorRole: owner, actor: 1, target: 2, current: admin, owners: 1, remove: true},
		{name: "outsider removes forbidden", actorRole: "", actor: 9, target: 3, current: member, owners: 1, remove: true, wantErr: ErrForbidden},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := CheckMemberMutation(tc.actorRole, tc.actor, tc.target, tc.current, tc.next, tc.owners, tc.remove)
			if tc.wantErr == nil && err != nil {
				t.Fatalf("err = %v, want nil", err)
			}
			if tc.wantErr != nil && !errors.Is(err, tc.wantErr) {
				t.Fatalf("err = %v, want %v", err, tc.wantErr)
			}
		})
	}
}

func TestInMemoryLastOwnerGuards(t *testing.T) {
	ctx := context.Background()
	store := NewInMemoryStoreForTests()
	ws, err := store.CreateWorkspace(ctx, CreateWorkspaceParams{Name: "w", OwnerUserID: 1})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpdateWorkspaceMemberRole(ctx, ws.PublicID, 1, 1, RoleAdmin); !errors.Is(err, ErrLastOwner) {
		t.Fatalf("demote last owner err = %v, want ErrLastOwner", err)
	}
	if _, err := store.RemoveWorkspaceMember(ctx, ws.PublicID, 1, 1, testMemberGrants{}); !errors.Is(err, ErrLastOwner) {
		t.Fatalf("remove last owner err = %v, want ErrLastOwner", err)
	}
}

func TestEnsureDefaultWorkspaceConcurrent(t *testing.T) {
	ctx := context.Background()
	store := NewInMemoryStoreForTests()
	const callers = 32
	results := make([]*Workspace, callers)
	errs := make([]error, callers)
	var wg sync.WaitGroup
	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func(slot int) {
			defer wg.Done()
			ws, err := store.EnsureDefaultWorkspace(ctx, 42)
			results[slot], errs[slot] = ws, err
		}(i)
	}
	wg.Wait()
	for i := range errs {
		if errs[i] != nil {
			t.Fatalf("provision err = %v", errs[i])
		}
	}
	for i := 1; i < callers; i++ {
		if results[i].PublicID != results[0].PublicID {
			t.Fatalf("concurrent provision diverged: %q vs %q", results[i].PublicID, results[0].PublicID)
		}
	}
	if !results[0].IsDefaultWorkspace() {
		t.Fatalf("provisioned workspace is not Default: %+v", results[0])
	}
	visible, err := store.ListWorkspacesForUser(ctx, 42)
	if err != nil || len(visible) != 1 {
		t.Fatalf("visible = %v, %v; want exactly one workspace", visible, err)
	}
	if visible[0].OrganizationID != nil {
		t.Fatal("default workspace must carry NULL organization_id")
	}
}

func TestEnsureDefaultWorkspaceSkipsArchived(t *testing.T) {
	ctx := context.Background()
	store := NewInMemoryStoreForTests()
	first, err := store.EnsureDefaultWorkspace(ctx, 7)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.ArchiveWorkspace(ctx, first.PublicID); err == nil {
		t.Fatal("archiving the Default workspace must be rejected")
	} else if !errors.Is(err, ErrDefaultWorkspaceProtected) {
		t.Fatalf("archive default err = %v, want ErrDefaultWorkspaceProtected", err)
	}
	// An archived non-Default row never satisfies Ensure: the live Default
	// is still returned.
	plain, err := store.CreateWorkspace(ctx, CreateWorkspaceParams{Name: "plain", OwnerUserID: 7})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.ArchiveWorkspace(ctx, plain.PublicID); err != nil {
		t.Fatal(err)
	}
	second, err := store.EnsureDefaultWorkspace(ctx, 7)
	if err != nil || second.PublicID != first.PublicID {
		t.Fatalf("live Default must be reused: %v, %v", second, err)
	}
}

// TestCreateMultipleNonDefaultWorkspaces proves plain POST creation without
// an organization is unlimited (Bug2 root fix): the legacy single-personal
// cap is gone.
func TestCreateMultipleNonDefaultWorkspaces(t *testing.T) {
	ctx := context.Background()
	store := NewInMemoryStoreForTests()
	if _, err := store.EnsureDefaultWorkspace(ctx, 9); err != nil {
		t.Fatal(err)
	}
	seen := map[string]struct{}{}
	for i := 0; i < 3; i++ {
		ws, err := store.CreateWorkspace(ctx, CreateWorkspaceParams{Name: fmt.Sprintf("plain-%d", i), OwnerUserID: 9})
		if err != nil {
			t.Fatalf("plain create %d failed: %v", i, err)
		}
		if ws.IsDefaultWorkspace() {
			t.Fatalf("plain create %d must not be Default: %+v", i, ws)
		}
		seen[ws.PublicID] = struct{}{}
	}
	visible, err := store.ListWorkspacesForUser(ctx, 9)
	if err != nil {
		t.Fatal(err)
	}
	if len(visible) != 4 {
		t.Fatalf("visible = %d, want Default + 3 plain rows", len(visible))
	}
}

// TestCreateSecondDefaultRejected proves the single-live-Default invariant:
// an explicit second Default create fails instead of forking.
func TestCreateSecondDefaultRejected(t *testing.T) {
	ctx := context.Background()
	store := NewInMemoryStoreForTests()
	if _, err := store.EnsureDefaultWorkspace(ctx, 11); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateWorkspace(ctx, CreateWorkspaceParams{Name: "dup", OwnerUserID: 11, IsDefault: true}); !errors.Is(err, ErrDefaultWorkspaceConflict) {
		t.Fatalf("second Default err = %v, want ErrDefaultWorkspaceConflict", err)
	}
	if _, err := store.CreateWorkspace(ctx, CreateWorkspaceParams{Name: "org-default", OrganizationID: ptr("org_x"), OwnerUserID: 11, IsDefault: true}); !errors.Is(err, ErrDefaultWorkspaceConflict) {
		t.Fatalf("org Default err = %v, want ErrDefaultWorkspaceConflict", err)
	}
}

// TestEnsureDefaultBackfillsWhenMissing proves a user holding only
// non-Default rows still gets a Default row on ensure.
func TestEnsureDefaultBackfillsWhenMissing(t *testing.T) {
	ctx := context.Background()
	store := NewInMemoryStoreForTests()
	if _, err := store.CreateWorkspace(ctx, CreateWorkspaceParams{Name: "plain-a", OwnerUserID: 13}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateWorkspace(ctx, CreateWorkspaceParams{Name: "plain-b", OwnerUserID: 13}); err != nil {
		t.Fatal(err)
	}
	def, err := store.EnsureDefaultWorkspace(ctx, 13)
	if err != nil {
		t.Fatal(err)
	}
	if !def.IsDefaultWorkspace() {
		t.Fatalf("ensured workspace is not Default: %+v", def)
	}
	visible, err := store.ListWorkspacesForUser(ctx, 13)
	if err != nil {
		t.Fatal(err)
	}
	if len(visible) != 3 {
		t.Fatalf("visible = %d, want 2 plain + 1 Default", len(visible))
	}
}

func ptr(s string) *string { return &s }

func TestCreateWorkspaceNameValidation(t *testing.T) {
	ctx := context.Background()
	store := NewInMemoryStoreForTests()
	if _, err := store.CreateWorkspace(ctx, CreateWorkspaceParams{Name: "  ", OwnerUserID: 1}); err == nil {
		t.Fatal("blank name must be rejected")
	}
	if err := ValidateName(fmt.Sprintf("%0120d", 0)[:120]); err != nil {
		t.Fatalf("120-char name must validate: %v", err)
	}
}

// TestUpdateWorkspaceDefaultProtected proves the Default workspace cannot
// be renamed (403 workspace_default_protected at the edge) while plain rows
// rename normally.
func TestUpdateWorkspaceDefaultProtected(t *testing.T) {
	ctx := context.Background()
	store := NewInMemoryStoreForTests()
	def, err := store.EnsureDefaultWorkspace(ctx, 15)
	if err != nil {
		t.Fatal(err)
	}
	plain, err := store.CreateWorkspace(ctx, CreateWorkspaceParams{Name: "plain", OwnerUserID: 15})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpdateWorkspace(ctx, def.PublicID, "Renamed Default"); !errors.Is(err, ErrDefaultWorkspaceProtected) {
		t.Fatalf("rename default err = %v, want ErrDefaultWorkspaceProtected", err)
	}
	renamed, err := store.UpdateWorkspace(ctx, plain.PublicID, "Renamed Plain")
	if err != nil || renamed.Name != "Renamed Plain" {
		t.Fatalf("rename plain = %+v, %v; want renamed", renamed, err)
	}
}
