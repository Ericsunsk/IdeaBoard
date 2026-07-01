# IdeaBoard Agent Notes

## Defaults

- 用简体中文回复；Technical Terms 保持 English。
- 优先小范围、可回滚改动；只在涉及 destructive / external side effects 时确认。
- 发现 repo-local docs 时优先读：`CONTEXT-MAP.md`、相关 `CONTEXT.md`、`docs/agents/*.md`。

## Stack

- Frontend: Vue 3 + Vite + TypeScript + Tailwind CSS v4 + PrimeVue v4 unstyled.
- Backend: Go + Gin + sqlc + River + NATS JetStream + PostgreSQL 17.
- Auth: Better Auth + Fastify + TypeScript.

## Key Conventions

- Vue SFC 用 `<script setup>`；`.ts/.tsx` 保持显式 imports，types 用 `import type`。
- Generated API wrappers 在 `shared/api/generated-query`，只在 feature/service 边界适配。
- Admin UI 使用 PrimeVue unstyled + Tailwind utilities；复用 `features/admin/shared/components/`。
- Composables 协调 UI lifecycle/reactive state；services 执行 API/domain operations；utils 保持 pure。
- Backend repository methods 接收 `context.Context` first，并使用 `dbWithContext(ctx)` pattern。
- Migrations 位于 `apps/api/internal/db/migrations/`；不要手改 `apps/api/internal/db/sqlc/*.go`。
- Task submission 必须保持 transactional：insert task 与 enqueue River job 在同一个 PG transaction。

## Local Verification

- Frontend unit/spec: `cd apps/web && npm run test:run`
- Frontend type/build: `cd apps/web && npm run build`
- Frontend lint/style: `cd apps/web && npm run lint` / `npm run lint:style`
- Backend tests/build: `cd apps/api && go test ./...` / `go build ./...`
- DB query/schema changes: `cd apps/api && go tool sqlc generate`

## Agent skills

### Issue tracker

Issues and PRDs are tracked in GitHub Issues for `Ericsunsk/IdeaBoard`; external PRs are also a triage surface. See `docs/agents/issue-tracker.md`.

### Triage labels

Use the default five-label vocabulary: `needs-triage`, `needs-info`, `ready-for-agent`, `ready-for-human`, `wontfix`. See `docs/agents/triage-labels.md`.

### Domain docs

Use a multi-context layout, with `CONTEXT-MAP.md` at the repo root and context-specific `CONTEXT.md` files. See `docs/agents/domain.md`.
