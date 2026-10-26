// Package funds is the deep funds-identity kernel (ADR-0038).
//
// It answers "whose money does this consumption spend" in one call inside
// the caller's transaction: workspace (ws_) to stable Billing Account (ba_).
// Resolution order is fixed at creation scope - the Organization pool for
// org workspaces, the owner's deterministic Personal survivor (the
// Default-workspace-anchored account) for personal workspaces - and fails
// closed on missing or ownerless scopes. There is no owner-wallet fallback,
// no per-workspace mint, and no link indirection.
//
// Transaction discipline lives in the unified borrow-or-begin module
// (dbutil.Begin/BorrowedQueries, the same seam
// settlement.BorrowOrBeginScope delegates to); this package mints no
// transaction and refuses no lifecycle itself. ResolveFunds borrows the ambient carrier when one is
// staged, or begins an owned transaction otherwise; ResolveFundsInTx runs
// statements only, never committing or rolling back the caller's unit.
//
// What it deliberately does NOT own (by design, not a gap):
//   - Prices stay with Billing (Quote seam), ledger writes stay with Credits
//     (Commands seam). Callers pass those seams in; funds only fixes WHO
//     pays, never how much or how the ledger moves.
//   - Row SQL shapes stay with sqlc (dbsqlc.* is the only insert primitive;
//     CreateBillingAccountStub is the sole billing_accounts insert, as
//     before). Funds owns the ORDER and the fail-closed guards, never the
//     column list.
//   - Authorization stays with the callers (workspace membership gates,
//     Billing Owner/Admin checks, closed-organization gates). Funds never
//     grants visibility; hidden workspaces fail closed as invalid before any
//     account is touched.
//
// Zero funding-semantics change:归属规则、共享/独立语义、Default 锚、org 池
// 与 settlement 行为一致. Personal/Organization 资金独立不变式仍由调用顺序
// 之外的固定顺序保证, handler 不再分支 workspace/org 映射.
package funds
