# IdeaBoard Durability 排障手册（Outbox / Settlement / Stale Hold）

> [!NOTE]
> 本页只覆盖 durability 收敛链路：task outbox sweeper、board 访问变更 outbox、billed-send 后 settlement 收敛、stale hold / reconciliation。
> 指标口径与 River 预算见 [backend-metrics.md](./backend-metrics.md)；部署与回滚见 [deployment.md](./deployment.md)。
> 所有恢复动作都走已有 API / River 重试，不要手工改 Ledger、Settlement 或删除 outbox pending 行。

## 机制速览（代码事实源）

| 链路 | River kind / 位置 | 节奏与预算 |
|------|-------------------|-----------|
| task status outbox sweeper | `task_status_event_outbox_replay`（`internal/runtime/interfaces/worker/outbox/task_status_outbox.go`），与 board sweeper 同队列 `task_settlement_sweep` | 周期 30s（`OutboxPeriod`），`RunOnStart: true`，单次 100 行（`OutboxReplayLimit`），`Timeout` 4min / `MaxAttempts` 5（`riverpolicy` 矩阵），priority 4 |
| board access outbox sweeper | `board_access_event_outbox_replay`（同目录 `board_access_outbox.go`） | 与上同队列 / 同预算 / 同周期 / 同 limit / 同 priority，`RunOnStart: true` |
| stalled settlement sweep | `task_settlement_sweep`（`internal/tasking/interfaces/worker/settle.go`，`TaskSettlementSweepWorker`） | 周期 15min，`RunOnStart: false`，单次最多 100（`StalledSettlementSweepBatch`），`Timeout` 4min / `MaxAttempts` 5；阈值 24h（`StalledSettlementThreshold`，按 `tasks.updated_at` liveness 计算） |
| credits stale-hold 信号 | `execute_credit_reconciliation()`（`internal/db/schema/fragments/credits.sql`），`stale_hold_after` 被 CHECK pin 为 24h | 只 INSERT `stale_hold` finding（`warning`/`low`），不做 Release |

Outbox 通用语义（`internal/events/outbox/generic.go`）：pending 条件为 `status_state='pending' AND next_attempt_at <= now()`，按 `internal_id` 排序；limit 非正或 `> 500` 回落到 100；失败记 `attempts+1` + `last_error=left(err,1000)`，下次执行时间为 `now() + LEAST(1h, 5s * 2^LEAST(attempts,8))`；`InsertTx` 与业务状态翻转同事务、`ON CONFLICT (event_id) DO NOTHING`。

## A. Outbox sweeper（task / board）

事实：task 行在 task 状态翻转同事务写入（`task_status_event_outbox`）；board 行在 access / membership 变更同事务写入（`board_access_event_outbox`，`postgres_workspace.go` 经 `InsertTx`）。
task `evt_` 跨重试稳定；board `evt_` 由 `(board, member, access_version)` 经 `StableEventID` 确定，live emit 与 replay 收敛而不重复。
task replay 下游为 task status channel（best-effort）→ hot log `Append` → JetStream `PublishWorkspaceEvent` → webhooks `DispatchErr`；单行失败只 `MarkAttempt` 并继续，`MarkDelivered` 失败才中止本轮。
board replay 下游只有 hot log + JetStream，永远不进 webhooks；`EventType` 为空的 task 内部 pending-retry 事件只走 task channel，不拼 v1 envelope。
只有 board 有 `PruneDelivered`（retention 7 天，对齐 NATS MaxAge；pending 永不 prune）；task 无 prune。

### 症状 → 排查命令 → 恢复步骤

