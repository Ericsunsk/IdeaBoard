// Package tenancy owns the Workspace, Organization, and membership
// lifecycle (issue #801).
//
// The durable schema (organizations, workspaces,
// organization_memberships, workspace_memberships) is created by a sibling
// stream; this package programs to the AGREED interface via the Store
// interface below so handlers and tests do not block on it. Repository
// methods take context.Context first and the future postgres
// implementation must use the dbWithContext(ctx) pattern.
//
// Personal workspace rule (agreed): organization_id IS NULL marks personal
// scope. Every user additionally owns exactly one live Default personal
// workspace (is_default): it is never deleted, archived, or renamed,
// and is auto-provisioned when missing (EnsureDefaultWorkspace, concurrent-
// safe via ux_workspaces_default_owner). Plain personal workspaces are
// unlimited. Provisioning is idempotent (INSERT ... ON CONFLICT DO NOTHING
// in postgres) and the in-memory fallback is mutex-guarded so concurrent
// provision converges to a single Default workspace.
//
// The in-memory store (memory.go) stays at the top level next to the Store
// interface as its test-only double; it must never be wired in production.
// The durable backend lives in infrastructure/postgres and swaps it out by
// implementing Store against sqlc (never hand-edit internal/db/sqlc/*).
package tenancy
