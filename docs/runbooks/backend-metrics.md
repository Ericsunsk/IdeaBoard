# <div align="center">IdeaBoard 后端 RED 指标与排障手册</div>

<p align="center">
  <strong>最小指标集的查询方法与指标到排障步骤的映射。</strong>
</p>

> [!NOTE]
> 本手册覆盖 [backend-hardening] 01（RED 最小指标集）。指标由 Prometheus `/metrics` 端点暴露（本地宿主监听 `127.0.0.1:9091`；production 容器监听 `0.0.0.0:9091`，经 Compose 映射至宿主 `127.0.0.1:9091`），label 保持封闭集合：`route` 为 Gin 路由模板（未匹配记 `unmatched`）、`status` 为数字状态码、`service` 为 workflow feature slug、`queue` 为 River 队列名。不记录 user ID、task ID、URL 或错误文本。

## 指标清单

| 指标 | 类型 | Labels | 含义 |
|------|------|--------|------|
| `ideaboard_http_requests_total` | Counter | `method, route, status` | HTTP 请求计数，按路由与状态码打标 |
| `ideaboard_http_request_duration_seconds` | Histogram | `method, route, status` | HTTP 请求延迟分布 |
| `ideaboard_gateway_requests_total` | Counter | `service, outcome` | 网关调用计数，`outcome=success/error` |
| `ideaboard_gateway_retries_total` | Counter | `service` | transient 网关失败后的 worker 层整任务重投计数 |
| `ideaboard_gateway_per_model_limited_total` | Counter | `model, reason` | 网关出口前 per-model 拦截计数，`reason=concurrency_limited/breaker_open`（限流/熔断，均以 429 重试语义重投） |
| `ideaboard_gateway_breaker_open` | Gauge | `model` | per-model 熔断器状态（1=open，0=closed）；阈值走 `AI_GATEWAY_*` 配置 |
| `ideaboard_model_fallback_total` | Counter | `service, outcome` | 有界模型兜底发送计数（快照 retry candidate，单任务至多一次），`outcome=success/error` 为兜底发送结果 |
| `ideaboard_river_queue_depth` | Gauge | `queue` | 等待执行的 River job 数（available/scheduled/retryable），15s 刷新 |
| `ideaboard_river_queue_lag_seconds` | Gauge | `queue` | 最老等待 job 的年龄（秒），15s 刷新 |
| `outbound_requests_total` / `outbound_retries_total` | Counter | `target, outcome/reason` | 标准 outbound 客户端计数（网关走 `target=ai-gateway`，vision 走 `vision-search`） |

实现位置：`internal/runtime/infrastructure/metrics`（RED Set + River 刷新器）、`internal/runtime/interfaces/http/metrics.go`（中间件）、`internal/tasking/infrastructure/queue.RiverQueue.QueueStats`（仅 enqueue/monitor 缝；policy 见 `platform/riverpolicy`）。

## 常用查询（PromQL）

```promql
# 1. 各路由 5xx 错误率（过去 5 分钟）
sum by (route) (rate(ideaboard_http_requests_total{status=~"5.."}[5m]))
/
sum by (route) (rate(ideaboard_http_requests_total[5m]))

# 2. 路由 P95 延迟（过去 5 分钟）
histogram_quantile(0.95,
  sum by (route, le) (rate(ideaboard_http_request_duration_seconds_bucket[5m])))

# 3. River 队列堆积与滞后（当前值；每个队列独立 series）
ideaboard_river_queue_depth{queue="drawing_default"}
ideaboard_river_queue_lag_seconds{queue="drawing_default"}
# 维护队列逐个看（credit_reconciliation / payment_reconciliation /
# payment_checkout_shutdown / vision_billing / media_reclamation /
# task_settlement_sweep）：
ideaboard_river_queue_depth{queue=~"credit_reconciliation|payment_.*|vision_billing|media_reclamation|task_settlement_sweep"}

# 4. 网关失败与 worker 重投速率
sum by (service) (rate(ideaboard_gateway_requests_total{outcome="error"}[5m]))
sum by (service) (rate(ideaboard_gateway_retries_total[5m]))

# 4b. 模型兜底发送速率与结果（单任务至多一次；error 涨而 gateway error
# 不涨时，检查兜底候选池是否系统性不可用）
sum by (service, outcome) (rate(ideaboard_model_fallback_total[5m]))

# 5. 本地 Gateway transport 应为 single-shot；非零值表示重试策略偏离
sum by (reason) (rate(outbound_retries_total{target="ai-gateway"}[5m]))
```

## 指标到排障步骤映射

### A. `ideaboard_http_requests_total{status="5xx"}` 突增

