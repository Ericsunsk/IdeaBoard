# Web Scripts

This directory is for durable Web app maintenance scripts. Prefer adding a
`package.json` script entry for anything that should be run more than once.

## Script Inventory

| File                                      | Entry                                          | Purpose                                                                                       |
| ----------------------------------------- | ---------------------------------------------- | --------------------------------------------------------------------------------------------- |
| `check-api-generated-drift.ts`            | `npm run api:check-generated`                  | Checks whether generated API wrappers are in sync with `orval.config.ts`.                     |
| `check-test-guardrails.ts`                | `npm run test:guardrails`                      | Enforces frontend test naming, component render helper, and generated-query mock guardrails.  |
| `user-route-smoke.ts`                     | `npm run smoke:user`                           | Runs mocked user route smoke coverage through Playwright.                                     |
| `mock-smoke-utils.ts`                     | Internal helper                                | Shared mocked browser smoke runtime helpers.                                                  |

## Cleanup Policy

Keep scripts when they are called by `package.json`, CI, docs, or recurring
manual workflows. Delete one-off investigation scripts after the issue is
resolved.
