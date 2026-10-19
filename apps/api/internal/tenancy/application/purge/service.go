// Package purge hard-deletes workspaces archived longer ago than the
// configured window (default 7 days, see
// config.DefaultWorkspacePurgeAfterDays).
//
// Archive itself stays a soft delete (workspaces.archived_at); the purge is
// the only hard-delete path for non-Default workspaces. The Default
// workspace is never purged: candidates are filtered on NOT is_default and
// each purge transaction re-checks the marker under row lock, so a flag
// flip cannot race the delete.
//
// Delete order inside one workspace transaction follows the RESTRICT edges
// so no step blocks on a later one:
//
//	task_outputs / execution string-keyed rows -> tasks -> billing_accounts
//	library_materializations -> library_assets -> library_folders
//	task_outputs + library refs -> media_assets -> upload_sessions
//	board_revisions (leaf-first, parent_revision_id is RESTRICT) -> boards
//
// Cascade edges (memberships, events, subscriptions, deliveries, documents,
// media refs, origins, idempotency) are deleted explicitly where they
// release a RESTRICT blocker and otherwise fall out of the final workspace
// row delete; the explicit statements keep the order auditable.
//
// Red lines (never touched): credit_* (immutable trigger),
// task_billing_snapshots/settlements (immutable), users, payments, rewards,
// credit_wallets, statistics aggregates. The TEXT outbox rows
// (task_status_event_outbox, board_access_event_outbox) carry no FK and are
// deleted explicitly with the workspace.
//
// Object bytes leave the database first: storage keys are collected inside
// the transaction, the rows commit, and only then are the objects deleted
// best-effort with retries outside the transaction. A storage failure never
// rolls back the committed row delete; it is logged and counted.
package purge

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	purgeports "ideaboard/api/internal/tenancy/ports/purge"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// maxWorkspacesPerRun bounds one purge pass so a single River job stays
// inside the retention timeout budget; leftovers converge on the next
// daily run.
const maxWorkspacesPerRun = 50

// maxRevisionPasses bounds the leaf-first board revision sweep; a healthy
// chain converges in depth-many passes.
const maxRevisionPasses = 1000

// objectDeleteAttempts bounds the best-effort post-commit storage cleanup
// per key.
const objectDeleteAttempts = 3

// ObjectDeleter is the narrow storage seam the purge needs. It is
// satisfied by media storage.Service; the purge depends on the shape,
// never on the media implementation.
type ObjectDeleter interface {
	DeleteObjectByKey(ctx context.Context, objectKey string) error
}

// Service purges expired archived workspaces.
type Service struct {
	db      *pgxpool.Pool
	objects ObjectDeleter
}

// NewService builds a purge Service over pool. objects may be nil in
// tests that only assert row effects; a nil deleter skips post-commit
// storage cleanup.
func NewService(db *pgxpool.Pool, objects ObjectDeleter) *Service {
	return &Service{db: db, objects: objects}
}

// PurgeExpiredWorkspaces hard-deletes non-Default workspaces with
// archived_at at or older than purgeAfterDays. Non-positive windows fall
// back to 7 days so a miswired caller cannot purge fresh archives.
func (s *Service) PurgeExpiredWorkspaces(ctx context.Context, purgeAfterDays int) (purgeports.PurgedWorkspaceStats, error) {
	var stats purgeports.PurgedWorkspaceStats
	if s == nil || s.db == nil {
		return stats, fmt.Errorf("workspace purge: service not configured")
	}
	if purgeAfterDays < 1 {
		purgeAfterDays = 7
	}
	rows, err := s.db.Query(ctx, `
SELECT internal_id
FROM workspaces
WHERE archived_at IS NOT NULL
  AND NOT is_default
  AND archived_at <= now() - make_interval(days => $1)
ORDER BY archived_at ASC
LIMIT $2`, purgeAfterDays, maxWorkspacesPerRun)
	if err != nil {
		return stats, fmt.Errorf("workspace purge: list candidates: %w", err)
	}
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return stats, fmt.Errorf("workspace purge: scan candidate: %w", err)
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return stats, fmt.Errorf("workspace purge: list candidates: %w", err)
	}
	for _, id := range ids {
		keys, deleted, err := s.purgeOne(ctx, id, purgeAfterDays)
		if err != nil {
			return stats, err
		}
		if !deleted {
			continue
		}
		stats.WorkspacesDeleted++
		objDeleted, objFailed := s.cleanupObjects(ctx, keys)
		stats.StorageObjectsDeleted += objDeleted
		stats.StorageDeleteFailures += objFailed
	}
	return stats, nil
}

