package postgres

import (
	"context"
	"errors"
	"testing"
	"time"

	"ideaboard/api/internal/boards"
	"ideaboard/api/internal/media/workspacemedia"
	tasks "ideaboard/api/internal/tasking/domain"
	"ideaboard/api/internal/tenancy"
	"ideaboard/api/internal/tenancy/funds"
	tenancypg "ideaboard/api/internal/tenancy/infrastructure/postgres"
)

func TestOrganizationBillingResolutionDoesNotDeadlockWorkspaceDeletion(t *testing.T) {
	db := newTaskTestDB(t)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	_, ownerID := seedTaskWorkspace(t, db)
	workspaces := tenancypg.NewStore(db, nil)
	organization, err := workspaces.CreateOrganization(ctx, ownerID, "billing-lock-order")
	if err != nil {
		t.Fatal(err)
	}
	workspace, err := workspaces.CreateOrganizationWorkspace(ctx, organization.PublicID, ownerID, "task-space")
	if err != nil {
		t.Fatal(err)
	}

	submit, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer submit.Rollback(ctx)
	// Use the production submission gate: it holds the Workspace membership
	// lock until the settlement transaction completes.
	authorizer := workspacemedia.TaskSubmissionAuthorizer{Boards: boards.NewPostgresWorkspaceBoardStore(db, nil)}
	if _, err := authorizer.AuthorizeSubmission(ctx, submit, tasks.CreateInput{
		WorkspaceID: workspace.PublicID, OwnerUserID: ownerID,
	}); err != nil {
		t.Fatalf("authorize submission: %v", err)
	}
	var submitPID int32
	if err := submit.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&submitPID); err != nil {
		t.Fatal(err)
	}

	deleted := make(chan error, 1)
	go func() { deleted <- workspaces.DeleteWorkspace(ctx, workspace.PublicID) }()
	for {
		var waiting bool
		if err := db.QueryRow(ctx, `SELECT EXISTS (
  SELECT 1 FROM pg_stat_activity
  WHERE $1 = ANY(pg_blocking_pids(pid))
    AND query LIKE '%FROM workspaces WHERE public_id = $1 FOR UPDATE%'
)`, submitPID).Scan(&waiting); err != nil {
			t.Fatalf("observe deletion waiting for submission: %v", err)
		}
		if waiting {
			break
		}
		select {
		case err := <-deleted:
			t.Fatalf("deletion finished before the submission released its Workspace lock: %v", err)
		case <-ctx.Done():
			t.Fatalf("deletion did not reach its Workspace lock: %v", ctx.Err())
		case <-time.After(10 * time.Millisecond):
		}
	}
	// DeleteWorkspace now holds the Billing Account lock and waits on our
	// Workspace. Resolving the already-provisioned payer must stay read-only,
	// not try to acquire that account lock in the opposite order.
	accountID, ownerType, resolveErr := NewStore(db).ResolveBillingAccountInTx(ctx, submit, workspace.PublicID, ownerID)
	if err := submit.Rollback(ctx); err != nil {
		t.Fatalf("finish submission transaction: %v", err)
	}
	deleteErr := <-deleted
	if resolveErr != nil || accountID <= 0 || ownerType != funds.OwnerTypeOrganization {
		t.Fatalf("resolve payer while deletion waits = %d, %v; want a stable account without reverse locking", accountID, resolveErr)
	}
	if deleteErr != nil {
		t.Fatalf("delete after submission transaction ended: %v", deleteErr)
	}
	if _, err := workspaces.GetWorkspace(ctx, workspace.PublicID); !errors.Is(err, tenancy.ErrNotFound) {
		t.Fatalf("workspace after deletion = %v, want not found", err)
	}
}
