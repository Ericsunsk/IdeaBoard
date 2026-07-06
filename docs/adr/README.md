# Architecture Decision Records

`docs/adr/` 保存 system-wide ADR：影响多个 context、runtime topology、
deployment constraints、data ownership 或 durable technology choices 的决策。

`README.md` 和 `TEMPLATE.md` 是脚手架文件，不是 ADR。context-local decisions
可以放在对应 context 本地的 `docs/adr/`，如果它存在。

## Naming

- 使用 `NNNN-short-slug.md`，从 `0001` 开始。
- 新增 ADR 前扫描已有 ADR 的最大编号，然后递增。
- slug 使用 lowercase `kebab-case`，描述 decision，而不是 implementation task。

## Status

每篇 ADR 应包含以下 status 之一：

- `Proposed`: decision 正在讨论，或还没有落地。
- `Accepted`: decision 仍然有效。
- `Deprecated`: decision 已不再推荐，但没有单一 ADR 替代它。
- `Superseded by ADR-NNNN`: later ADR 已替代该 decision。

不要改写 accepted history 来隐藏已经变化的 decision。当变化原因值得保留时，新增
ADR，并把旧 ADR 标记为 superseded。

## When to Add an ADR

只有同时满足以下条件时才新增 ADR：

- Hard to reverse。
- Surprising without context。
- The result of a real trade-off among credible options。

适合记录的 decision 包括 cross-context boundaries、task/event ownership、
security or operational constraints、durable technology choices，以及 deliberate
deviations from the obvious path。

不要用 ADR 记录 routine implementation details、temporary experiments，或已经在
`DESIGN.md`、`DEVELOPMENT.md`、`CONTEXT.md`、runbook 中清楚记录的事实。

## Template

从 [TEMPLATE.md](./TEMPLATE.md) 开始。ADR 应足够短，让未来读者能快速理解
trade-off。