// purgeOne hard-deletes one workspace when it is still eligible under row
// lock. It reports whether the row was deleted plus its stored object keys
// to clean after commit.
func (s *Service) purgeOne(ctx context.Context, workspaceID int64, purgeAfterDays int) ([]string, bool, error) {
	if s == nil || s.db == nil {
		return nil, false, fmt.Errorf("workspace purge: service not configured")
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, false, fmt.Errorf("workspace purge: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var (
		publicID   string
		isDefault  bool
		archivedAt pgtype.Timestamptz
	)
	// Re-check eligibility under lock: the candidate snapshot may be stale,
	// and the Default marker is immutable but the exemption must hold even
	// if archive guards ever change.
	if err := tx.QueryRow(ctx, `
SELECT public_id, is_default, archived_at
FROM workspaces WHERE internal_id = $1 FOR UPDATE`, workspaceID).Scan(&publicID, &isDefault, &archivedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("workspace purge: lock workspace: %w", err)
	}
	if isDefault || !archivedAt.Valid {
		return nil, false, nil
	}
	cutoff := time.Now().AddDate(0, 0, -purgeAfterDays)
	if archivedAt.Time.After(cutoff) {
		return nil, false, nil
	}
	objects, err := collectStorageKeys(ctx, tx, workspaceID)
	if err != nil {
		return nil, false, err
	}
	if err := deleteWorkspaceRows(ctx, tx, workspaceID, publicID); err != nil {
		return nil, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, false, fmt.Errorf("workspace purge: commit: %w", err)
	}
	slog.Info("workspace purge deleted", "workspace_id", publicID)
	return objects, true, nil
}

// collectStorageKeys snapshots media asset and upload-session object keys.
// Pending upload sessions carry NULL keys and are skipped.
func collectStorageKeys(ctx context.Context, tx pgx.Tx, workspaceID int64) ([]string, error) {
	var objects []string
	rows, err := tx.Query(ctx, `SELECT storage_key FROM media_assets WHERE workspace_id = $1`, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("workspace purge: collect media keys: %w", err)
	}
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			rows.Close()
			return nil, fmt.Errorf("workspace purge: scan media key: %w", err)
		}
		objects = append(objects, key)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("workspace purge: collect media keys: %w", err)
	}
	sessRows, err := tx.Query(ctx, `SELECT storage_key FROM upload_sessions WHERE workspace_id = $1 AND storage_key IS NOT NULL`, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("workspace purge: collect session keys: %w", err)
	}
	for sessRows.Next() {
		var key string
		if err := sessRows.Scan(&key); err != nil {
			sessRows.Close()
			return nil, fmt.Errorf("workspace purge: scan session key: %w", err)
		}
		objects = append(objects, key)
	}
	sessRows.Close()
	if err := sessRows.Err(); err != nil {
		return nil, fmt.Errorf("workspace purge: collect session keys: %w", err)
	}
	return dedupeStorageKeys(objects), nil
}

func dedupeStorageKeys(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := in[:0]
	for _, raw := range in {
		key := strings.TrimSpace(raw)
		if key == "" {
			continue
		}
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, key)
	}
	return out
}

