# Frontend Design System PRD

Status: Draft
Audit date: 2026-07-01
Scope: `apps/web` design system inventory and migration plan only.

## 1. Current Inventory

### Discovery Sources

- `codebase-memory` project: `Users-sunshikang-Desktop-IdeaBoard`, status `ready`, 9372 nodes / 27252 edges.
- Primary design-system entry points found through `codebase-memory` and targeted file reads:
  - Global tokens and global skins: `apps/web/src/app/style.css`
  - PrimeVue theme and pass-through: `apps/web/src/app/primeTheme.ts`, `apps/web/src/app/primePassThrough.ts`, `apps/web/src/shared/theme/baseDialogPt.ts`
  - Admin tokens/helpers: `apps/web/src/shared/theme/adminThemeTokens.ts`, `apps/web/src/features/admin/shared/lib/adminUiClasses.ts`, `adminOverlayPt.ts`, `adminPanelPt.ts`, `adminDataTablePt.ts`, `adminCardPt.ts`, `adminChart.ts`
  - Admin shared components: `AdminManagementFrame.vue`, `AdminTablePanel.vue`, `AdminResourceToolbar.vue`, `AdminToolbarSearch.vue`, `AdminFilterPopover.vue`, `AdminPagerControls.vue`, `AdminFormField.vue`, `AdminSummaryCard.vue`, `AdminChartCard.vue`
  - Auth modal local system: `apps/web/src/features/auth/components/authModalUi.ts`, `AuthFormField.vue`, `LoginModal.vue`, `RegisterModal.vue`
  - Boards local system: `apps/web/src/features/boards/lib/boardCatalogSkin.ts`, `menuStyles.ts`, `BoardCard.vue`, `BoardsPage.vue`
  - Profile/User shell surfaces: `apps/web/src/features/profile/pages/ProfileCenterPage.vue`, `apps/web/src/features/user/layout/UserLayout.vue`, `UserShareJoinPage.vue`
  - Canvas boundary: `apps/web/src/features/canvas/pages/CanvasPage.vue`, `CanvasLandingPage.vue`, `components/styles/canvas-controls.css`, `components/nodes/styles/*`, `image-editor/components/styles/*`, `mask-editor/*`

### Existing Token Layers

- `style.css` uses Tailwind CSS v4 `@theme` and currently mixes:
  - Admin tokens: `--color-admin-*`, `--text-admin-*`, `--leading-admin-*`
  - Canvas tokens: `--color-canvas-*`
  - Boards tokens: `--color-board-*`
  - App-global controls: `--ib-control-*`, focus outline vars, avatar button vars, toast skin vars
- `adminThemeTokens.ts` mirrors many Admin colors for TypeScript consumers and PrimeVue theme mapping.
- `primeTheme.ts` maps PrimeVue semantic primary/surface/formField tokens mostly from `adminThemeTokens`.
- `primePassThrough.ts` applies global PrimeVue PT for `button`, `dialog`, `datatable`, `inputtext`, `select`, `tag`, `textarea`, etc.; some rules are admin-route scoped with `[body.admin-route_&]`.
- `CanvasPage.vue` defines a separate runtime CSS variable boundary under `.canvas-layout`, including surface, text, accent, motion, Vue Flow edge, minimap, and panel vars.

### Existing Local Systems

- Admin is the most mature conventional app surface:
  - Shared frame, toolbar, table, chart, filter, pager, form field, card, panel, overlay helpers exist.
  - Density is deliberate: small type, compact tables, high information density, tabular numeric font settings on `body.admin-route`.
- Auth modal has a compact local class system in `authModalUi.ts`:
  - Polished creative feel, canvas accent, translucent shell, custom typography, custom PrimeVue PT.
  - Not yet mapped to app semantic tokens beyond Canvas accent tokens.
- Boards catalog has a local skin object:
  - Page, header, toolbar, search, sort, cards, menu, delete dialog, loading, empty state styles are centralized in `boardCatalogSkin.ts` and `menuStyles.ts`.
  - Many colors/radius/shadow values are hardcoded even when related `--color-board-*` tokens exist.
- Profile center is visually complete but mostly inline:
  - `ProfileCenterPage.vue` uses many hardcoded hex colors, radius, shadow, gradient, and PrimeVue class overrides.
  - It duplicates form-field, nav button, card/panel, primary button, and error-state patterns already present in Auth/Admin.
