package purge

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"ideaboard/api/internal/testdb"

	"github.com/jackc/pgx/v5/pgxpool"
)

// recordingDeleter is a test-only ObjectDeleter: it records keys and
// fails configured keys failuresBeforeSuccess times before
// succeeding.
type recordingDeleter struct {
	mu               sync.Mutex
	keys             []string
	failuresBeforeOK map[string]int
	deleteAttempts   map[string]int
}

func (f *recordingDeleter) DeleteObjectByKey(_ context.Context, key string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.keys = append(f.keys, key)
	if f.deleteAttempts == nil {
		f.deleteAttempts = make(map[string]int)
	}
	f.deleteAttempts[key]++
	if f.failuresBeforeOK != nil && f.deleteAttempts[key] <= f.failuresBeforeOK[key] {
		return fmt.Errorf("injected storage failure for %s", key)
	}
	return nil
}

func (f *recordingDeleter) sawKey(key string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, got := range f.keys {
		if got == key {
			return true
		}
	}
	return false
}

// purgeFixture seeds one workspace graph: membership, billing account,
// upload session + media asset, task + output + execution rows, library
// folder/asset/materialization/link, board + document + revision chain +
// membership + media ref, origins, idempotency, events/subscriptions/
// deliveries/attempts, and both TEXT outboxes. Every RESTRICT edge the
// purge must order around is populated.
type purgeFixture struct {
	workspaceInternal int64
	workspacePublic   string
	mediaKey          string
	sessionKey        string
	assetPublic       string
	taskPublics       []string
}

func seedPurgeUser(t *testing.T, db *pgxpool.Pool) int64 {
	t.Helper()
	// Fixture user 42 is seeded by testdb; reuse it as the universal owner.
	return 42
}

func seedPurgeWorkspace(t *testing.T, db *pgxpool.Pool, ctx context.Context, userID int64, publicID, name string, isDefault bool, archivedExpr string) purgeFixture {
	t.Helper()
	var fx purgeFixture
	fx.workspacePublic = publicID
	err := db.QueryRow(ctx, `
INSERT INTO workspaces (public_id, owner_user_id, name, is_default, archived_at)
VALUES ($1, $2, $3, $4, `+archivedExpr+`)
RETURNING internal_id`, publicID, userID, name, isDefault).Scan(&fx.workspaceInternal)
	if err != nil {
		t.Fatalf("seed workspace %s failed: %v", publicID, err)
	}
	if _, err := db.Exec(ctx, `INSERT INTO workspace_memberships (workspace_id, user_id, role) VALUES ($1, $2, 'owner')`, fx.workspaceInternal, userID); err != nil {
		t.Fatalf("seed membership failed: %v", err)
	}
	return fx
}

