# Frontend UI Agent Protocol

This file is agent-facing. It is the UI/design execution protocol for changes under `apps/web`.

It is not a product design essay, system architecture doc, API contract, or local-dev guide.

## Read Before UI Work

1. Read this file for UI rules.
2. Read [../../DESIGN.md](../../DESIGN.md) for system contracts, Frontend Context Map, Architecture Contract, and feature-first layout rules.
3. Read [../../DEVELOPMENT.md](../../DEVELOPMENT.md) for commands and code entry points.
4. Read [src/features/canvas/CONTEXT.md](./src/features/canvas/CONTEXT.md) for Canvas domain language.

## Surface Decision

MUST choose the target surface before styling.

| Surface          | Tone                                      | Prefer                                                                                | Avoid                                                             |
| ---------------- | ----------------------------------------- | ------------------------------------------------------------------------------------- | ----------------------------------------------------------------- |
| Canvas Workspace | Creative tool, calm, precise, image-first | Compact nodes, clear tool states, low-interruption overlays                           | Marketing hero layout, decorative copy, form-heavy UI             |
| Retouch          | Instrument panel, exact, reversible       | Stable controls, clear preview/commit distinction, readable histograms/sliders/curves | Dramatic visuals over control hierarchy                           |
| Admin            | Operational, dense, scannable, restrained | Tables, filters, cards, dialogs, charts using Admin shared patterns                   | Hero sections, large decorative cards, illustration-heavy layouts |
| Boards           | Gallery + file manager                    | Clear previews, quiet metadata, stable card actions                                   | Hover layout shift, operations detached from board identity       |
| Auth/User Shell  | Low-friction, trustworthy                 | Clear forms, stable avatar/menu/header controls, direct errors                        | Brand showcase pages, noisy visual effects                        |

## Hard UI Rules

- MUST design loading, empty, error, disabled, hover, focus, active, selected, and destructive states.
- MUST keep fixed-format UI stable: boards cards, toolbars, icon buttons, node controls, sliders, tables, tiles.
- MUST preserve image-first hierarchy on Canvas, Boards, and result views.
- MUST keep Admin dense and scannable; do not turn operational pages into marketing pages.
- MUST provide confirmation and clear copy for destructive actions.
- DO NOT add decorative gradients, large hero treatments, or one-off visual systems to tool surfaces.
- DO NOT reduce readability, information density, or workflow speed for visual novelty.

## Tokens

`src/theme/theme.css` is the token source of truth: the sole Tailwind import,
token imports in dependency order
(`primitives` → `semantic` → `admin` → `canvas` → `motion` → `type` → `z`),
the dark variant, the `@theme inline` bridge that maps standard semantic
`--color-*` utility keys to runtime roles, and the explicit `type-*` role
utilities. `src/theme/tokens/semantic.css` owns the canonical Tier 2 runtime
roles plus Tier 2 domain dark assignments; `src/theme/tokens/admin.css` owns
the Admin-distinct Tier 2b roles (light `@theme` + dark base); `src/theme/tokens/canvas.css`
owns the Tier 3 canvas runtime (light + dark); `src/theme/tokens/motion.css`,
`type.css`, and `z.css` own motion, type, and stacking roles.
`src/app/style.css` is the entry named by `components.json`; it imports
theme, base, and atoms, and owns app-wide utilities, toast
styling, and shell rules. Do not duplicate token tables in docs or components.

Enforcement: `npm run lint:token-graph` (`scripts/checkTokenGraph.ts`) gates
token graph integrity (defined, acyclic, down-tier only), exact color-role
mode parity, and measured contrast floors; `src/theme/tokens/contracts/tokenGraph.contract.spec.ts`
holds the positive/negative fixtures. Authored color ingredients belong to
Tier 1; Tier 3 roles reference Tier 2 only, without retained-value exemptions.
Normal action text (including hover/pressed) clears 4.5:1; chart marks and
sidebar focus rings clear 3:1; body/card/sidebar text clears 7:1.

