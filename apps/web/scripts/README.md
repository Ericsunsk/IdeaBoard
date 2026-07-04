# Web Scripts

This directory is for durable Web app maintenance scripts. Prefer adding a
`package.json` script entry for anything that should be run more than once.

## Script Inventory

| File                                      | Entry                                          | Purpose                                                                                       |
| ----------------------------------------- | ---------------------------------------------- | --------------------------------------------------------------------------------------------- |
| `check-api-generated-drift.ts`            | `npm run api:check-generated`                  | Checks whether generated API wrappers are in sync with `orval.config.ts`.                     |
| `check-test-guardrails.ts`                | `npm run test:guardrails`                      | Enforces frontend test naming, component render helper, and generated-query mock guardrails.  |
| `admin-mock-smoke.ts`                     | `npm run smoke:admin-mock`                     | Runs mocked Admin UI smoke coverage through Playwright.                                       |
| `user-route-smoke.ts`                     | `npm run smoke:user`                           | Runs mocked user route smoke coverage through Playwright.                                     |
| `canvas-interaction-baseline.ts`          | `npm run benchmark:canvas-interactions`        | Records Canvas interaction smoothness metrics for node drag, viewport pan, and viewport zoom. |
| `image-editor-plan-executor-benchmark.ts` | `npm run benchmark:image-editor-plan-executor` | Benchmarks image editor adjustment plan execution.                                            |
| `audit-css-migration.ts`                  | `npm run css:audit`                            | Audits CSS migration leftovers.                                                               |
| `mock-smoke-utils.ts`                     | Internal helper                                | Shared mocked browser smoke/benchmark runtime helpers.                                        |

## Cleanup Policy

Keep scripts when they are called by `package.json`, CI, docs, or recurring
manual workflows. Delete one-off investigation scripts after the issue is
resolved, unless their output is useful as a benchmark artifact or documented
debugging workflow.
