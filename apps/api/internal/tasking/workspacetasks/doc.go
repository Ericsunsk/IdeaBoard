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
// sibling. The settlement Service still proves the same all-or-nothing
// contract the PG transaction will enforce: Quote -> Hold -> Task insert
// -> River enqueue; any failure compensates all prior steps.
package workspacetasks