1. 按 `route` 定位：`sum by (route)` 找出错误率最高的路由。
2. 用 `X-Request-ID` 关联日志：中间件对每请求打结构化日志（含 `request_id`），5xx 自动记 `slog.Error`。
3. 若集中在 `/api/v1/workspaces/{workspace_id}/tasks`：检查 Billing/Credits 可用性与任务提交事务（同一 PG 事务 Quote/Hold/Task/River）。
4. 若集中在 `/api/control/v1/*`：检查对应 context 的 repository 与 Admin 编排（见 DESIGN.md Admin vertical contexts 表）。

### B. `ideaboard_http_request_duration_seconds` P95 上升

1. 按 `route` 定位慢路由；先看是否为上传类（64MB body 上限）或统计聚合类。
2. 若全路由普遍上升：检查 PostgreSQL 连接池与慢查询（`QueueStats` 这类单语句应 <100ms），再看宿主机资源。
3. 若仅某路由：沿该路由的 handler → service → repository 链路排查，必要时加该路径的 typed 超时。

### C. `ideaboard_river_queue_depth` 持续增长 / `lag_seconds` 超过分钟级

1. 先看 worker 是否在消费：`running` attempt 数（Admin monitor）与 `RIVER_WORKER_CONCURRENCY`（默认 5，范围 1–64，改后重启 `api-worker` 生效）。depth/lag gauge 由 http `api` 进程导出（轮询 DB 真相），消费只发生在 `api-worker`。
2. 若 depth 涨、running 为 0：`api-worker` 进程异常或 River client 未启动；检查 `api-worker` 启动日志（其健康检查只是 DB 存活探针，`0 running` 必须看日志与本节 gauge，不要看 `/api/readyz`）。
3. 若 depth 涨、running 满：消费能力不足——检查网关 P95（下游慢会拖住 worker 360s 执行预算）或临时调大 concurrency。
4. 若 lag 抖动但 depth 不大：偶发长任务（大图/高分辨率）属正常；持续超过 10 分钟按 C.3 处理。
5. 注意：NATS 与此无关（NATS 只是 event 层，不承载任务队列）。

### D. `ideaboard_gateway_requests_total{outcome="error"}` 上升

1. 按 `service` 定位受影响能力；用 worker 日志中的 HTTP status／错误分类区分 429、5xx、超时和传输失败。本地 Gateway transport 不重发，不能用 `outbound_retries_total` 的 `reason` 统计这些失败。
2. 429 集中：先检查 `ideaboard_gateway_per_model_limited_total`，区分本地并发／熔断拦截和上游限流；结合对应 provider/model 映射的调用量决定是否调整 concurrency。
3. 5xx/超时集中：上游故障——查看网关 CacheStatus 与 LogID（receipt 头），必要时切换模型映射（价格不受模型影响，见 ADR-0010）。
4. 其余确定性 4xx 集中：配置或输入问题——检查 AI Gateway 凭据、模型映射、workflow 参数校验；确定性失败不重投。

### E. `ideaboard_gateway_retries_total` 上升

1. 该计数 = transient 网关失败后的 worker 层重投（上限 2 次，以 `tasks.retry_count` 为界），不表示已观测到 Cloudflare 内部重试次数。
2. 偶发少量：正常韧性行为，无需处理。
3. 持续上升：同 D（上游持续 transient），且注意重投会放大调用量——与 D 联动判断是否需要降级或切换模型。
4. 若 retries 涨但 gateway error 不涨：检查 worker 是否存在非网关触发的重投路径回归（当前仅 provider 失败路径计数）。

## River 重试矩阵（[backend-hardening] 08）

重试/超时 policy 由 Worker 代码持有（`internal/platform/riverpolicy.RiverRetryMatrix` 为事实源，insert 点引用同包常量）。attempt 间隔为 River 默认指数 backoff + jitter；本地 Gateway transport 每次调用只发送一次，不对结果不明的请求重发。Provider 重试由 Cloudflare policy 管理（`MaxFailCount=5`，probe 显式 `MaxAttempts=1`）；worker 层 transient 整任务重投最多 2 次（`tasks.retry_count` 为界）。