func seedPurgeGraph(t *testing.T, db *pgxpool.Pool, ctx context.Context, userID int64, fx *purgeFixture, tag string) {
	t.Helper()
	wid := fx.workspaceInternal
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := db.Exec(ctx, query, args...); err != nil {
			t.Fatalf("seed %s: %v\n%s", tag, err, query)
		}
	}
	// Billing account (tasks -> billing_accounts RESTRICT).
	billingPublic := "ba_" + tag
	var billingID int64
	if err := db.QueryRow(ctx, `INSERT INTO billing_accounts (public_id, workspace_id) VALUES ($1, $2) RETURNING internal_id`, billingPublic, wid).Scan(&billingID); err != nil {
		t.Fatalf("seed billing account: %v", err)
	}
	// Upload session + media asset (media -> session RESTRICT).
	fx.sessionKey = "uploads/ws/" + tag + "/session.bin"
	fx.mediaKey = "uploads/ws/" + tag + "/asset.png"
	fx.assetPublic = "asset_" + tag
	var sessionID int64
	if err := db.QueryRow(ctx, `
INSERT INTO upload_sessions (public_id, workspace_id, uploader_user_id, declared_mime_type, declared_size_bytes, storage_key, status, completed_at)
VALUES ($1, $2, $3, 'image/png', 16, $4, 'completed', now()) RETURNING internal_id`,
		"upload_"+tag, wid, userID, fx.sessionKey).Scan(&sessionID); err != nil {
		t.Fatalf("seed upload session: %v", err)
	}
	var mediaID int64
	if err := db.QueryRow(ctx, `
INSERT INTO media_assets (public_id, workspace_id, upload_session_id, owner_user_id, storage_key, mime_type, size_bytes, checksum, media_kind, width, height)
VALUES ($1, $2, $3, $4, $5, 'image/png', 16, 'deadbeef', 'image', 8, 8) RETURNING internal_id`,
		fx.assetPublic, wid, sessionID, userID, fx.mediaKey).Scan(&mediaID); err != nil {
		t.Fatalf("seed media asset: %v", err)
	}
	// Task + output (outputs -> media RESTRICT) + string-keyed execution rows.
	taskPublic := "task_" + tag
	fx.taskPublics = []string{taskPublic}
	var taskInternal int64
	if err := db.QueryRow(ctx, `
INSERT INTO tasks (public_id, workspace_id, submitter_user_id, billing_account_id, service, workflow_input, status, completed_at)
VALUES ($1, $2, $3, $4, 'generation', '{}', 'completed', now()) RETURNING internal_id`,
		taskPublic, wid, userID, billingID).Scan(&taskInternal); err != nil {
		t.Fatalf("seed task: %v", err)
	}
	exec(`INSERT INTO task_outputs (task_id, output_index, output_kind, media_asset_id) VALUES ($1, 0, 'media_asset', $2)`, taskInternal, mediaID)
	exec(`INSERT INTO task_execution_attempts (task_id, execution_token, attempt_no, status, completed_at) VALUES ($1, $2, 1, 'completed', now())`, taskPublic, "tok_"+tag)
	exec(`INSERT INTO task_execution_metrics (task_id, current_stage) VALUES ($1, 'completed')`, taskPublic)
	exec(`INSERT INTO task_cancellations (task_id, expires_at) VALUES ($1, now() + interval '1 day')`, taskPublic)
	exec(`INSERT INTO task_submission_idempotency (workspace_id, operation, idempotency_key, request_hash, task_public_id)
VALUES ($1, 'task:create', $2, '0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef', $3)`, wid, "key_"+tag, taskPublic)
	exec(`INSERT INTO media_asset_task_origins (workspace_id, task_public_id, output_index, media_asset_id, submitter_user_id)
VALUES ($1, $2, 0, $3, $4)`, wid, taskPublic, mediaID, userID)
	// Library folder/asset/materialization/link (assets -> media + folder RESTRICT).
	var folderID int64
	if err := db.QueryRow(ctx, `INSERT INTO library_folders (public_id, workspace_id, name) VALUES ($1, $2, $3) RETURNING internal_id`,
		"lfolder_"+tag, wid, "folder-"+tag).Scan(&folderID); err != nil {
		t.Fatalf("seed library folder: %v", err)
	}
	var libraryAssetID int64
	if err := db.QueryRow(ctx, `
INSERT INTO library_assets (public_id, workspace_id, folder_id, media_asset_id) VALUES ($1, $2, $3, $4) RETURNING internal_id`,
		"lasset_"+tag, wid, folderID, mediaID).Scan(&libraryAssetID); err != nil {
		t.Fatalf("seed library asset: %v", err)
	}
	exec(`INSERT INTO library_materializations (public_id, workspace_id, library_asset_id, status, media_asset_id, completed_at)
VALUES ($1, $2, $3, 'completed', $4, now())`, "lmat_"+tag, wid, libraryAssetID, mediaID)
	exec(`INSERT INTO library_asset_media_assets (library_asset_id, owner_user_id, media_asset_id) VALUES ($1, $2, $3)`,
		libraryAssetID, userID, mediaID)
	// Board + document + revision chain (parent_revision_id RESTRICT) +
	// membership + media ref.
	boardPublic := "board_" + tag
	var boardID int64
	if err := db.QueryRow(ctx, `INSERT INTO boards (public_id, workspace_id, title, owner_user_id) VALUES ($1, $2, $3, $4) RETURNING internal_id`,
		boardPublic, wid, "board-"+tag, userID).Scan(&boardID); err != nil {
		t.Fatalf("seed board: %v", err)
	}
	exec(`INSERT INTO board_documents (board_id, snapshot_data, schema_version, etag) VALUES ($1, '{}', 1, 'etagtest1')`, boardID)
	var rev1 int64
	if err := db.QueryRow(ctx, `INSERT INTO board_revisions (public_id, board_id, revision_number, author_user_id, revision_data)
VALUES ($1, $2, 1, $3, '{}') RETURNING internal_id`, "rev_"+tag+"_1", boardID, userID).Scan(&rev1); err != nil {
		t.Fatalf("seed revision 1: %v", err)
	}
	exec(`INSERT INTO board_revisions (public_id, board_id, revision_number, author_user_id, parent_revision_id, revision_data)
VALUES ($1, $2, 2, $3, $4, '{}')`, "rev_"+tag+"_2", boardID, userID, rev1)
	exec(`INSERT INTO board_memberships (board_id, user_id) VALUES ($1, $2)`, boardID, userID)
	exec(`INSERT INTO board_document_media_refs (board_id, media_asset_id) VALUES ($1, $2)`, boardID, mediaID)
	// Events + subscription + delivery + attempt.
	var eventID int64
	if err := db.QueryRow(ctx, `
INSERT INTO workspace_events (public_id, workspace_id, event_type, resource_type, resource_id, event_data)
VALUES ($1, $2, 'task.completed', 'task', $3, '{}') RETURNING internal_id`,
		"evt_"+tag, wid, taskPublic).Scan(&eventID); err != nil {
		t.Fatalf("seed event: %v", err)
	}
	var subID int64
	if err := db.QueryRow(ctx, `
INSERT INTO event_subscriptions (public_id, workspace_id, endpoint_url, signing_secret, event_types)
VALUES ($1, $2, 'https://example.com/hook', 'whsec_testsecret', '{task.completed}') RETURNING internal_id`,
		"sub_"+tag, wid).Scan(&subID); err != nil {
		t.Fatalf("seed subscription: %v", err)
	}
	var deliveryID int64
	if err := db.QueryRow(ctx, `INSERT INTO event_deliveries (event_id, subscription_id) VALUES ($1, $2) RETURNING internal_id`,
		eventID, subID).Scan(&deliveryID); err != nil {
		t.Fatalf("seed delivery: %v", err)
	}
	exec(`INSERT INTO event_delivery_attempts (delivery_id, attempt_number, outcome) VALUES ($1, 1, 'delivered')`, deliveryID)
	// TEXT outboxes (no FK; must leave with the workspace).
	exec(`INSERT INTO task_status_event_outbox (event_id, workspace_id, task_id, status, resource_id, occurred_at, next_attempt_at)
VALUES ($1, $2, $3, 'completed', $3, now(), now())`, "evt_outbox_"+tag, fx.workspacePublic, taskPublic)
	exec(`INSERT INTO board_access_event_outbox (event_id, workspace_id, board_id, member_public_id, access_version, occurred_at, next_attempt_at)
VALUES ($1, $2, $3, 'usr_0123456789abcdef0123456789abcdef', 1, now(), now())`,
		"evt_bao_"+tag, fx.workspacePublic, boardPublic)
}

