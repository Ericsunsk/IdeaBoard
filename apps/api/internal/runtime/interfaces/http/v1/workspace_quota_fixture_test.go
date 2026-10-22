package v1

import (
	"context"
	"testing"

	"ideaboard/api/internal/tenancy"
	tenancypg "ideaboard/api/internal/tenancy/infrastructure/postgres"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Quota fixtures no longer seed an account default (#953 retires the #947
// default gate): invites always succeed and absent quotas read as unlimited.
func configureV1WorkspaceDefaultQuota(t *testing.T, db *pgxpool.Pool, workspace *tenancy.Workspace) {
	t.Helper()
	ctx := context.Background()
	if _, err := tenancypg.NewBillingAccountStore(db).EnsureBillingAccountForWorkspace(ctx, workspace.InternalID, workspace.PublicID); err != nil {
		t.Fatal(err)
	}
}
