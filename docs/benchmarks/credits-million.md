# Credits Million-Transaction Benchmark

该 harness 验证 PostgreSQL Credits Core 在 100 万 Ledger Transactions / 200 万 Postings 数据集上的 correctness、Reconciliation、查询延迟、锁隔离和多 Wallet throughput。它默认跳过，不增加 CI 时间。

## Run

```bash
cd apps/api
export API_TEST_DATABASE_DSN='postgres://...'
CREDITS_LOAD_TEST=1 go test ./internal/credits \
  -run '^TestCreditMillionTransactionDataset$' -count=1 -v
```

`API_TEST_DATABASE_DSN` 必须是隔离 test database 的 migration owner，而不是 API runtime role。`CREDITS_LOAD_TEST_TRANSACTIONS` 可提高规模，但不能低于 1,000,000。Harness 以 owner 权限在隔离 schema 中 bulk-seed 数据，projection 仍由 posting trigger 生成；重新启用 integrity triggers 后执行独立 balance query 和完整 Reconciliation，再通过 public Service seam 验证 Wallet、Ledger、同钱包竞争、不同钱包锁隔离，以及 1,000 个不同 Wallet 的并发 Hold throughput。测试结束自动删除 schema。

## Reference Result

2026-07-12 本地 Docker 环境（Docker Desktop PostgreSQL，warm local filesystem cache，单次 wall-clock 测量）：

| Item | Result |
| ---- | ------ |
| PostgreSQL | 17.10 |
| Go | 1.26.4 |
| Host | macOS arm64, 8 logical CPUs |
| Dataset | 1,000,000 Transactions / 2,000,000 Postings |
| Seed | 1m11.099s |
| Independent balance verification | 893.543ms |
| Full Reconciliation | 2.741s, 0 findings |
| PostgreSQL database size | 1,286,600,371 → 1,286,633,139 bytes (+32 KiB report metadata) |
| Go process HeapAlloc around Reconciliation | 841,528 → 849,776 bytes (+8,248 bytes) |
| Wallet balance read | 0.776ms |
| Latest 50 Ledger Transactions | 64.886ms |
| 32 concurrent Holds on one Wallet | 83.841ms, no overspend |
| Grant on another Wallet while first Wallet transaction stayed open | 5.647ms, no global system-account blocking |
| 1,000 Holds on 1,000 pre-provisioned Wallets, 64 goroutines | 539.579ms = 1,853.3 commands/s |

“100 万笔/天”平均约 11.6 tx/s；20× 目标为 232 commands/s。本次先创建 1,000 个测试用户并完成 Wallet/2 点子 Grant 预置，预置时间不计入 throughput；计时边界只包含 64 个 goroutine 从开始并发执行到 1,000 个不同 Wallet 的 Hold 全部 commit。实测 1,853.3 commands/s，约为平均负载 160×、最低 20× 目标的 8.0×。命令使用 per-Wallet system account shard，open transaction 锁住一个 Wallet 时另一 Wallet 仍在 5.647ms 完成，未出现全局 system account 串行化。

资源指标在同一 harness 内采集：数据库占用通过 Reconciliation 前后 `pg_database_size(current_database())` 读取；Go heap 通过前后两次 `runtime.ReadMemStats().HeapAlloc` 读取。它们是进程/数据库级 point-in-time 指标，不等同于 peak RSS 或 PostgreSQL buffer peak，因此用于判断扫描是否把结果集拉入 Go process，以及 report metadata 的量级；production capacity planning 仍应接入容器 RSS、PostgreSQL shared buffers/temp bytes 与 I/O telemetry。

当前 100 万 Transactions 下完整扫描为 2.741s，现有 `credit_postings(transaction_id)`、`credit_postings(account_id)`、`credit_transactions(related_transaction_id)`、Wallet/status 和 Task Billing PK/index 足够。暂不 partition，避免增加 routing/maintenance complexity。触发重新评估的明确阈值是：单表达到 1 亿 rows、full scan 超过 5 分钟运营窗口、或扫描产生持续 temp-file spill / I/O saturation 任一项；届时优先按 `credit_transactions.created_at` 做 monthly time-range partition，并保持 `transaction_id`/idempotency 全局唯一策略，Reconciliation 再改为 high-water-mark incremental scan + 周期性 full scan。findings/run tables 体量远小于 Ledger，继续使用普通 B-tree。

这些数字用于回归比较，不是 production SLO；硬件、PostgreSQL cache 和 container storage 会显著影响结果。

## User Usage Read Model Baseline

用户侧用量统计使用同一个显式 load-test 开关，但由独立 harness 负责造数、Query Plan 与 latency 采样：

```bash
cd apps/api
export API_TEST_DATABASE_DSN='postgres://...'
CREDITS_LOAD_TEST=1 go test ./internal/usage \
  -run '^TestUsageMillionTransactionDataset$' -count=1 -v
```

`API_TEST_DATABASE_DSN` 仍必须指向 migration owner；harness 在隔离 schema 中运行并自动清理。`CREDITS_LOAD_TEST_TRANSACTIONS` 可提高规模，但不能低于 1,000,000。数据集包含 100 个用户、1 个目标重用户和跨用户 noise；目标用户占 10%，用于同时观察 selective range scan 与较重的 Lifetime 聚合。latency 采样直接调用未缓存的 Usage compute seam，因此不会被应用层 30 秒 cache 命中掩盖。

2026-07-13 本地 Docker 环境结果：

| Item | Result |
| ---- | ------ |
| PostgreSQL | 17.10 |
| Go | 1.26.5 |
| Host | macOS arm64, 8 logical CPUs |
| Dataset | 1,000,000 Capture Transactions / 2,000,000 balanced Postings / 1,000,000 Task Billing Snapshots |
| Users | 100；目标用户 100,000 Capture facts + 100,000 Task facts |
| Capture seed | 1m35.331s |
| Task Billing snapshot seed | 22.164s |
| Independent fact verification | 2.217s |
| Planner check | `EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON)` 命中 `idx_credit_transactions_capture_user_created` |
| Uncached `7d` | p50 74.129ms / p95 82.243ms |
| Uncached `30d` | p50 74.273ms / p95 91.466ms |
| Uncached `1y` | p50 100.195ms / p95 102.928ms |
| Samples | 每个 range 1 次 warm-up + 20 次采样 |

该基线下 365 个 daily buckets 的 uncached p95 仍低于 110ms，因此首版继续实时聚合 immutable Credit Capture 与 Task Billing snapshot，不引入 Daily Projection。API 实例使用 feature-local、容量 2,048、TTL 约 30 秒的 LRU cache；多实例之间不做 distributed invalidation，因此最多约 30 秒 eventual freshness 是明确 contract。上述 latency 仍是 warm local filesystem/PostgreSQL cache 下的回归基线，不是 production SLO；若线上 p95、buffer reads 或用户级数据倾斜明显超出该量级，再单独设计 projection，而不是把它隐式加入当前 read path。
