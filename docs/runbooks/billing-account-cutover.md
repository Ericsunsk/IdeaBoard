# Billing Account 资金链切换运维手册（#915–#924；#944–#948 扩展见范围）

本文回答 Billing Account 统一资金模型上线时的三个运维问题：如何停止新资金写入、
如何验证数据转换、失败后如何恢复与回退。通用 backup/restore、密钥轮换与发布流程见
[data-protection.md](./data-protection.md) 与 [deployment.md](./deployment.md)，
本文不复制，只给账务特有的步骤与校验。

范围：`20261021000000_billing_account_sharing`（账户共享基础）、
`20261022000000_billing_account_wallets`（Wallet 账户化 + 存量归属回填，
含评审后破坏性 DDL，`-- destructive: approved`）、
`20261023000000_vision_workspace_settlement`（Vision 结算三元组）、
`20261024000000_payment_billing_account`（订单 payer 归属）、
`20261027000000_task_billing_workspace_attribution`（Task 结算行内 workspace 归属）、
`20261028000000_billing_handover_invites`（Studio handover invite 审计事实）、
`20261029000000_workspace_budgets`（Workspace 共享消费预算，#945）、
`20261030000000_member_quotas`（成员跨空间消费额度，#946）、
`20261031000000_default_member_quotas`（账户默认新成员额度，#947）。
最终结构以 [ADR-0024](../adr/0024-billing-account-resolution.md)～
[ADR-0031](../adr/0031-admin-explicit-billing-accounts.md)、
[ADR-0032](../adr/0032-billing-account-integration-gate.md)、
[ADR-0033](../adr/0033-workspace-creation-billing-account.md)～
[ADR-0037](../adr/0037-account-default-member-quota.md) 与
[Credits CONTEXT](../../apps/api/internal/credits/CONTEXT.md) 为准。

## 1. 停止新资金写入

没有全局资金总开关（设计如此：避免一个误操作冻结全平台）。停写 = 停入口 +
排空执行器，再校验静默：

1. 反代切维护模式，停掉新请求（含充值 checkout、任务提交、兑换、Vision 搜索）。
2. 停 API 与 worker（生产 Compose 服务名）：
   `docker compose stop api api-worker`（`API_MODE` 拆分部署见
   [deployment.md](./deployment.md)；Auth/Realtime 可保持登录态，只停写账路径）。
3. 确认静默（owner 连接，只读查询）：
   - River 无待执行任务（按部署的队列语义查 `river_job`，无可执行行）；
   - 无进行中的对账：`SELECT count(*) FROM credit_reconciliation_runs
     WHERE status IN ('queued','running');` 必须为 0；
   - 未完 Hold 是 durable 资金事实，原地保留，不得为停写而释放或删除；
     恢复服务后它们随 Task 终态经 Capture/Release 照常收敛。
4. 按 [deployment.md](./deployment.md) §4 做 PostgreSQL + uploads backup 并校验
   checksum；记录操作人、时间、backup 路径。

## 2. 数据转换验证

转换语义（只加归属列、不改资金事实）：既有个人余额、历史 Ledger、未完 Hold
经 `ensure_personal_billing_account_for_user` 归入每人初始个人账户；金额与
postings 不动；个人资金不注入组织（组织/共享账户从 0 开始）；
Vision 历史行 `workspace_id` 保持 NULL 不重放；订单/退款/争议行回填确定的原始
payer。Workspace hard delete 后 Wallet、projection、Ledger、snapshot、operation
行在 `billing_account_id` 下保留可读（与 `task_billing_snapshots.task_id`
无 task FK 的先例一致）。

Migration 成功后、恢复写入前，逐项执行（任一非 0 即停）：

