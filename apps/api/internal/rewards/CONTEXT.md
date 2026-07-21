# Rewards Context

Rewards 管理平台 Idea 奖励的 RewardCode 与 Redemption。用户展示使用 `Idea/Ideas`；内部数量、代码、API 与 schema 使用 `Credit/credits`。

## Domain Language

| Term | Meaning |
| ---- | ------- |
| RewardCode | 名称、HMAC-only bearer Code、Credit 奖励、可选总 cap/过期时间与 enabled intent 的单一 aggregate |
| Redemption | 用户成功领取 RewardCode 的不可变事实 |
| Admin Action Journal | Rewards context 内 append-only 的 actor/action/time/safe metadata 记录 |

## Public Seams

- `AdminCommands` 提供 Create、Update、SetEnabled 与 Delete：Create 以 client idempotency key 创建；Update 在单个 transaction 内保存规则并可选轮换 bearer Code；SetEnabled 是 set-state；Delete 仅接受零 Redemption aggregate。
- `AdminReads.ListRewardCodes` 提供不含 plaintext/full digest 的 keyset-paginated 列表。
- `UserCommands.Redeem` 同步兑换 RewardCode，返回 Redemption、credited amount、available balance 与 replay marker。
- `UserReads.ListRedemptions` 按 ID cursor 返回当前用户的成功 Redemption history，仅含 RewardCode display name、credited amount、masked Code hint 与兑换时间。

Hash normalization、HMAC candidates、row locking、persistence 和 Credits transaction orchestration 都留在 module implementation 内。

## Invariants

- RewardCode 创建以 normalized Code 的 key-independent PostgreSQL advisory transaction lock 串行化；HMAC digest alias 同时覆盖 current/previous key，避免 rolling rotation 期间相同 plaintext 的双写。
- RewardCode create idempotency 以非敏感业务字段 fingerprint 和 persisted digest alias 双重校验；old-only 与 current+previous 实例在 rotation overlap 期间可对称 replay，同时不同 Code 或不同业务 payload 仍 conflict。
- RewardCode row lock、same-user replay、有效性/cap gate、capacity counter、Credits Grant 与 Redemption insert 严格位于同一 PostgreSQL transaction；Credit business reference 使用独立 `reward_code_redemption` namespace。
- 每个 User 每个 RewardCode 最多成功兑换一次；`max_redemptions = 0` 表示 unlimited。
- RewardCode plaintext 只允许出现在首次 successful create，或携带新 Code 的 successful update response；create replay、普通 update、Code replay、list、history、Journal、日志与错误均不得包含 plaintext 或 full digest，相关 HTTP response 始终 `Cache-Control: no-store`。
- Update 只改变后续 Redemption 使用的名称、Credit amount、cap 与 expiry；历史 Redemption 的 amount、Credit Transaction ID 与时间保持不可变。有限 cap 不得低于当前 Redemption count，`0` 恢复 unlimited。
- `enabled` 是 Admin intent；有效状态按 `expired > exhausted > disabled > enabled` 推导，前端以独立 Toggle 展示配置 intent，避免与实际可兑换原因混淆。
- Update 将规则变更与 optional Code rotation 收敛为单个 Admin PATCH 和同一 transaction；任一校验、collision 或 persistence 失败会整体 rollback。新 Code 会替换 primary digest 与全部 current/previous aliases，旧 Code commit 后立即失效；提交 Code 已命中目标当前 alias 时视为 replay，不重复 rotation Journal 且不返回 plaintext。
- Update/SetEnabled 对相同目标状态不重复 Journal。零 Redemption RewardCode 可 hard delete；Journal 的 nullable FK 使用 `ON DELETE SET NULL`，并永久保留 immutable target ID 与 safe target snapshot。
- Code 明文或可逆密文不得进入 database、Admin list、URL、Journal 或日志；只保存 key version、HMAC-SHA256 digest 与四字符 hint。
- Rewards 拥有 caller transaction，并只通过 `credits.Service.InTx(tx).Grant` 写入 Credits Ledger；capacity、Grant 与 Redemption 同时 commit 或 rollback。
- Runtime role 执行 `reserve_reward_code_redemption_v1` 是受限能力，仍可消耗一条当前 eligible RewardCode 的 cap；expiry 只由数据库时间判定，Grant 与 Redemption insert 继续由 application transaction 编排。
- Credit Grant 固定使用 source `reward_redemption`、reason `兑换奖励` 和 Redemption ID business reference。
- authenticated Admin 可通过 Credits Correction 逆转 Grant 的账务结果；Correction 是新的 Ledger 事实，不删除 Redemption，也不恢复 RewardCode capacity。
- User Redemption history 只读取已提交 Redemption，并按 `(user_id, id DESC)` 隔离和分页；不 join 或复制 Identity PII、Code digest 或明文。
- `reward_admin_actions` 自动记录 actor/action/time/safe metadata，不接收手填 reason，并由 database trigger 与 runtime privileges 双重保证 append-only。Guard 只放行 `ON DELETE SET NULL` 引发的 FK referential nullification：parent 必须已不存在且其他 Journal 字段完全不变。Runtime 不拥有 RewardCode/Alias/Journal 的直接 UPDATE/DELETE，只能执行 transaction-bound Rewards command functions，避免绕过 Journal。
- `REWARDS_CODE_HMAC_KEYS` 只接受 current 与可选 previous；每个 base64 key 解码后至少 32 bytes，配置无效时 startup fail closed。
- Code normalization 在任何 Unicode normalization 前执行 128-byte raw bound，再做 NFKC、uppercase、trim 与 Unicode whitespace/hyphen removal；最终 payload 只接受 8–32 个 `A-Z0-9`。
- HMAC rotation overlap 期间 lookup 同时验证 current/previous，新发行只使用 current；任何 persisted RewardCode（包括 disabled、expired 或未兑换）仍依赖其 primary key version，因此依赖旧 version 的记录存在时 previous key 不得退休。
- 用户 Redeem endpoint 自有 per-user in-process token bucket，默认 10 attempts / 10 minutes，并在流量到来时清理 idle entries；它不是跨实例业务 invariant，不扩展为 Platform interface 或持久化 bucket。
- Redeem 的不可用 Code 状态使用统一 public error；response 与日志不得包含 raw Code、full digest 或 request body，Redeem response 始终 `no-store`。
- Redemption observability 只记录固定 outcome、credited Ideas 与 latency；User/RewardCode/Code/digest 不得成为 metrics field 或 label。

## Current Boundary

当前使用普通 PostgreSQL B-tree、RewardCode row lock 与精确 counter，不包含 partitioning、sharded counter、Redis/quota block、distributed limiter、Platform RateLimiter abstraction 或 PostgreSQL bucket。