- Use existing tokens before adding new ones.
- Use semantic tokens, not raw color intent: prefer `--color-canvas-accent` over a hard-coded purple.
- Map feature-local CSS variables back to existing app/surface tokens.
- Add global tokens only for stable app-level or surface-level semantics.
- DO NOT put page-specific styling in `src/app/style.css`.
- DO NOT hard-code brand colors, shadows, z-index stacks, or complex transitions inside components unless narrowly local and justified.

Token ownership:

- Tier 1 (`tokens/primitives.css`, `:root` only): mode-blind physical color
  ramps, radii, shadows, and the `palette-light-*` / `palette-dark-*`
  ingredient scales under distinct names. Never `.dark` overrides here.
- Tier 2 (`tokens/semantic.css` + `theme.css` bridge): canonical shadcn roles
  (`--background`, `--card`, `--foreground`, `--border`, `--primary`,
  `--muted`, `--chart-*`, `--sidebar-*`, and state variants) with explicit
  light + dark assignments; Tier 2 domain dark assignments (brand, board,
  auth, toast, overlay, page-chrome, home, scrollbar, mask) with light values
  - utility keys in the `theme.css` `@theme` bridge. Bare names are
    the runtime roles; the matching `--color-*` names exist only as
    `@theme inline` utility keys (`bg-primary`, `text-foreground`, …).
- Tier 2b (`tokens/admin.css`): Admin-distinct operational density,
  table/dialog/popover chrome, chart, and status roles with bounded Admin
  consumer scope (see ADR-0016).
- Tier 3 (`tokens/canvas.css` + feature skins): canvas-scoped roles
  referencing Tier 2 only, including workspace, tool, text, tooltip, and
  progress roles.
- Motion (`tokens/motion.css` + `core/motion/motionPolicy.ts`): single
  control/enter/exit/feedback/image policy (160/220/220/160/200ms); CSS, JS, and
  reduced-motion stay in sync.
  Reka overlays share the mask/dialog/popover/drawer classes in
  `theme/atoms/overlay-motion.css`; distinct enter/exit keyframes retain
  content until exit finishes and disable animations under reduced motion.
- Type (`tokens/type.css` + `type-*` utilities in `theme.css`): family-only
  `--font-*`, size `--text-*` (full Admin variant ladder kept), explicit
  `type-{display,title,body,caption,code}` shorthands. `--font-sans`/`--font-admin`/
  `--font-display` resolve to Inter Variable (fontsource, Latin subset,
  `font-display: swap`, declared in `tokens/type.css`) with the PingFang SC →
  Microsoft YaHei → Noto Sans CJK SC → system chain; mono stays on the system
  stack. Typography roles use 400/500/600; Admin strong/emphasis/display use
  600, with their existing size and leading variants.
- Stacking (`tokens/z.css`): native `--z-index-*` roles emitting `z-*`
  utilities (`z-admin-dialog`, `z-banner`, …); ascending numeric order is the
  contract.
- Radius: shared `control`, `panel`, `overlay`, and `pill` roles for every
  surface. Use `rounded-control` for controls and menu items, `rounded-panel`
  for cards and popovers, `rounded-overlay` for dialogs, and `rounded-pill`
  for fully rounded shapes. Canvas clipping and nested instrument geometry
  may use local radii when required by their shape.
- Shadows: Tier 2 resting/floating roles (`--shadow-focus-ring`,
  `--shadow-floating`, `--shadow-raised`, admin/board/auth tiers); mode-varying
  shadows carry explicit dark assignments.
- `--color-admin-*`, `--text-admin-*`, `--leading-admin-*`: Admin.
- `--color-canvas-*`: Canvas nodes, tools, overlays, image interactions.
- `--color-board-*`: Boards.
- Canonical semantic tokens (`--background`, `--card`,
  `--foreground`, `--border`, `--primary`,
  `--muted`, and their state variants): shared UI semantics.
- `--color-auth-*`: Auth/User Shell.
- `--color-page-chrome-*`: shared page navigation and account chrome.
- `--color-overlay-*`: shared menus and contextual overlays.
- `--color-toast-*`: toast surface adapter.
- `--color-brand-creative-*`: expressive brand accent ramp.

### Legacy Token Decommission (Contract)

