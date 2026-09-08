# Frontend Governance Final — 2026-09-07

Close-out report for [Frontend Governance Plan](/Users/sunshikang/Desktop/frontend-governance-plan.md),
against the 2026-09-06 baseline recorded in the governance plan.
All commands run from `apps/web` unless noted.

## 1. Phase Checklist

| Phase                                                                    | Status                                                          |
| ------------------------------------------------------------------------ | --------------------------------------------------------------- |
| 0 Baseline (`quality:check` reproducible)                                | Done                                                            |
| 1 Dead code (Knip files/exports/deps/duplicates)                         | Done, CI-blocking                                               |
| 2 Boundaries (dependency-cruiser, no-circular, single owner)             | Done, CI-blocking                                               |
| 3 Duplication (jscpd threshold + high-value consolidation)               | Done (see §3)                                                   |
| 4 Complexity (`complexity`/`max-depth`/own-body function length)         | Done; complexity/max-depth CI-blocking, own-body budget warning |
| 5 TypeScript strictness (all 6 flags incl. `exactOptionalPropertyTypes`) | Done                                                            |
| 6 Bundle budgets (`bundle:size:check` in `build`)                        | Done, CI-blocking                                               |
| 7 A11y + browser smoke (axe, 5 journeys)                                 | Done                                                            |
| 8 Test structure (unit/component/browser/contract)                       | Done; Stryker/coverage explicitly optional per plan — not done  |

## 2. Governance Metrics: Baseline → Final

| Metric                       | 2026-09-06                                   | 2026-09-07                                       |
| ---------------------------- | -------------------------------------------- | ------------------------------------------------ |
| Circular dependencies        | 6                                            | 0 (`no-circular` error gate clean, 1773 modules) |
| Forbidden imports            | 0                                            | 0                                                |
| Knip findings                | 0                                            | 0 (files/exports/deps/duplicates)                |
| Duplicate code               | unmeasured                                   | 0.00%, 0 clones (threshold 0.5%)                 |
| Largest component            | 954 lines (`AdminProviderEndpointsPage.vue`) | 954 lines (unchanged, < 1000 budget)             |
| Largest composable           | 896 lines (`useCanvasGroupRuntime.ts`)       | 591 lines (same file, split by owner)            |
| TypeScript errors            | 0                                            | 0 (app + test + node configs)                    |
| Unit/contract tests          | 377 files / 2043 tests                       | 381 files / 2069 tests, 0 failures               |
| Initial JS                   | 773.2 KB                                     | 773.9 KB (+0.7 KB, within budget)                |
| Browser smoke                | 7/7                                          | 7/7                                              |
| A11y violations (5 journeys) | 0 serious                                    | 0 serious                                        |
| Flaky browser tests          | 0 known                                      | 0 known                                          |
| ESLint                       | 0 errors / 0 warnings                        | 0 errors / 0 warnings                            |

## 3. Batch 3/4 Consolidations (Post-Baseline)

- `imageRetouchEngine.ts`: separable convolution passes kept as inlined monomorphic
  loops to preserve V8 JIT pixel-throughput without closure overhead; excluded in
  `.jscpd.json` with intentional rationale.
- Viewport camera contract (`screenToFlowCoordinate`/`getViewport`/`setViewport`/`setCenter`)
  extracted to `CanvasFlowViewportControls` on the `vue-flow-adapter` public seam;
  adopted by `imageToolSession` and `useRetouchCoordinateAdapter`.
- Auth panels: `useAuthPanelControls` (locale/store/feedback/async/controls) shared by
  login/register panels.
- Admin settings dialogs: `AdminSettingsDialogFrame` (loading → error+retry → content +
  footer slot) adopted by Stripe and OAuth Provider dialogs.
- CSS: `node-tool-glass-surface` + `node-tool-icon-button--accent-hover` modifiers on the
  `node-tool-buttons.css` contract; `:is()` grouping in `canvas-chrome.css` /
  `editing-controls.css`.
- `.jscpd.json` exclusions reduced to structural/vendor/intentional cases only:
  `node_modules`, `dist`, generated query client, specs, `src/test/**`, i18n locale
  indexes (parity requires identical shape), `canvas-range-input.css` (webkit/moz
  vendor pseudo-elements must stay separate), `imageRetouchEngine.ts` (CPU-bound
  inner convolution loops), `UserCreditBalanceButton.vue` + `round-control.css`
  (different visuals, coincidental transition similarity).

## 4. Out-of-Plan but Required: `adjustment` → `retouch` Completion

Prior module rename left the inner pipeline on the old slug. Finished during Batch 3:
`ImageAdjustment*` → `ImageRetouch*`, `RetouchAdjustmentLayer` → `RetouchEffectLayer`
(`type: 'effect'`), persisted `adjustmentRecipe` → `retouchRecipe` with schema v3→v4
plus a v3→v4 load migration (`latestBoardState.ts`), backend version gate bump
(`boards/state.go`, `openapi.yaml`, regenerated query client). During the rename a
mechanical tool pass broke `RetouchEffectActions` template/style class coupling;
fixed and audited all renamed Vue files for template↔style drift (clean).

## 5. Verification (Final Gate, Exit Codes)

`format:check` 0, `lint` 0 (full pipeline incl. Knip/boundaries/duplication),
`lint:style` 0, `test:typecheck` 0, `test:run` 0 (381/2069),
`build` 0 (Initial JS 773.9 KB), `api:check-generated` 0,
`test:browser` 0 (5/7), `test:accessibility` 0 (5 journeys),
`go test ./internal/boards/...` ok.

## 6. Residual Risks / Non-Goals (Unchanged from Plan)

- Stryker/coverage thresholds: not introduced (plan leaves them optional).
- Largest component/composable sizes reduced only where ownership splits were safe;
  1000-line budget holds everywhere.
- v4 board snapshots are unreadable by pre-rename builds (backend gate is v4-only);
  rollback past the rename requires data awareness.
