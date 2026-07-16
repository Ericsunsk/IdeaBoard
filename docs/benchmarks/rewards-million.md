# Rewards Million-Redemption Benchmark

该 harness 验证 Rewards 在百万级累计 Redemption 下的关键读取计划与 Hot Public Campaign 并发容量不变量。两个测试默认跳过，不增加日常 `go test ./...` 时间。

## Run

`API_TEST_DATABASE_DSN` 必须指向可创建 schema 和执行 migration 的隔离 PostgreSQL test database。每个测试使用 `testdb` 创建独立 schema，并在结束时自动删除。

百万 Redemption dataset：

```bash
cd apps/api
export API_TEST_DATABASE_DSN='postgres://...'
REWARDS_LOAD_TEST=1 go test ./internal/rewards \
  -run '^TestRewardMillionRedemptionDataset$' -count=1 -v
```

`REWARDS_LOAD_TEST_REDEMPTIONS` 可提高规模，但不能低于 `1,000,000`。Harness 使用 `generate_series` 批量生成 Users、Wallets、balanced Credit Transactions/Postings、Public Campaign Redemptions、目标用户跨 Campaign 历史，以及 Batch read 数据。昂贵的逐行 Ledger/Redemption trigger 仅在同一 seed transaction 内临时关闭，并在 commit 前恢复；随后通过独立查询校验 balanced postings、Redemption 数量、用户唯一性和历史事实。

完成 `ANALYZE` 后，harness 直接复用 production SQL constants，对 dual-HMAC exact Code lookup、User history、Admin Campaign Redemption pagination、Campaign search/mode/derived-status list、detail 和完整 Batch metadata reads 执行正常 planner 的 `EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON)`。选择性查询必须命中对应 composite B-tree；当 fixture 分布使 PK backward scan 只读取 page limit 数量的行时允许 bounded PK path，但始终拒绝 `reward_codes`、`reward_redemptions` 上的无界 `Seq Scan`。独立 migration integration test 另外硬校验全部 composite index 的存在、定义、rollback 与 reapply。Harness 同时记录 public Service seam 的 lookup/history/Admin page p50 与 p95。

Hot Public Campaign：

```bash
cd apps/api
export API_TEST_DATABASE_DSN='postgres://...'
REWARDS_LOAD_TEST=1 go test ./internal/rewards \
  -run '^TestRewardHotPublicCampaign$' -count=1 -v
```

Hot harness 以 10ms 间隔提交 1,000 次不同用户的同一 Public Code 兑换，目标约 100 RPS；Campaign capacity 为 800。它记录完成吞吐、p50/p95/p99，以及从 `pg_stat_activity` 采样的 Lock wait 次数与峰值，并断言 Campaign projection、Redemptions、Credit Grants、用户可用余额增量完全一致且不超发。没有 application retry、partition、cache 或额外 counter。

## Reference Result

2026-07-16 本地 Docker Desktop PostgreSQL 环境（warm local filesystem cache，单次 wall-clock 测量）：

| Item | Result |
| ---- | ------ |
| PostgreSQL / Go / Host | 17.10 / Go 1.26.5 / macOS arm64, 8 logical CPUs |
| Dataset | 1,000,000 Redemptions、1,000,000 Users/Wallets/Credit Transactions、2,000,000 balanced Postings；另含 200 条目标用户历史、1,000 Batches / 10,000 Batch Codes |
| Seed duration | 3m37.856s |
| Query plans | 所有 guards 通过；无 `reward_codes` / `reward_redemptions` 无界 `Seq Scan` |
| Exact Code lookup p50 / p95 | 565.833µs / 763.209µs |
| User history p50 / p95 | 736.125µs / 989.708µs |
| Admin Redemption page p50 / p95 | 563.083µs / 626.542µs |
| Hot throughput | 1,000 requests / 10.005s = 99.9 requests/s |
| Hot p50 / p95 / p99 | 5.589ms / 7.412ms / 12.413ms |
| Lock wait samples / peak waiters | 1 / 1 |
| Capacity / committed Redemptions | 800 / 800；其余 200 正确拒绝，Credit Grants 与余额增量一致，无超发 |

这些结果是回归基线，不是 production SLO。只有 query plan、唯一性、Ledger balance、capacity 和不超发断言属于硬性 contract；延迟会受硬件、PostgreSQL cache、storage 与并发连接数影响。
