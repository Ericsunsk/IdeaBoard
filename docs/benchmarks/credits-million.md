# Credits Million-Transaction Benchmark

该 harness 验证 PostgreSQL Credits Core 在 100 万 Ledger Transactions / 200 万 Postings 数据集上的 correctness、查询延迟和锁隔离。它默认跳过，不增加 CI 时间。

## Run

```bash
cd apps/api
export API_TEST_DATABASE_DSN='postgres://...'
CREDITS_LOAD_TEST=1 go test ./internal/credits \
  -run '^TestCreditMillionTransactionDataset$' -count=1 -v
```

`API_TEST_DATABASE_DSN` 必须是隔离 test database 的 migration owner，而不是 API runtime role。`CREDITS_LOAD_TEST_TRANSACTIONS` 可提高规模，但不能低于 1,000,000。Harness 以 owner 权限在隔离 schema 中 bulk-seed 数据，projection 仍由 posting trigger 生成；重新启用 integrity triggers 后执行独立 balance query，再通过 public Service seam 验证 Wallet、Ledger、同钱包竞争和不同钱包并行。测试结束自动删除 schema。

## Reference Result

2026-07-11 本地 Docker 环境：

| Item | Result |
| ---- | ------ |
| PostgreSQL | 17.10 |
| Go | 1.26.4 |
| Host | macOS arm64, 8 logical CPUs |
| Dataset | 1,000,000 Transactions / 2,000,000 Postings |
| Seed | 1m12.398s |
| Independent balance verification | 867.410ms |
| Wallet balance read | 1.556ms |
| Latest 50 Ledger Transactions | 55.803ms |
| 32 concurrent Holds on one Wallet | 90.555ms, no overspend |
| Grant on another Wallet while first Wallet transaction stayed open | 5.712ms, no global system-account blocking |

这些数字用于回归比较，不是 production SLO；硬件、PostgreSQL cache 和 container storage 会显著影响结果。
