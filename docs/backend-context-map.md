# 后端 Context Map

本文记录 `apps/api` 的后端 context map。它用于指导后续架构演进，不表示要把所有包强行塞进分层 DDD 模板。

## 目标

- 保持后端第一层目录有业务或运行时含义：技术性的 runtime、platform 模块保持显式；足够大的业务区域按 context 收口。
- 优先做 deep module，而不是装饰性分层。context 应该减少调用方需要理解的细节，而不是默认增加 `domain`、`application`、`infrastructure` 目录。
- 一次移动一个 context。只有当多个顶层包已经共享同一套领域语言，或调用方被迫理解过多实现细节时，才把它们归组。
- 优先替换而不是兼容。内部后端重构不保留旧 package path 兼容；移动 context 时同步删除旧 import、alias 和 facade。

## Context 列表

| Context | 目录 | 职责 | 主要接口 | 协作者 |
| --- | --- | --- | --- | --- |
| Runtime | `internal/runtime`, `cmd` | 启动 API 进程、装配 HTTP routes、持有 worker adapter，并服务 embedded frontend dist。 | `bootstrap` wiring, `httpapi.RegisterRoutes`, `worker.RiverTaskWorker` | Platform, Tasking, AccountPool, Boards, Prompts, Media, Identity |
| Platform | `internal/platform`, `internal/db`, `internal/events` | 基础设施 adapter 和横切运行时能力，包括 config、database、HTTP safety、response helper、settings、event bus 和 generated SQL。 | `config.Config`, database pools, sqlc queries, event publishers, safe HTTP helpers | Runtime 和各业务 context |
| Tasking | `internal/tasking` | task model、提交、排队、媒体校验、task runtime lifecycle、持久化和统计。 | `taskservice.Service`, `tasksubmission.Store`, `taskqueue.TaskAssigner`, `taskruntime.Service`, `taskstats.Service`, `taskstore.Repository` | Runtime worker/bootstrap, AccountPool, Media, Workflow, Prompts |
| Identity | `internal/identity/auth`, `internal/identity/autherrors`, `internal/identity/users` | Auth middleware/client、用户持久化、用户与管理员管理。 | Auth handlers/middleware, `users.UserRepository`, `users.UserService` | Runtime HTTP wiring, Media, Boards |
| AccountPool | `internal/accountpool/accounts`, `internal/accountpool/channels` | account pool、channel config、account load 和 task scheduling 可见性。 | `accounts.AccountService`, account/channel handlers and repositories | Tasking queue/store, Runtime HTTP wiring |
| Boards | `internal/boards` | board ownership、访问检查和 board 管理 API。 | `boards.BoardService`, board handler/repository | Runtime HTTP wiring, Media upload authorization |
| Media | `internal/media/storage`, `internal/media/assets`, `internal/media/imageformat`, `internal/media/mediarefs` | object storage、upload serving、upload authorization、media references、image format validation primitives 和 bundled business assets。 | `storage.Service`, media stores/authorizers, media reference helpers, asset handlers | Runtime HTTP wiring, Boards, Identity, Tasking |
| Workflow | `internal/workflow`, `internal/prompts` | 可执行 workflow feature contract、task 参数整形、prompt templates 和 prompt assembly。 | Workflow helpers, prompt engine/repository/handler | Tasking submission/service, Runtime HTTP wiring |

## 依赖方向

- `cmd` 可以依赖 `internal/runtime/bootstrap`，自身保持轻量。
- `internal/runtime` 可以组合 platform module 和业务 context。runtime adapter 可以依赖 Tasking interface，但业务 context 不应依赖 `internal/runtime/worker` 实现细节。
- 业务 context 可以通过自己的 repository 或 adapter 依赖 `internal/platform` 和 `internal/db/sqlc`。
- 业务 context 应避免直接依赖彼此实现细节。一个 context 需要另一个 context 时，优先在调用方 seam 放一个小的 service/repository interface。
- `internal/tasking` 可以包含多个小 Go package，因为 Tasking 同时覆盖 model、submission、queue、persistence、statistics、media validation 和 lifecycle state。
- `internal/platform` 不应 import 业务 context。

## 重构策略

- 内部 context 重构可以打破旧 package path。调用方必须在同一变更里改到新路径，不创建 legacy forwarding package。
- 模块已经有更好的语义接口时，不保留旧 facade 名称；调用方改到新接口后删除 compatibility method。
- 不保留重复目录来降低迁移摩擦。context move 只有在旧目录和旧 import 都清空后才算完成。
- 除非 schema 或 query contract 本身变化，否则不改 generated `sqlc` code 和 migrations。

## 当前非目标

- 不为了对称而把所有后端包都移动到 context 下。
- 不引入默认的 `domain/application/infrastructure` 目录，除非某个模块的 interface 真的因此变小、implementation 更集中。
- 不把 `internal/runtime/worker` 移入 Tasking。worker 是 execution adapter：它消费 Tasking interface，同时也持有 provider calls、River worker execution、process-level retry 等 runtime concern。
- 不为了 context 美观重命名 generated `sqlc` code、migrations 或 query names。

## 何时创建或扩展 Context

满足以下任一条件时，才创建或扩展 context：

- 一个领域区域有多个顶层包，且这些包名只有放在一起才有清晰含义。
- 调用方需要知道 sequencing、status transitions、persistence rules 或 error handling，而这些规则应藏在一个模块 interface 后面。
- 测试一个业务 workflow 时需要组装太多低层 collaborator。
- 一组包已经有稳定的领域语言，比技术层命名更能说明问题。

如果这些条件都不成立，保留现有包位置，直接改进模块 interface。

## 已落地与后续观察

- Identity 已收口到 `internal/identity`。后续只有当 auth、auth errors、users、roles 和 admin session policy 需要更小的共享 interface 时，才继续深化。
- Media 已收口到 `internal/media`。后续只有当其子包 interface 还能明显变深时，才继续拆改。
- AccountPool 已收口到 `internal/accountpool`。后续只有当 account selection、channel config 和调度负载规则需要更小的共享 interface 时，才继续深化。
- Workflow 和 Prompts 暂时保持分离：workflow 持有可执行 feature contract，prompts 持有 template storage 和 rendering。只有 prompt contract 开始跨两边泄漏时再复访。
