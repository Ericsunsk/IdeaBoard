package postgres

import (
	"context"
	"testing"
)

// Live-insert coverage for billing-account provisioning: the ensure path
// must mint and persist a valid public_id (ba_ CHECK), not just read.
func TestEnsureBillingAccountForWorkspace(t *testing.T) {
	db := newTenancyTestDB(t)
	store := NewStore(db)
	userID := createTenancyTestUser(t, db)
	ctx := context.Background()

	workspace, err := store.EnsureDefaultWorkspace(ctx, userID)
	if err != nil {
		t.Fatalf("ensure default workspace failed: %v", err)
	}
	t.Logf("workspace internal=%d public=%s", workspace.InternalID, workspace.PublicID)

	baStore := NewBillingAccountStore(db)
	account, err := baStore.EnsureBillingAccountForWorkspace(ctx, workspace.InternalID, workspace.PublicID)
	if err != nil {
		t.Fatalf("EnsureBillingAccountForWorkspace failed: %v", err)
	}
	t.Logf("billing account created: %+v", account)
}
