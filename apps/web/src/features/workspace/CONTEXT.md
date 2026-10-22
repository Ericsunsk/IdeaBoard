# Workspace Shell Context

Workspace Shell owns user-side route assembly, shell layout, global navigation, and shell account controls.
Public `/user/*` URLs are an external product contract and stay unchanged (not a compat shim).
It does NOT own business content (owned by submodules like `board`) or identity concept components like `UserAccountMenu` in `shared/components`.
Shape: flat assembly layer (`layout/` + `routes/` + `shell/` + `pages/` + `selection/` for selection state) — it assembles submodule content and owns no tenant lifecycle rules of its own.

## Domain Language

| Term                       | Meaning                                                                                                                                                                     |
| -------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Workspace Layout           | Main wrapper for user-side routes (`WorkspaceLayout.vue`): top navigation, global feedback containers (Toast / Confirm Dialog), main content slot.                          |
| Workspace Routes           | User-side route assembly containing public, guest, and authenticated destinations; each route declares its own access requirement.                                          |
| Workspace Navigation       | Post-login top/main nav (`WorkspaceNavigation.vue`): board entry, inspiration search, Idea balance, avatar, global settings menu.                                           |
| Workspace Account Controls | Top-level account interaction orchestration (`useWorkspaceAccountControls.ts`): routes clicks to `accountOverlay.open(tab)`, sign-out cleanup (`signOut`), session refresh. |

The Workspace Shell is a frontend composition boundary. A tenant Workspace is owned by backend Tenancy; Canvas Workspace names the editor interaction surface. These are not interchangeable identities.

## Public Seams

- `workspaceRoutes: RouteRecordRaw[]` — full user-side route record array in `routes/workspaceRoutes.ts`; mounted by the global Vue Router with dynamic `import()` lazy loading.
- `WorkspaceLayout.vue` — page shell in `layout`: injects `WorkspaceNavigation`, manages viewport height/width and scrollbars; no page-specific business state.
- `WorkspaceNavigation.vue` — top nav bar in `shell/components`: logo, route highlight, Idea balance widget, avatar and dropdown; collapsed-menu expansion is local interaction state.
- `useWorkspaceAccountControls.ts` — top-bar user behavior controller in `shell`: `openAccountTab`, `handleSignOut`, `userMenuItems`; aggregates Auth Store and Account Overlay into declarative menu items.
- `workspace/index.ts` — sole cross-feature entry for workspace.
- `fetchWorkspaceMembers` / `workspaceMembersQueryOptions` — canonical roster transport, parser and Query options shared by Workspace settings and Board Share. Each entry has one public identity, a strict membership role and optional display-only email/avatar; no consumer owns a second roster parser or cache.

## Invariants

- Private routes require authentication; login/register are guest routes, while Search and the auth callback are public. Route URLs remain stable.
- Business content comes from submodules (e.g. `board`); `workspace` only assembles and shells.
- The nav bar only raises intents; recharge or billing must go through `accountOverlay`, never inline billing DOM.
- Sign-out must clear sensitive local caches and redirect to the landing or login page.
- Nav items render dynamically by current user role, including the admin entry.
- Never duplicate global containers or the global nav bar in business pages.
- Admin routes remain in the Admin route table; public Search, auth callback, and legal routes retain their explicit access metadata.
- Never call deep auth-store mutations directly from nav components.
- Creating a tenant Workspace preserves the current route and selected Workspace; settings focus the new row locally. Explicit navigation owns scope changes and must drop a stale Canvas Board ID.
- Workspace creation is scope-only (ADR-0038): personal creates resolve to the caller's Personal account with no new wallet and no billing choice, org scopes always use the org pool, and the create dialog states the fixed scope before submit. `provisionWorkspace` carries name plus optional organization scope and the dedupe key covers it, so the same name in a different scope never replays.
- Organization setup is explicit in Workspace creation: resolve the caller's single Organization (including one with no Workspace), create the Organization and its funds account as one server transaction, then create its first Workspace as a separate visible step. Workspace creation errors retain the selected Organization for retry; Organization scope never falls back to personal funding.
- Organization scope switching (#951): Personal and the caller's single Organization switch via `organization/scope` (persisted scope, URL workspace param still owns nav). The Organization itself is the management entry (no boards/content/consumption); `organization/ui/OrganizationPanel` lists/invites/removes members and changes Member/Admin roles (owner-only for admin, owner immutable outside handover, second-org joins fail closed). The shell exposes the panel opaquely to the account settings dialog through `UserAccountMenuWorkspace.organizationPanel` (the account feature never imports workspace). Workspace access follows the role mapping (owner/admin automatic, member explicit per-workspace, non-member hidden until join); leaving removes all org Workspace grants while content stays org-owned.
- Organization closure (#955, ADR-0038): owner-only close requires typed confirmation and permanently ends organization access, management, recharge, spending, and remaining balance use. There is no restore entry or API, automatic refund, or Personal transfer; legal requirements and explicit purchase commitments remain applicable. In-flight tasks/search operations, payment orders, refunds, disputes, or pending handover block with `organization_closure_blocked`. Billing and closure audit facts remain; the shell drops back to Personal on close.
- Workspace settings shows the member cross-workspace quota (#946): self usage visible to every member, per-member limit management gated to billing managers off the same consumption scope (workspace admins cannot raise a pool-granted quota; the backend enforces 403). Personal collaboration (#952): the Personal Default workspace is private (its member section hides the invite bar and shows the private note; invites render 403 `workspace_default_protected`), other Personal workspaces invite/remove collaborators directly with no per-invite holder approval, collaborator usage charges the holder's shared Personal account attributed to the real operator, one quota aggregates that member across all Personal workspaces of the account, absent rows read as unlimited, and removal/rejoin preserves quota state plus history.
- Member limits have no account-default editor or invite prerequisite (ADR-0038): an absent collaborator quota reads as unlimited, no quota row is seeded on join, and explicit caps are set through the member-quota editor.

## Non-goals

- No business state in layout; no page-owned data facts.
- No direct billing DOM in navigation.
- No ownership of `shared/components` identity components.