// deleteWorkspaceRows removes every workspace-owned row in RESTRICT-safe
// order. Red-line tables (credit_*, task_billing_snapshots/settlements,
// users, payments, rewards, credit_wallets, statistics aggregates) share
// no FK into these deletes and are never named here.
func deleteWorkspaceRows(ctx context.Context, tx pgx.Tx, workspaceID int64, publicID string) error {
	exec := func(query string, args ...any) error {
		_, err := tx.Exec(ctx, query, args...)
		return err
	}
	// Event delivery attempts before their deliveries (attempts CASCADE,
	// explicit for a bounded per-workspace delete).
	if err := exec(`DELETE FROM event_delivery_attempts WHERE delivery_id IN (
  SELECT d.internal_id FROM event_deliveries d
  LEFT JOIN workspace_events e ON e.internal_id = d.event_id
  LEFT JOIN event_subscriptions s ON s.internal_id = d.subscription_id
  WHERE e.workspace_id = $1 OR s.workspace_id = $1)`, workspaceID); err != nil {
		return fmt.Errorf("workspace purge: delete delivery attempts: %w", err)
	}
	if err := exec(`DELETE FROM event_deliveries WHERE event_id IN (
  SELECT internal_id FROM workspace_events WHERE workspace_id = $1)
  OR subscription_id IN (
  SELECT internal_id FROM event_subscriptions WHERE workspace_id = $1)`, workspaceID); err != nil {
		return fmt.Errorf("workspace purge: delete deliveries: %w", err)
	}
	// String-keyed execution rows reference tasks by public_id (no FK);
	// delete them before the task rows disappear.
	if err := exec(`DELETE FROM task_execution_attempts WHERE task_id IN (
  SELECT public_id FROM tasks WHERE workspace_id = $1)`, workspaceID); err != nil {
		return fmt.Errorf("workspace purge: delete execution attempts: %w", err)
	}
	if err := exec(`DELETE FROM task_execution_metrics WHERE task_id IN (
  SELECT public_id FROM tasks WHERE workspace_id = $1)`, workspaceID); err != nil {
		return fmt.Errorf("workspace purge: delete execution metrics: %w", err)
	}
	if err := exec(`DELETE FROM task_cancellations WHERE task_id IN (
  SELECT public_id FROM tasks WHERE workspace_id = $1)`, workspaceID); err != nil {
		return fmt.Errorf("workspace purge: delete cancellations: %w", err)
	}
	// task_outputs -> media_assets is RESTRICT: release outputs first.
	if err := exec(`DELETE FROM task_outputs WHERE task_id IN (
  SELECT internal_id FROM tasks WHERE workspace_id = $1)`, workspaceID); err != nil {
		return fmt.Errorf("workspace purge: delete task outputs: %w", err)
	}
	// Materializations reference library_assets (CASCADE) and media_assets
	// (RESTRICT): clear them before both parents.
	if err := exec(`DELETE FROM library_materializations WHERE workspace_id = $1`, workspaceID); err != nil {
		return fmt.Errorf("workspace purge: delete materializations: %w", err)
	}
	// library_asset_media_assets carries no FK (orphan risk): clear by
	// either side before parents disappear.
	if err := exec(`DELETE FROM library_asset_media_assets WHERE library_asset_id IN (
  SELECT internal_id FROM library_assets WHERE workspace_id = $1)
  OR media_asset_id IN (
  SELECT internal_id FROM media_assets WHERE workspace_id = $1)`, workspaceID); err != nil {
		return fmt.Errorf("workspace purge: delete library asset links: %w", err)
	}
	// library_assets -> media_assets and -> folders are RESTRICT: clear
	// assets before both parents.
	if err := exec(`DELETE FROM library_assets WHERE workspace_id = $1`, workspaceID); err != nil {
		return fmt.Errorf("workspace purge: delete library assets: %w", err)
	}
	// tasks -> billing_accounts is RESTRICT: tasks go before accounts.
	if err := exec(`DELETE FROM tasks WHERE workspace_id = $1`, workspaceID); err != nil {
		return fmt.Errorf("workspace purge: delete tasks: %w", err)
	}
	if err := exec(`DELETE FROM task_submission_idempotency WHERE workspace_id = $1`, workspaceID); err != nil {
		return fmt.Errorf("workspace purge: delete idempotency claims: %w", err)
	}
	if err := exec(`DELETE FROM media_asset_task_origins WHERE workspace_id = $1`, workspaceID); err != nil {
		return fmt.Errorf("workspace purge: delete task origins: %w", err)
	}
	// board_revisions.parent_revision_id is RESTRICT on a self edge:
	// sweep leaves first until the chain drains.
	if err := deleteBoardRevisionsLeafFirst(ctx, tx, workspaceID); err != nil {
		return err
	}
	// Boards cascade documents, memberships, and media refs.
	if err := exec(`DELETE FROM boards WHERE workspace_id = $1`, workspaceID); err != nil {
		return fmt.Errorf("workspace purge: delete boards: %w", err)
	}
	// media_assets -> upload_sessions is RESTRICT: media first.
	if err := exec(`DELETE FROM media_assets WHERE workspace_id = $1`, workspaceID); err != nil {
		return fmt.Errorf("workspace purge: delete media assets: %w", err)
	}
	if err := exec(`DELETE FROM upload_sessions WHERE workspace_id = $1`, workspaceID); err != nil {
		return fmt.Errorf("workspace purge: delete upload sessions: %w", err)
	}
	if err := exec(`DELETE FROM library_folders WHERE workspace_id = $1`, workspaceID); err != nil {
		return fmt.Errorf("workspace purge: delete library folders: %w", err)
	}
	if err := exec(`DELETE FROM billing_accounts WHERE workspace_id = $1`, workspaceID); err != nil {
		return fmt.Errorf("workspace purge: delete billing accounts: %w", err)
	}
	if err := exec(`DELETE FROM workspace_events WHERE workspace_id = $1`, workspaceID); err != nil {
		return fmt.Errorf("workspace purge: delete events: %w", err)
	}
	if err := exec(`DELETE FROM event_subscriptions WHERE workspace_id = $1`, workspaceID); err != nil {
		return fmt.Errorf("workspace purge: delete subscriptions: %w", err)
	}
	if err := exec(`DELETE FROM workspace_memberships WHERE workspace_id = $1`, workspaceID); err != nil {
		return fmt.Errorf("workspace purge: delete memberships: %w", err)
	}
	// TEXT outbox rows carry no FK; they leave with the workspace.
	if err := exec(`DELETE FROM task_status_event_outbox WHERE workspace_id = $1`, publicID); err != nil {
		return fmt.Errorf("workspace purge: delete task outbox: %w", err)
	}
	if err := exec(`DELETE FROM board_access_event_outbox WHERE workspace_id = $1`, publicID); err != nil {
		return fmt.Errorf("workspace purge: delete board access outbox: %w", err)
	}
	if err := exec(`DELETE FROM workspaces WHERE internal_id = $1`, workspaceID); err != nil {
		return fmt.Errorf("workspace purge: delete workspace: %w", err)
	}
	return nil
}

