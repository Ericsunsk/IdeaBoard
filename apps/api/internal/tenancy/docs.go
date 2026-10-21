// Package tenancy owns the Workspace, Organization, and membership
// lifecycle (issue #801) plus stable Billing Account resolution and
// financial management (issue #915).
//
// The durable schema (organizations, workspaces,
// organization_memberships, workspace_memberships, billing_accounts,
// billing_account_members, workspace_billing_links) is created by a sibling
// stream; this package programs to the AGREED interface via the Store
// interface below so handlers and tests do not block on it. Repository
// methods take context.Context first and the future postgres
// implementation must use the dbWithContext(ctx) pattern.
//
// Billing Account model (#915): the stable funds identity holds the unified
// balance; many Workspaces may share one account (org pool or explicit
// no-org studio link), independent accounts stay isolated. Resolution joins
// the caller transaction and fails closed on missing, ambiguous, or
// conflicting ownership; joining a Workspace never merges personal funds.
// Financial Owner/Admin manages recharge, bills, quotas, and budgets;
// Workspace Owner/Admin standing alone grants no financial management, and
// financial management grants no Board content access. Ownership is
// transferable, never lifetime-bound.
//
// The durable schema (organizations, workspaces,
// organization_memberships, workspace_memberships) is created by a sibling
// stream; this package programs to the AGREED interface via the Store
// interface below so handlers and tests do not block on it. Repository
// methods take context.Context first and the future postgres
// implementation must use the dbWithContext(ctx) pattern.
//
// Personal workspace rule (agreed): organization_id IS NULL marks personal
// scope. Every user additionally owns exactly one Default personal
// workspace (is_default): it is never deleted or renamed,
// and is auto-provisioned when missing (EnsureDefaultWorkspace, concurrent-
// safe via ux_workspaces_default_owner). Plain personal workspaces are
// unlimited. Provisioning is idempotent (INSERT ... ON CONFLICT DO NOTHING
// in postgres) and the in-memory fallback is mutex-guarded so concurrent
// provision converges to a single Default workspace.
//
// Deletion is an immediate irreversible hard delete (DeleteWorkspace):
// the workspace row leaves with its boards and files in one operation.
// Billing records, the credit ledger, user rows and control audit records
// are retained. There is no archive/restore lifecycle and no background
// purge.
//
// The in-memory store (memory.go) stays at the top level next to the Store
// interface as its test-only double; it must never be wired in production.
// The durable backend lives in infrastructure/postgres and swaps it out by
// implementing Store against sqlc (never hand-edit internal/db/sqlc/*).
package tenancy
