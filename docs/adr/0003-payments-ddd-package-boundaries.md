# ADR-0003: Payments DDD Package Boundaries

- Status: Accepted
- Date: 2026-09-03

## Context

Payments 同时协调外部支付事实、Credits 入账与异步恢复。领域规则、外部 I/O 和本地
transaction 必须有明确边界，避免把 Stripe 网络调用放进数据库 transaction，或让
HTTP、read model 越过 use case 改写支付状态。

## Decision

Payments remains a Modular Monolith bounded context, but its internal code is
organized by DDD responsibility:

```text
internal/payments/
  domain/
  application/
  ports/
  infrastructure/{postgres,stripe,events}/
  interfaces/http/
  composition/
```

The dependency direction is:

```text
interfaces/http -> application -> domain + ports
infrastructure -> domain + ports
composition -> application + infrastructure
```

`domain` must not import `sqlc`, Gin, Stripe SDK, or Credits persistence.
`application` owns use-case orchestration and may call the typed Credits seam.
`infrastructure/postgres` remains a transaction adapter and may depend on
Credits while preserving Payment↔Credits atomicity. Stripe I/O remains outside
database transactions.

`PaymentOrder` is the primary aggregate. Refund, Dispute, Webhook Inbox,
Credential Generation, Reconciliation Claim, and Checkout Shutdown remain
independent durable facts/process states. They coordinate through application
ports and immutable IDs; domain packages do not import one another.

The root `payments` package is a public facade delegating to the owning
application services. Lower layers must not import it.

## Consequences

- Navigation and dependency ownership become explicit without prematurely
  creating microservices or CQRS read databases.
- Public facades expose use cases without duplicating their implementation.
- Transaction, lock, idempotency, Credit hold/release, and event ordering remain
  invariants across package boundaries.

## Rejected alternatives

- One package per aggregate: too many Go import boundaries for the current
  shared transaction and process-manager code.
- Immediate microservices: adds network/distributed transaction complexity
  without a scaling or ownership requirement.
- Forking the Stripe code generator templates: unrelated to this boundary
  decision and creates generator drift.
