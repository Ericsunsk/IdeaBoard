# AccountPool ProviderEndpoints Bounded Context

## Responsibilities

`internal/accountpool/providerendpoints` manages upstream AI gateway endpoints, network protocols, capability specifications, model discovery/detection, and endpoint scheduling state.

## Ubiquitous Language

- **Endpoint**: An upstream network gateway host and credential configuration supporting specific AI capabilities.
- **Protocol**: The communication wire format and routing specification (`model-api`, `image-pipeline`, `image-fill`).
- **Capability**: A distinct AI task operation supported by an endpoint (`generation`, `cutout`, `upscale`, `fill`).
- **RuntimeAdapter**: A provider-agnostic runtime execution bridge (`model-image`, `cutout`, `upscale`, `flux-fill`) mapped by the worker.
- **ModelDetection**: Dynamic inspection of upstream AI models supported by an endpoint using Google Cloud countTokens or standard OpenAI models probes.

## Architectural Layers

- `domain`: Pure domain entities (`Endpoint`, `ProtocolDefinition`), protocol registries, runtime adapter routing, validation rules, capability policies, and error sentinels. Has zero external dependencies.
- `ports`: Contracts defining persistence (`Repository`).
- `application`: Use-case orchestration (`Service`) handling endpoint lifecycle (list, get, create, update, status update, delete) and model detection probes.
- `infrastructure`:
  - `postgres`: PostgreSQL repository implementation (`Repository`) handling atomic capability synchronization, encrypted managed account credentials, and conflict mapping.
- `interfaces`:
  - `http`: REST API handlers (`Handler`) and request/response DTOs for admin endpoint configuration and model detection.
- `tests`: Isolated integration, edge-case, and scenario test suite.
- Root: Contains no production implementation files. Application startup composes the canonical domain, application, ports, and infrastructure packages directly.
