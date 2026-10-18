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
// Personal workspace rule (agreed): organization_id IS NULL +
// unique(owner_user_id) where archived_at IS NULL. Provisioning is
// idempotent (INSERT ... ON CONFLICT DO NOTHING in postgres) and the
// in-memory fallback is mutex-guarded so concurrent provision converges
// to a single workspace.
//
// The in-memory store (memory.go) stays at the top level next to the Store
// interface as its test-only double; it must never be wired in production.
// The durable backend lives in infrastructure/postgres and swaps it out by
// implementing Store against sqlc (never hand-edit internal/db/sqlc/*).
package tenancy
