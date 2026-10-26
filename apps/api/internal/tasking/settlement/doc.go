// Package settlement is the deep settlement kernel for paid operations.
//
// It owns the settlement discipline every money movement routes through:
//   - SettlementState validation (the only home for free/held/captured/
//     released checks; the durable CHECK constraints stay the final gate).
//   - CaptureSplit math (per-output unit billing capped at the held total).
//   - Ledger idempotency-key derivation (durable-claim-scoped task keys and
//     operation-scoped vision keys; byte-identical outputs, single home).
//   - Borrow-or-begin transaction discipline on the dbutil interface
//     (BorrowOrBeginScope joins the ambient carrier or opens the store's
//     own Scope; ownership travels with Scope.Owned, lifecycle with
//     Scope.Commit/Scope.Rollback, and post-commit deferral with
//     StageOrDefer on the dbutil registry; this package mints no
//     transaction and refuses no lifecycle itself).
//
// What it deliberately does NOT own (by design, not a gap):
//   - Prices stay with Billing (Quote seam), ledger writes stay with Credits
//     (Commands seam). Callers pass those seams in; settlement only fixes
//     their order (Quote -> quota -> Hold -> persist -> enqueue) and guards.
//   - Row SQL stays at the owning stores (task_billing_* vs
//     ai_search_operations have different shapes); settlement validates the
//     state words those statements persist, never their columns.
//   - Workspacetasks keeps its SettlementTransactor shape on the Scope
//     seam (fakes, worker shims, and the postgres Store return Scopes);
//     settlement converges the helpers around it.
package settlement
