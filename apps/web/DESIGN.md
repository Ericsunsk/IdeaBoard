# Frontend UI Agent Protocol

This file is agent-facing. It is the UI/design execution protocol for changes under `apps/web`.

It is not a product design essay, system architecture doc, API contract, or local-dev guide.

## Read Before UI Work

1. Read this file for UI rules.
2. Read [../../DESIGN.md](../../DESIGN.md) for system contracts when the UI change crosses feature/API/backend boundaries.
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
- `--color-ib-*`: cross-feature semantic aliases.
- `--color-ib-auth-*`: Auth/User Shell.

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

## PrimeVue And Tailwind

- PrimeVue usage defaults to `unstyled`.
- Use Tailwind utilities for layout, spacing, typography, and simple states.
- Use PrimeVue `pt` helpers or wrapper components for PrimeVue customization.
- For Admin, prefer `src/features/admin/shared/components/*` and `src/features/admin/shared/lib/*Pt.ts`.
- Use feature-local CSS for complex Canvas node/tool/editor interactions, but reference tokens.
- DO NOT target PrimeVue generated DOM classes from route pages.
- DO NOT copy table, popover, dialog, toolbar, or chart styling into each Admin page.

## Component Placement

Default to feature-local components.

Promote to `src/shared` only when:

- At least two different features use it.
- The semantics are stable and UI/domain-neutral.
- It hides reusable accessibility, keyboard, overlay, or PrimeVue `pt` behavior.
- Its props do not leak feature payloads or feature-specific flags.

Admin local shared is different: `src/features/admin/shared/*` may serve all Admin vertical contexts without becoming global shared.

DO NOT promote a component just because two places look similar. Similar visuals with different behavior stay local.

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
4. Did you use PrimeVue unstyled with `pt`/wrappers instead of DOM overrides?
5. Did you keep feature-only components out of `src/shared`?
6. Did you cover loading, empty, error, disabled, hover, focus, active, selected, and destructive states?
7. Did you check mobile and desktop for overflow, overlap, and layout shift?
8. Did you provide accessible names, labels, focus states, and keyboard paths?
9. Did destructive actions include confirmation and recovery/audit clarity?
10. Did you run the smallest relevant check from [../../DEVELOPMENT.md](../../DEVELOPMENT.md)?
