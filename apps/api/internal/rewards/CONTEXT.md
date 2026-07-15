# Rewards Context

Rewards 管理平台 Idea 奖励的 Campaign、Code 与 Redemption。用户展示使用 `Idea/Ideas`；内部数量、代码、API 与 schema 使用 `Credit/credits`。

## Domain Language

| Term | Meaning |
| ---- | ------- |
| Campaign | 固定 Credit 数量、有限总上限与可选有效时间的奖励活动；当前只支持 `public` mode |
| Public Code | 多人共享的 bearer Code；明文只在首次发行响应返回 |
| Redemption | 用户成功领取 Campaign 的不可变事实 |
| Admin Action Journal | Rewards context 内 append-only 的 actor/reason 操作记录 |

## Public Seams

- `AdminCommands` 创建或更新 public Draft、发行唯一 Public Code，并执行 activate、pause、resume、cancel 与空 Draft delete lifecycle commands。
- `AdminReads` 提供不含 Code 明文或 digest 的 Campaign read。
- `UserCommands` 同步兑换 Code，返回 Redemption、credited amount、available balance 与 replay marker。

Hash normalization、HMAC candidates、row locking、persistence 和 Credits transaction orchestration 都留在 module implementation 内。

## Invariants

- Code 明文或可逆密文不得进入 database、Admin list、URL、Audit 或日志；只保存 key version、HMAC-SHA256 digest 与四字符 hint。
- Rewards 拥有 caller transaction，并只通过 `credits.Service.InTx(tx).Grant` 写入 Credits Ledger。
- Campaign capacity、Credits Grant 与 Redemption 同时 commit 或 rollback。
- Credit Grant 固定使用 source `reward_redemption`、reason `兑换奖励` 和 Redemption ID business reference。
- authenticated Admin 可通过 Credits Correction 逆转该 Grant 的账务结果；Correction 是新的 Ledger 事实，不删除 Redemption，也不恢复 Campaign capacity。
- Public Campaign 必须有正整数 Credit amount、有限正整数总上限和唯一 Public Code才可激活。
- Campaign 持久化 lifecycle 为 `draft | active | paused | cancelled`；read status 按 `draft → cancelled → expired → exhausted → paused → scheduled → active` precedence 派生。
- `starts_at` 与 `expires_at` 以 UTC instant 保存，区间 start-inclusive/end-exclusive；pause 不顺延过期时间，cancelled 是不可恢复终态。
- 首个 Code 发行后冻结 display name、Credit amount、mode、cap、schedule 与 creator；internal note 和合法 lifecycle transition 仍可变。
- 只有无 Code 的 Draft 可物理删除；取消原子 revokes Code，但保留 Campaign、Redemption、Ledger 与 Journal。
- 每个 User 每个 Campaign 最多一个 Redemption；Campaign row lock 使 capacity、状态切换、Redemption 和 Credits Grant 在同一 transaction 中保持一致。
- `reward_admin_actions` 由 database trigger 和 runtime privileges 双重保证 append-only。
- `REWARDS_CODE_HMAC_KEYS` 只接受 current 与可选 previous；每个 base64 key 解码后至少 32 bytes，配置无效时 startup fail closed。

## Current Boundary

当前不包含 single-use Code Batch、用户兑换 history、rate limiter、安全扩展或 CSV export；这些由后续 Issues 扩展。