// redlineTables are never touched by the purge: ledger/billing immutables,
// users, payments, rewards.
var redlineTables = []string{
	"credit_wallets", "credit_accounts", "credit_transactions", "credit_postings",
	"credit_holds", "credit_charges", "task_billing_snapshots", "task_billing_settlements",
	"users", "payment_orders", "payment_refunds", "reward_codes", "reward_redemptions",
}

func snapshotCounts(t *testing.T, db *pgxpool.Pool, ctx context.Context, tables []string) map[string]int64 {
	t.Helper()
	out := make(map[string]int64, len(tables))
	for _, table := range tables {
		var n int64
		if err := db.QueryRow(ctx, `SELECT COUNT(*) FROM `+table).Scan(&n); err != nil {
			t.Fatalf("count %s failed: %v", table, err)
		}
		out[table] = n
	}
	return out
}

func workspaceScopedCount(t *testing.T, db *pgxpool.Pool, ctx context.Context, table, column string, wid int64) int64 {
	t.Helper()
	var n int64
	if err := db.QueryRow(ctx, `SELECT COUNT(*) FROM `+table+` WHERE `+column+` = $1`, wid).Scan(&n); err != nil {
		t.Fatalf("count %s failed: %v", table, err)
	}
	return n
}

func TestPurgeExpiredWorkspaceDeletesFullGraph(t *testing.T) {
	db := testdb.Open(t)
	ctx := context.Background()
	userID := seedPurgeUser(t, db)

	fx := seedPurgeWorkspace(t, db, ctx, userID, "ws_purge_expired", "expired", false, "now() - make_interval(days => 8)")
	seedPurgeGraph(t, db, ctx, userID, &fx, "expired1")

	sibling := seedPurgeWorkspace(t, db, ctx, userID, "ws_purge_sibling", "sibling", false, "NULL")
	seedPurgeGraph(t, db, ctx, userID, &sibling, "sibling1")

	before := snapshotCounts(t, db, ctx, redlineTables)

	objects := &recordingDeleter{}
	stats, err := NewService(db, objects).PurgeExpiredWorkspaces(ctx, 7)
	if err != nil {
		t.Fatalf("PurgeExpiredWorkspaces failed: %v", err)
	}
	if stats.WorkspacesDeleted != 1 {
		t.Fatalf("WorkspacesDeleted = %d, want 1", stats.WorkspacesDeleted)
	}
	if stats.StorageDeleteFailures != 0 {
		t.Fatalf("StorageDeleteFailures = %d, want 0", stats.StorageDeleteFailures)
	}
	if !objects.sawKey(fx.mediaKey) || !objects.sawKey(fx.sessionKey) {
		t.Fatalf("storage keys not cleaned: got %v, want %q and %q", objects.keys, fx.mediaKey, fx.sessionKey)
	}

	// Every workspace-owned row for the expired workspace is gone.
	for table, column := range map[string]string{
		"workspace_memberships": "workspace_id", "billing_accounts": "workspace_id",
		"upload_sessions": "workspace_id", "media_assets": "workspace_id",
		"media_asset_task_origins": "workspace_id", "tasks": "workspace_id",
		"task_submission_idempotency": "workspace_id", "library_folders": "workspace_id",
		"library_assets": "workspace_id", "library_materializations": "workspace_id",
		"workspace_events": "workspace_id", "event_subscriptions": "workspace_id",
		"boards": "workspace_id",
	} {
		if n := workspaceScopedCount(t, db, ctx, table, column, fx.workspaceInternal); n != 0 {
			t.Fatalf("%s rows for purged workspace = %d, want 0", table, n)
		}
	}
	var nOutputs int64
	if err := db.QueryRow(ctx, `SELECT COUNT(*) FROM task_outputs WHERE task_id IN (SELECT internal_id FROM tasks WHERE workspace_id = $1)`, fx.workspaceInternal).Scan(&nOutputs); err != nil {
		t.Fatalf("count task outputs failed: %v", err)
	}
	if nOutputs != 0 {
		t.Fatalf("task_outputs rows for purged workspace = %d, want 0", nOutputs)
	}
	for _, query := range []string{
		`SELECT COUNT(*) FROM task_execution_attempts WHERE task_id = 'task_expired1'`,
		`SELECT COUNT(*) FROM task_execution_metrics WHERE task_id = 'task_expired1'`,
		`SELECT COUNT(*) FROM task_cancellations WHERE task_id = 'task_expired1'`,
		`SELECT COUNT(*) FROM task_status_event_outbox WHERE workspace_id = 'ws_purge_expired'`,
		`SELECT COUNT(*) FROM board_access_event_outbox WHERE workspace_id = 'ws_purge_expired'`,
		`SELECT COUNT(*) FROM library_asset_media_assets WHERE media_asset_id NOT IN (SELECT internal_id FROM media_assets)`,
		`SELECT COUNT(*) FROM workspaces WHERE public_id = 'ws_purge_expired'`,
	} {
		var n int64
		if err := db.QueryRow(ctx, query).Scan(&n); err != nil {
			t.Fatalf("count (%s) failed: %v", query, err)
		}
		if n != 0 {
			t.Fatalf("residual rows (%s) = %d, want 0", query, n)
		}
	}

	// Sibling workspace is fully intact.
	var siblingName string
	if err := db.QueryRow(ctx, `SELECT name FROM workspaces WHERE public_id = 'ws_purge_sibling'`).Scan(&siblingName); err != nil {
		t.Fatalf("sibling workspace missing: %v", err)
	}
	for table, column := range map[string]string{
		"tasks": "workspace_id", "media_assets": "workspace_id", "boards": "workspace_id",
		"library_assets": "workspace_id", "billing_accounts": "workspace_id",
	} {
		if n := workspaceScopedCount(t, db, ctx, table, column, sibling.workspaceInternal); n == 0 {
			t.Fatalf("%s rows for sibling workspace = 0, want intact", table)
		}
	}

	// Red lines untouched.
	after := snapshotCounts(t, db, ctx, redlineTables)
	for _, table := range redlineTables {
		if after[table] != before[table] {
			t.Fatalf("redline %s count changed: %d -> %d", table, before[table], after[table])
		}
	}
}

