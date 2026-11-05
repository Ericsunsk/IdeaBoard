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
`vertical` 指 admin-only 纵切（admin 各垂直域），非通用分层术语；`control` 出现处写全称（UI control / Control API / Control State）。
`session/` 为分层目录名（zone：core/session/ui），非领域 Session 概念；与租户/领域 Session 同名时以路径为准。

## Public Seams

- `workspaceRoutes: RouteRecordRaw[]` — full user-side route record array in `routes/workspaceRoutes.ts`; mounted by the global Vue Router with dynamic `import()` lazy loading.
- `WorkspaceLayout.vue` — page shell in `layout`: injects `WorkspaceNavigation`, manages viewport height/width and scrollbars; no page-specific business state.
- `WorkspaceNavigation.vue` — top nav bar in `shell/components`: logo, route highlight, Idea balance widget, avatar and dropdown; collapsed-menu expansion is local interaction state.
- `useWorkspaceAccountControls.ts` — top-bar user behavior controller in `shell`: `openAccountTab`, `handleSignOut`, `userMenuItems`; aggregates Auth Store and Account Overlay into declarative menu items.
- `workspace/index.ts` is the default cross-feature entry. Account menu composition uses the exact `ui/settings/index.ts` and `ui/dialogs/index.ts` public subpaths declared in the [Seam inventory](../../../../../docs/frontend-seam-inventory.md), avoiding a cycle through the shell barrel.
- The settings dialog owns the Member and Limits sections, while Workspace creation uses its local dialog. Settings and menu consumers use the explicit UI subpaths above rather than duplicate root-barrel exports.
- `fetchWorkspaceMembers` / `workspaceMembersQueryOptions` — canonical roster transport, parser and Query options shared by Workspace settings and Board Share. Each entry has one public identity, a strict membership role and optional display-only email/avatar; no consumer owns a second roster parser or cache.

Member Quota settings composition belongs to `session/useMemberQuotaSettings.ts`: it owns identity-scoped Organization membership reads, combined read readiness and retry, editor drafts, unlimited confirmation, and closing after a successful save. `WorkspaceLimitsSection` renders its state and dispatches intents. Account retains quota facts, target selection and labels, validation, role policy, write authorization, feedback, and invalidation through `useMemberQuotaSection`; Workspace supplies Organization facts without redefining that policy.

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
- Workspace creation is scope-only (ADR-0038, superseded Personal funding rules in ADR-0042): no new wallet or billing choice is created, and the create dialog states the scope before submit. Personal consumption uses the submitting Actor's own Personal Billing Account; Organization consumption uses the org pool. `provisionWorkspace` carries name plus optional organization scope and the dedupe key covers it, so the same name in a different scope never replays.
- Organization setup is explicit in Workspace creation: resolve the caller's single Organization (including one with no Workspace), create the Organization and its funds account as one server transaction, then create its first Workspace as a separate visible step. Workspace creation errors retain the selected Organization for retry; Organization scope never falls back to personal funding.
- Organization scope switching (#951): Personal and the caller's single Organization switch via `organization/scope` (persisted scope, URL workspace param still owns nav). The Organization itself is the management entry (no boards/content/consumption); `organization/session/useManagement` owns identity-scoped Organization/member/invitation reads, capabilities, setup/rename/member/closure commands, feedback and fact refresh; `organization/ui/OverviewPanel` renders that interface. The module lists/invites/removes members and changes Member/Admin roles (owner-only for admin, owner immutable outside handover, second-org joins fail closed). The shell exposes the panel opaquely to the account settings dialog through `UserAccountMenuWorkspace.organizationPanel` (Account consumes only the declared Workspace UI seams and does not resolve Organization facts or roles). Shell retains only the gallery navigation adapter after scope exit. Workspace access follows the role mapping (owner/admin automatic, member explicit per-workspace, non-member hidden until join); leaving removes all org Workspace grants while content stays org-owned.
- Organization closure (#955, ADR-0038): owner-only close requires typed confirmation and permanently ends organization access, management, recharge, spending, and remaining balance use. There is no restore entry or API, automatic refund, or Personal transfer; legal requirements and explicit purchase commitments remain applicable. In-flight tasks/search operations, payment orders, refunds, disputes, or pending handover block with `organization_closure_blocked`. Billing and closure audit facts remain; the shell drops back to Personal on close.
- Workspace settings shows the Organization member cross-workspace quota (#946): self usage is visible to every Org member; the server capability gates management, and authoritative Organization membership determines the actor/target matrix (Owner sets Admin, Owner/Admin set Member, Admin never touches Owner/Admin; Owner remains unlimited). Direct Workspace roles do not grant or narrow Organization quota authority; unknown or failed role reads disable writes, and the backend enforces 403. Personal collaboration (#952, ADR-0042): the Personal Default workspace is private (its member section hides the invite bar; invites return 403 `workspace_default_protected`); other Personal workspaces invite/remove collaborators directly without per-invite holder approval. Each collaborator pays from their own Personal Billing Account, with no Personal quota editor or check; in-flight and terminal settlement retains the submitted account and Actor facts after removal.
- Organization member limits have no account-default editor or invite prerequisite (ADR-0038): an absent Org member quota reads as unlimited, no quota row is seeded on join, and explicit caps are set through the member-quota editor. Limit changes and removal/rejoin preserve usage and history.

## Non-goals

- No business state in layout; no page-owned data facts.
- No direct billing DOM in navigation.
- No ownership of `shared/components` identity components.
