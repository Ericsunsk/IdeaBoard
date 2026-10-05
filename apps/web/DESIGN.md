# Frontend UI Agent Protocol

This file is agent-facing. It is the UI/design execution protocol for changes under `apps/web`.

It is not a product design essay, system architecture doc, API contract, or local-dev guide.

## Read Before UI Work

1. Read this file for UI rules.
2. Read [../../DESIGN.md](../../DESIGN.md) for system contracts, Frontend Context Map, Architecture Contract, and DDD layering rules.
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

`src/app/style.css` is the token source of truth. Do not duplicate token tables in docs or components.

- Use existing tokens before adding new ones.
- Use semantic tokens, not raw color intent: prefer `--color-canvas-accent` over a hard-coded purple.
- Map feature-local CSS variables back to existing app/surface tokens.
- Add global tokens only for stable app-level or surface-level semantics.
- DO NOT put page-specific styling in `src/app/style.css`.
- DO NOT hard-code brand colors, shadows, z-index stacks, or complex transitions inside components unless narrowly local and justified.

Token ownership:

- `--color-admin-*`, `--text-admin-*`, `--leading-admin-*`: Admin.
- `--color-canvas-*`: Canvas nodes, tools, overlays, image interactions.
- `--color-board-*`: Boards.
- Canonical semantic tokens (`--color-background`, `--color-card`,
  `--color-foreground`, `--color-border`, `--color-primary`,
  `--color-muted`, and their state variants): shared UI semantics.
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
  (`boardPreviewStyle.ts`, `BoardCard.vue` preview gradients).
- Private resolution infrastructure: `--color-*` declarations in
  `src/app/style.css` / `src/shared/theme/tokens/dark.css` and the
  `adminThemeTokens.ts` / `taskMetricTokens.ts` bridges. Tier-3 standard
  tokens (`bg-card`, `text-foreground`, `border-border`, `bg-primary`, …)
  resolve directly from canonical semantic tokens; no product-prefixed alias
  tier is permitted.

Type scale (`text-admin-*` sizes) is unaffected by the color-token freeze.
Dark mode ships through token resolution (`html.dark` overrides in
`src/shared/theme/tokens/dark.css`); `dark:` overrides stay banned because
semantic tokens already cover both modes.

Out of freeze scope, by design:

- Elevation (`shadow-page-chrome-*`, `shadow-auth-*`, `shadow-board-*`):
  shadow tokens are elevation, not color. Components may keep domain-owned
  elevation tokens; generic `shadow-ib-*` aliases are forbidden.
- Tier-3 dual source: `src/app/style.css` `@theme` and
  `src/shared/theme/tokens/semantic-light.css` (`@layer base` `:root`) both
  declare standard names (`--color-primary`, `--color-foreground`, …), and the
  same holds for canvas tokens (`@theme` vs `tokens/canvas.css`).
  Verified against the built bundle: `@layer base` wins over `@layer theme`,
  so the token files are the effective runtime values and `@theme` keeps the
  keys for utility generation. Token edits must update both sides; collapsing
  to a single tier is deferred cleanup, not a freeze violation.

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

Enforcement: `src/features/admin/shared/lib/textRampContrast.contract.spec.ts`.
A token edit that drops any role below its floor, collapses two rungs, breaks an
alias, or desynchronizes `adminThemeTokenFallbacks` from `style.css` fails the
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
`searchKeyword` uses on `BoardsPage`. `pageChrome.contract.spec.ts` fails if a
`v-model` reappears there.

## Touch Targets: 32px Floor, 24px For Drag Handles

WCAG 2.2 SC 2.5.8 (Target Size Minimum, AA) asks for 24x24 CSS px. This repo holds a
stricter 32px floor, declared once as `--spacing-touch-target-min` in
`src/app/style.css` and enforced by `npm run lint:touch-target`
(`scripts/checkTouchTargetContract.ts`).

Controls that are visually small by design do not inflate to 32px. Instead the _hit
area_ grows past the visual box through `src/shared/theme/atoms/touch-target.css`:

| Class                   | Grows to           | Use for                                                                        |
| ----------------------- | ------------------ | ------------------------------------------------------------------------------ |
| `.hit-target-32`        | 32px both axes     | switch tracks, canvas zoom buttons, compact toolbar buttons                    |
| `.hit-target-32-inline` | 32px vertical only | chip rails, chart range segments, node action buttons — rows keep their rhythm |
| `.hit-target-drag`      | 24px (AA floor)    | drag handles only — see below                                                  |

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

Default to feature-local components.

Promote to `src/shared` only when:

- At least two different features use it.
- The semantics are stable and UI/domain-neutral.
- It hides reusable accessibility, keyboard, or overlay (`reka-ui`) behavior.
- Its props do not leak feature payloads or feature-specific flags.

Admin local shared is different: `src/features/admin/shared/*` may serve all Admin vertical contexts without becoming global shared.

DO NOT promote a component just because two places look similar. Similar visuals with different behavior stay local.

## Directory Structure & File Naming Governance

Follow the system architecture contract in [../../DESIGN.md](../../DESIGN.md) for full DDD details. In `apps/web`, all modules MUST adhere to:

- **Directories**: `kebab-case` (e.g., `board/`, `image-tools/`, `workspace-runtime/`).
- **Vue SFC Components**: `PascalCase.vue` (e.g., `BoardCard.vue`, `CanvasScreen.vue`).
- **Non-component TS Modules**: `camelCase.ts` (e.g., `useCatalog.ts`, `contract.ts`, `feed.ts`; do NOT use PascalCase for non-component files).
- **Anti-Stuttering Rule**: MUST NOT repeat the parent directory name inside filenames (e.g., use `resource.ts` instead of `libraryResource.ts`, use `curves.ts` instead of `retouchCurves.ts`). Use named exports from the feature's root `index.ts` to disambiguate.
- **Public API & Boundary Isolation**: Features that expose cross-feature APIs MUST use a root `index.ts`; route- or parent-owned internal features may keep explicit internal entrypoints. Outside modules MUST NOT deep-import private files or internal components across feature boundaries.
- **Tiered DDD**: Complex core domains (`board`, `canvas`, `search`, `account`, `admin`) use layered structures (`domain/`, `application/`, `infrastructure/`, `presentation/` or `ui/`). Lightweight features stay flat.

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