Component-layer usage of legacy domain color tokens is frozen and enforced
by the `legacy-token` rule in `scripts/checkStyleGovernance.ts`: any new
`admin-*` / `board-*` color utility or direct
`var(--color-admin/board/...)` reference outside the pinned infrastructure
files fails the Web Quality lane, and shrinking a pinned count trips the
stale-allowance check. The former `ib` color alias tier is removed entirely;
`--color-ib-*`, `--text-ib-*`, and the generic `shadow-ib-*` aliases are
forbidden in source and token declarations. Structural utility/class names such
as `.control-state`, `.round-control`, and `.page-dot-bg` remain separate
CSS behavior contracts.

Deliberately kept, not omissions:

- Domain color systems: task status palette (`status.ts`), credit health
  presentation mapping, auth shell skin (`modalUi.ts`,
  `RegisterAuthPanel.vue`), canvas chrome identity (`canvas-chrome.css`,
  pinned by `canvasSurfaceSkin.contract.spec.ts`), board thumbnail bake
  (gallery `skin.ts` empty-card surface, `BoardCard.vue` preview gradients).
- Private resolution infrastructure: runtime role declarations in
  `src/theme/tokens/semantic.css` / `src/theme/tokens/admin.css` / `src/theme/tokens/canvas.css`,
  utility keys in `src/theme/theme.css`, and the
  `adminThemeTokens.ts` / `taskMetricTokens.ts` bridges. Tier-3 standard
  tokens (`bg-card`, `text-foreground`, `border-border`, `bg-primary`, …)
  resolve through the bridge to the canonical semantic roles; no
  product-prefixed alias tier is permitted.

Type scale (`text-admin-*` sizes) is unaffected by the color-token freeze.
Dark mode ships through token resolution (`html.dark` overrides in the tier
owners: `tokens/admin.css`, `tokens/semantic.css`, `tokens/canvas.css`);
`dark:` overrides stay banned because
semantic tokens already cover both modes.

Out of freeze scope, by design:

- Elevation (`shadow-page-chrome-*`, `shadow-auth-*`, `shadow-board-*`):
  shadow tokens are elevation, not color. Components may keep domain-owned
  elevation tokens; generic `shadow-ib-*` aliases are forbidden.
- Domain utility values use plain `@theme`; Canvas declarations and dark mode overrides live in
  `tokens/canvas.css` and the tier-owner dark blocks (`tokens/admin.css`,
  `tokens/semantic.css`). Existing Canvas declarations
  are pinned by `token-alias`/`dark-base-closure`; no new duplicates may be added.

### Text Color Ramp Standard

Every surface names its text ramp by **role**, never by ordinal. Ordinal names
(`text-1/2/3`) force a renumber whenever a level is added or removed, and they
invite the mistake of reading a higher number as "more important". Role names
stay meaningful as the ramp grows.

| Role             | Use for                                        | Minimum ratio |
| ---------------- | ---------------------------------------------- | ------------- |
| `text-primary`   | Headings, body copy, the main reading color    | 7:1 (AAA)     |
| `text-secondary` | Labels, supporting copy, field values          | 4.5:1 (AA)    |
| `text-label`     | Group headings / fieldset legends (Admin only) | 4.5:1 (AA)    |
| `text-tertiary`  | Captions, metadata, hints                      | 4.5:1 (AA)    |
| `text-muted`     | Decorative or placeholder text only            | 4.5:1 (AA)    |
| `text-disabled`  | Inactive controls (WCAG exempt, no floor)      | none          |
| `text-inverse`   | Text on saturated/dark fills                   | 4.5:1 (AA)    |

Rules:

- Minimums are measured against the **darkest surface the role actually rests
  on**, not the page background. A caption inside a muted chip is not a page-level
  caption.
- Small type does not get a lower bar. `text-tertiary` at 11px still needs 4.5:1.
- `text-muted` may only be used when the information is also available elsewhere
  (adjacent label, `aria-label`, or an icon with an accessible name). It is not a
  licence to render unreadable text.
- Rungs must be monotonically lighter and visually distinct. Adding a rung that
  lands within 0.95x luminance of its neighbor is rejected — it reads as a bug,
  not a hierarchy.
- Each ramp declares its binding (darkest resting) surface. The contract test
  walks the full role × surface matrix, compositing alpha values the way the
  browser does.

