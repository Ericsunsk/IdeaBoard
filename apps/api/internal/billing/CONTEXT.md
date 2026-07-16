# Billing Context

Billing 是平台级商品身份与 Credit pricing context。用户看到的货币名称是 `Idea/Ideas`，内部价格字段和 domain terminology 继续使用 `Credit/credits`。它为 IdeaBoard 当前和未来产品提供全局 SKU Catalog、append-only price versions 与 server-side Quote；不拥有 Wallet、Ledger、Provider Account routing、subscription 或 invoice。

## Domain Language

| Term | Meaning |
| ---- | ------- |
| SKU | 全局 immutable 商品代码，格式为 `<namespace>:<local-key>`；canonicalization 为 trim + lowercase ASCII，最长 128 字符 |
| Billable Item | 产品 context 注册到 Billing 的可计价能力；包含稳定 SKU、namespace、product kind、展示文案与排序，不包含 Provider Model |
| Billable Item Registry | Billing-owned immutable registration seam；runtime 组合各产品 context 的定义，拒绝 duplicate SKU，并返回 deterministic copy |
| Catalog Read Model | 将当前 Registry 与持久化 SKU/current price facts 动态合并为 Admin 卡片；不依赖 startup write 或同步任务 |
| SKU namespace | 产品拥有的 1–32 字符 lowercase ASCII namespace，可含 `-` |
| local key | namespace 内的 1–95 字符商品 key，可含 `.`, `_`, `-` |
| Price Version | 某 SKU 的 append-only 单价事实；`version` 在 SKU 内单调递增，`unit_credits` 是不可拆分整数且可显式为 0 |
| Current Price | SKU 原子指向的最新 Price Version；创建新版本与切换 pointer 在同一 database command 完成 |
| Archived | 持久化 Catalog item 已不在当前 Registry；不可 Quote 或启用，但仍可读取 immutable Price History |
| Price Snapshot | Quote 返回并由后续 Billing Command 持久化的 immutable value object |
| Quote | 服务端以 current Price Snapshot × positive integer quantity 计算的结果 |
| Admin Action | Admin 对 Catalog/Price 的一次写操作；包含 authenticated actor、target、old/new、outcome 与时间，并以 append-only audit 记录持久化 |

## Public Seams

- `BillableItemRegistry.Items()` 是产品计价身份的只读注册 seam；当前 Workflow adapter 使用 `ai-image:<feature-slug>`，未来产品可注册自己的 namespace。
- `BillableItemCatalog.List(ctx)` 是 Admin 功能定价页的只读 seam；Registry 中无持久化价格的项目仍显示为 `Unpriced`，持久化项目不在 Registry 时显示为 `Archived`。
- `BillableItemCatalog.PublishPrice(ctx, sku, unitCredits, actorID)` 是功能价格发布 seam；只接受 Registry 中的 Billable Item，首次发布原子 materialize Catalog item 并转为 `Active`。
- `BillableItemCatalog.SetAvailability(ctx, sku, enabled, actorID)` 是 Feature availability 写 seam；只接受 Registry 中已有 current price 的项目，`Unpriced` 不可启用，重复命令保持幂等并记录真实 outcome。
- `BillableItemCatalog.ListPriceHistory(ctx, sku, beforeVersion, limit)` 是只读 immutable Price History seam，支持当前 Registry item 与 Archived item。
- `BillableItemCatalog.DeleteArchived(ctx, sku, actorID)` 只接受当前 Registry 不存在的 item；database command 在 SKU row lock 内重新检查 Task Billing Snapshot 等 durable references。成功时删除 Catalog aggregate（SKU + Price Versions），保留既有 Admin Audit 并追加 `delete_archived` audit。
- `BillableItemCatalog.Quote/QuoteInTx` 是 Registry-aware runtime seam；`Unpriced`、`Disabled` 与 `Archived` 统一 fail closed，Task submission 通过 `QuoteInTx` 保持 Snapshot、Hold、Task 与 River enqueue 的单事务边界。
- `Service.Quote(ctx, sku, quantity)` 是产品读取价格的主要 seam。Caller 只传 SKU 与 quantity，不能传 price。
- `PriceSnapshot` 包含 `sku`, `display_name`, `price_version_id`, `price_version`, `unit_credits`, `quoted_at`；后续 Hold/Charge 必须保存并复用该 snapshot，不得重新报价。`display_name` 在 Quote 时固化，SKU 后续改名不得改变历史任务账单语义。
- 用户侧 Billing Records 只读取 Task Billing Snapshot 中固化的 `display_name`；不得回查当前 SKU display fields 改写历史账单名称。
- Admin 功能定价只发布新的 immutable Price Version；不提供 Price Version enable/disable command，也不要求 Admin 手填 reason。Database command 自动记录 actor、target、old/new 与 outcome。
- Registry 中的 SKU identity 没有 rename/delete contract；停用使用 `enabled=false`。只有 Registry 已移除且无 durable reference 的 Archived aggregate 可 hard delete。
- Admin Catalog、Quote 与 Task Billing 统一使用 feature-level identity `ai-image:<feature>`。Provider Model 只属于 execution/idempotency contract，不参与用户价格 identity；切换模型不会改变功能价格。计费资格来自 Workflow registry 的 `UsesAIProvider` product contract，不来自 Provider Account。

## Invariants

- Registry 中的 SKU 全局唯一且不可 rename；display name 与 description 可改。Archived aggregate hard delete 后若未来重新注册同一 SKU，会按新的 Registry definition 重新 materialize。
- Price Version 严格 append-only，没有 mutable enabled state；恢复旧价格也必须发布一个新版本。
- 每次发布锁定 SKU row，在一个 typed PostgreSQL command 内 materialize Catalog item（如需要）、分配单调 version、插入 price、更新 current pointer 并写 append-only audit。
- 首次发布或尚无 current price 的项目自动启用；已有 current price 的 Disabled 项目调价后仍保持 Disabled。
- Feature availability 只由 Billing Catalog 的 SKU enabled state 拥有；Prompt revision 不携带 enabled/published state。启停 command 自动记录 actor、target、old/new、outcome 与时间。
- Quote 使用单条 MVCC statement 读取 SKU、current pointer 与 Price Version，不会混合并发版本。
- quantity 必须为正 `int64`；`unit_credits × quantity` 超过 `9_000_000_000_000_000` 时 fail closed。
- 0 Credits 是显式免费价格；SKU 不存在、SKU disabled 或没有 current price 时 fail closed。
- Billing 不 import `internal/accountpool`，价格不随 Provider Account routing 变化。
- Registry 是 constructor-owned immutable value，不使用 global mutable registration，避免并发启动和测试污染。
- API runtime 对 Catalog/Price tables 只有 SELECT，对 `billing_admin_actions` 无直接权限，所有写入都通过 typed `SECURITY DEFINER` functions；functions 固定 trusted schema、`pg_catalog`, `pg_temp` search path 并 `REVOKE PUBLIC`。
- Archived delete 以 SKU row lock 串行化并发 Snapshot FK acquisition；任何 durable reference 都使整个 command 回滚，既不删除 Price/SKU，也不写 delete audit。
- Admin 历史任务动态显示当前 Registry 是否仍包含该 feature；用户侧账单始终沿用 immutable Snapshot `display_name`，不追加删除状态文案。
- Archived hard delete 的 migration down 只支持尚未发生真实删除的 schema rollback；发生删除后的精确恢复必须使用 migration 前 database snapshot。

## Non-goals

第一版不提供 tiered pricing、scheduled pricing、subscription、invoice、tax、promotion，或对已产生 durable Billing history 的 Catalog item 做物理 DELETE。