- Canvas has strong local boundaries and interaction-specific CSS:
  - Node/tool/editor controls use CSS variables and component CSS.
  - Many fallback colors remain hardcoded, but Canvas behavior and interaction styling are high-risk and should not be broadly migrated in phase 1.

### Hardcoded Style Signal

Targeted `rg` patterns for hex / `rgba()` / arbitrary Tailwind radius, text, background, border, font, tracking, and shadow found this approximate concentration:

| Area | Approx. matches | Interpretation |
| --- | ---: | --- |
| `features/canvas` | 164 | Expected: interaction-heavy local CSS; phase 1 boundary only. |
| `app` | 134 | Mostly global tokens and global CSS; should become canonical contract. |
| `features/admin` | 80 | Admin is partly tokenized, but detail pages still carry one-off styles. |
| `shared` | 51 | Includes assets and shared theme helpers; review before treating as debt. |
| `features/boards` | 32 | Centralized but hardcoded local skin. |
| `features/profile` | 29 | High-value conventional surface with inline style debt. |
| `features/auth` | 11 | Small local system; good candidate for token bridge. |
| `features/user` | 5 | User shell has small but visible banner/share surface styling. |

### Visual Audit Evidence

- Local Vite service started successfully at `http://127.0.0.1:5173/`.
- Playwright checks completed:
  - `/admin/login`: rendered successfully. Snapshot showed login card, account/password fields, submit button, user-login button. Console had expected unauthenticated `/api/user/info` 401.
  - `/user/home`: rendered successfully. Snapshot showed current illustrative hero surface.
  - `/user/login`: rendered home background plus Auth login modal successfully. Snapshot showed modal title, fields, password toggle, submit, switch-to-register.
- Blocked by auth/session:
  - `/user/boards`, `/user/center`, `/user/canvas` redirected to `/user/login?redirect=...`.
  - `/admin/users` redirected to `/admin/login?redirect=/admin/users`.
  - Therefore Admin dashboard/table/form/dialog, Boards page, Profile center, and Canvas page visual conclusions are based on code audit, not authenticated screenshots.
- Temporary screenshots were generated for audit and removed from the repo to honor this planning-only, docs-only scope.

## 2. Problem Areas

1. No minimal cross-surface semantic token contract.
   - Admin, Canvas, Boards, Auth, Profile each has useful tokens/classes, but there is no small app-wide semantic layer for surface, text, border, focus, status, radius, shadow, and control states.
   - `style.css` is currently both global contract and feature-specific token bucket.

2. PrimeVue and local helpers have overlapping responsibilities.
   - `primeTheme.ts` maps Admin color semantics into PrimeVue globally.
   - `primePassThrough.ts` globally skins PrimeVue, but some rules are admin-route-specific and some are generic.
   - Feature-level PT objects (`authDialogPt`, `boardCatalogDeleteDialogPt`, `admin*Pt`) solve similar dialog/menu/popover problems with different naming and values.

3. Conventional app surfaces do not share enough primitives.
   - Admin has shared primitives.
   - Profile, Auth, Boards, and User shell repeat button, icon button, form field, card/panel, dialog, empty/loading/error, menu/popover, and toolbar patterns.
   - These surfaces are not interaction-specialized like Canvas, so they are safer and higher-return migration targets.

4. Visual density risks are asymmetric.
   - Admin must preserve compact operational density.
   - Auth/Boards/Profile can be calmer and more creative, but should not drift into marketing/hero patterns inside operational flows.
   - Canvas should keep interaction-specific affordances and motion.

5. Hardcoded arbitrary values make future design changes expensive.
   - Boards and Profile carry many inline hex/radius/shadow values.
   - Admin detail pages still use one-off radius/shadow/gradient classes despite shared helpers.
   - Canvas hardcoded values are mostly boundary/risk inventory for now, not migration targets.

6. State feedback is not consistently modeled.
   - Focus, loading, disabled, hover, selected, active, danger, empty, and error states exist but are implemented per surface.
   - There is no shared acceptance checklist for keyboard focus, density, mobile wrapping, or table overflow.

## 3. Design Direction

Target direction: quiet operational SaaS + creative workflow tool.

