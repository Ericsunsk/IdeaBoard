# IdeaBoard Agent Protocol

This file is the only agent-facing entry point. Keep it short, executable, and in English. Human-facing docs stay in Simplified Chinese with English Technical Terms.

## Response Rules

- Reply to the user in Simplified Chinese.
- Keep Technical Terms in English.
- Prefer direct execution over long explanations.
- Report verification and unresolved risks clearly.

## Read Order

Read only what is relevant to the task:

1. [DESIGN.md](./DESIGN.md) for system contracts and context map.
2. The relevant `CONTEXT.md` for domain language.
3. [apps/web/DESIGN.md](./apps/web/DESIGN.md) for frontend UI/design work.
4. [DEVELOPMENT.md](./DEVELOPMENT.md) for commands, code entry points, and verification.
5. `docs/agents/*.md` only when using the matching workflow.

Do not create a new doc if an existing canonical doc can answer the same question.

## Stack Snapshot

- Frontend: Vue 3, Vite, TypeScript, Tailwind CSS v4, PrimeVue v4 unstyled.
- Backend: Go, Gin, sqlc, River, NATS JetStream, PostgreSQL 17.
- Auth: Better Auth, Fastify, TypeScript.

## Hard Constraints

- Keep changes small, reversible, and within scope.
- Never hand-edit generated frontend API wrappers in `apps/web/src/shared/api/generated-query`.
- Never hand-edit generated sqlc files in `apps/api/internal/db/sqlc`.
- Backend migrations live in `apps/api/internal/db/migrations`.
- Task submission must stay transactional: insert the task and enqueue the River job in the same PostgreSQL transaction.
- River + PostgreSQL are the task queue and state fact source; NATS JetStream is the event layer, not a task queue.
- Admin route pages must not call generated API directly; use the relevant feature/resource/adapter boundary.
- Frontend `shared` must not depend on concrete feature modules.
- For UI work, follow the frontend UI agent protocol in [apps/web/DESIGN.md](./apps/web/DESIGN.md) and reuse existing tokens/components before adding new ones.
- Backend repository methods take `context.Context` first and use the `dbWithContext(ctx)` pattern.

## Risk Modes

Treat these as Production-mode work: deployment, auth/permission/billing/security logic, schema changes, bulk mutations, destructive actions, and real external writes.

For Production-mode work, provide a brief plan, key risks, minimal compatible changes, relevant verification, and rollback/fallback notes. Ask for confirmation only for destructive, irreversible, or meaningful external side effects.

## Verification

Use the smallest relevant check:

- Frontend unit/spec: `cd apps/web && npm run test:run`
- Frontend build/type: `cd apps/web && npm run build`
- Frontend lint/style: `cd apps/web && npm run lint` / `npm run lint:style`
- Backend tests/build: `cd apps/api && go test ./...` / `go build ./...`
- DB query/schema changes: `cd apps/api && go tool sqlc generate`
- Auth build: `cd apps/auth && npm run build`

If you cannot run a relevant check, say so and explain the residual risk.

## Agent Workflows

- Issue tracker: GitHub Issues for `Ericsunsk/IdeaBoard`; see [docs/agents/issue-tracker.md](./docs/agents/issue-tracker.md).
- Triage labels: use `needs-triage`, `needs-info`, `ready-for-agent`, `ready-for-human`, `wontfix`; see [docs/agents/triage-labels.md](./docs/agents/triage-labels.md).
- Domain docs: use `DESIGN.md` as the context map and local `CONTEXT.md` files for vocabulary; see [docs/agents/domain.md](./docs/agents/domain.md).
