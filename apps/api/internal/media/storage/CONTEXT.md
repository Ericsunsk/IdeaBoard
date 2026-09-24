# Media Storage Context

Media Storage owns file/object storage drivers, profile configurations,
presigned URL generation, upload authorization, media integrity verification,
and asset location resolution.
It does not own image derivative generation, prompt orchestration,
task scheduling, or billing ledger facts.

## Package Map

- `domain`: pure storage entities, storage profile models, upload resource
  definitions, object key canonicalization, and upload validation rules.
  Free of external SDKs, SQL, and HTTP libraries.
- `ports`: inbound/outbound contracts including `StorageDriver`,
  `StorageProfileRepository`, `ObjectStore`, `UploadAuthorizer`,
  `StoredUploadLocator`, and transfer models.
- `infrastructure`: technical adapters: `local` (filesystem driver with safety
  boundary checks); `s3` (MinIO/AWS S3 compatible driver and capability probe,
  SDK and credentials sealed within); `postgres` (profile repository with
  encrypted secret rotation); `remote` (SSRF-protected image downloader).
- `application`: `runtime` (upload, fetch, presign, object lifecycle);
  `profiles` (management, activation, credential testing);
  `authorization` (path and resource read/write authorization);
  `integrity` (SHA-256 post-write checksum verification).
- `interfaces/http`: Gin controllers for admin storage profile management.
- Root (`service.go`): composition surface for the runtime storage service;
  domain contracts live in the dedicated subpackages.
- Dependency direction: `interfaces` -> `application` -> `domain` / `ports`;
  `infrastructure` -> `ports` / `domain`;
  root (`service.go`) -> `application` / `infrastructure` /
  `interfaces` / `ports` / `domain`.

## Public Seams

- `StorageDriver`: file/object storage driver contract.
- `StorageProfileRepository`: storage profile persistence contract.
- `ObjectStore`: object-store access contract.
- `UploadAuthorizer`: upload authorization contract.
- `StoredUploadLocator`: asset location resolution contract.
- `runtime`: upload, fetch, presign, and object lifecycle orchestration.
- `profiles`: profile management, activation, and credential testing.
- `authorization`: path and resource read/write authorization.
- `integrity`: SHA-256 post-write checksum verification.
- HTTP `interfaces`: Gin controllers for admin storage profile management.
- `service.go`: root composition surface for the runtime storage service.

## Invariants

- Third-party storage SDKs (MinIO, AWS) and raw credentials
  (`SecretAccessKey`) must NEVER leak into `domain`, `ports`, or `application`.
- Storage profile secret rotation must protect past revisions until
  explicitly retired/revoked.
- Path traversal and object key attacks must be blocked at domain boundaries
  (`NormalizeUploadObjectKey`, `NormalizeCanvasStorageUserSegment`).
- Presigned URLs must clamp TTL to safe bounds
  (`DefaultPresignTTL` / `MaxPresignTTL`).
- The root package is a composition surface only; production code depends
  on the owning subpackage (`domain`, `ports`, `application`,
  or `infrastructure`), never on root-level contracts.
