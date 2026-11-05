package postgres

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"ideaboard/api/internal/db/dbutil"
	"ideaboard/api/internal/tenancy"
)

func TestOrganizationMembersListIncludesEveryMember(t *testing.T) {
	db := newTenancyTestDB(t)
	ctx := context.Background()
	store := NewOrganizationStore(db)
	owner := createTenancyTestUser(t, db)
	org, err := store.CreateOrganization(ctx, owner, "complete-roster")
	if err != nil {
		t.Fatal(err)
	}
	expected := map[int64]string{owner: tenancy.RoleOwner}
	// Put the Admin after the former 500-row boundary: dropping this role
	// would incorrectly hide legitimate quota management from its caller.
	for i := 0; i < 501; i++ {
		userID := createTenancyTestUser(t, db)
		role := tenancy.RoleMember
		if i == 500 {
			role = tenancy.RoleAdmin
		}
		if _, err := store.AddOrganizationMember(ctx, org.PublicID, userID, role); err != nil {
			t.Fatalf("add member %d: %v", i, err)
		}
		expected[userID] = role
	}
	outsider := createTenancyTestUser(t, db)
	if _, err := store.CreateOrganization(ctx, outsider, "other-organization"); err != nil {
		t.Fatal(err)
	}
	members, err := store.ListOrganizationMembers(ctx, org.PublicID)
	if err != nil {
		t.Fatal(err)
	}
	if len(members) != len(expected) {
		t.Fatalf("member count = %d, want %d", len(members), len(expected))
	}
	var previousID int64
	for _, member := range members {
		role, ok := expected[member.UserID]
		if !ok || member.Role != role || member.OrganizationPublicID != org.PublicID {
			t.Fatalf("unexpected organization member: %+v", member)
		}
		if member.UserID <= previousID {
			t.Fatalf("member ids are not strictly ordered: %d after %d", member.UserID, previousID)
		}
		if _, err := tenancy.ParseUserPublicID(member.UserExternalID); err != nil {
			t.Fatalf("member %d external id: %v", member.UserID, err)
		}
		previousID = member.UserID
	}
}

