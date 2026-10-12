# Frontend bounded-context ownership

Status: Accepted
Owner: Frontend Architecture
Decision date: 2026-09-10

## Decision

Frontend 按 bounded contexts 组织。`board` 拥有 persisted Board document/schema、media references、catalog CRUD、collaboration/share join、revision/sync、persistence 和 previews，是唯一 Board repository 与 sync fact source。Canvas `board-session` 通过 Board ports 编排 session，不建立第二 repository 或 sync fact source。

`workspace` 拥有 workspace identity 与 navigation。`canvas`、`search`、`library`、`account`、`admin`、`auth` 和 `legal` 保持独立 context。Cross-context communication 通过 public barrel 或明确登记的 port；匿名 public landing 属于 Canvas presentation，public `/user/*` URLs 是稳定外部契约。

需要保护的 durable contracts 包括 persisted Board schema 与 load behavior、API request/response semantics、backend error codes、workflow feature slugs、storage keys、query invalidation semantics、public route URLs 和 serialized Board/media references。修改这些契约时必须更新对应 canonical contract 或 ADR。

## Review triggers

新增 durable contract、第二 repository/sync owner、cross-context deep import 或 context split/merge 时需要 review。Frontend local layout 与 purity 规则由 [ADR-0013](./0013-frontend-feature-first-saas-layout.md) 定义。
