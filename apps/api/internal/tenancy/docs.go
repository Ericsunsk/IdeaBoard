// Package tenancy owns the Workspace, Organization, and membership
// lifecycle (issue #801) plus stable Billing Account resolution and
// financial management (ADR-0038).
//
// Repository methods take context.Context first. The PostgreSQL adapters
// use dbWithContext(ctx) so reads and writes join the caller transaction.
//
// Billing Account model (ADR-0038): the stable funds identity holds the
// unified balance; one Personal account exists per user and one shared
// account per Organization. Workspace scope is fixed at creation and
// never switches. Resolution joins the caller transaction and fails
// closed on missing or ownerless scopes; joining a Workspace never merges
// personal funds. Financial Owner/Admin manages recharge and bills.
// Personal member quotas require the account holder; Organization quotas
// follow the Organization Owner/Admin role policy. Workspace Owner/Admin
// standing alone grants no financial management, and financial management
// grants no Board content access.
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
// The durable backend lives in infrastructure/postgres and implements Store
// against sqlc (never hand-edit internal/db/sqlc/*). Its OrganizationStore
// owns Organization creation, membership, Workspace creation, and permanent
// closure transactions; the composite Store embeds it alongside Workspace
// lifecycle. Organization mutations lock the Organization before checking
// state or roles, and all reads/writes join the same caller transaction.
// Workspace row creation is a transaction-local persistence primitive shared
// by Personal and Organization creation, not a nested lifecycle call.
package tenancy