Enforcement: `src/features/admin/shared/core/textRampContrast.contract.spec.ts`.
A token edit that drops any role below its floor, collapses two rungs, breaks an
alias, or desynchronizes `adminThemeTokenFallbacks` from `theme/theme.css` fails the
web test lane.

Current ramps, measured on each ramp's binding surface:

| Ramp   | primary | secondary | label | tertiary | muted | Binding surface               |
| ------ | ------- | --------- | ----- | -------- | ----- | ----------------------------- |
| Admin  | 13.0:1  | 5.6:1     | 6.8:1 | 4.8:1    | —     | `--color-admin-surface-hover` |
| Canvas | 11.7:1  | 5.4:1     | —     | —        | 4.7:1 | `--color-canvas-workspace-bg` |
| Boards | 14.8:1  | 5.4:1     | —     | —        | 4.9:1 | `--color-board-page`          |

`--color-canvas-text-control` and `--color-canvas-text-option` alias the Canvas
ramp rather than declaring standalone greys; Canvas previously carried five greys
of which two were the same value. Shared text uses the canonical semantic ramp
(`--color-foreground`, `--color-muted-foreground`, and state variants); do not
reintroduce product-prefixed text aliases.

## Style Ownership

Keep style ownership explicit. A selector should live at the narrowest layer that can own it without leaking implementation details.

| Layer                    | Owns                                                                                                                   | Avoid                                                                    |
| ------------------------ | ---------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------ |
| App global               | Tailwind entry, app tokens, base reset, browser/app shell rules, tiny vendor-neutral mechanical resets                 | Feature/page skins, component variants, Admin visual semantics           |
| Third-party adapter skin | Vendor DOM that cannot be styled through props or pass-through APIs, such as Vue Flow pane/node/edge/minimap internals | Product business rules or component-private classes                      |
| Surface skin             | Stable surface-level layout and visual language for Canvas, Admin, Boards, Auth/User Shell, or Retouch                 | Cross-surface tokens that only one feature understands                   |
| Feature CSS contract     | Classes intentionally shared by several components inside one feature, such as Canvas node/tool contracts              | Styling private child implementation from unrelated parents              |
| Component private style  | Markup and states owned by one component                                                                               | Selectors relied on by parents unless promoted to a feature CSS contract |

Rules:

- `:deep(...)` is an escape hatch. Use it for third-party or adapter DOM only, and keep those selectors behind a named adapter-skin boundary where practical.
- Business components must not add new `:deep(...)` selectors to style their own child components. Use props, explicit classes, slots, or CSS variables instead.
- `:global(...)` is allowed for browser/app shell state, vendor skin, or documented feature CSS contracts. Do not use it for page-local convenience.
- Stable raw colors, shadows, motion durations/easing, radii, and z-index stacks should be semantic tokens. Local raw values are acceptable only when they are one-off geometry, canvas drawing constants, test fixtures, or documented vendor fallbacks.
- Route pages may compose layout and bind state, but repeated visual primitives should move to feature-local shared components or helpers before being copied.

## UI Primitives And Tailwind

- User shell primitives live in `src/components/ui/*` (`Button`, `Dialog`,
  `Input`, `Popover`, `DropdownMenu`, `Select`, `Card`, `Badge`, `Avatar`, `Skeleton`, `Spinner`),
  built on official `shadcn-vue` standards (`reka-ui` + `cva` + `@/lib/utils` `cn`). No PrimeVue dependency remains.
- Use Tailwind utilities for layout, spacing, typography, and simple states.
- Use micro primitives (`DialogRoot`/`DialogContent`/…) composed explicitly per caller
  with `cn(dialogContentBaseClass, …)` for widths and surfaces. The old managed
  `Dialog` (`:visible`, `:content-class`, size maps) is deleted; do not reintroduce it.
- For Admin, prefer `src/features/admin/shared/components/*` (`AdminDialogShell`,
  `AdminTable`, …) and `src/features/admin/shared/lib/*`.