- Prioritize consistency, scanability, state feedback, and component reuse.
- Keep Admin quiet, dense, and operational. Do not add hero composition, decorative cards, or marketing-style empty states.
- Keep creative energy in Boards/Auth/Canvas entry points, but map color/radius/shadow/focus values to a shared semantic base.
- Treat Canvas as a specialized workspace with its own token boundary. Phase 1 only audits Canvas token boundary and risk.
- Prefer additive aliases and small migrations over destructive rewrites.
- Preserve existing frameworks: Vue 3, Tailwind CSS v4, PrimeVue unstyled/PT, Vue Flow, TanStack Query.

## 4. Scope Boundaries

### Phase 1 In Scope

- Design system inventory and PRD.
- Token contract proposal.
- Migration ticket backlog for Admin/Auth/Boards/Profile/User shell conventional surfaces.
- Canvas token boundary and risk audit only.
- Visual audit evidence where unauthenticated local pages are available.

### Phase 1 Out of Scope

- UI implementation.
- Business logic changes.
- Dependency changes.
- Backend/auth/db/generated API changes.
- Canvas large-scale visual migration.
- Canvas node/tool/tooltip/image editor/mask editor behavior changes.
- Replacing PrimeVue, Tailwind, Vue Flow, or TanStack Query.
- Adding a new UI framework.
- Bulk rewriting Tailwind classes.

## 5. Minimal Semantic Token Contract

This contract should be additive first. Do not delete existing `--color-admin-*`, `--color-canvas-*`, or `--color-board-*` tokens until a replacement is proven by screenshot QA.

### App-Level CSS Tokens

Define these in `apps/web/src/app/style.css` under `@theme` or `:root` depending on Tailwind utility needs:

| Token | Purpose | Initial mapping |
| --- | --- | --- |
| `--color-ib-page` | default app page background | `--color-admin-bg` or `#f8f9fd` after visual check |
| `--color-ib-surface` | normal panel/card surface | `#ffffff` |
| `--color-ib-surface-raised` | elevated/translucent surface | `--color-admin-card` / existing Auth/Boards white alpha |
| `--color-ib-surface-muted` | subtle container / table header / input bg | `--color-admin-surface-muted` |
| `--color-ib-surface-hover` | hover bg | `--color-admin-surface-hover` |
| `--color-ib-surface-pressed` | pressed/selected bg | `--color-admin-surface-pressed` |
| `--color-ib-border` | default border | `--color-admin-border` |
| `--color-ib-border-soft` | low-emphasis border | `--color-admin-border-soft` |
| `--color-ib-border-strong` | active/selected border | `--color-admin-border-strong` |
| `--color-ib-text` | primary text | `--color-admin-text-1` |
| `--color-ib-text-muted` | secondary text | `--color-admin-text-2` |
| `--color-ib-text-subtle` | tertiary/helper text | `--color-admin-text-3` |
| `--color-ib-text-inverse` | inverse text | `#ffffff` |
| `--color-ib-accent` | neutral brand/action accent | `--color-admin-accent` |
| `--color-ib-creative-accent` | creative workflow accent | `--color-canvas-accent` |
| `--color-ib-focus-ring` | shared focus ring | existing `--ib-focus-outline-color` / Admin focus ring per surface |
| `--color-ib-success` | success status | `--color-admin-success` |
| `--color-ib-warning` | warning status | `--color-admin-warning` |
| `--color-ib-danger` | danger status | `--color-admin-danger` |
| `--color-ib-info` | info status | `--color-admin-info` |
| `--radius-ib-control` | buttons/inputs | `10px` or `12px` depending surface density |
| `--radius-ib-panel` | panels/cards | `14px` / `16px` |
| `--radius-ib-overlay` | menus/popovers/dialogs | `14px` |
| `--shadow-ib-control` | subtle input/button shadow | current Admin toolbar shadow |
| `--shadow-ib-overlay` | menu/dialog shadow | current Admin/Boards overlay shadow |
| `--shadow-ib-panel` | raised panel/card shadow | current Admin card shadow |
| `--duration-ib-fast` | quick state transition | `160ms` |
| `--duration-ib-standard` | default transition | `200ms` |

### Surface-Specific Aliases

- Admin can keep `--color-admin-*` as high-density aliases. New shared primitives should use app tokens only when they do not reduce Admin density.
- Boards should keep `--color-board-*` as creative catalog aliases but map more hardcoded values to existing board/app tokens.
- Auth should keep modal-specific classes but source core color/radius/shadow/focus from app/creative aliases.
- Canvas should keep `.canvas-layout` tokens as the boundary. Only document equivalences to app tokens in phase 1.