| 症状 | 排查命令 | 恢复步骤 |
|------|---------|---------|
| SSE / 任务状态长时间不推进，但 task 行本身已翻转 | `ideaboard logs --tail=200 api` 查 worker 错误；PromQL `ideaboard_river_queue_depth{queue="task_settlement_sweep"}` / `ideaboard_river_queue_lag_seconds{queue="task_settlement_sweep"}`（见 backend-metrics.md C 节）；DB 查 `SELECT event_id, status, attempts, next_attempt_at, left(last_error,200) FROM task_status_event_outbox WHERE status_state='pending' AND next_attempt_at <= now() ORDER BY internal_id LIMIT 20;` | 保持 River worker 运行等 30s sweep 重放；不要重发业务请求（`evt_` 稳定，重放幂等）。`attempts` 持续涨看 `last_error` 修下游（JetStream / webhook 目标），outbox 行不动它 |
| 被撤销 board 访问的用户仍收到旧访问事件，或反之收不到 `board_access_changed` | DB 查 `SELECT event_id, board_id, member_public_id, access_version, attempts, next_attempt_at, left(last_error,200) FROM board_access_event_outbox WHERE status_state='pending' ORDER BY internal_id LIMIT 20;`；确认对应变更事务是否提交（`access_version` 是否已推进） | 同上等 sweep；不要手工补发事件——`evt_` 确定性保证 live 与 replay 收敛，手工补发只会制造重复投递。确认该信号本就不进 webhooks，webhook 侧缺失是预期行为 |
| `board_access_event_outbox` delivered 行无限增长 | 查 `SELECT count(*) FROM board_access_event_outbox WHERE status_state='delivered';` | 这是 audit trail，7 天窗口外由 `PruneDelivered` 清理；pending 行永不删，不要 `DELETE` pending 行“止血” |
| sweep job 反复失败（非单行失败） | 查启动日志与 `/api/readyz`；确认 worker 注册了两种 kind（`task_status_event_outbox_replay` / `board_access_event_outbox_replay`） | 修 wiring（replayer 为 nil 会直接返回 `... outbox ... unavailable`）；sweeper 本身无业务写入，重起 API/Worker 后下一周期自动追赶 |

## B. Board 访问变更 outbox（专项）

- 写入点：`InMemoryWorkspaceBoardStore.accessSignals` / `postgres_workspace.go`（`InsertTx` 与 access/membership 同事务）。
- 收敛保证：同一 `(board, member, access_version)` 重复 `InsertTx` 由 `ON CONFLICT DO NOTHING` 去重；`Deliver` 走 joined 语义，单行失败记 `MarkAttempt` 后把错误 join 返回，不丢弃整批。
- 排障只看三列：`attempts`（是否在退避）、`next_attempt_at`（下次重放）、`last_error`（下游 publish 失败原文）。不要按 `occurred_at` 判断是否该重放，pending 条件只看 `next_attempt_at <= now()`。

## C. Settlement：billed-send 后提交失败（无 marker 表）

当前实现（`internal/tasking/interfaces/worker/execution.go` `convergeUnsettledSend`）：provider 已返回即视为已计费，**不再返回 River 可重试错误、不消耗 `tasks.retry_count`**，而是在 detached context（5s 预算）内最多 3 次重试 `settler.Fail`（terminal 收敛：task → failed + hold Release + settlement projection + staged status event）。

| 症状（日志原文 grep） | 含义 | 处理 |
|------|------|------|
| `workspace task settlement failed after a billed send; converging terminally` | Complete 提交失败，已转 detached terminal 收敛 | 等 detached 内的 3 次写；成功会跟一条 `workspace task terminally failed after an unsettled billed send`，属正常收敛，无需处理 |
| `workspace task terminal convergence after an unsettled billed send failed`（带 `attempt/total`） | detached 单次写失败，还在预算内重试 | 若最终成功即收敛；只在连续出现且无成功行时按下一行处理 |
| `workspace task stays running: provider send succeeded but neither settlement nor terminal convergence committed` | 3 次全失败，task 保持 `running`，claim token + attempt fact + hold 全部保留 | 不要重提任务（会产生第二次计费发送）。保持服务运行，用已有 task cancel / terminal settle 路径收敛该 task；收敛后凭 attempt fact 与 ledger 核对只收了一次费 |
| renderer 失败导致的 `failed`（`hold released, no retry burn`） | 确定性 finalize 失败，非 settlement 故障 | 按普通失败处理，不走本节 |

> [!WARNING]
> 不要寻找“settlement marker 行 / snooze / settlement-only fallback job”来排障：当前代码没有这类表与 job，上面三条日志就是全部信号。按日志采取本节动作即可。

## D. Stale hold / reconciliation

事实（`workspacetasks/settle_stalled.go` + `workspace_tasks.sql` `ListStalledHeldWorkspaceTasks`）：
候选 = `tasks.status IN ('pending','running')` 且 `updated_at < now()-24h`（liveness 时钟，由 `set_tasks_updated_at` trigger 维护；任何 reclaim 都会写 `updated_at`，所以 24h 无触碰 = 真遗弃，不是“慢任务”）且 billing snapshot `total_credits > 0` 且 `hold_transaction_id IS NOT NULL` 且 settlement 未到 `captured`/`released`；跨 workspace 扫描、oldest-first、单次 100。
收敛 = 同 worker 的 workspace-scoped `Fail`（`FailTaskInTx` → `ReleaseInTx` → settlement projection → commit，一事务；任一失败全部回滚），terminal error 为 `execution lease abandoned past the final-timeout threshold; credit hold released`。
`ErrStateConflict` / `ErrNotFound` 计 `Skipped`（worker、operator cancel 或上轮 sweep 已先收敛）；其余单任务失败隔离为 `Failures` 并打日志，不中止整批；job 只在 wiring 缺失 / discovery 失败 / cancel 时整体失败（`ErrSettlementUnavailable`）。
Credits 对账的 `stale_hold` 只是审计 finding（`holds.status='held' AND created_at < now()-24h`），**从不 Release**；Release 只走 sweep 的 `Fail` 路径（含 `task-release:<task_id>` 幂等 key 与 ledger held-status CAS 双 fence，不会 double-release）。

