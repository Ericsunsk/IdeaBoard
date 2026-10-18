package postgres

import (
	"context"
	"errors"
	"sync"
	"testing"

	"ideaboard/api/internal/db/dbutil"
	"ideaboard/api/internal/tenancy"
)

func TestWorkspaceOwnerMutationsKeepOneOwnerPostgres(t *testing.T) {
	for _, mutation := range []string{"demote", "remove"} {
		t.Run(mutation, func(t *testing.T) {
			db := newTenancyTestDB(t)
			store := NewStore(db)
			ctx := context.Background()
			owners := []int64{createTenancyTestUser(t, db), createTenancyTestUser(t, db)}
			workspace, err := store.CreateWorkspace(ctx, tenancy.CreateWorkspaceParams{Name: "owners", OwnerUserID: owners[0]})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := store.AddWorkspaceMember(ctx, workspace.PublicID, owners[1], tenancy.RoleOwner); err != nil {
				t.Fatal(err)
			}
			start := make(chan struct{})
			errs := make([]error, len(owners))
			var wg sync.WaitGroup
			for i, ownerID := range owners {
				wg.Add(1)
				go func(i int, ownerID int64) {
					defer wg.Done()
					<-start
					if mutation == "remove" {
						_, errs[i] = store.RemoveWorkspaceMember(ctx, workspace.PublicID, ownerID, ownerID, testMemberGrants{})
					} else {
						_, errs[i] = store.UpdateWorkspaceMemberRole(ctx, workspace.PublicID, ownerID, ownerID, tenancy.RoleMember)
					}
				}(i, ownerID)
			}
			close(start)
			wg.Wait()
			succeeded := 0
			for _, err := range errs {
				if err == nil {
					succeeded++
				} else if !errors.Is(err, tenancy.ErrLastOwner) {
					t.Fatalf("mutation failed: %v", err)
				}
			}
			if succeeded != 1 {
				t.Fatalf("successful owner mutations = %d, want 1", succeeded)
			}
			members, err := store.ListWorkspaceMembers(ctx, workspace.PublicID)
			if err != nil {
				t.Fatal(err)
			}
			remainingOwners := 0
			for _, member := range members {
				if member.Role == tenancy.RoleOwner {
					remainingOwners++
				}
			}
			if remainingOwners != 1 {
				t.Fatalf("remaining owners = %d, want 1", remainingOwners)
			}
		})
	}
}

func TestWorkspaceMemberMutationsRollBackWithAmbientTransaction(t *testing.T) {
	for _, mutation := range []string{"role", "remove"} {
		t.Run(mutation, func(t *testing.T) {
			db := newTenancyTestDB(t)
			store := NewStore(db)
			ctx := context.Background()
			ownerID, memberID := createTenancyTestUser(t, db), createTenancyTestUser(t, db)
			workspace, err := store.CreateWorkspace(ctx, tenancy.CreateWorkspaceParams{Name: "rollback", OwnerUserID: ownerID})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := store.AddWorkspaceMember(ctx, workspace.PublicID, memberID, tenancy.RoleMember); err != nil {
				t.Fatal(err)
			}
			tx, err := db.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(ctx)
			carrier := dbutil.ContextWithAmbientTx(ctx, tx)
			if mutation == "remove" {
				_, err = store.RemoveWorkspaceMember(carrier, workspace.PublicID, ownerID, memberID, testMemberGrants{})
			} else {
				_, err = store.UpdateWorkspaceMemberRole(carrier, workspace.PublicID, ownerID, memberID, tenancy.RoleAdmin)
			}
			if err != nil {
				t.Fatal(err)
			}
			if err := tx.Rollback(ctx); err != nil {
				t.Fatal(err)
			}
			role, exists, err := store.GetWorkspaceRole(ctx, workspace.PublicID, memberID)
			if err != nil || !exists || role != tenancy.RoleMember {
				t.Fatalf("rolled-back member = %q, %v, %v", role, exists, err)
			}
		})
	}
}
