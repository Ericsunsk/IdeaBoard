# AccountPool Accounts Bounded Context

## Responsibilities

`internal/accountpool/accounts` manages upstream AI provider account lifecycle, model capability registration, concurrency limits, secret storage, health monitoring, and routing visibility.

## Ubiquitous Language

- **Account / ApiAccount**: An upstream AI service provider account credential configured with endpoint, concurrency limits, timeout, and enabled models.
- **AccountModel**: An AI model associated with an account, configured with enablement, weight, concurrency caps, and resolution capabilities (2K/4K).
- **RuntimeAccountAccess**: A scoped, immutable runtime credential snapshot passed to execution workers for an assigned task.
- **ProviderEndpoint**: A network gateway / protocol definition (e.g. OpenAI-compatible, Google Vertex) that multiple accounts can belong to.
- **FailCount**: Number of consecutive provider execution failures before an account is marked unhealthy.

## Architectural Layers

- `domain`: Domain entities (`Account` / `ApiAccount`, `AccountModel`, `RuntimeAccountAccess`), status constants (`RunningStatusStopped`, `RunningStatusFaulted`, `RunningStatusRunning`), validation rules, resolution filters, capability policies, and error sentinels. Depends only on shared workflow/validation primitives.
- `ports`: Contracts defining persistence (`AccountRepository`, `RunningTaskFinalizer`, `AccountTaskStore`) and queue metrics (`QueueStatsStore`, `AccountLoadStore`, `AccountConcurrencyStore`).
- `application`: Use-case orchestration (`Service`) handling account lifecycle (create, update, patch model, start, stop, archive), concurrency monitoring, and task failure side-effects.
- `infrastructure`:
  - `postgres`: PostgreSQL repository implementation (`Repository`) with encrypted secret material rotation (`secrets.RotateSecretMaterial`), atomic model synchronization, and transaction-bound cleanup.
- `interfaces`:
  - `http`: REST API handlers (`Handler`) for admin management endpoints.
- `tests`: Isolated integration and scenario test suite.
- Root: Contains no production implementation files. Application startup composes the canonical domain, application, ports, and infrastructure packages directly.
