# 跨 Store 事务锁序 (Transaction Lock Ordering)

所有跨 store 事务共享同一个 PostgreSQL，参与者通过 caller-owned `pgx.Tx`（或 Control guard 的 ambient borrowed Tx）join 同一事务。
锁序不一致 = 死锁。本页是唯一的锁序事实源：新增跨 store 事务必须遵守文末规则，并同步门禁测试。

门禁：`apps/api/internal/tasking/transaction_lock_ordering_test.go`（`TestTransactionLockOrdering`，纯源码顺序断言，无需 DB）。

## 全局锁层次（从先到后，禁止逆序加锁）

| Rank | 锁 / Fence | 实现位置 |
| ---- | ---------- | -------- |
| L0 | 幂等 fence：`task_submission_idempotency` PK (`ON CONFLICT DO NOTHING`)；`pg_advisory_xact_lock(idempotency_key)`；Service Model Pool per-service advisory lock | `internal/db/queries/workspace_tasks.sql:314`；`internal/db/migrations/20261020000000_baseline.sql:934`；`internal/generation/catalog/catalog.go:396` |
| L1 | `workspaces` 行 `FOR UPDATE`（membership 守卫）。Settlement submit 中它必须在 L0 claim insert 之前：claim 的 workspace FK 会先取同行的 `FOR KEY SHARE`，再升级 `FOR UPDATE` 即与并发提交互锁（40P01，#917）。守卫锁先行则同 workspace 提交在入口串行，claim 仍在 Quote/Hold 之前守住重放 | `internal/db/queries/workspaces.sql:21`，经 `LockWorkspaceMemberships` |
| L2 | 领域行（稳定顺序内）：`boards` 行 `FOR UPDATE`（多 board 按 `public_id ASC`）；`credit_wallets` 行 `FOR UPDATE` 后 `credit_accounts ORDER BY id FOR UPDATE`；`tasks` 行经状态翻转 `UPDATE` | `internal/db/queries/workspace_boards.sql:105,121,163`；`20261020000000_baseline.sql:1000,1014`；`internal/db/queries/workspace_tasks.sql:126,139,169` |
| L3 | 派生/引用行（无外序要求，但必须在 L2 之后）：`task_billing_*`、`board_document_media_refs`、`board_memberships`、`task_outputs`、`river_job`、`ai_search_operations` marker、`event outbox` | 各 store 的 `*InTx` 实现 |

## 各事务锁序表（读代码确认）

### 1. Settlement submit：Authorize（Workspace 守卫锁）→ Claim → Resolve → Quote → Hold → Task → Snapshot → Billing → Enqueue

`internal/tasking/workspacetasks/settle_tx.go:createInTx`（约 243–363）：`AuthorizeSubmission`（`LockWorkspaceMemberships` 取 `workspaces FOR UPDATE`，守卫先行串行同 workspace 提交）→ `ClaimSubmissionInTx` → `txAccounts.ResolveBillingAccountInTx`（稳定账户，join 同一 tx）→ `resolver.ResolveInTx`（catalog pool lock + 读，`catalog.go:384`）→ `QuoteInTx`（纯 `SELECT`，`billing/infrastructure/postgres/store.go:195`，全文件无 `FOR UPDATE`）→ `HoldInTx`（`credits.Service.InTx(tx)`，`tasking/infrastructure/adapters/workspace_credits_adapter.go:178`）→ `CreateTaskInTx`（`tasking/infrastructure/postgres/store.go:310`）→ `saveSnapshot`（`generation/catalog/snapshot.go:61`，`UPDATE tasks SET routing_snapshot`）→ `CreateBillingFactsInTx`（`store.go:327`）→ `EnqueueInTx`（`client.InsertTx` join 同一 tx，`tasking/infrastructure/adapters/workspace_queue_adapter.go:116`）。Claim 仍在 Quote/Hold 之前，是并发 fence：重试在唯一约束上撞见赢家，直接 replay，永远到不了 Ledger；守卫锁在 Claim 之前，是升级死锁（claim FK 的 `FOR KEY SHARE` 再升 `FOR UPDATE`，并发 distinct-key 提交互锁 40P01，#917）唯一的入口串行点。

### 2. Settlement terminal：Task 翻转 → Credits 收敛 → Billing settlement 行

`settle_tx.go:completeInTx`（约 405–460）：`CompleteTaskInTx` → `CaptureInTx`/`ReleaseInTx` → `SetBillingSettlementInTx`（`store.go:415`）。`failInTx`/`cancelInTx` 同形：`FailTaskInTx`/`CancelTaskInTx` → `ReleaseInTx` → `SetBillingSettlementInTx`。与 submit 的区别是 terminal 先翻转 task 行（state-guard `UPDATE … WHERE status IN (…)` 零行即收敛），再动账；顺序固定，不可反转。

### 3. Board accept（boards + media）：先锁 Workspace，再锁 Board，校验只读，引用收尾

