# Media Storage Context

Media Storage owns file/object storage drivers, profile configurations, presigned URL generation, upload authorization, media integrity verification, and asset location resolution. It does not own image derivative generation, prompt orchestration, task scheduling, or billing ledger facts.

## Package Map

- `domain`: pure storage entities, storage profile models, upload resource definitions, object key canonicalization, and upload validation rules. Completely free of external SDKs, SQL, or HTTP libraries.
- `ports`: inbound/outbound contracts including `StorageDriver`, `StorageProfileRepository`, `ObjectStore`, `UploadAuthorizer`, `StoredUploadLocator`, and transfer models.
- `infrastructure`: technical adapters including:
  - `local`: filesystem-backed driver with safety boundary checks.
  - `s3`: MinIO / AWS S3 compatible object storage driver and capability probe (SDK and credentials sealed within).
  - `postgres`: PostgreSQL storage profile repository with encrypted secret rotation.
  - `remote`: SSRF-protected remote image downloader.
- `application`: orchestrates domain operations and ports:
  - `runtime`: upload, fetch, presign, and object lifecycle operations.
  - `profiles`: profile management, activation, and credential testing.
  - `authorization`: path and resource read/write authorization.
  - `taskmedia`: task-oriented media materialization and reading.
  - `integrity`: SHA-256 post-write checksum verification.
- `interfaces/http`: Gin controllers for admin storage profile management.
- Root (`service.go`): composition surface for the runtime storage service; domain contracts live in the dedicated subpackages.

## Invariants

- Third-party storage SDKs (MinIO, AWS) and raw credentials (`SecretAccessKey`) must NEVER leak into `domain`, `ports`, or `application`.
- Storage profile secret rotation must protect past revisions until explicitly retired/revoked.
- Path traversal and object key attacks must be blocked at domain boundaries (`NormalizeUploadObjectKey`, `NormalizeCanvasStorageUserSegment`).
- Presigned URLs must clamp TTL to safe bounds (`DefaultPresignTTL` / `MaxPresignTTL`).
- The root package is a composition surface only; production code should depend on the owning subpackage (`domain`, `ports`, `application`, or `infrastructure`) rather than importing root-level contracts.

## Dependency Direction

```text
interfaces -> application -> domain / ports
infrastructure -> ports / domain
root (service.go) -> application / infrastructure / interfaces / ports / domain
```