- Use feature-local CSS for complex Canvas node/tool/editor interactions, but reference tokens.
- DO NOT target third-party generated DOM classes from route pages.
- DO NOT copy table, popover, dialog, toolbar, or chart styling into each Admin page.
- Dialog widths are keyed by stable `data-size` (`slim/compact/form/medium/wide`) in
  `src/app/style.css` for `AdminDialogShell`; user-shell dialogs set their own
  widths via `cn(dialogContentBaseClass, …)` + style.

## Select: One Primitive, Per-Surface Skin

Every value picker on both surfaces mounts `@/components/ui/select`. That module owns
structure and the keyboard model — arrow keys, typeahead, Home/End, Escape, and a
correct `role="combobox"` / `aria-expanded` pair. It must stay neutral: it carries only
the shadcn default geometry in `selectVariants.ts` and may not reference a surface
token. `npm run lint:admin-dropdown` fails the build if it does.

Surfaces own their skin and adapt on top:

| Surface    | Adapter                | Skin source                           |
| ---------- | ---------------------- | ------------------------------------- |
| Admin      | `AdminSelect.vue`      | `admin/shared/lib/adminSelectSkin.ts` |
| User shell | call `Select` directly | caller's class props                  |

**Command menu vs value picker.** An icon trigger that immediately runs an action
(`UserLocaleSelect`) is a command menu and belongs on `DropdownMenu`. A control that
holds a chosen value (Boards sorting, Admin status filters) is a value picker and
belongs on `Select`. Never declare `aria-haspopup="menu"` on a value picker, and never
hand-roll `role="listbox"` over a `Popover` — that advertised a composite widget with no
keyboard navigation.

**Wrappers must forward a live binding.** `Select.vue` uses
`useForwardPropsEmits` plus a `computed` wrapper, matching `Popover.vue` and
`DropdownMenu.vue`. A plain `v-bind="cleanProps(props)"` freezes `modelValue` at mount
and the option list silently never opens. Local variant props (`size` on the trigger,
`inset` on the item) must be stripped before forwarding, or they render as stray
attributes on the underlying element.

**Readonly viewModels need commands.** Pages destructure `controller.viewModel`, whose
fields are all `readonly(readonly(ref))`. `v-model` on one compiles, renders, and
silently drops the write — the page looks right until a setting stops responding.
Bind `:model-value` and forward through the command instead, the same shape
`searchKeyword` uses on `BoardsPage`. `npm run lint:retirement-replacements`
(`readonly-controller` rule in `scripts/checkRetirementReplacements.ts`) fails if a
`v-model` reappears there.

## Touch Targets: 32px Floor, 44px Coarse, 24px For Drag Handles

WCAG 2.2 SC 2.5.8 (Target Size Minimum, AA) asks for 24x24 CSS px. This repo holds a
stricter 32px floor for fine-pointer controls and a 44px floor for ordinary
coarse-pointer/mobile controls, declared once as `--spacing-touch-target-min` (32px)
and `--spacing-touch-target-coarse` (44px) in
`src/theme/theme.css` alongside the precision (`--spacing-touch-target-aa`,
24px) profile, and enforced by `npm run lint:touch-target`
(`scripts/checkTouchTargetContract.ts`), which resolves CSS-owned profiles and verifies floors and pointer routing.

Controls that are visually small by design do not inflate to 32px. Instead the _hit
area_ grows past the visual box through `src/theme/atoms/touch-target.css`:

| Class                   | Grows to           | Use for                                                                        |
| ----------------------- | ------------------ | ------------------------------------------------------------------------------ |
| `.hit-target-32`        | 32px both axes     | switch tracks, canvas zoom buttons, compact toolbar buttons                    |
| `.hit-target-32-inline` | 32px vertical only | chip rails, chart range segments, node action buttons — rows keep their rhythm |
| `.hit-target-drag`      | 24px (AA floor)    | drag handles only — see below                                                  |
| `.hit-target-control`   | coarse profile     | real boxes in compact Admin icon and table row action families                 |

The ordinary `32` overlays select the fine profile by default and the coarse
profile under `(pointer: coarse)` (44px hit bounds on touch); the inline variant grows the block axis
only. `.hit-target-control` applies the coarse profile as a real minimum box
size so neighboring controls participate in layout. Precision handles use an
independent 24px profile.

