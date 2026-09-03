# ADR-0003: Payments DDD Package Boundaries

- Status: Accepted
- Date: 2026-09-03

## Context

`internal/payments` is a single Go package containing domain facts, use-case
orchestration, PostgreSQL transaction adapters, Stripe adapters, HTTP handlers,
and event delivery. The bounded context is correct, but the package is flat and
its dependency direction is implicit. Moving all files at once would also mix
import-path changes with payment behavior changes.

## Decision

Payments remains a Modular Monolith bounded context, but its internal code is
organized by DDD responsibility:

```text
internal/payments/
  domain/
  application/{checkout,configuration,facts,reconciliation,readmodel}/
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

The existing root `payments` package is migrated last. During migration it may
temporarily provide composition-only entry points, but new business logic must
not be added there and lower layers must not import it.

## Migration order

1. Extract pure domain types/rules and define ports; add dependency contract tests.
2. Invert Stripe adapter dependencies to `domain`/`ports`.
3. Move PostgreSQL implementations while retaining one Repository transaction
   boundary and lock ordering.
4. Move application workflows by vertical slice.
5. Move HTTP/event adapters and then remove the flat root implementation.
6. Move package-internal tests with their production seams; do not add test-only
   compatibility APIs.

Each step must pass `go test ./...`, `go test -race ./...`, `go vet ./...`,
`golangci-lint`, `deadcode`, and `git diff --check`.

## Consequences

- Navigation and dependency ownership become explicit without prematurely
  creating microservices or CQRS read databases.
- The first two migration steps have the highest import-cycle risk.
- Existing same-package tests will need to move with the code rather than force
  production compatibility wrappers.
- Transaction, lock, idempotency, Credit hold/release, and event ordering remain
  acceptance criteria for every move.

## Rejected alternatives

- One package per aggregate: too many Go import boundaries for the current
  shared transaction and process-manager code.
- Immediate microservices: adds network/distributed transaction complexity
  without a scaling or ownership requirement.
- Forking the Stripe code generator templates: unrelated to this boundary
  decision and creates generator drift.
