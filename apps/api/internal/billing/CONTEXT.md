# Billing Context

Billing 是平台级商品身份与 Credit pricing context。用户看到的货币名称是 `Idea/Ideas`，内部价格字段和 domain terminology 继续使用 `Credit/credits`。它为 IdeaBoard 当前和未来产品提供全局 SKU Catalog、append-only price versions 与 server-side Quote；不拥有 Wallet、Ledger、Provider Account routing、subscription 或 invoice。

## Domain Language

| Term | Meaning |
| ---- | ------- |
| SKU | 全局 immutable 商品代码，格式为 `<namespace>:<local-key>`；canonicalization 为 trim + lowercase ASCII，最长 128 字符 |
| SKU namespace | 产品拥有的 1–32 字符 lowercase ASCII namespace，可含 `-` |
| local key | namespace 内的 1–95 字符商品 key，可含 `.`, `_`, `-` |
| Price Version | 某 SKU 的 append-only 单价事实；`version` 在 SKU 内单调递增，`unit_credits` 是不可拆分整数且可显式为 0 |
| Price State | Price Version 的可变 operational enabled 状态，不修改价格事实 |
| Current Price | SKU 原子指向的最新 Price Version；创建新版本与切换 pointer 在同一 database command 完成 |
| Price Snapshot | Quote 返回并由后续 Billing Command 持久化的 immutable value object |
| Quote | 服务端以 current Price Snapshot × positive integer quantity 计算的结果 |
| Admin Action | Admin 对 Catalog/Price 的一次写操作；包含 authenticated actor、non-empty reason 与 action metadata，并以 append-only audit 记录持久化 |

## Public Seams

- `Service.Quote(ctx, sku, quantity)` 是产品读取价格的主要 seam。Caller 只传 SKU 与 quantity，不能传 price。
- `PriceSnapshot` 包含 `sku`, `display_name`, `price_version_id`, `price_version`, `unit_credits`, `quoted_at`；后续 Hold/Charge 必须保存并复用该 snapshot，不得重新报价。`display_name` 在 Quote 时固化，SKU 后续改名不得改变历史任务账单语义。
- Admin handler 创建 SKU、修改 display fields/SKU enabled、追加 Price Version、修改 Price State，以及读取 Catalog/Price history。
- SKU identity 没有 rename/delete contract；停用使用 `enabled=false`。
- AI Image Workflow 的 SKU 由 canonical `feature/model` 确定性计算：`ai-image:<feature>.<sha256-hex(feature + "\\x00" + lower(trim(model)))>`。计费资格来自 Workflow registry 的 `UsesAIProvider` product contract，不来自 Provider Account；Admin 必须先创建对应 SKU 与 current Price，Readiness/Submit 才可用，`models/` 等 provider path alias 不会被折叠。

## Invariants

- SKU 全局唯一、不可 rename、不可 delete/reuse；display name 与 description 可改。
- Price Version 严格 append-only；enabled 存在独立 state table。
- 每次调价锁定 SKU row，在一个 typed PostgreSQL command 内分配单调 version、插入 price/state 并更新 current pointer。
- 所有 Admin Catalog/Price 写操作必须提供 trim 后 1–1000 字符的 reason；业务变更与 `billing_admin_actions` append-only audit 必须在同一 typed PostgreSQL command/transaction 内提交。
- Quote 使用单条 MVCC statement 读取 SKU、current pointer、Price Version 和 state，不会混合并发版本。
- quantity 必须为正 `int64`；`unit_credits × quantity` 超过 `9_000_000_000_000_000` 时 fail closed。
- 0 Credits 是显式免费价格；SKU 不存在、SKU/price disabled 或没有 current price 时 fail closed。
- Billing 不 import `internal/accountpool`，价格不随 Provider Account routing 变化。
- API runtime 对 Catalog/Price tables 只有 SELECT，对 `billing_admin_actions` 无直接权限，所有写入都通过 typed `SECURITY DEFINER` functions；functions 固定 trusted schema、`pg_catalog`, `pg_temp` search path 并 `REVOKE PUBLIC`。

## Non-goals

第一版不提供 tiered pricing、scheduled pricing、subscription、invoice、tax、promotion 或物理 DELETE。