The pseudo-element belongs to the control itself, so pointer and keyboard
activation still route to the real element and the accessible name, role, and
focus ring are untouched. Prefer a real 32px box (`min-h-8`, `!size-8`) when the
visual can afford it; use the overlay only where the small size is the design.

**Drag handles stop at 24px.** Curve points and histogram markers must stay
visually small (editing precision), their handles can sit pixels apart (a 32px
overlay would steal neighbouring grabs), and both are fully keyboard-operable
(arrow keys move, Delete removes). When even the pseudo-element is taken — the
histogram marker paints its dot on `::after` and its stem on `::before` — grow
the transparent button box itself instead.

## Listbox: Declare It Only If You Implement It

A `role="listbox"` promises the APG composite model: one Tab stop, arrow keys
between options, Home/End to jump. Two places in this repo declared the role
without the model:

- `AdminOptionFilter` hand-rolled `role="listbox"` over a `Popover` with no arrow
  keys at all — replaced with the shared `Select` primitive.
- Retouch `LayersPanel` gave every `role="option"` its own `tabindex="0"` with
  only Enter/Space bound — now roving tabindex (`LayersPanel.component.spec.ts`
  pins single-tab-stop, arrows, Home/End, no-wrap, and Enter/Space).

If a list cannot offer arrow-key navigation, do not give it `listbox` semantics.
Render plain buttons and let Tab do the work.

## Form Inputs: One Contract, Two Skins

User-side inputs (everything except the search bar) and Admin inputs share one
contract and differ only in tokens and density. The structure is identical on
both sides: a neutral primitive, a surface skin, a field shell with persistent
error text, and an invalid state that survives hover and focus.

| Layer       | User                                                | Admin                                           |
| ----------- | --------------------------------------------------- | ----------------------------------------------- |
| Primitive   | `@/components/ui/input` (`Input`, `PasswordInput`)  | same primitive                                  |
| Skin        | `ibFormInputClass` (h-10) + `ibFormInputErrorClass` | `adminInputChromeClass` + `!border-destructive` |
| Field shell | `IbFormField`                                       | `AdminFormField`                                |
| Error text  | persistent `<p role="alert">` below                 | persistent `<p>` below                          |

Three rules keep the two sides from diverging again (`formControls.contract.spec.ts`
enforces them):

1. **The primitive stays neutral.** It must not set a focus border or an invalid
   border — every skin owns those. The primitive once shipped
   `focus-visible:border-ring`, which tied at (0,2,0) with the skins' solid
   accent border; stylesheet order then decided whether focus showed a stroke or
   just a faint glow.
2. **Border and glow share one modality.** Both ride `focus-visible`, so mouse
   and keyboard focus look identical.
3. **Error beats hover.** The base chrome's hover border (0,2,0) otherwise beats
   a plain error border (0,1,0) and resting the mouse on an invalid field hides
   the error. Both skins declare the error border `!`-important for this reason.

Supporting rule: `cn()` (`src/lib/utils.ts`) registers the domain color
namespaces (`admin-*`, `board-*`, `page-chrome-*`, and `overlay-*`) in
tailwind-merge's `border-color` group, so `cn()` keeps the last border class
deterministically instead of leaving base and error tied for stylesheet order
to decide. Generic `ib-*` color aliases are intentionally absent.

Errors never render as placeholder text: a placeholder vanishes on first
keystroke and is not exposed as the accessible error. `IbFormField` shows every
error — including required-field errors — as persistent text below the control.

## Component Placement

Default to feature-local components. Inside a tool scope migrated to the
SaaS layout (ADR-0013), controls default to control-colocated `ui/` directories
(one directory per control with its SFC, local helpers, and specs), not to
shared or kind-grouped folders.

Promote to `src/shared` only when:

- At least two different features use it.
- The semantics are stable and UI/domain-neutral.
- It hides reusable accessibility, keyboard, or overlay (`reka-ui`) behavior.
- Its props do not leak feature payloads or feature-specific flags.

Admin local shared is different: `src/features/admin/shared/*` may serve all Admin vertical contexts without becoming global shared.

DO NOT promote a component just because two places look similar. Similar visuals with different behavior stay local.

- Component events use `update:` for v-model sync and `onXxx` for callbacks.
- Boolean props start with `is`, `can`, or `has`.