### TypeScript Token Mirror

Future implementation may add a small `shared/theme/appThemeTokens.ts` only if TS consumers need the same values as CSS. Do not duplicate every CSS token in TS by default.

## 6. Component Pattern Gaps

| Pattern | Current state | Gap |
| --- | --- | --- |
| Button | Admin helpers and PrimeVue PT exist; Auth/Boards/Profile define local classes. | Need semantic variants: primary, secondary, text, danger, icon, toolbar icon, creative primary. |
| Icon button | Admin has framed/round/toolbar classes; Canvas has specialized icon buttons; Boards/Profile use one-offs. | Need conventional app icon button contract outside Canvas. |
| Form field | `AdminFormField` and `AuthFormField` exist; Profile repeats password field styles inline. | Need shared field anatomy rules: label, hint, error, `aria-describedby`, invalid state. |
| Dialog | `baseDialogPt`, `authDialogPt`, Boards delete dialog PT, Admin dialog helpers exist. | Need dialog shell/header/body/footer PT naming and size/density guidance. |
| Popover/menu | Admin overlay helpers and Boards menu helpers overlap. | Need overlay semantic classes for menu item, danger item, section header, compact width. |
| Card/panel | Admin cards/panels exist; Profile and Boards repeat cards. | Need panel/card surface tokens and guidance for information density. |
| Empty/loading/error state | Admin has loading/empty class strings; Boards has loading/empty; Auth/Profile errors local. | Need state patterns with consistent tone and `aria-live` guidance. |
| Toolbar/search/filter/pager | Admin has strong implementation; Boards toolbar is local. | Need reusable conventional toolbar anatomy that can be dense Admin or relaxed Boards. |
| Toast/notification | Global Sonner skin exists. | Need status color mapping and route-specific density guidance. |
| Chart | Admin chart theme exists with hardcoded colors. | Need chart palette source from Admin/app tokens without changing chart semantics. |

## 7. Migration Tickets

Tickets are sorted by expected benefit and reversibility.

### Ticket 1: Add Minimal App Semantic Token Aliases

Scope:
- Add additive `--color-ib-*`, `--radius-ib-*`, `--shadow-ib-*`, and `--duration-ib-*` aliases.
- Do not migrate large UI surfaces in this ticket.
- Optionally add `shared/theme/appThemeTokens.ts` only if a TS consumer needs it.

Likely touched files:
- `apps/web/src/app/style.css`
- Optional: `apps/web/src/shared/theme/appThemeTokens.ts`
- Optional: `apps/web/src/app/primeTheme.ts` if PrimeVue needs aliases without visual change.

Out of scope:
- Replacing existing Admin/Canvas/Boards tokens.
- Bulk Tailwind class rewrites.
- Canvas visual migration.

Acceptance criteria:
- Existing app renders visually unchanged on audited pages.
- New tokens are documented in comments or a short token map.
- Existing `--color-admin-*`, `--color-canvas-*`, and `--color-board-*` keep working.
- No new one-off color palette is introduced.

Verification command:
- `cd apps/web && npm run lint:style`
- `cd apps/web && npm run build`

Visual QA target:
- `/admin/login`
- `/user/home`
- `/user/login`
- Authenticated smoke screenshots for `/admin/statistics`, `/user/boards`, `/user/center`, `/user/canvas` when session is available.

Rollback note:
- Remove only the additive aliases and any unused imports; no migrated call sites should exist in this ticket.

### Ticket 2: Normalize Admin Shared Surface Boundary

Scope:
- Align Admin shared helpers and PrimeVue PT around the new app/Admin token boundary.
- Preserve Admin density, table compactness, and operational scanability.
- Reduce one-off Admin shadows/radius/overlays where equivalent helpers already exist.

Likely touched files:
- `apps/web/src/app/primePassThrough.ts`
- `apps/web/src/app/primeTheme.ts`
- `apps/web/src/shared/theme/baseDialogPt.ts`
- `apps/web/src/shared/theme/adminThemeTokens.ts`
- `apps/web/src/features/admin/shared/lib/adminUiClasses.ts`
- `adminOverlayPt.ts`, `adminPanelPt.ts`, `adminDataTablePt.ts`, `adminCardPt.ts`, `adminChart.ts`
- `apps/web/src/features/admin/app/AdminLayout.vue`
- `apps/web/src/features/admin/pages/login/AdminLoginPage.vue`

