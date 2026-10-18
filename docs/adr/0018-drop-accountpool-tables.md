# ADR-0018: Drop AccountPool Tables

Supersedes ADR-0009 Decision-2（行保留停写）。

## Context

ADR-0009 退役 AccountPool 时决定 `api_accounts`、`provider_endpoints`、`api_account_secrets` 行保留为只读历史，理由是“删表等于删账单事实”。db 冗余审计确认：项目无上线包袱，账单事实已由 Task Billing snapshot 与 Credits Ledger 独立承载，不依赖这三张表；保留三表反而阻塞 schema 装配清理（manifest/assembly 强引用死表）与后续治理。

## Decision

1. 删除三表：`provider_endpoints`、`api_accounts`、`api_account_secrets`（migration `20261019000000_drop_account_pool.sql`，destructive approved），同步移除 manifest/assembly/fragment 引用与 lint guard。
2. “行保留停写”结论作废；ADR-0009 其余决策（执行事实、execution token fencing）继续有效。
3. 账单事实以 Task Billing snapshot + Credits Ledger 为准，任何“删表等于删事实”的主张须先指认具体消费者。

## Consequences

- 正面：schema 装配无死引用；退役彻底完成。
- 代价：旧表行不可恢复（已接受，无上线数据）。

## Status

Accepted