func TestPurgeKeepsFreshArchive(t *testing.T) {
	db := testdb.Open(t)
	ctx := context.Background()
	userID := seedPurgeUser(t, db)

	fx := seedPurgeWorkspace(t, db, ctx, userID, "ws_purge_fresh", "fresh", false, "now() - make_interval(days => 1)")
	seedPurgeGraph(t, db, ctx, userID, &fx, "fresh001")

	objects := &recordingDeleter{}
	stats, err := NewService(db, objects).PurgeExpiredWorkspaces(ctx, 7)
	if err != nil {
		t.Fatalf("PurgeExpiredWorkspaces failed: %v", err)
	}
	if stats.WorkspacesDeleted != 0 {
		t.Fatalf("WorkspacesDeleted = %d, want 0", stats.WorkspacesDeleted)
	}
	if len(objects.keys) != 0 {
		t.Fatalf("storage keys cleaned for fresh archive: %v", objects.keys)
	}
	var n int64
	if err := db.QueryRow(ctx, `SELECT COUNT(*) FROM workspaces WHERE public_id = 'ws_purge_fresh'`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("fresh workspace missing after purge: n=%d err=%v", n, err)
	}
	if n := workspaceScopedCount(t, db, ctx, "tasks", "workspace_id", fx.workspaceInternal); n == 0 {
		t.Fatal("fresh workspace tasks deleted, want kept")
	}
}