| 症状 | 排查命令 | 恢复步骤 |
|------|---------|---------|
| 用户 Ideas 被锁、task 停在 `pending`/`running` 超过 24h | DB 按发现查询核对：`tasks`（`status`、`updated_at`）+ `task_billing_snapshots`（`total_credits>0`、`hold_transaction_id`）+ `task_billing_settlements`（非 `captured`/`released`）；日志 grep `stalled task settlement sweep completed`（看 `scanned/converged/skipped/failures`）与 `stalled task settlement convergence failed`（看 `workspace_id/task_id/err`，即仍被锁定的 hold） | 等 15min sweep；`Skipped` 为主属正常竞态（worker/cancel 先赢）。`Failures` 按 `err` 修 DB/ledger 后等下一轮，不要手工 `UPDATE tasks` 或手工 Release |
| Admin 对账出现 `stale_hold`（`warning`/`low`）finding | Control API：`GET /credits/reconciliation/runs`、`GET /credits/reconciliation/runs/{run_id}`、`GET /credits/reconciliation/findings`（见 `openapi.control.yaml`）；DB `credit_reconciliation_runs` / `credit_reconciliation_findings` | 这是审计信号不是动作：24h 内的 hold 出现 finding 属正常（sweep 阈值未到）；超 24h 的 finding 应伴随 sweep `converged+1` 自动消失，手动触发一次对账 run 确认即可，不要手工清 finding |
| `terminal_task_settlement_mismatch` / `active_task_not_held` / `task_hold_missing_billing_snapshot` 等 error/critical finding | 同上 control API + run 详情 | 按 finding 修业务根因（提交事务、snapshot、trace），保持 River worker 运行让对账重试；不要把 Dashboard 状态手工写入 PostgreSQL（同 deployment.md Payments 事故处理） |
| sweep job 整体失败（非单任务 Failures） | 按 [deployment runbook](./deployment.md#operations) 初始化当前包 `DEPLOY`，执行 `"${DEPLOY[@]}" doctor` → `"${DEPLOY[@]}" logs --tail=200 api` → `"${DEPLOY[@]}" doctor --deep`；确认 `/api/readyz` 与 `task_settlement_sweep` depth/lag | 修 DB 可用性 / wiring 后重启 API/Worker；sweep `RunOnStart: false`，重启后最多等一个 15min 周期 |

## 何时升级

- `stalled task settlement convergence failed` 同一 `(workspace, task)` 连续两轮以上：用户资金仍锁定，需人工介入修 settle 事务（DB 约束 / ledger 状态），不要放过夜。
- `workspace task stays running: ... neither settlement nor terminal convergence committed`：billed-send 未收敛，需人工用 cancel/settle 路径收敛并核对“一次计费”，涉及资金必须双人复核。
- outbox `pending` 积压且 `last_error` 指向同一外部依赖（NATS / webhook 目标）超过 30min：按 deployment.md 先看当前签名包的 `doctor` 与反代/网络，再升级到对应依赖负责人。
- 对账出现 `critical` finding（`ledger_postings_invalid`、`command_shape_invalid`、`terminal_task_settlement_mismatch` 等）：立即升级，不要手工改表；止血优先（Payments 事故先关 `purchases_enabled`，见 deployment.md）。

## 关联文档

| 文档 | 用途 |
|------|------|
| [backend-metrics.md](./backend-metrics.md) | RED 指标、`task_settlement_sweep` depth/lag 查询、重试/超时矩阵 |
| [deployment.md](./deployment.md) | 当前签名包的 `doctor/logs/status/smoke`、升级回滚、Payments 事故处理 |
| [auth-operations.md](./auth-operations.md) | 账号与安全操作边界 |
| [data-protection.md](./data-protection.md) | backup/restore 与密钥材料处理 |
| [DESIGN.md](../../DESIGN.md) | 系统契约与 River/NATS 职责边界 |
