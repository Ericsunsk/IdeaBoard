# Backend Context

This file records the `apps/api` backend context map. It guides future architecture work, but it does not require every package to fit a layered DDD template.

## Goals

- Keep first-level backend directories meaningful: runtime and platform modules stay explicit; large business areas are grouped by context.
- Prefer deep modules over decorative layering. A context should reduce what callers need to know, not add default `domain`, `application`, or `infrastructure` directories.
- Move one context at a time. Group packages only when they share stable domain language or callers are forced to understand too many implementation details.
- Prefer replacement over compatibility. Internal backend refactors do not preserve old package-path compatibility; context moves should update imports and delete old aliases/facades in the same change.

## Contexts

| Context | Directories | Responsibility | Main Interfaces | Collaborators |
| --- | --- | --- | --- | --- |
| Runtime | `internal/runtime`, `cmd` | Starts the API process, wires HTTP routes, owns worker adapters, and serves embedded frontend dist. | `bootstrap` wiring, `httpapi.RegisterRoutes`, `worker.RiverTaskWorker` | Platform, Tasking, AccountPool, Boards, Prompts, Media, Identity |
| Platform | `internal/platform`, `internal/db`, `internal/events` | Infrastructure adapters and cross-cutting runtime capabilities: config, database, HTTP safety, response helpers, settings, event bus, and generated SQL. | `config.Config`, database pools, sqlc queries, event publishers, safe HTTP helpers | Runtime and business contexts |
| Tasking | `internal/tasking` | Task model, submission, queueing, media validation, task runtime lifecycle, persistence, and statistics. | `taskservice.Service`, `tasksubmission.Store`, `taskqueue.TaskAssigner`, `taskruntime.Service`, `taskstats.Service`, `taskstore.Repository` | Runtime worker/bootstrap, AccountPool, Media, Workflow, Prompts |
| Identity | `internal/identity/auth`, `internal/identity/autherrors`, `internal/identity/users` | Auth middleware/client, user persistence, and user/admin management. | Auth handlers/middleware, `users.UserRepository`, `users.UserService` | Runtime HTTP wiring, Media, Boards |
| AccountPool | `internal/accountpool/accounts`, `internal/accountpool/channels` | Account pool, channel config, account load, and task scheduling visibility. | `accounts.AccountService`, account/channel handlers and repositories | Tasking queue/store, Runtime HTTP wiring |
| Boards | `internal/boards` | Board ownership, access checks, and board management API. | `boards.BoardService`, board handler/repository | Runtime HTTP wiring, Media upload authorization |
| Media | `internal/media/storage`, `internal/media/assets`, `internal/media/imageformat`, `internal/media/mediarefs` | Object storage, upload serving, upload authorization, media references, image format validation primitives, and bundled business assets. | `storage.Service`, media stores/authorizers, media reference helpers, asset handlers | Runtime HTTP wiring, Boards, Identity, Tasking |
| Workflow | `internal/workflow`, `internal/prompts` | Executable workflow feature contract, task parameter shaping, prompt templates, and prompt assembly. | Workflow helpers, prompt engine/repository/handler | Tasking submission/service, Runtime HTTP wiring |

## Dependency Direction

- `cmd` may depend on `internal/runtime/bootstrap`; keep `cmd` lightweight.
- `internal/runtime` may compose platform modules and business contexts. Runtime adapters may depend on Tasking interfaces, but business contexts must not depend on `internal/runtime/worker` implementation details.
- Business contexts may depend on `internal/platform` and `internal/db/sqlc` through their own repositories or adapters.
- Business contexts should avoid directly depending on each other's implementation details. When one context needs another, prefer a small service/repository interface at the caller boundary.
- `internal/tasking` may contain multiple small Go packages because Tasking covers model, submission, queueing, persistence, statistics, media validation, and lifecycle state.
- `internal/platform` must not import business contexts.

## Refactor Strategy

- Internal context refactors may break old package paths. Callers must move to the new path in the same change; do not create legacy forwarding packages.
- When a module has a better semantic interface, do not preserve old facade names. Update callers to the new interface and delete compatibility methods.
- Do not keep duplicate directories to reduce migration friction. A context move is complete only after old directories and imports are gone.
- Unless schema or query contracts change, do not edit generated `sqlc` code or migrations.

## Non-Goals

- Do not move all backend packages under contexts just for symmetry.
- Do not introduce default `domain/application/infrastructure` directories unless they make a module interface smaller and implementation more local.
- Do not move `internal/runtime/worker` into Tasking. The worker is an execution adapter: it consumes Tasking interfaces while owning provider calls, River worker execution, process-level retry, and other runtime concerns.
- Do not rename generated `sqlc` code, migrations, or query names for context aesthetics.

## When to Create or Expand a Context

Create or expand a context only when at least one of these is true:

- A domain area has multiple top-level packages whose names become clearer when grouped together.
- Callers need to understand sequencing, status transitions, persistence rules, or error handling that should live behind a module interface.
- Testing a business workflow requires assembling too many low-level collaborators.
- A group of packages already shares stable domain language that is clearer than technical-layer naming.

If none of these apply, keep the current package location and improve the module interface directly.

## Current State and Watch Points

- Identity has moved under `internal/identity`. Deepen it further only when auth, auth errors, users, roles, and admin session policy need smaller shared interfaces.
- Media has moved under `internal/media`. Deepen it further only when subpackage interfaces can become meaningfully smaller.
- AccountPool has moved under `internal/accountpool`. Deepen it further only when account selection, channel config, and scheduling load rules need smaller shared interfaces.
- Workflow and Prompts remain separate for now: workflow owns executable feature contracts, while prompts owns template storage and rendering. Revisit only when prompt contracts start leaking across both sides.