func TestPurgeExemptsDefaultWorkspace(t *testing.T) {
	db := testdb.Open(t)
	ctx := context.Background()
	userID := seedPurgeUser(t, db)

	// A Default row archived long ago (archive guards forbid this live;
	// the exemption must hold regardless).
	fx := seedPurgeWorkspace(t, db, ctx, userID, "ws_purge_default", "My Workspace", true, "now() - make_interval(days => 30)")
	seedPurgeGraph(t, db, ctx, userID, &fx, "default1")

	objects := &recordingDeleter{}
	stats, err := NewService(db, objects).PurgeExpiredWorkspaces(ctx, 7)
	if err != nil {
		t.Fatalf("PurgeExpiredWorkspaces failed: %v", err)
	}
	if stats.WorkspacesDeleted != 0 {
		t.Fatalf("WorkspacesDeleted = %d, want 0 (Default exempt)", stats.WorkspacesDeleted)
	}
	var isDefault bool
	if err := db.QueryRow(ctx, `SELECT is_default FROM workspaces WHERE public_id = 'ws_purge_default'`).Scan(&isDefault); err != nil || !isDefault {
		t.Fatalf("Default workspace missing after purge: err=%v", err)
	}
}

func TestPurgeStorageFailureKeepsCommittedRows(t *testing.T) {
	db := testdb.Open(t)
	ctx := context.Background()
	userID := seedPurgeUser(t, db)

	fx := seedPurgeWorkspace(t, db, ctx, userID, "ws_purge_flaky", "flaky", false, "now() - make_interval(days => 8)")
	seedPurgeGraph(t, db, ctx, userID, &fx, "flaky001")

	objects := &recordingDeleter{failuresBeforeOK: map[string]int{fx.mediaKey: 100}}
	stats, err := NewService(db, objects).PurgeExpiredWorkspaces(ctx, 7)
	if err != nil {
		t.Fatalf("PurgeExpiredWorkspaces failed: %v", err)
	}
	if stats.WorkspacesDeleted != 1 {
		t.Fatalf("WorkspacesDeleted = %d, want 1 despite storage failure", stats.WorkspacesDeleted)
	}
	if stats.StorageDeleteFailures == 0 {
		t.Fatal("StorageDeleteFailures = 0, want > 0 for the poisoned key")
	}
	var n int64
	if err := db.QueryRow(ctx, `SELECT COUNT(*) FROM workspaces WHERE public_id = 'ws_purge_flaky'`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("workspace not deleted despite storage failure: n=%d err=%v", n, err)
	}
}