// deleteBoardRevisionsLeafFirst drains board revisions without tripping
// the parent_revision_id RESTRICT edge: each pass deletes only revisions
// no surviving row names as parent.
func deleteBoardRevisionsLeafFirst(ctx context.Context, tx pgx.Tx, workspaceID int64) error {
	for pass := 0; pass < maxRevisionPasses; pass++ {
		tag, err := tx.Exec(ctx, `DELETE FROM board_revisions WHERE board_id IN (
  SELECT internal_id FROM boards WHERE workspace_id = $1)
  AND internal_id NOT IN (
  SELECT parent_revision_id FROM board_revisions
  WHERE parent_revision_id IS NOT NULL AND board_id IN (
  SELECT internal_id FROM boards WHERE workspace_id = $2))`, workspaceID, workspaceID)
		if err != nil {
			return fmt.Errorf("workspace purge: delete board revisions: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return nil
		}
	}
	return fmt.Errorf("workspace purge: board revision chain did not drain")
}

// cleanupObjects deletes stored bytes after the rows committed.
// Best-effort with retries: failures are logged and counted, never
// returned, so storage trouble cannot roll back (or fail) the purge.
func (s *Service) cleanupObjects(ctx context.Context, keys []string) (deleted, failed int) {
	if s == nil || s.objects == nil {
		return 0, 0
	}
	for _, key := range keys {
		if err := deleteObjectWithRetry(ctx, s.objects, key); err != nil {
			failed++
			slog.Warn("workspace purge storage delete failed", "storage_key", key, "err", err)
			continue
		}
		deleted++
	}

	return deleted, failed
}

func deleteObjectWithRetry(ctx context.Context, objects ObjectDeleter, key string) error {
	var err error
	for attempt := 0; attempt < objectDeleteAttempts; attempt++ {
		if err = objects.DeleteObjectByKey(ctx, key); err == nil {
			return nil
		}
	}
	return err
}
