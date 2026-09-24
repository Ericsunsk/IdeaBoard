package postgres

import (
	"context"
	"testing"
)

// Temporary R4 reproduction: live insert via the ensure path.
func TestTempR4EnsureBillingAccountRepro(t *testing.T) {
	db := newTenancyTestDB(t)
	store := NewStore(db)
	userID := createTenancyTestUser(t, db)
	ctx := context.Background()

	workspace, err := store.EnsurePersonalWorkspace(ctx, userID)
	if err != nil {
		t.Fatalf("ensure personal workspace failed: %v", err)
	}
	t.Logf("workspace internal=%d public=%s", workspace.InternalID, workspace.PublicID)

	baStore := NewBillingAccountStore(db)
	account, err := baStore.EnsureBillingAccountForWorkspace(ctx, workspace.InternalID, workspace.PublicID)
	if err != nil {
		t.Fatalf("R4 REPRODUCED: EnsureBillingAccountForWorkspace failed: %v", err)
	}
	t.Logf("R4 OK: billing account created: %+v", account)
}
