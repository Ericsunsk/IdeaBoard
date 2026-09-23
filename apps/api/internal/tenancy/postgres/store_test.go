package postgres

import (
	"context"
	"crypto/rand"
	"sync"
	"testing"

	"ideaboard/api/internal/db/dbutil"
	dbsqlc "ideaboard/api/internal/db/sqlc"
	"ideaboard/api/internal/platform/database"
	"ideaboard/api/internal/tenancy"
	"ideaboard/api/internal/testdb"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TestUserPublicIDTranslatorPinsOpaqueForm pins the DB stream's translator
// contract: users.public_id stays UUID in storage, usr_<uuid_hex> is the
// only opaque external form. The users column type must not change here.
func TestUserPublicIDTranslatorPinsOpaqueForm(t *testing.T) {
	id := uuid.MustParse("123e4567-e89b-12d3-a456-426614174000")
	external := tenancy.FormatUserPublicID(id)
	if external != "usr_123e4567e89b12d3a456426614174000" {
		t.Fatalf("FormatUserPublicID = %q, want usr_<32 hex>", external)
	}
	parsed, err := tenancy.ParseUserPublicID(external)
	if err != nil {
		t.Fatalf("ParseUserPublicID failed: %v", err)
	}
	if parsed != id {
		t.Fatalf("ParseUserPublicID = %s, want %s", parsed, id)
	}
	for _, bad := range []string{"", "usr_", "org_123e4567e89b12d3a456426614174000", "usr_nothex!!", "usr_123e4567e89b12d3a45642661417400zz"} {
		if _, err := tenancy.ParseUserPublicID(bad); err == nil {
			t.Fatalf("ParseUserPublicID(%q) succeeded, want error", bad)
		}
	}
}

func newTenancyTestDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	return testdb.Open(t)
}

func createTenancyTestUser(t *testing.T, db *pgxpool.Pool) int64 {
	t.Helper()
	storageUID := randomStorageUID(t)
	var id int64
	err := db.QueryRow(
		context.Background(),
		`INSERT INTO users (display_name, storage_uid, role, status)
		 VALUES ($1, $2, 'user', 1) RETURNING id`,
		"tenancy-test-user", storageUID,
	).Scan(&id)
	if err != nil {
		t.Fatalf("seed tenancy test user failed: %v", err)
	}
	return id
}

func randomStorageUID(t *testing.T) string {
	t.Helper()
	const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	buf := make([]byte, 6)
	if _, err := rand.Read(buf); err != nil {
		t.Fatalf("random storage uid failed: %v", err)
	}
	for i := range buf {
		buf[i] = alphabet[int(buf[i])%len(alphabet)]
	}
	return string(buf)
}

// TestEnsurePersonalWorkspaceConcurrent proves personal provisioning is
// idempotent under concurrency: N callers converge to one row with one
// public_id, backed by ux_workspaces_personal_owner.
func TestEnsurePersonalWorkspaceConcurrent(t *testing.T) {
	db := newTenancyTestDB(t)
	store := NewStore(db)
	userID := createTenancyTestUser(t, db)
	ctx := context.Background()

	const callers = 16
	publicIDs := make([]string, callers)
	errs := make([]error, callers)
	var wg sync.WaitGroup
	for i := range publicIDs {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			workspace, err := store.EnsurePersonalWorkspace(ctx, userID)
			if err != nil {
				errs[index] = err
				return
			}
			publicIDs[index] = workspace.PublicID
		}(i)
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("provision caller %d failed: %v", i, err)
		}
	}
	for i := 1; i < len(publicIDs); i++ {
		if publicIDs[i] != publicIDs[0] {
			t.Fatalf("converged public_id differs: %q vs %q", publicIDs[0], publicIDs[i])
		}
	}
	if publicIDs[0] == "" {
		t.Fatal("converged public_id is empty")
	}

	var rowCount int64
	if err := db.QueryRow(ctx, `SELECT COUNT(*) FROM workspaces WHERE owner_user_id = $1 AND organization_id IS NULL AND archived_at IS NULL`, userID).Scan(&rowCount); err != nil {
		t.Fatalf("count personal workspaces failed: %v", err)
	}
	if rowCount != 1 {
		t.Fatalf("personal workspace rows = %d, want exactly 1", rowCount)
	}

	workspace, err := store.GetWorkspace(ctx, publicIDs[0])
	if err != nil {
		t.Fatalf("get provisioned workspace failed: %v", err)
	}
	if !workspace.IsPersonal() || workspace.OwnerUserID == nil || *workspace.OwnerUserID != userID {
		t.Fatalf("provisioned workspace is not the user personal workspace: %+v", workspace)
	}
	role, ok, err := store.GetWorkspaceRole(ctx, publicIDs[0], userID)
	if err != nil || !ok || role != tenancy.RoleOwner {
		t.Fatalf("owner role = %q ok=%v err=%v, want owner/true/nil", role, ok, err)
	}
	members, err := store.ListWorkspaceMembers(ctx, publicIDs[0])
	if err != nil {
		t.Fatalf("list members failed: %v", err)
	}
	if len(members) != 1 || members[0].UserID != userID || members[0].UserExternalID == "" {
		t.Fatalf("unexpected owner membership projection: %+v", members)
	}
}

// TestBillingAccountOwnerStubProvesSeam proves the #801 billing stub carries
// the workspace owner reference #804 will build on.
func TestBillingAccountOwnerStubProvesSeam(t *testing.T) {
	db := newTenancyTestDB(t)
	store := NewStore(db)
	userID := createTenancyTestUser(t, db)
	ctx := context.Background()

	workspace, err := store.EnsurePersonalWorkspace(ctx, userID)
	if err != nil {
		t.Fatalf("ensure personal workspace failed: %v", err)
	}
	queries := dbsqlc.New(db)
	stub, err := queries.CreateBillingAccountStub(ctx, dbsqlc.CreateBillingAccountStubParams{
		WorkspaceID: dbutil.NullInt64(&workspace.InternalID),
	})
	if err != nil {
		t.Fatalf("create billing account stub failed: %v", err)
	}
	loaded, err := queries.GetBillingAccountByWorkspaceID(ctx, stub.WorkspaceID)
	if err != nil {
		if database.IsNotFound(err) {
			t.Fatal("billing account stub not found by workspace")
		}
		t.Fatalf("get billing account stub failed: %v", err)
	}
	if loaded.InternalID != stub.InternalID {
		t.Fatalf("stub internal_id = %d, want %d", loaded.InternalID, stub.InternalID)
	}
}