## Directory Structure & File Naming Governance

Follow the system architecture contract in [../../DESIGN.md](../../DESIGN.md) for the feature-first layout. In `apps/web`, all modules MUST adhere to:

- **Directories**: `kebab-case` (e.g., `board/`, `image-tools/`, `workspace-runtime/`).
- **Vue SFC Components**: `PascalCase.vue` (e.g., `BoardCard.vue`, `CanvasScreen.vue`).
- **Non-component TS Modules**: `camelCase.ts` (e.g., `useCatalog.ts`, `contract.ts`, `feed.ts`; do NOT use PascalCase for non-component files).
- **Anti-Stuttering Rule**: MUST NOT repeat the parent directory name inside filenames (e.g., use `resource.ts` instead of `libraryResource.ts`, use `curves.ts` instead of `retouchCurves.ts`). Use named exports from the feature's root `index.ts` to disambiguate.
- **Public API & Boundary Isolation**: Features that expose cross-feature APIs MUST use a root `index.ts`; route- or parent-owned internal features may keep explicit internal entrypoints. Outside modules MUST NOT deep-import private files or internal components across feature boundaries.
- **Feature-first layout**: Complex feature domains (`board`, `canvas`, `search`, `account`, `admin`) use the `core/` / `session/` / `ui/` zones with control-colocated views and one `index.ts` seam per scope (see Tool-Scoped SaaS Layout below). Lightweight features stay flat.

### Tool-Scoped SaaS Layout (core / session / ui)

Per [ADR-0013](../../docs/adr/0013-frontend-feature-first-saas-layout.md),
each migrated tool/feature scope (reference: `features/canvas/tools/retouch/`)
is split into three zones:

- **`core/` — pure math and types only.** Framework-free: no `vue`
  reactivity, no router/Pinia/TanStack Query, no `generated-query` DTOs. The
  only zone where domain purity is enforced.
- **`session/` — stateful orchestration and adapters.** `useXSession`,
  `useXMode`, coordinate/histogram/stage adapters, chrome sources, apply
  pipelines, runtime render handoffs. May import `vue` and `core/`; owns
  lifecycle, reactivity, and IO boundaries.
- **`ui/` — one directory per control.** Controls colocate by control
  (`curves/`, `levels/`, `histogram/`), each with its SFC, local helpers, and
  specs (`.(unit|contract|component|browser).spec.ts`). Cross-control
  contracts stay at the `ui/` root or in `core/`.
- **`index.ts` seam.** The scope's only legal cross-scope exit: named
  re-exports of model types, session entry, apply/chrome factories, and the
  chrome component. Scope internals stay private behind it; no deep imports
  across tool scopes.

## State, Responsive, Accessibility

- Focus visible must be clear and must not cause layout shift.
- Disabled state must remain readable and explain unavailable actions when needed.
- Loading state must preserve layout size.
- Empty state must point to the next useful action.
- Error state must give a recovery path, not only red styling.
- Mobile/desktop layouts must not overflow, overlap, or hide primary actions.
- Long text must wrap, truncate, or move into a menu/overlay intentionally.
- Tables on narrow screens need explicit scroll, stack, or card behavior.
- Icon-only buttons need `aria-label` or an equivalent accessible name.
- Non-obvious icon actions need tooltips.
- Inputs need labels and error text; do not rely only on placeholders.
- Color must not be the only state signal.

## Final Checklist

Before finishing UI work:

1. Did you choose the correct surface and follow its tone?
2. Did you reuse existing tokens/components before adding new ones?
3. Did you keep page-specific CSS out of `src/app/style.css`?
4. Did you use `components/ui` micro primitives composed explicitly instead of DOM overrides?
5. Did you keep feature-only components out of `src/shared`?
6. Did you cover loading, empty, error, disabled, hover, focus, active, selected, and destructive states?
7. Did you check mobile and desktop for overflow, overlap, and layout shift?
8. Did you provide accessible names, labels, focus states, and keyboard paths?
9. Did destructive actions include confirmation and recovery/audit clarity?
10. Did you run the smallest relevant check from [../../DEVELOPMENT.md](../../DEVELOPMENT.md)?
