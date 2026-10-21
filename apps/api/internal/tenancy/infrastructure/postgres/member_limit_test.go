package postgres

import (
	"context"
	"errors"
	"sync"
	"testing"

	"ideaboard/api/internal/tenancy"
)

func TestWorkspaceMemberLimitConcurrentPostgres(t *testing.T) {
	db := newTenancyTestDB(t)
	store := NewStore(db, nil)
	ctx := context.Background()
	ownerID := createTenancyTestUser(t, db)
	workspace, err := store.CreateWorkspace(ctx, tenancy.CreateWorkspaceParams{Name: "member limit", OwnerUserID: ownerID})
	if err != nil {
		t.Fatal(err)
	}
	configureWorkspaceDefaultQuota(t, db, workspace)
	const candidates = 12
	userIDs := make([]int64, candidates)
	for i := range userIDs {
		userIDs[i] = createTenancyTestUser(t, db)
	}
	errs := make([]error, candidates)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i, userID := range userIDs {
		wg.Add(1)
		go func(i int, userID int64) {
			defer wg.Done()
			<-start
			_, errs[i] = store.AddWorkspaceMember(ctx, workspace.PublicID, userID, tenancy.RoleMember)
		}(i, userID)
	}
	close(start)
	wg.Wait()
	added := 0
	var addedID int64
	for i, err := range errs {
		if err == nil {
			added++
			addedID = userIDs[i]
		} else if !errors.Is(err, tenancy.ErrWorkspaceMemberLimitReached) {
			t.Fatal(err)
		}
	}
	if added != tenancy.MaxWorkspaceMembers-1 {
		t.Fatalf("added %d, want %d", added, tenancy.MaxWorkspaceMembers-1)
	}
	members, err := store.ListWorkspaceMembers(ctx, workspace.PublicID)
	if err != nil {
		t.Fatal(err)
	}
	if len(members) != tenancy.MaxWorkspaceMembers {
		t.Fatalf("members = %d", len(members))
	}
	existing, err := store.AddWorkspaceMember(ctx, workspace.PublicID, addedID, tenancy.RoleAdmin)
	if err != nil || existing.Role != tenancy.RoleMember {
		t.Fatalf("existing member = %+v, %v", existing, err)
	}
	if _, err := store.RemoveWorkspaceMember(ctx, workspace.PublicID, ownerID, addedID, testMemberGrants{}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.AddWorkspaceMember(ctx, workspace.PublicID, addedID, tenancy.RoleMember); err != nil {
		t.Fatal(err)
	}
}
