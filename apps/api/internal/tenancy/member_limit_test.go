package tenancy

import (
	"context"
	"errors"
	"sync"
	"testing"
)

func TestWorkspaceMemberLimitIncludesOwner(t *testing.T) {
	ctx := context.Background()
	store := NewInMemoryStoreForTests()
	workspace, err := store.CreateWorkspace(ctx, CreateWorkspaceParams{Name: "capacity", OwnerUserID: 1})
	if err != nil {
		t.Fatal(err)
	}
	for id := int64(2); id <= MaxWorkspaceMembers; id++ {
		if _, err := store.AddWorkspaceMember(ctx, workspace.PublicID, id, RoleMember); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := store.AddWorkspaceMember(ctx, workspace.PublicID, 6, RoleMember); !errors.Is(err, ErrWorkspaceMemberLimitReached) {
		t.Fatalf("sixth member: %v", err)
	}
	existing, err := store.AddWorkspaceMember(ctx, workspace.PublicID, 2, RoleAdmin)
	if err != nil || existing.Role != RoleMember {
		t.Fatalf("existing member = %+v, %v", existing, err)
	}
	if _, err := store.RemoveWorkspaceMember(ctx, workspace.PublicID, 1, 2, testMemberGrants{}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.AddWorkspaceMember(ctx, workspace.PublicID, 6, RoleMember); err != nil {
		t.Fatal(err)
	}
}

func TestWorkspaceConcurrentMemberLimit(t *testing.T) {
	ctx := context.Background()
	store := NewInMemoryStoreForTests()
	workspace, err := store.CreateWorkspace(ctx, CreateWorkspaceParams{Name: "concurrent capacity", OwnerUserID: 1})
	if err != nil {
		t.Fatal(err)
	}
	const candidates = 12
	errs := make([]error, candidates)
	var wg sync.WaitGroup
	for i := range candidates {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, errs[i] = store.AddWorkspaceMember(ctx, workspace.PublicID, int64(i+2), RoleMember)
		}(i)
	}
	wg.Wait()
	added := 0
	for _, err := range errs {
		if err == nil {
			added++
		} else if !errors.Is(err, ErrWorkspaceMemberLimitReached) {
			t.Fatal(err)
		}
	}
	if added != MaxWorkspaceMembers-1 {
		t.Fatalf("added %d, want %d", added, MaxWorkspaceMembers-1)
	}
}
