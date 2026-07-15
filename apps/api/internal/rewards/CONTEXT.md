# Rewards Context

Rewards 管理平台 Idea 奖励的 Campaign、Code 与 Redemption。用户展示使用 `Idea/Ideas`；内部数量、代码、API 与 schema 使用 `Credit/credits`。

## Domain Language

| Term | Meaning |
| ---- | ------- |
| Campaign | 固定 Credit 数量、可选有效时间与 mode-specific 总上限的奖励活动；支持 `public` 与 `single_use` mode |
| Public Code | 多人共享的 bearer Code；明文只在首次发行响应返回 |
| Single-use Code | 由 Code Batch 生成、最多完成一次 Redemption 的 bearer Code |
| Code Batch | Single-use Code 的原子发行与不可逆未使用撤销边界；明文只在首次 generation 响应返回 |
| Redemption | 用户成功领取 Campaign 的不可变事实 |
| Admin Action Journal | Rewards context 内 append-only 的 actor/reason 操作记录 |

## Public Seams

- `AdminCommands` 创建或更新 Campaign、发行唯一 Public Code 或幂等生成 Single-use Code Batch，并执行 Batch revoke、activate、pause、resume、cancel 与空 Draft delete commands。
- `AdminReads` 提供不含 Code 明文或 digest 的 Campaign 与 keyset-paginated Code Batch metadata read。
- `UserCommands` 同步兑换 Code，返回 Redemption、credited amount、available balance 与 replay marker。
- `UserReads` 按 ID cursor 返回当前用户的成功 Redemption history，仅含 Campaign display name、credited amount、masked Code hint 与兑换时间。

Hash normalization、HMAC candidates、row locking、persistence 和 Credits transaction orchestration 都留在 module implementation 内。

## Invariants

- Code 明文或可逆密文不得进入 database、Admin list、URL、Audit 或日志；只保存 key version、HMAC-SHA256 digest 与四字符 hint。
- Single-use Code 使用 CSPRNG 生成 80-bit payload，以 Crockford Base32 的稳定 `XXXX-XXXX-XXXX-XXXX` 格式首次返回；单 Batch 同步上限为 10,000。
- Batch generation 以 `(campaign_id, idempotency_key)` 幂等；相同 payload replay 只返回 metadata 与 `export_unavailable`，不同 payload conflict，任何随机源、唯一性或持久化失败均整批 rollback。
- Rewards 拥有 caller transaction，并只通过 `credits.Service.InTx(tx).Grant` 写入 Credits Ledger。
- Campaign capacity、Credits Grant 与 Redemption 同时 commit 或 rollback。
- Credit Grant 固定使用 source `reward_redemption`、reason `兑换奖励` 和 Redemption ID business reference。
- authenticated Admin 可通过 Credits Correction 逆转该 Grant 的账务结果；Correction 是新的 Ledger 事实，不删除 Redemption，也不恢复 Campaign capacity。
- Public Campaign 必须有正整数 Credit amount、有限正整数总上限和唯一 Public Code 才可激活；Single-use Campaign 的 `max_redemptions = 0` 表示 unlimited，正数表示有限总 cap。
- Campaign mode 互斥：Public Campaign 不得含 Batch Code，Single-use Campaign 不得含 Public Code。
- Campaign 持久化 lifecycle 为 `draft | active | paused | cancelled`；read status 按 `draft → cancelled → expired → exhausted → paused → scheduled → active` precedence 派生。
- `starts_at` 与 `expires_at` 以 UTC instant 保存，区间 start-inclusive/end-exclusive；pause 不顺延过期时间，cancelled 是不可恢复终态。
- 首个 Code 发行后冻结 display name、Credit amount、mode、cap、schedule 与 creator；internal note 和合法 lifecycle transition 仍可变。
- 只有无 Code 的 Draft 可物理删除；取消原子 revokes Code，但保留 Campaign、Redemption、Ledger 与 Journal。
- 每个 User 每个 Campaign 最多一个 Redemption；Campaign row lock 使 capacity、状态切换、Redemption 和 Credits Grant 在同一 transaction 中保持一致。
- User Redemption history 只读取已提交 Redemption，并按 `(user_id, id DESC)` 隔离和分页；不 join 或复制 Identity PII、Admin note、Code digest 或明文。
- 每个 Single-use Code 最多一个 Redemption；service 与 database Code row lock guard 双重约束并发消费。
- Single-use Batch capacity 使用 `completed + valid unused + new <= cap`；Draft、scheduled、active、paused 与仅因 Code 用尽的 exhausted 可追加，expired、cancelled 或总 cap 耗尽不可追加。
- Batch revoke 与 Single-use Campaign cancel 只不可逆撤销 unused Code；已兑换 Code、Redemption 与 Credits Ledger 不变。
- `reward_admin_actions` 由 database trigger 和 runtime privileges 双重保证 append-only。
- `REWARDS_CODE_HMAC_KEYS` 只接受 current 与可选 previous；每个 base64 key 解码后至少 32 bytes，配置无效时 startup fail closed。

## Current Boundary

当前不包含 rate limiter/security expansion 或 server-side CSV persistence。CSV 由 Admin UI 仅从首次 generation response 生成；server 不保存 export 文件或可恢复 Code 明文。