| Queue | MaxAttempts | 排障含义 |
|-------|-------------|----------|
| `drawing_default` | 25 | depth 涨先看 C 节；`attempt` 接近 25 的 job 多为反复 panic/基础设施故障，查 worker 日志而非网关 |
| `credit_reconciliation` | 5 | 预算小：失败 5 次即进 River `discarded`，直接查对账 Run 而非等自愈 |
| `payment_reconciliation` / `payment_checkout_shutdown` | 8 | Stripe/DB transient 可自愈；耗尽看 Payments 事故处理 |
| `vision_billing` | 5 | 快照对账失败看 searchcatalog 计费链路 |
| `media_reclamation` | 10 | 存储抖动自愈空间最大；持续失败看 storage profile 与对象可达性 |
| `workspace_invitation_mail` | 8 | 事务内入队的 Workspace 邀请邮件；失败看 Auth 邮件 readiness／Provider；密钥版本不匹配需恢复原主密钥，不能跳过解密检查 |
| `webhook_deliver` | 8 | 单 delivery 8 次耗尽即进 River `discarded`（即 durable DLQ），`discarded` 行即投递终态；attempt 间隔为 30s 起 doubling + jitter（1h cap） |
| `task_settlement_sweep` | 5 | 「最终 timeout」收敛 sweep：把 24h 无 worker 触碰、仍持 Credit Hold 的 running/pending 任务收敛为 failed 并 Release。job 失败只可能是 sweep 级故障（DB 不可用/wiring 缺失）；单任务 Release 失败会打 `stalled task settlement convergence failed` 并被隔离，不消耗 job 的 attempt 预算 |

`ideaboard_gateway_retries_total` 只统计 `drawing_default` 的 worker 层重投，维护队列不计入该指标；Cloudflare 内部 attempts 不由本地 outbound retry 指标观测。

## River 超时矩阵（#783）

超时 policy 由 Worker 代码持有（`internal/platform/riverpolicy.RiverTimeoutMatrix` 为事实源，每个 worker 的 `Timeout()` 引用同包常量）。Drawing Worker 的 River `Timeout()` 固定为 17min；`Work` 内通过 `WorkspaceExecutionBudget()` 单独限制 provider execution 为 360s，Complete／Fail／Retry 继续使用外层 context。River 的 worker `Timeout()` 会覆盖 client `JobTimeout`，两者不会叠加。

| Queue | Worker | Timeout | 排障含义 |
|-------|--------|---------|----------|
| `drawing_default` | TaskWorker | 17min | 内部 provider execution 单独限制为 360s（含主模型和 fallback），网关单次请求为 300s；执行超时先看网关 P95 与 C 节 |
| `drawing_default` | FinalFailureWorker | 1min | settlement-only，无 provider 调用；超时看终态收敛 DB 写入 |
| `credit_reconciliation` | CreditReconciliationWorker | 4min | 单次对账 Run；超时看 credits 对账链路 |
| `credit_reconciliation` | CreditReconciliationScheduleWorker | 1min | 单次 schedule 写；超时基本可判为 DB 故障 |
| `payment_reconciliation` | PaymentReconciliationWorker | 4min | Stripe/DB sweep；超时看 Payments 事故处理 |
| `payment_checkout_shutdown` | PaymentCheckoutShutdownWorker / ScheduleWorker | 4min | 同上 |
| `vision_billing` | VisionBillingReconciliationWorker | 4min | 低于 5min 调度周期；超时看 searchcatalog 计费链路（超时不会堆积到下个周期） |
| `media_reclamation` | ReclaimWorker | 2min | 低于 5min claim lease；超时看 storage profile 与对象可达性 |
| `media_reclamation` | ScheduleWorker | 2min | scan（上限 100）+ 入队扇出；超时看 DB 慢查询 |
| `workspace_invitation_mail` | WorkspaceInvitationMailWorker | 1min | 单次 Auth 邮件投递；队列只存 Token 密文，取消／过期邀请直接收敛，无外发 |
| `webhook_deliver` | WebhookDeliverWorker | 1min | 单次签名 POST 投递；超时看目标端点可达性 |
| `task_settlement_sweep` | TaskSettlementSweepWorker | 4min | 单次有界 sweep（发现上限 100 + 逐任务 settle 事务），无 provider 调用；低于 15min 调度周期，超时看 DB 慢查询与 settle 事务锁等待 |
| `identity_admin_reconciliation` | IdentityReconciliationExecuteWorker | 4min | 有界 identity drift scan 与 finding 写入；超时看 Identity 对账与 DB 慢查询 |
| `identity_admin_reconciliation` | IdentityReconciliationScheduleWorker | 1min | 单次幂等 schedule/catch-up 写入与事务内 enqueue；超时看 DB 故障 |

## 告警建议阈值（初始值，按线上观察调优）

| 信号 | 建议阈值 |
|------|----------|
| 路由 5xx率 | `> 1%` 持续 5m（提交/支付类 `> 0.5%`） |
| 路由 P95 | 提交路由 `> 2s` 持续 10m |
| River depth | `> 100` 持续 5m |
| River lag | `> 300s` 持续 5m |
| 网关 error率 | 按 service `> 5%` 持续 5m |
| 网关 retries | 突增（环比 3 倍）持续 10m |

## 相关文档

- 系统契约与 context map：[DESIGN.md](../../DESIGN.md)
- 部署与反代：[deployment.md](./deployment.md)
- 网关 sole-egress 与重试/缓存/超时：ADR-0008、ADR-0010（`docs/adr/`）