Out of scope:
- Changing Admin navigation structure.
- Redesigning Admin pages.
- Lowering information density.
- Backend/admin API changes.

Acceptance criteria:
- Admin shell, login, overlays, cards, dense tables, and dialogs use named helpers or semantic tokens for core surface/radius/shadow/focus values.
- Admin route typography and tabular numeric behavior remain intact.
- DataTable header/body density remains the same or tighter; no marketing-style spacing appears.
- PrimeVue global PT rules have clear generic vs admin-route responsibilities.

Verification command:
- `cd apps/web && npm run lint:style`
- `cd apps/web && npm run build`
- `cd apps/web && npm run test:run`

Visual QA target:
- `/admin/login`
- Authenticated `/admin/statistics`
- Authenticated `/admin/users` table, filter popover, pager, action popover
- Authenticated Admin form/dialog surface such as user ban dialog or storage form

Rollback note:
- Revert the Admin helper/PT changes as one small commit; page-level callers should remain compatible with previous helper exports.

### Ticket 3: Migrate Profile Center and User Shell to Conventional App Tokens

Scope:
- Replace Profile/User conventional surface hardcoded colors/radius/shadow/focus with app semantic tokens and small local helper constants.
- Preserve password/avatar/logout workflows and existing route behavior.
- Keep Profile visually calm and app-like, not Admin-dense and not landing-page-like.

Likely touched files:
- `apps/web/src/features/profile/pages/ProfileCenterPage.vue`
- `apps/web/src/features/user/layout/UserLayout.vue`
- `apps/web/src/features/user/pages/UserShareJoinPage.vue`
- Optional: new `apps/web/src/features/profile/profileUiClasses.ts`

Out of scope:
- Auth/session workflow changes.
- Avatar upload behavior changes.
- Password validation behavior changes.
- Admin impersonation behavior changes.

Acceptance criteria:
- Profile card, nav, form fields, password inputs, avatar actions, and action buttons source core style values from semantic tokens or named local classes.
- Error, disabled, loading, hover, active, and focus states remain visible.
- Mobile layout does not introduce text overlap or clipped controls.
- User impersonation banner remains prominent and compact.

Verification command:
- `cd apps/web && npm run lint:style`
- `cd apps/web && npm run build`
- `cd apps/web && npm run test:run`

Visual QA target:
- Authenticated `/user/center` desktop and mobile.
- Password form empty/error/loading/success states.
- Avatar upload button disabled/loading state.
- Impersonation banner in `UserLayout` if an admin impersonation session is available.

Rollback note:
- Revert Profile/User class extraction and token substitutions; business composables should be untouched.

### Ticket 4: Tokenize Boards Catalog and Menus

Scope:
- Map hardcoded Boards catalog colors, borders, menu danger states, loading state, create card, search, sort select, and delete dialog to `--color-board-*` and app semantic aliases.
- Preserve card preview rendering and board actions.

Likely touched files:
- `apps/web/src/features/boards/lib/boardCatalogSkin.ts`
- `apps/web/src/features/boards/lib/menuStyles.ts`
- `apps/web/src/features/boards/components/BoardCard.vue`
- `apps/web/src/features/boards/pages/BoardsPage.vue`
- `apps/web/src/app/style.css` only for additive board/app aliases if needed.

Out of scope:
- Board data sync behavior.
- Preview generation.
- Rename/duplicate/delete behavior.
- Canvas board navigation.

Acceptance criteria:
- Boards page still reads as creative workflow catalog, but core surface/text/border/focus/menu/delete values are token-backed.
- Create card and board cards preserve size, preview area, hover, focus, and rename input behavior.
- Menu and delete dialog use consistent overlay/danger semantics.
- Empty/loading states are accessible and visually consistent.

Verification command:
- `cd apps/web && npm run lint:style`
- `cd apps/web && npm run build`
- `cd apps/web && npm run test:run`

Visual QA target:
- Authenticated `/user/boards` with empty state, loading state, several board cards, search no-results, sort menu, board menu, delete dialog, rename input.
- Mobile width around 360px and desktop width around 1440px.

Rollback note:
- Revert token substitutions in Boards skin/menu files; board behavior code should not be touched.

