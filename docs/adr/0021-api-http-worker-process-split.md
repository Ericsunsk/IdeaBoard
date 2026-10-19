---
owner: Runtime
decision_date: 2026-10-19
supersedes: none
---

# API HTTP/Worker process split

## Context

`apps/api` 当前是单进程：同一进程既挂 Gin HTTP 路由（含 SSE），又启动 River
client 消费全部队列（drawing/GPU 长耗时执行、media、retention/sweep、对账）。
GPU/长耗时任务占满 worker 并发时会挤占轻量维护 job 的调度，而把 HTTP 与消费
者绑在同一进程里，重启、扩缩、排障都只能整进程一起动。需求是同库双进程拆分，
且明确不是真微服务：同一二进制、同一 PostgreSQL、不做代码级服务拆分。

## Decision

1. **同二进制同库零代码拆分的双进程。** `apps/api` 仍是一个二进制、一套
   wiring；进程角色只由 `API_MODE`（`all | http-only | worker-only`，默认
   `all`，cobra `--mode` 可覆盖）决定。`http-only` 不注册任何 River worker、不
   调度 periodic job（enqueue-only client，只负责 submit 事务内的 River
   enqueue）；`worker-only` 不挂任何 HTTP 监听（无 Gin 路由、无 metrics
   listener，只跑 workers）；`all` 保持现行行为，本地开发不动。
2. **队列归属：worker 跑全部 River 队列，HTTP 只留 API + SSE。** worker-only
   消费 `riverpolicy.RiverRetryMatrix` 的全部队列（`drawing_default` 的
   drawing/GPU/长耗时执行与 settlement-only 收敛、`media_reclamation`、
   `task_retention`（含 workspace purge）、`task_settlement_sweep`、
   `webhook_deliver`、credit/payment/vision/identity 对账）；periodic schedule
   job 只归 worker（单一 scheduler，避免双进程重复入队）。http-only 只做
   API + SSE 与事务内 enqueue；跨实例 SSE fanout 仍走 NATS（#940 bridge 不变），
   两进程共享同一 DB 与同一 River 表。
3. **重访条件：出现以下任一情况时再议真微服务。** GPU 执行需要独立伸缩
   （独立 concurrency/实例数/机型）、队列出现 ownership 分治（某队列需要独立
   deploy/回滚/权限面）、或发生需要故障隔离的事故（慢队列拖垮 API 读写的真实
   案例）。在此之前不做 per-queue 进程、不拆二进制、不拆库。

## Consequences

- 正面：GPU/长耗时与 API 读写可独立重启、独立调 concurrency；`compose.yml`
  的 `api`（http-only）+ `api-worker`（worker-only）同镜像不同 mode，
  发布与回滚仍是单 artifact；River depth/lag gauge 仍由 http 进程的 metrics
  端点导出（轮询的是 DB 真相，与消费进程无关），排障口径不变。
- 代价：worker-only 无任何 HTTP 监听（metrics 也不挂），其 RED gauge 在
  worker 侧不可 scrape，只能经 http 进程的 gauge 或 DB 直行查询观察；compose
  的 worker 健康检查从 HTTP 探针改为 DB liveness 探针（`SELECT 1`），只能证明
  存活，不能证明消费进度（消费进度看 queue depth/lag 与 `running` attempt）。
  `api-worker` 保持单副本：多 worker 副本会多 scheduler 重复入队，扩 worker
  之前先验证 periodic-job 去重；http 副本可水平扩展（NATS fanout 已覆盖多实例）。
- 账本/结算簇不拆：credit/payment 对账与 `task_settlement_sweep` 留在同一个
  worker 进程内，因为提交与终态结算是同一 pgx 事务原子完成的（server-side
  Quote snapshot + RoutingSnapshot 固化 + Credit Hold + task insert + River
  enqueue 一事务；成功终态与 Capture、失败/取消/最终 timeout 与 Release 各一
  事务，见 DESIGN.md）。把 Hold/Capture/Release 拆到不同进程就是把单事务拆成
  分布式事务，需要 saga/补偿、对账窗口与跨进程幂等，代价远超同进程内一次提交；
  账本写入面保持 `credits.Commands` 单一入口，不为进程拆分开第二写面。

## Status

Accepted
