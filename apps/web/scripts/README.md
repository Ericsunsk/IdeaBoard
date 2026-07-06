# Web Scripts

This directory is for durable Web app maintenance scripts. Prefer adding a
`package.json` script entry for anything that should be run more than once.

## Script Inventory

| File                                      | Entry                                          | Purpose                                                                                       |
| ----------------------------------------- | ---------------------------------------------- | --------------------------------------------------------------------------------------------- |
| `check-api-generated-drift.ts`            | `npm run api:check-generated`                  | Checks whether generated API wrappers are in sync with `orval.config.ts`.                     |
| `check-test-guardrails.ts`                | `npm run test:guardrails`                      | Enforces frontend test naming, component render helper, and generated-query mock guardrails.  |
| `check-style-governance.ts`               | `npm run lint:style-governance`                | Enforces frontend style ownership, selector, token, and raw-value guardrails.                 |
| `canvas-interactions-benchmark.ts`         | `npm run benchmark:canvas-interactions`        | Runs mocked Canvas interaction benchmarks and writes JSON reports under `.benchmark-reports`. |
| `user-route-smoke.ts`                     | `npm run smoke:user`                           | Runs mocked user route smoke coverage through Playwright.                                     |
| `mock-smoke-utils.ts`                     | Internal helper                                | Shared mocked browser smoke runtime helpers.                                                  |

## Canvas Interaction Benchmark

Smallest useful local run:

```bash
npm run benchmark:canvas-interactions -- --nodes 20 --paths initial-load,viewport-zoom --json --output .benchmark-reports/canvas-interactions-smoke
```

The repeatable large-board case uses the `structure-heavy` fixture:

```bash
npm run benchmark:canvas-interactions -- --fixture structure-heavy --nodes 500
```

The runner starts a Vite dev server, mocks auth and user Canvas APIs in Playwright,
and exits non-zero when configured performance budgets are exceeded.

## Cleanup Policy

Keep scripts when they are called by `package.json`, CI, docs, or recurring
manual workflows. Delete one-off investigation scripts after the issue is
resolved.
