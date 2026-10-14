# Workspace Shell Context

Workspace Shell owns user-side route assembly, shell layout, global navigation, shell account controls, and the share-join entry flow.
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
| Share Join Page            | Collaboration share-join flow (`WorkspaceShareJoinPage.vue`): parses `share_token` from URL, runs the board join workflow, navigates to canvas.                             |

The Workspace Shell is a frontend composition boundary. A tenant Workspace is owned by backend Tenancy; Canvas Workspace names the editor interaction surface. These are not interchangeable identities.

## Public Seams

- `workspaceRoutes: RouteRecordRaw[]` — full user-side route record array in `routes/workspaceRoutes.ts`; mounted by the global Vue Router with dynamic `import()` lazy loading.
- `WorkspaceLayout.vue` — page shell in `layout`: injects `WorkspaceNavigation`, manages viewport height/width and scrollbars; no page-specific business state.
- `WorkspaceShareJoinPage.vue` — collaboration invite landing in `pages`: validates token, shows confirmation, executes join via the `board` domain `joinSharedBoardWorkflow` without building its own transport.
- `WorkspaceNavigation.vue` — top nav bar in `shell/components`: logo, route highlight, Idea balance widget, avatar and dropdown; collapsed-menu expansion is local interaction state.
- `useWorkspaceAccountControls.ts` — top-bar user behavior controller in `shell`: `openAccountTab`, `handleSignOut`, `userMenuItems`; aggregates Auth Store and Account Overlay into declarative menu items.
- `workspace/index.ts` — sole cross-feature entry for workspace.

## Invariants

- Private routes require authentication; login/register are guest routes, while Search and the auth callback are public. Route URLs remain stable.
- Business content comes from submodules (e.g. `board`); `workspace` only assembles and shells.
- The nav bar only raises intents; recharge or billing must go through `accountOverlay`, never inline billing DOM.
- Sign-out must clear sensitive local caches and redirect to the landing or login page.
- Nav items render dynamically by current user role, including the admin entry.
- Never duplicate global containers or the global nav bar in business pages.
- Admin routes remain in the Admin route table; public Search, auth callback, and legal routes retain their explicit access metadata.
- Never call deep auth-store mutations directly from nav components.
- Never scatter collaboration-join logic across generic pages.

## Non-goals

- No business state in layout; no page-owned data facts.
- No direct billing DOM in navigation.
- No ownership of `shared/components` identity components.
