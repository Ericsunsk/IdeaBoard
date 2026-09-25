// Package workspacetasks owns the workspace-scoped async job lifecycle
// (issue #805): Task as the single typed async job resource for workspace
// AI workflows.
//
// Additive only: legacy submit/readiness/matrix/admin-duplicate routes are
// untouched (removal is #809). This package never writes the Credits ledger
// directly (only via injected Holder seam), never enqueues outside the
// injected Enqueuer seam (River in production, fake in tests), and never
// publishes events inside the settlement transaction (staged, flushed only
// after commit; NATS fanout itself is #807).
//
// Persistence here is an in-memory store mirroring the #802 boards pattern:
// production postgres mapping (workspace_tasks_v2 link + billing snapshot
// join) is a documented #809 follow-up; until it lands, HTTP handlers fail
// closed with 501 when the store is nil and tests inject the in-memory
// sibling. The settlement Service settles transaction-first: when the
// store, quoter, holder, and queue expose their Tx seams (see settle_tx.go)
// the whole boundary commits in ONE shared PostgreSQL transaction;
// backends without those seams (the in-memory sibling, stub adapters)
// keep the compensating contract: Quote -> Hold -> Task insert -> River
// enqueue with Release/Delete rollback on failure.
package workspacetasks