`internal/boards/postgres_workspace.go:lockWritableBoardTx`（约 242–263）：`lockWorkspaceMembersTx`（`workspaces FOR UPDATE`，234）→ `LockWorkspaceBoardAccess`（`boards FOR UPDATE`，246）→ 重读 membership + `ResolveRole`。`ReplaceDocument`（约 497–621）：`lockWritableBoardTx` → `LockWorkspaceBoard`（511）→ `validateDocumentAssets`（只读：`media/workspacemedia/document_access.go:16`，走同一 tx 的 `q`，先读活引用再覆盖，新 id 不能自授权）→ `InsertBoardRevision` + `UpdateBoardDocument` → `DeleteBoardDocumentMediaRefs`（595）→ `UpsertBoardDocumentMediaRef`（602）。`ReplaceBoardAccess`（约 817–906）同骨架：`lockWorkspaceMembersTx` → `LockWorkspaceBoardAccess` → `ClearBoardMemberships` → `Insert…` → `SetBoardWorkspaceAccess`。

### 4. Member departure（tenancy + boards）：Workspace 锁 → Boards 有序锁 → 删 membership

`internal/tenancy/infrastructure/postgres/store.go:RemoveWorkspaceMember`（约 492–535）：`LockWorkspaceMemberships`（495）→ `grants.RemoveWorkspaceMemberGrants`（ambient borrowed tx，515）→ `DeleteWorkspaceMembership`（520）。Boards 侧 `postgres_workspace.go:RemoveWorkspaceMemberGrants`（约 912–993）：`ListWorkspaceMemberBoardGrants`（枚举）→ `LockWorkspaceMemberBoards`（`ORDER BY public_id ASC FOR UPDATE`，944）→ `DeleteWorkspaceMemberBoardGrants`（951）→ `BumpWorkspaceMemberBoardsAccess`（957）。跨包共享同一铁律：Workspace membership 锁永远在 Board 行之前，多 Board 永远按 `public_id` 排序加锁。

### 5. Billing / Credits caller-owned Tx：调用方拥有 commit/rollback，参与者只借用

`credits/composition.go:209`（`Service.InTx`）/`220 CommandsInTx`（无 Service 场景，payments 用它）只把 `tx` 包成 `ports.Commands`，绝不 begin/commit。`execute_credit_command_v2`（`20261020000000_baseline.sql:886`）内部锁序：advisory（934）→ `credit_holds`/`credit_charges`/原交易行 `FOR UPDATE`（952–998）→ `credit_wallets FOR UPDATE`（1000）→ `credit_accounts ORDER BY id FOR UPDATE`（1014）→ 写 transaction/postings。`QuoteAtResolutionInTx` 永远无锁，可放在 Hold 前任意位置；`Restriction` 创建/释放先锁 Wallet 行（`create/release_credit_wallet_restriction`）。同构例子：vision settlement（`searchcatalog/application/vision_settlement.go:30`）：advisory → operation 行 `FOR UPDATE` → `QuoteAtResolutionInTx` → `InTx(tx).Hold` → insert；terminal 先 `FOR UPDATE` operation 行再 Capture/Release。

## 新增跨 store 事务必须遵守的规则

1. 保持全局 Rank 顺序 L0 → L1 → L2 → L3；需要 Workspace 守卫的写永远先 `LockWorkspaceMemberships`，再碰 Board 行。
2. 一次锁多行必须有稳定排序：Boards 按 `public_id ASC`（用 `LockWorkspaceMemberBoards` 式查询），账务行按 `account_id ASC`（直接调 Credits seam 即自动满足，禁止手写 Ledger SQL）。
3. 幂等 fence（claim insert / advisory lock）在 Ledger/Task 写之前、Quote/Hold 之前；但 claim insert 的 workspace FK 会取同行 `FOR KEY SHARE`，因此 Workspace 守卫锁（L1 `LockWorkspaceMemberships`）必须在 claim 之前——守卫先行、fence 随后是唯一的死锁自由序（#917）。`QuoteAtResolutionInTx` 只读无锁，放在 Hold 前。
4. 只经窄缝写 чужой context：Ledger 只经 `credits.Service.InTx` / `CommandsInTx`；价格只经 `QuoteAtResolutionInTx`；Media 只经 `boards.RoleReader` + 引用列表事实，不复制 role 规则、不碰对方存储实现。
5. 借用事务绝不 commit/rollback：经 `dbutil.WithTxDBTX` 或 `q.WithTx(tx)` 加入 ambient carrier（`db/dbutil/ambient_tx.go:62`，`dbutil.go:51`）；outbox/event 行与业务行同事务写，发布只在 commit 后（guard 用 post-commit hook，`#889`）。
6. 改动任一锁序必须同步更新本页 + `TestTransactionLockOrdering`；测试 fail closed（marker 找不到即失败），先修代码再对齐文档，不静默跳过。

## 相关契约

- `DESIGN.md` §3（Tasking 单事务边界）、§7（Media/Board 引用边界）、Backend Context Map（Credits 唯一 Ledger writer）。
- `apps/api/internal/credits/CONTEXT.md`（InTx seam、account_id 有序加锁）、`apps/api/internal/billing/CONTEXT.md`（`QuoteAtResolutionInTx`）。
- ADR-0012（RoutingSnapshot 同事务固化，worker 只读快照）。
