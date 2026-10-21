package v1

import (
	"context"
	"testing"

	"ideaboard/api/internal/tenancy"
	tenancypg "ideaboard/api/internal/tenancy/infrastructure/postgres"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Existing membership/media fixtures configure a real account policy
// explicitly; the unconfigured-default gate remains covered separately.
func configureV1WorkspaceDefaultQuota(t *testing.T, db *pgxpool.Pool, workspace *tenancy.Workspace) {
	t.Helper()
	ctx := context.Background()
	account, err := tenancypg.NewBillingAccountStore(db).EnsureBillingAccountForWorkspace(ctx, workspace.InternalID, workspace.PublicID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `INSERT INTO billing_account_default_member_quotas (billing_account_id, period) VALUES ($1, 'unlimited')`, account.InternalID); err != nil {
		t.Fatal(err)
	}
}