### Ticket 5: Align Auth Modal With App/Creative Token Boundary

Scope:
- Keep Auth modal expressive but source its shell, input, focus, error, primary button, close button, and switch-link values from semantic tokens.
- Preserve existing modal transition and validation behavior.

Likely touched files:
- `apps/web/src/features/auth/components/authModalUi.ts`
- `apps/web/src/features/auth/components/AuthFormField.vue`
- `apps/web/src/features/auth/components/LoginModal.vue`
- `apps/web/src/features/auth/components/RegisterModal.vue`
- Optional: `apps/web/src/app/style.css` for auth-specific aliases if needed.

Out of scope:
- Login/register API behavior.
- Validation rule changes.
- Modal route behavior.
- Replacing current background/creative feel.

Acceptance criteria:
- Login and register modals still feel creative and lightweight.
- Input hover/focus/error states remain clear.
- Modal width, mobile padding, close button, and password toggle stay stable.
- No new hardcoded brand color is introduced where an app/creative token exists.

Verification command:
- `cd apps/web && npm run lint:style`
- `cd apps/web && npm run build`
- `cd apps/web && npm run test:run`

Visual QA target:
- `/user/login` and `/user/register` desktop/mobile.
- Empty-submit validation.
- Password visibility toggle.
- Switch between login/register.

Rollback note:
- Revert `authModalUi.ts` token substitutions; modal component logic should remain unchanged.

### Ticket 6: Fill Conventional Component Pattern Gaps

Scope:
- Add or consolidate small conventional primitives where repetition remains after tickets 2-5.
- Candidate primitives: app icon button, app field anatomy, overlay menu item classes, empty/loading/error state helpers, relaxed toolbar anatomy.
- Use existing Admin primitives as reference but keep Admin density separate.

Likely touched files:
- `apps/web/src/shared/theme/*`
- `apps/web/src/shared/components/*`
- `apps/web/src/features/admin/shared/components/*`
- `apps/web/src/features/boards/*`
- `apps/web/src/features/profile/*`
- `apps/web/src/features/auth/*`

Out of scope:
- New UI framework.
- Replacing PrimeVue.
- Global component registry.
- Bulk migration of every call site.

Acceptance criteria:
- Each new primitive has at least two real call sites or removes meaningful duplication.
- Props/classes are small and intention-revealing.
- Accessibility anatomy is explicit for fields, dialogs, menus, loading, and errors.
- Admin density is not diluted by relaxed app primitives.

Verification command:
- `cd apps/web && npm run lint`
- `cd apps/web && npm run lint:style`
- `cd apps/web && npm run build`
- `cd apps/web && npm run test:run`

Visual QA target:
- The first two migrated call sites per primitive.
- Desktop and mobile for any layout primitive.

Rollback note:
- Revert primitive and call-site migrations together; do not leave dead shared abstractions.

### Ticket 7: Canvas Token Boundary Audit and Risk Register

Scope:
- Produce a Canvas-specific token boundary map and risk register.
- Identify which Canvas variables are workspace-level, node-level, editor-level, mask-level, or Vue Flow integration-level.
- No broad visual migration in this ticket.

Likely touched files:
- `apps/web/src/features/canvas/CONTEXT.md` or a future Canvas design notes doc.
- Optional docs only: `docs/canvas-design-token-boundary.md`
- If code is absolutely needed, only additive comments or aliases in `CanvasPage.vue` / CSS files after visual QA approval.

Out of scope:
- Canvas node/tool/tooltip/image editor/mask editor interaction behavior changes.
- Re-skinning nodes, toolbars, Vue Flow pane, image editor, mask editor, or minimap.
- Bulk replacing CSS variables.

Acceptance criteria:
- Canvas token categories and ownership are documented.
- Known hardcoded fallback colors are listed by risk level.
- A future migration strategy distinguishes safe aliases from behavior-sensitive styles.
- No Canvas visual or interaction behavior changes are made.

Verification command:
- Docs-only: `git diff --check`
- If code comments/aliases are added: `cd apps/web && npm run lint:style && npm run build`

Visual QA target:
- Authenticated `/user/canvas` with existing board.
- Node selected/running states.
- Tool palette, zoom controls, board menu, context menu.
- Image editor and mask editor open/close, with no interaction changes.

Rollback note:
- Docs-only rollback removes the boundary note. If code aliases are added later, revert only additive alias/comment changes.

