package postgres

import (
	"context"
	"crypto/rand"
	"errors"
	"sync"
	"testing"

	"ideaboard/api/internal/db/dbutil"
	dbsqlc "ideaboard/api/internal/db/sqlc"
	"ideaboard/api/internal/platform/contract"
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

// TestEnsureDefaultWorkspaceConcurrent proves default provisioning is
// idempotent under concurrency: N callers converge to one Default row with
// one public_id, backed by ux_workspaces_default_owner.
func TestEnsureDefaultWorkspaceConcurrent(t *testing.T) {
	db := newTenancyTestDB(t)
	store := NewStore(db, nil)
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
			workspace, err := store.EnsureDefaultWorkspace(ctx, userID)
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
	if err := db.QueryRow(ctx, `SELECT COUNT(*) FROM workspaces WHERE owner_user_id = $1 AND organization_id IS NULL AND is_default`, userID).Scan(&rowCount); err != nil {
		t.Fatalf("count default workspaces failed: %v", err)
	}
	if rowCount != 1 {
		t.Fatalf("default workspace rows = %d, want exactly 1", rowCount)
	}

	workspace, err := store.GetWorkspace(ctx, publicIDs[0])
	if err != nil {
		t.Fatalf("get provisioned workspace failed: %v", err)
	}
	if !workspace.IsDefaultWorkspace() || workspace.OwnerUserID == nil || *workspace.OwnerUserID != userID {
		t.Fatalf("provisioned workspace is not the user default workspace: %+v", workspace)
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
	store := NewStore(db, nil)
	userID := createTenancyTestUser(t, db)
	ctx := context.Background()

	workspace, err := store.EnsureDefaultWorkspace(ctx, userID)
	if err != nil {
		t.Fatalf("ensure default workspace failed: %v", err)
	}
	queries := dbsqlc.New(db)
	// The #804 hard cut requires a ba_-prefixed public_id on every billing
	// account row; mint one through the contract kernel like production code.
	billingPublicID, err := contract.MintPublicID("ba_", 12)
	if err != nil {
		t.Fatalf("mint billing account public id failed: %v", err)
	}
	stub, err := queries.CreateBillingAccountStub(ctx, dbsqlc.CreateBillingAccountStubParams{
		PublicID:    billingPublicID,
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

// TestDefaultWorkspaceSemantics pins the Bug2 product contract against
// PostgreSQL: plain personal creates are unlimited and non-Default, a
// second Default create is rejected, the Default row cannot be deleted or
// renamed, non-Default rows rename normally, and a user holding only plain
// rows still gets a Default on ensure.
func TestDefaultWorkspaceSemantics(t *testing.T) {
	db := newTenancyTestDB(t)
	store := NewStore(db, nil)
	userID := createTenancyTestUser(t, db)
	ctx := context.Background()

	plain, err := store.CreateWorkspace(ctx, tenancy.CreateWorkspaceParams{Name: "plain-a", OwnerUserID: userID})
	if err != nil {
		t.Fatalf("plain create failed: %v", err)
	}
	if plain.IsDefaultWorkspace() {
		t.Fatalf("plain create must not be Default: %+v", plain)
	}
	plainB, err := store.CreateWorkspace(ctx, tenancy.CreateWorkspaceParams{Name: "plain-b", OwnerUserID: userID})
	if err != nil {
		t.Fatalf("second plain create failed (must be unlimited): %v", err)
	}

	def, err := store.EnsureDefaultWorkspace(ctx, userID)
	if err != nil {
		t.Fatalf("ensure default failed: %v", err)
	}
	if !def.IsDefaultWorkspace() {
		t.Fatalf("ensured workspace is not Default: %+v", def)
	}
	if _, err := store.CreateWorkspace(ctx, tenancy.CreateWorkspaceParams{Name: "dup", OwnerUserID: userID, IsDefault: true}); !errors.Is(err, tenancy.ErrDefaultWorkspaceConflict) {
		t.Fatalf("second Default err = %v, want ErrDefaultWorkspaceConflict", err)
	}
	if err := store.DeleteWorkspace(ctx, def.PublicID); !errors.Is(err, tenancy.ErrDefaultWorkspaceProtected) {
		t.Fatalf("delete default err = %v, want ErrDefaultWorkspaceProtected", err)
	}
	if _, err := store.UpdateWorkspace(ctx, def.PublicID, "Renamed Default"); !errors.Is(err, tenancy.ErrDefaultWorkspaceProtected) {
		t.Fatalf("rename default err = %v, want ErrDefaultWorkspaceProtected", err)
	}
	renamed, err := store.UpdateWorkspace(ctx, plainB.PublicID, "Renamed Plain")
	if err != nil || renamed.Name != "Renamed Plain" {
		t.Fatalf("rename plain = %+v, %v; want renamed", renamed, err)
	}
	if err := store.DeleteWorkspace(ctx, plain.PublicID); err != nil {
		t.Fatalf("delete plain failed: %v", err)
	}
	if _, err := store.GetWorkspace(ctx, plain.PublicID); !errors.Is(err, tenancy.ErrNotFound) {
		t.Fatalf("get deleted err = %v, want ErrNotFound", err)
	}
}
