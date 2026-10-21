package postgres

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"ideaboard/api/internal/credits"
	creditdomain "ideaboard/api/internal/credits/domain"
	creditports "ideaboard/api/internal/credits/ports"

	"ideaboard/api/internal/tenancy"

	"github.com/jackc/pgx/v5/pgxpool"
)

// recordingDeleter is a test-only ObjectDeleter: it records keys and
// fails configured keys failuresBeforeSuccess times before succeeding.
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

// deleteFixture seeds one workspace graph: membership, billing account,
// upload session + media asset, task + output + execution rows, library
// folder/asset/materialization/link, board + document + revision chain +
// membership + media ref, origins, idempotency, events/subscriptions/
// deliveries/attempts, and both TEXT outboxes. Every RESTRICT edge the
// hard delete must order around is populated.
type deleteFixture struct {
	workspaceInternal int64
	workspacePublic   string
	mediaKey          string
	sessionKey        string
	assetPublic       string
	taskPublics       []string
}

func seedDeleteWorkspace(t *testing.T, db *pgxpool.Pool, ctx context.Context, userID int64, publicID, name string, isDefault bool) deleteFixture {
	t.Helper()
	var fx deleteFixture
	fx.workspacePublic = publicID
	err := db.QueryRow(ctx, `
INSERT INTO workspaces (public_id, owner_user_id, name, is_default)
VALUES ($1, $2, $3, $4)
RETURNING internal_id`, publicID, userID, name, isDefault).Scan(&fx.workspaceInternal)
	if err != nil {
		t.Fatalf("seed workspace %s failed: %v", publicID, err)
	}
	if _, err := db.Exec(ctx, `INSERT INTO workspace_memberships (workspace_id, user_id, role) VALUES ($1, $2, 'owner')`, fx.workspaceInternal, userID); err != nil {
		t.Fatalf("seed membership failed: %v", err)
	}
	return fx
}