## 8. Acceptance Criteria

Phase 1 is complete when:

- `docs/frontend-design-system-prd.md` exists and contains inventory, problem areas, design direction, scope boundaries, token contract, component gaps, migration tickets, acceptance criteria, out-of-scope list, and agent prompt templates.
- No UI code, business logic, dependency, backend, auth, db, or generated API files are changed.
- Canvas is treated as token boundary/risk audit only.
- Migration tickets are ordered by benefit and include scope, likely touched files, out of scope, acceptance criteria, verification command, visual QA target, and rollback note.
- Visual audit blockers are recorded instead of inferred.
- Whitespace check passes.

Future migration work is acceptable only when:

- It is small, reversible, and tied to one ticket.
- It preserves Admin information density.
- It uses additive aliases before destructive rewrites.
- It includes screenshot QA for affected pages and states.
- It does not replace the frontend stack.

## 9. Out of Scope

- UI implementation in this phase.
- Canvas large-scale visual migration.
- Canvas node/tool/tooltip/image editor/mask editor behavior changes.
- Backend/auth/db/generated API changes.
- PrimeVue, Tailwind, Vue Flow, or TanStack Query replacement.
- New UI framework.
- Bulk rewrite of all Tailwind classes.
- Marketing/landing/hero redesign for conventional app surfaces.
- Sacrificing Admin density for visual uniformity.
- Deleting local systems before a replacement has screenshots and rollback.
- Persisting visual audit screenshot artifacts in the repo unless explicitly requested.

## 10. Agent Prompt Templates

### Audit Prompt

```text
你在 /Users/sunshikang/Desktop/IdeaBoard 工作。请审计 apps/web 某个 frontend surface 的 design system 一致性，只输出结论和建议，不改 UI 代码。

必须：
- 简体中文正文，Technical Terms 保持 English。
- 优先用 codebase-memory MCP 发现入口；不足时再用 rg。
- 读取 docs/frontend-design-system-prd.md、CONTEXT-MAP.md、相关 CONTEXT.md。
- 对照 Minimal Semantic Token Contract、Component Pattern Gaps、Scope Boundaries。
- Canvas 只审计 token boundary 和风险，不提大范围视觉迁移实现。
- 如能启动本地服务，用 Playwright 检查关键页面；缺少登录态/后端数据要记录 blocker，不伪造视觉结论。
- 最终列出：发现、风险、建议 ticket、验证建议、未覆盖页面。
```

### Single Ticket Implementation Prompt

```text
你在 /Users/sunshikang/Desktop/IdeaBoard 工作。请只实现 docs/frontend-design-system-prd.md 中的 Ticket <N>: <title>。

必须：
- 简体中文汇报，Technical Terms 保持 English。
- 只改 ticket scope 内文件；不要改 backend/auth/db/generated API，不改依赖。
- 不要 bulk rewrite Tailwind classes。
- Prefer additive aliases and small migrations over destructive rewrites.
- 保留现有交互、路由、API、validation、loading/error 行为。
- Admin 不降低信息密度；Canvas 不做大范围视觉迁移。
- 改动前先读 relevant files 和 PRD ticket。
- 改动后运行 ticket 指定 verification command。
- 如本地服务可启动，用 Playwright 做 ticket 指定 visual QA；缺登录态则记录 blocker。
- 最终说明改了什么、验证结果、visual QA 结果、rollback 方式。
```

### 不推荐的坏 Prompt 示例

```text
把整个 apps/web UI 统一成一个新的设计风格，所有 Tailwind class 都整理一下。
```

为什么不好：
- 范围过大，不可回滚。
- 会误伤 Canvas interaction CSS。
- 可能破坏 Admin density。
- 没有 token boundary、visual QA target、rollback note。

```text
重做 Canvas、Admin、Auth、Boards，让它们看起来完全一样。
```

为什么不好：
- 把 workspace、operational SaaS、auth modal、catalog page 的不同任务强行合并。
- 违反 Canvas 第一阶段只审计 boundary 的约束。
- “完全一样”会牺牲可扫描性和上下文适配。

```text
换一个 UI framework，顺便统一组件。
```

为什么不好：
- 明确违反不替换 PrimeVue/Tailwind/Vue Flow/TanStack Query 和不新增 UI framework 的约束。
- 会引入大规模 dependency、behavior 和 accessibility 风险。
