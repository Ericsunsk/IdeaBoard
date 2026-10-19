package purge

// PurgedWorkspaceStats reports one workspace purge pass over workspaces
// archived longer ago than the configured window.
//
// Billing facts (billing snapshots/settlements, the credit ledger),
// statistics aggregates, user rows, payments, and rewards are never
// touched by the purge. The orphan TEXT outbox rows
// (task_status_event_outbox, board_access_event_outbox) keyed by the
// purged workspace public id are deleted with the workspace.
type PurgedWorkspaceStats struct {
	WorkspacesDeleted     int
	StorageObjectsDeleted int
	StorageDeleteFailures int
}
