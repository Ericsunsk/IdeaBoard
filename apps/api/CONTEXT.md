# Backend Context

`apps/api` 的后端代码按 runtime/platform/business contexts 组织。本文只记录 backend-local context language、dependency direction 和扩展规则；系统级设计 contract 见 [../../DESIGN.md](../../DESIGN.md)。

## Contexts

| Context | Directories | Responsibility | Main Interfaces |
| ------- | ----------- | -------------- | --------------- |
| Runtime | `cmd`, `internal/runtime` | 启动 API 进程、注册 HTTP routes、装配 worker adapters、服务 embedded frontend dist | `bootstrap`, `httpapi.RegisterRoutes`, `worker.RiverTaskWorker` |
| Platform | `internal/platform`, `internal/db`, `internal/events` | config、database pools、response helpers、safe HTTP、settings、event bus、generated SQL | `config.Config`, sqlc queries, event publishers |
| Tasking | `internal/tasking` | Task model、submission、queueing、media validation、runtime lifecycle、persistence、statistics | `taskservice.Service`, `tasksubmission.Store`, `taskqueue.TaskAssigner`, `taskruntime.Service`, `taskstats.Service`, `taskstore.Repository` |
| Identity | `internal/identity/auth`, `internal/identity/autherrors`, `internal/identity/users` | Auth middleware/client、user persistence、user/admin management | Auth handlers/middleware, `users.UserRepository`, `users.UserService` |
| AccountPool | `internal/accountpool/accounts`, `internal/accountpool/channels` | Account pool、channel config、account load、task scheduling visibility | `accounts.AccountService`, account/channel handlers and repositories |
| Boards | `internal/boards` | Board ownership、access checks、board management API | `boards.BoardService`, board handler/repository |
| Media | `internal/media/storage`, `internal/media/assets`, `internal/media/imageformat`, `internal/media/mediarefs` | Object storage、upload serving、upload authorization、media references、image format validation、bundled business assets | `storage.Service`, media stores/authorizers, media reference helpers |
| Workflow/Prompts | `internal/workflow`, `internal/prompts` | Executable workflow feature contract、task parameter shaping、prompt templates、prompt assembly | Workflow helpers, prompt engine/repository/handler |

## Dependency Direction

- `cmd` stays lightweight and delegates process wiring to `internal/runtime/bootstrap`.
- `internal/runtime` may compose platform modules and business contexts.
- Runtime adapters may depend on Tasking interfaces, but business contexts must not depend on `internal/runtime/worker` implementation details.
- Business contexts may depend on `internal/platform` and `internal/db/sqlc` through their own repositories or adapters.
- Business contexts should avoid importing each other's implementation details. Prefer a small service/repository interface at the caller boundary.
- `internal/platform` must not import business contexts.
- `internal/tasking` may contain multiple focused packages because Tasking covers model, submission, queueing, persistence, statistics, media validation and lifecycle state.

## Current Design Rules

- Task submission is transactional: insert the business task and enqueue the River job in the same PostgreSQL transaction.
- Task submission idempotency uses `client_action_id` as the user-action boundary: retries of the same user action return the existing Task, while intentional reruns send a new `client_action_id`.
- River + PostgreSQL are the task queue and task state fact source.
- NATS JetStream is an event layer for task status, audit and realtime fanout; it is not a task queue.
- Worker implementation stays in `internal/runtime/worker` as an execution adapter.
- Workflow owns executable feature contracts; Prompts owns template storage and rendering.
- Unless schema or query contracts change, do not edit migrations or generated sqlc code.

## When To Expand A Context

Create or expand a backend context only when at least one condition is true:

- A domain area has multiple packages whose names become clearer when grouped.
- Callers need to understand sequencing, status transitions, persistence rules or error handling that should live behind a smaller module interface.
- Testing a business workflow requires assembling too many low-level collaborators.
- A group of packages already shares stable domain language that is clearer than technical-layer naming.

If none of these apply, keep the current package location and improve the local module interface directly.