func seedDeleteGraph(t *testing.T, db *pgxpool.Pool, ctx context.Context, userID int64, fx *deleteFixture, tag string) {
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

// deleteRedlineTables are never touched by the hard delete:
// ledger/billing immutables, users, payments, rewards.
var deleteRedlineTables = []string{
	"credit_wallets", "credit_accounts", "credit_transactions", "credit_postings",
	"credit_holds", "credit_charges", "task_billing_snapshots", "task_billing_settlements",
	"users", "payment_orders", "payment_refunds", "reward_codes", "reward_redemptions",
}

func snapshotDeleteCounts(t *testing.T, db *pgxpool.Pool, ctx context.Context, tables []string) map[string]int64 {
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

func workspaceScopedDeleteCount(t *testing.T, db *pgxpool.Pool, ctx context.Context, table, column string, wid int64) int64 {
	t.Helper()
	var n int64
	if err := db.QueryRow(ctx, `SELECT COUNT(*) FROM `+table+` WHERE `+column+` = $1`, wid).Scan(&n); err != nil {
		t.Fatalf("count %s failed: %v", table, err)
	}
	return n
}

func TestDeleteWorkspaceDeletesFullGraph(t *testing.T) {
	db := newTenancyTestDB(t)
	ctx := context.Background()
	userID := createTenancyTestUser(t, db)

	fx := seedDeleteWorkspace(t, db, ctx, userID, "ws_delete_full", "full", false)
	seedDeleteGraph(t, db, ctx, userID, &fx, "full0001")

	sibling := seedDeleteWorkspace(t, db, ctx, userID, "ws_delete_sibling", "sibling", false)
	seedDeleteGraph(t, db, ctx, userID, &sibling, "sibling1")

	before := snapshotDeleteCounts(t, db, ctx, deleteRedlineTables)

	objects := &recordingDeleter{}
	if err := NewStore(db, objects).DeleteWorkspace(ctx, "ws_delete_full"); err != nil {
		t.Fatalf("DeleteWorkspace failed: %v", err)
	}
	if !objects.sawKey(fx.mediaKey) || !objects.sawKey(fx.sessionKey) {
		t.Fatalf("storage keys not cleaned: got %v, want %q and %q", objects.keys, fx.mediaKey, fx.sessionKey)
	}

	// Every workspace-owned row for the deleted workspace is gone.
	for table, column := range map[string]string{
		"workspace_memberships": "workspace_id", "billing_accounts": "workspace_id",
		"upload_sessions": "workspace_id", "media_assets": "workspace_id",
		"media_asset_task_origins": "workspace_id", "tasks": "workspace_id",
		"task_submission_idempotency": "workspace_id", "library_folders": "workspace_id",
		"library_assets": "workspace_id", "library_materializations": "workspace_id",
		"workspace_events": "workspace_id", "event_subscriptions": "workspace_id",
		"boards": "workspace_id",
	} {
		if n := workspaceScopedDeleteCount(t, db, ctx, table, column, fx.workspaceInternal); n != 0 {
			t.Fatalf("%s rows for deleted workspace = %d, want 0", table, n)
		}
	}
	var nOutputs int64
	if err := db.QueryRow(ctx, `SELECT COUNT(*) FROM task_outputs WHERE task_id IN (SELECT internal_id FROM tasks WHERE workspace_id = $1)`, fx.workspaceInternal).Scan(&nOutputs); err != nil {
		t.Fatalf("count task outputs failed: %v", err)
	}
	if nOutputs != 0 {
		t.Fatalf("task_outputs rows for deleted workspace = %d, want 0", nOutputs)
	}
	for _, query := range []string{
		`SELECT COUNT(*) FROM task_execution_attempts WHERE task_id = 'task_full0001'`,
		`SELECT COUNT(*) FROM task_execution_metrics WHERE task_id = 'task_full0001'`,
		`SELECT COUNT(*) FROM task_cancellations WHERE task_id = 'task_full0001'`,
		`SELECT COUNT(*) FROM task_status_event_outbox WHERE workspace_id = 'ws_delete_full'`,
		`SELECT COUNT(*) FROM board_access_event_outbox WHERE workspace_id = 'ws_delete_full'`,
		`SELECT COUNT(*) FROM library_asset_media_assets WHERE media_asset_id NOT IN (SELECT internal_id FROM media_assets)`,
		`SELECT COUNT(*) FROM workspaces WHERE public_id = 'ws_delete_full'`,
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
	if err := db.QueryRow(ctx, `SELECT name FROM workspaces WHERE public_id = 'ws_delete_sibling'`).Scan(&siblingName); err != nil {
		t.Fatalf("sibling workspace missing: %v", err)
	}
	for table, column := range map[string]string{
		"tasks": "workspace_id", "media_assets": "workspace_id", "boards": "workspace_id",
		"library_assets": "workspace_id", "billing_accounts": "workspace_id",
	} {
		if n := workspaceScopedDeleteCount(t, db, ctx, table, column, sibling.workspaceInternal); n == 0 {
			t.Fatalf("%s rows for sibling workspace = 0, want intact", table)
		}
	}

	// Red lines untouched.
	after := snapshotDeleteCounts(t, db, ctx, deleteRedlineTables)
	for _, table := range deleteRedlineTables {
		if after[table] != before[table] {
			t.Fatalf("redline %s count changed: %d -> %d", table, before[table], after[table])
		}
	}

	// Second delete reads as not found (idempotent 404).
	if err := NewStore(db, nil).DeleteWorkspace(ctx, "ws_delete_full"); !errors.Is(err, tenancy.ErrNotFound) {
		t.Fatalf("second DeleteWorkspace error = %v, want ErrNotFound", err)
	}
}

func TestDeleteWorkspaceDeduplicatesStorageKeys(t *testing.T) {
	db := newTenancyTestDB(t)
	ctx := t.Context()
	userID := createTenancyTestUser(t, db)
	fx := seedDeleteWorkspace(t, db, ctx, userID, "ws_delete_keys", "keys", false)
	seedDeleteGraph(t, db, ctx, userID, &fx, "keys0001")
	// The completed upload and its Media Asset can name the same object.
	if _, err := db.Exec(ctx, `UPDATE upload_sessions SET storage_key = $1 WHERE workspace_id = $2`, "  "+fx.mediaKey+"  ", fx.workspaceInternal); err != nil {
		t.Fatal(err)
	}
	// Pending uploads have no stored bytes to clean.
	if _, err := db.Exec(ctx, `INSERT INTO upload_sessions
  (public_id, workspace_id, uploader_user_id, declared_mime_type, declared_size_bytes)
  VALUES ('upload_keys_pending', $1, $2, 'image/png', 16)`, fx.workspaceInternal, userID); err != nil {
		t.Fatal(err)
	}
	objects := &recordingDeleter{}
	if err := NewStore(db, objects).DeleteWorkspace(ctx, fx.workspacePublic); err != nil {
		t.Fatal(err)
	}
	if len(objects.keys) != 1 || objects.keys[0] != fx.mediaKey {
		t.Fatalf("deleted storage keys = %v, want exactly [%s]", objects.keys, fx.mediaKey)
	}
}

func TestDeleteWorkspaceDefaultProtected(t *testing.T) {
	db := newTenancyTestDB(t)
	ctx := context.Background()
	userID := createTenancyTestUser(t, db)

	def, err := NewStore(db, nil).EnsureDefaultWorkspace(ctx, userID)
	if err != nil {
		t.Fatalf("ensure default failed: %v", err)
	}
	objects := &recordingDeleter{}
	if err := NewStore(db, objects).DeleteWorkspace(ctx, def.PublicID); !errors.Is(err, tenancy.ErrDefaultWorkspaceProtected) {
		t.Fatalf("DeleteWorkspace(default) error = %v, want ErrDefaultWorkspaceProtected", err)
	}
	if len(objects.keys) != 0 {
		t.Fatalf("storage keys cleaned for Default workspace: %v", objects.keys)
	}
	var n int64
	if err := db.QueryRow(ctx, `SELECT COUNT(*) FROM workspaces WHERE public_id = $1`, def.PublicID).Scan(&n); err != nil || n != 1 {
		t.Fatalf("Default workspace missing after protected delete: n=%d err=%v", n, err)
	}
}

func TestDeleteWorkspaceStorageFailureKeepsCommittedRows(t *testing.T) {
	db := newTenancyTestDB(t)
	ctx := context.Background()
	userID := createTenancyTestUser(t, db)

	fx := seedDeleteWorkspace(t, db, ctx, userID, "ws_delete_flaky", "flaky", false)
	seedDeleteGraph(t, db, ctx, userID, &fx, "flaky001")

	objects := &recordingDeleter{failuresBeforeOK: map[string]int{fx.mediaKey: 100}}
	if err := NewStore(db, objects).DeleteWorkspace(ctx, "ws_delete_flaky"); err != nil {
		t.Fatalf("DeleteWorkspace failed despite storage failure: %v", err)
	}
	var n int64
	if err := db.QueryRow(ctx, `SELECT COUNT(*) FROM workspaces WHERE public_id = 'ws_delete_flaky'`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("workspace not deleted despite storage failure: n=%d err=%v", n, err)
	}
}

func TestDeleteWorkspaceNotFound(t *testing.T) {
	db := newTenancyTestDB(t)
	ctx := context.Background()
	if err := NewStore(db, nil).DeleteWorkspace(ctx, "ws_missing0001"); !errors.Is(err, tenancy.ErrNotFound) {
		t.Fatalf("DeleteWorkspace(unknown) error = %v, want ErrNotFound", err)
	}
}

// Deleting active Task facts would strand their preserved Credit Hold.
func TestDeleteWorkspaceRejectsActiveTasksWithoutChangingGraph(t *testing.T) {
	for _, status := range []string{"pending", "running"} {
		t.Run(status, func(t *testing.T) {
			db := newTenancyTestDB(t)
			ctx := t.Context()
			userID := createTenancyTestUser(t, db)
			fx := seedDeleteWorkspace(t, db, ctx, userID, "ws_delete_active", "active", false)
			seedDeleteGraph(t, db, ctx, userID, &fx, "active01")
			if _, err := db.Exec(ctx, `UPDATE tasks SET status = $1, completed_at = NULL WHERE public_id = $2`, status, fx.taskPublics[0]); err != nil {
				t.Fatal(err)
			}
			service := credits.NewService(db)
			tx, err := db.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = tx.Rollback(ctx) }()
			deleteGuardAccountID, err := service.LifecycleInTx(tx).ResolvePersonalAccount(ctx, userID)
			if err != nil {
				t.Fatal(err)
			}
			if err := service.LifecycleInTx(tx).ProvisionAccountWallet(ctx, deleteGuardAccountID); err != nil {
				t.Fatal(err)
			}
			metadata := func(key string) creditdomain.CommandMetadata {
				return creditdomain.CommandMetadata{
					IdempotencyKey: key,
					Actor:          creditdomain.Actor{Type: creditdomain.ActorSystem, ID: "test"},
					Source:         "test", Reason: "workspace delete guard",
					BusinessReference: creditdomain.BusinessReference{Type: "task", ID: fx.taskPublics[0]},
				}
			}
			amount, err := creditdomain.NewCreditAmount(100)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := service.InTx(tx).Grant(ctx, creditports.GrantCommand{BillingAccountID: deleteGuardAccountID, Amount: amount, Metadata: metadata("delete-guard-grant")}); err != nil {
				t.Fatal(err)
			}
			hold, err := service.InTx(tx).Hold(ctx, creditports.HoldCommand{BillingAccountID: deleteGuardAccountID, Amount: amount, Metadata: metadata("delete-guard-hold")})
			if err != nil {
				t.Fatal(err)
			}
			if err := tx.Commit(ctx); err != nil {
				t.Fatal(err)
			}
			tables := append(append([]string{}, deleteRedlineTables...), "workspaces", "tasks", "task_outputs", "media_assets", "boards", "workspace_memberships")
			before := snapshotDeleteCounts(t, db, ctx, tables)
			objects := &recordingDeleter{}
			if err := NewStore(db, objects).DeleteWorkspace(ctx, fx.workspacePublic); !errors.Is(err, tenancy.ErrWorkspaceActiveTasks) {
				t.Fatalf("DeleteWorkspace(%s) error = %v, want ErrWorkspaceActiveTasks", status, err)
			}
			for table, after := range snapshotDeleteCounts(t, db, ctx, tables) {
				if after != before[table] {
					t.Fatalf("rejected delete changed %s rows: %d -> %d", table, before[table], after)
				}
			}
			if len(objects.keys) != 0 {
				t.Fatalf("rejected delete removed storage objects: %v", objects.keys)
			}
			var holdStatus string
			if err := db.QueryRow(ctx, `SELECT status FROM credit_holds WHERE hold_transaction_id = $1`, hold.TransactionID).Scan(&holdStatus); err != nil || holdStatus != "held" {
				t.Fatalf("active Task Hold changed: status=%s err=%v", holdStatus, err)
			}
		})
	}
}

func TestDeleteWorkspaceIgnoresOtherWorkspaceActiveTasks(t *testing.T) {
	db := newTenancyTestDB(t)
	ctx := t.Context()
	userID := createTenancyTestUser(t, db)
	fx := seedDeleteWorkspace(t, db, ctx, userID, "ws_delete_idle", "idle", false)
	sibling := seedDeleteWorkspace(t, db, ctx, userID, "ws_delete_busy", "busy", false)
	if _, err := db.Exec(ctx, `WITH account AS (INSERT INTO billing_accounts (public_id, workspace_id) VALUES ('ba_busy0001', $1) RETURNING internal_id) INSERT INTO tasks (public_id, workspace_id, submitter_user_id, billing_account_id, service, workflow_input, status) SELECT 'task_busy0001', $1, $2, internal_id, 'generation', '{}', 'pending' FROM account`, sibling.workspaceInternal, userID); err != nil {
		t.Fatal(err)
	}
	if err := NewStore(db, nil).DeleteWorkspace(ctx, fx.workspacePublic); err != nil {
		t.Fatalf("DeleteWorkspace(idle) blocked by sibling task: %v", err)
	}
	if got := workspaceScopedDeleteCount(t, db, ctx, "tasks", "workspace_id", sibling.workspaceInternal); got != 1 {
		t.Fatalf("sibling active Tasks = %d, want 1", got)
	}
}

func TestDeleteWorkspaceWaitsForInFlightTaskSubmission(t *testing.T) {
	db := newTenancyTestDB(t)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	userID := createTenancyTestUser(t, db)
	fx := seedDeleteWorkspace(t, db, ctx, userID, "ws_delete_race", "race", false)
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var submitPID int32
	if err := tx.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&submitPID); err != nil {
		t.Fatal(err)
	}
	// The FK keeps KEY SHARE on the Workspace until submission commits.
	if _, err := tx.Exec(ctx, `WITH account AS (INSERT INTO billing_accounts (public_id, workspace_id) VALUES ('ba_race0001', $1) RETURNING internal_id) INSERT INTO tasks (public_id, workspace_id, submitter_user_id, billing_account_id, service, workflow_input, status) SELECT 'task_race0001', $1, $2, internal_id, 'generation', '{}', 'pending' FROM account`, fx.workspaceInternal, userID); err != nil {
		t.Fatal(err)
	}
	deleted := make(chan error, 1)
	go func() { deleted <- NewStore(db, nil).DeleteWorkspace(ctx, fx.workspacePublic) }()
	for {
		var blocked bool
		if err := db.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM pg_stat_activity WHERE $1 = ANY(pg_blocking_pids(pid)) AND query LIKE '%FROM workspaces WHERE public_id = $1 FOR UPDATE%')`, submitPID).Scan(&blocked); err != nil {
			t.Fatalf("observe delete waiting for submission: %v", err)
		}
		if blocked {
			break
		}
		select {
		case err := <-deleted:
			t.Fatalf("delete finished before task submission committed: %v", err)
		case <-ctx.Done():
			t.Fatalf("delete did not wait for task submission: %v", ctx.Err())
		case <-time.After(10 * time.Millisecond):
		}
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err := <-deleted; !errors.Is(err, tenancy.ErrWorkspaceActiveTasks) {
		t.Fatalf("delete after submission error = %v, want ErrWorkspaceActiveTasks", err)
	}
	if got := workspaceScopedDeleteCount(t, db, ctx, "tasks", "workspace_id", fx.workspaceInternal); got != 1 {
		t.Fatalf("concurrently submitted Tasks = %d, want 1", got)
	}
}