// Exercise the standalone lifecycle interface through one caller transaction:
// reads must see its uncommitted writes, and no nested mutation may escape it.
func TestOrganizationLifecycleUsesCallerTransaction(t *testing.T) {
	db := newTenancyTestDB(t)
	ctx := context.Background()
	owner := createTenancyTestUser(t, db)
	organizations := NewOrganizationStore(db)
	workspaces := NewStore(db, nil)
	billingAccounts := NewBillingAccountStore(db)
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	carrier := dbutil.ContextWithAmbientTx(ctx, tx)
	var member int64
	if err := tx.QueryRow(carrier, `INSERT INTO users (display_name, storage_uid, role, status)
VALUES ('staged member', $1, 'user', 1) RETURNING id`, randomStorageUID(t)).Scan(&member); err != nil {
		t.Fatalf("stage member: %v", err)
	}

	org, err := organizations.CreateOrganization(carrier, owner, "transaction-org")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := organizations.GetOrganization(carrier, org.PublicID); err != nil {
		t.Fatalf("read uncommitted organization: %v", err)
	}
	if role, ok, err := organizations.GetOrganizationRole(carrier, org.PublicID, owner); err != nil || !ok || role != tenancy.RoleOwner {
		t.Fatalf("owner role = %q, %v, %v", role, ok, err)
	}
	if _, err := organizations.AddOrganizationMember(carrier, org.PublicID, member, tenancy.RoleMember); err != nil {
		t.Fatalf("add member: %v", err)
	}
	if members, err := organizations.ListOrganizationMembers(carrier, org.PublicID); err != nil || len(members) != 2 {
		t.Fatalf("members = %d, %v", len(members), err)
	}
	if _, err := organizations.CreateOrganizationWorkspace(carrier, org.PublicID, member, "forbidden"); !errors.Is(err, tenancy.ErrForbidden) {
		t.Fatalf("member workspace creation = %v, want forbidden", err)
	}
	if _, err := organizations.UpdateOrganizationMemberRole(carrier, org.PublicID, owner, member, tenancy.RoleAdmin); err != nil {
		t.Fatalf("promote member: %v", err)
	}
	workspace, err := organizations.CreateOrganizationWorkspace(carrier, org.PublicID, member, "team-space")
	if err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if got, err := workspaces.GetWorkspace(carrier, workspace.PublicID); err != nil || got.OrganizationID == nil || *got.OrganizationID != org.PublicID {
		t.Fatalf("read uncommitted workspace = %v, %v", got, err)
	}
	if got, err := workspaces.ListWorkspacesForUser(carrier, member); err != nil || !slices.ContainsFunc(got, func(w *tenancy.Workspace) bool {
		return w.PublicID == workspace.PublicID
	}) {
		t.Fatalf("list uncommitted workspaces = %v, %v", got, err)
	}
	if role, ok, err := workspaces.GetWorkspaceRole(carrier, workspace.PublicID, member); err != nil || !ok || role != tenancy.RoleAdmin {
		t.Fatalf("workspace role = %q, %v, %v", role, ok, err)
	}
	if got, err := workspaces.ListWorkspaceMembers(carrier, workspace.PublicID); err != nil || len(got) != 1 || got[0].UserID != member || got[0].UserExternalID == "" {
		t.Fatalf("list uncommitted workspace members = %v, %v", got, err)
	}
	account, err := billingAccounts.EnsureBillingAccountForWorkspace(carrier, workspace.InternalID, workspace.PublicID)
	if err != nil {
		t.Fatalf("resolve uncommitted billing account: %v", err)
	}
	if _, err := billingAccounts.GetBillingAccountByInternalID(carrier, account.InternalID); err != nil {
		t.Fatalf("read uncommitted billing account: %v", err)
	}
	if _, err := workspaces.GetWorkspace(ctx, workspace.PublicID); !errors.Is(err, tenancy.ErrNotFound) {
		t.Fatalf("workspace outside caller transaction = %v, want not found", err)
	}
	if _, err := organizations.UpdateOrganization(carrier, org.PublicID, "renamed"); err != nil {
		t.Fatalf("rename: %v", err)
	}
	if err := organizations.RemoveOrganizationMember(carrier, org.PublicID, owner, member); err != nil {
		t.Fatalf("remove member: %v", err)
	}
	if _, err := organizations.CloseOrganization(carrier, org.PublicID, owner, "test"); err != nil {
		t.Fatalf("close: %v", err)
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := organizations.GetOrganization(ctx, org.PublicID); !errors.Is(err, tenancy.ErrNotFound) {
		t.Fatalf("organization after rollback = %v, want not found", err)
	}
	if _, err := NewStore(db, nil).GetWorkspace(ctx, workspace.PublicID); !errors.Is(err, tenancy.ErrNotFound) {
		t.Fatalf("workspace after rollback = %v, want not found", err)
	}
	var accounts int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM billing_accounts WHERE organization_id = $1`, org.InternalID).Scan(&accounts); err != nil || accounts != 0 {
		t.Fatalf("billing accounts after rollback = %d, %v", accounts, err)
	}
}

func TestOrganizationMutationsSerializeWithClosure(t *testing.T) {
	mutations := []struct {
		name string
		run  func(context.Context, *Store, string, int64, int64, int64) error
	}{
		{"rename", func(ctx context.Context, s *Store, org string, _, _, _ int64) error {
			_, err := s.UpdateOrganization(ctx, org, "late rename")
			return err
		}},
		{"add member", func(ctx context.Context, s *Store, org string, _, _, invitee int64) error {
			_, err := s.AddOrganizationMember(ctx, org, invitee, tenancy.RoleMember)
			return err
		}},
		{"update role", func(ctx context.Context, s *Store, org string, owner, member, _ int64) error {
			_, err := s.UpdateOrganizationMemberRole(ctx, org, owner, member, tenancy.RoleAdmin)
			return err
		}},
		{"remove member", func(ctx context.Context, s *Store, org string, owner, member, _ int64) error {
			return s.RemoveOrganizationMember(ctx, org, owner, member)
		}},
		{"create workspace", func(ctx context.Context, s *Store, org string, owner, _, _ int64) error {
			_, err := s.CreateOrganizationWorkspace(ctx, org, owner, "late workspace")
			return err
		}},
		{"generic workspace creation", func(ctx context.Context, s *Store, org string, owner, _, _ int64) error {
			_, err := s.CreateWorkspace(ctx, tenancy.CreateWorkspaceParams{Name: "late workspace", OrganizationID: &org, OwnerUserID: owner})
			return err
		}},
	}
	for _, mutation := range mutations {
		t.Run(mutation.name, func(t *testing.T) {
			db := newTenancyTestDB(t)
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			store, owner, _, member, org, _, _ := closureFixture(t, db, ctx)
			invitee := createTenancyTestUser(t, db)
			tx, err := db.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(context.Background())
			var closerPID int32
			if err := tx.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&closerPID); err != nil {
				t.Fatal(err)
			}
			if _, err := store.CloseOrganization(dbutil.ContextWithAmbientTx(ctx, tx), org.PublicID, owner, "race"); err != nil {
				t.Fatal(err)
			}
			done := make(chan error, 1)
			go func() { done <- mutation.run(ctx, store, org.PublicID, owner, member, invitee) }()

			// Observe a real PostgreSQL lock wait before releasing closure;
			// scheduling alone cannot make this concurrency test pass.
			for {
				var blocked bool
				if err := db.QueryRow(ctx, `SELECT EXISTS (
SELECT 1 FROM pg_stat_activity WHERE $1 = ANY(pg_blocking_pids(pid)))`, closerPID).Scan(&blocked); err != nil {
					t.Fatal(err)
				}
				if blocked {
					break
				}
				select {
				case err := <-done:
					t.Fatalf("mutation did not wait for closure: %v", err)
				case <-ctx.Done():
					t.Fatal(ctx.Err())
				case <-time.After(10 * time.Millisecond):
				}
			}
			if err := tx.Commit(ctx); err != nil {
				t.Fatal(err)
			}
			if err := <-done; !errors.Is(err, tenancy.ErrOrganizationClosed) {
				t.Fatalf("mutation after closure = %v, want ErrOrganizationClosed", err)
			}
		})
	}
}