```sql
-- 归属完整性：三类核心表不允许 NULL 归属（CHECK 约束同样保证增量写入）
SELECT count(*) FROM credit_wallets WHERE billing_account_id IS NULL;
SELECT count(*) FROM credit_transactions WHERE billing_account_id IS NULL;
SELECT count(*) FROM credit_holds WHERE billing_account_id IS NULL;
SELECT count(*) FROM credit_wallet_restrictions WHERE billing_account_id IS NULL;
SELECT count(*) FROM payment_orders WHERE billing_account_id IS NULL;
SELECT count(*) FROM payment_customers WHERE billing_account_id IS NULL;
-- 旧例程已下线（只剩 v3 通用 + v2 支付专用账户例程）
SELECT proname FROM pg_proc
WHERE proname IN ('execute_credit_command_v2','provision_credit_wallet',
  'set_credit_wallet_status','execute_credit_purchase_v1');
-- 未完 Hold 逐笔可解释：每笔 held 必须能在源事务找到归属账户
SELECT h.hold_transaction_id FROM credit_holds h
LEFT JOIN credit_transactions t
  ON t.transaction_id = h.hold_transaction_id
  AND t.billing_account_id = h.billing_account_id
WHERE h.status = 'held' AND t.transaction_id IS NULL;
```

然后触发一次手动对账并跟到终态：Control `POST /credits/reconciliation/runs`
（manual），在 Admin Credit Health 确认状态回到 `healthy`；
`warning/low` 只报告不限制钱包，`error|critical/high` 必须先修资金事实再放行。

本 gate 已在隔离库演练：7 个 migration Fresh apply（575 statements）；
确定性归属幂等、新账户 0 起、Grant→Hold→Capture 收敛与余额（900/0）、
同 key 同 hash 重放、同 key 异 hash 冲突、跨账户结算拒绝、零余额争议只记
unrecovered 不写账、hard-delete 后钱包/投影/Ledger/快照保留可读。
上述演练只覆盖 #915–#924 的 4 个账务 migration（21–24）；`20261027` 起的
归属列与 28–31 的交接/预算/额度表不在该次演练内，执行前按当前 baseline 重演。
演练脚本只跑隔离库，绝不动生产库。

## 3. 失败恢复

- Migration 中途失败：`atlas migrate apply` 是 versioned、失败即停；
  按 [deployment.md](./deployment.md) 的 migration 失败流程处理：
  migration 前失败恢复 `.env` snapshot；migration 后失败不自动 restore，
  用备份 + release 记录人工决策。绝不手改 `atlas_schema_revisions`、
  绝不补空 migration 兼容旧链（pre-launch 只认 Fresh + 当前 baseline）。
- Task 终态暂时写不下：settlement-only marker + snooze 重试只做终态收敛，
  不重调 provider；marker 本身写失败则进 durable fallback 队列。恢复服务后
  等 River 自然收敛，不要手补 Ledger。
- 退款/争议：`purchase_refund` 绑定 Refund Hold，累计上限与幂等防止双结算；
  余额不足的退款呈 `ErrInsufficientCredits` durable anomaly 走人工；
  零余额争议记 unrecovered，充资后按原 dispute 重放（restore 按已扣金额封顶）。
- Welcome/Reward：claim 指纹绑定验证身份，重放不二次发放；HMAC key 漂移
  fail closed，先对齐 key 指纹再重试。

## 4. 回退

- 应用镜像回退不等于数据库恢复。`previous.json` 的 digest 回滚只换代码；
  已 apply 的 migration 不会自动撤销，Funds 事实（Ledger append-only、
  snapshot immutable）本来就不支持反向修改。
- 需要回退数据时唯一路径是整库 restore（[data-protection.md](./data-protection.md)
  受控流程：隔离校验 → 解密 → 确认目标可覆盖 → 恢复 → smoke → 密钥轮换），
  且必须同时评估 uploads backup 与 migration 版本：restore 到旧 backup 后，
  必须用与该 backup 一致的镜像重跑，不存在"新代码 + 旧库"的混搭状态。
- `ATLAS_CLEAN_BREAK_ALLOW_RESET=true` 是一次性清库开关，只在确认放弃全库
  数据后的一次性 migration 容器上设置，用完即删；绝不进入长期运行容器。
- Workspace 级误删不在此列：hard delete 无恢复期，单 workspace 永不触发整库
  restore（见 [data-protection.md](./data-protection.md) 误删政策）。
- 本手册所有写操作（停服、migration、restore、清库开关）均为人工确认操作；
  任何自动化不得执行生产数据变更、push 或部署。