func TestPurgeStorageRetryThenSucceeds(t *testing.T) {
	db := testdb.Open(t)
	ctx := context.Background()
	userID := seedPurgeUser(t, db)

	fx := seedPurgeWorkspace(t, db, ctx, userID, "ws_purge_retry", "retry", false, "now() - make_interval(days => 8)")
	seedPurgeGraph(t, db, ctx, userID, &fx, "retry001")

	objects := &recordingDeleter{failuresBeforeOK: map[string]int{fx.mediaKey: 2}}
	stats, err := NewService(db, objects).PurgeExpiredWorkspaces(ctx, 7)
	if err != nil {
		t.Fatalf("PurgeExpiredWorkspaces failed: %v", err)
	}
	if stats.WorkspacesDeleted != 1 || stats.StorageDeleteFailures != 0 {
		t.Fatalf("stats = %+v, want 1 deleted with 0 failures after retry", stats)
	}
	objects.mu.Lock()
	attempts := objects.deleteAttempts[fx.mediaKey]
	objects.mu.Unlock()
	if attempts != 3 {
		t.Fatalf("delete attempts for retried key = %d, want 3", attempts)
	}
}

func TestPurgeStorageKeyDeduplication(t *testing.T) {
	got := dedupeStorageKeys([]string{"  ", "k", " k "})
	if len(got) != 1 || got[0] != "k" {
		t.Fatalf("storage keys = %v, want one normalized key", got)
	}
}

func TestPurgeRequiresDB(t *testing.T) {
	if _, err := NewService(nil, nil).PurgeExpiredWorkspaces(context.Background(), 7); err == nil {
		t.Fatal("PurgeExpiredWorkspaces(nil db) error = nil, want error")
	}
	if _, _, err := NewService(nil, nil).purgeOne(context.Background(), 1, 7); err == nil {
		t.Fatal("purgeOne(nil db) error = nil, want error")
	}
}
