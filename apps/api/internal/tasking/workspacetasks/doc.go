// Package workspacetasks owns the workspace-scoped async job lifecycle
// (issue #805): Task as the single typed async job resource for workspace
// AI workflows.
//
// This package never writes the Credits ledger directly (only via the
// injected Holder seam), never enqueues outside the injected Enqueuer seam,
// and never publishes events inside the settlement transaction (staged,
// flushed only after commit; NATS fanout itself is #807).
//
// Persistence here is an in-memory store for unit tests. Production uses the
// PostgreSQL store and settles Quote -> Hold -> Task insert -> River enqueue
// in one shared transaction through the seams in settle_tx.go.
package workspacetasks
