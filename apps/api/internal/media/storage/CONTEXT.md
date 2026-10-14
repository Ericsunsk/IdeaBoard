# Media Storage Context

Media Storage owns file/object storage drivers, profile configurations,
presigned URL generation, and media integrity verification. Object bytes
are served only through the unified object-storage + pull pipeline (v1
media-assets content/derivatives, public avatar/logo routes): nothing is
served statically. It does not own image derivative generation, prompt
orchestration, task scheduling, or billing ledger facts.

## Domain Language

| Term                   | Meaning                                                                                                                  |
| ---------------------- | ------------------------------------------------------------------------------------------------------------------------ |
| Storage Profile        | A configured storage location and its access policy, independent of the media's business identity.                       |
| Active Storage Profile | The selected Storage Profile for operations that follow the active location, including Avatar and Branding Logo storage. |
| Stored Object          | Image bytes at one Object Key in a Storage Profile; not a Library Asset or an independently identified Media Asset.      |
| Object Key             | A storage-relative locator for a Stored Object; not a public URL.                                                        |
| Original Image         | The validated source image from which display renditions are produced.                                                   |
| Avatar                 | A user's profile image.                                                                                                  |
| Branding Logo          | The site's configured brand image.                                                                                       |

## Package Map

- `domain`: pure storage entities, storage profile models,
  object key canonicalization, and upload validation rules.
  Free of external SDKs, SQL, and HTTP libraries.
- `ports`: inbound/outbound contracts including `StorageDriver`,
  `ProfileRepository`, `ObjectStore`,
  and transfer models.
- `infrastructure`: technical adapters: `local` (filesystem driver with safety
  boundary checks); `s3` (MinIO/AWS S3 compatible driver and capability probe,
  SDK and credentials sealed within); `postgres` (profile repository with
  encrypted secret rotation); `remote` (SSRF-protected image downloader).
- `application`: `runtime` (upload, fetch, presign, object lifecycle);
  `profiles` (management, activation, credential testing);
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
- `ProfileRepository`: storage profile persistence contract.
- `ObjectStore`: object-store access contract.
- `runtime`: upload, fetch, presign, and object lifecycle orchestration.
- `profiles`: profile management, activation, and credential testing.
- `integrity`: SHA-256 post-write checksum verification.
- HTTP `interfaces`: Gin controllers for admin storage profile management.
- `service.go`: root composition surface for the runtime storage service.

## Invariants

- Workspace Media uses the fixed `builtin-local` profile. Avatar and Branding Logo use the active Storage Profile; changing that profile does not move existing objects.
- Third-party storage SDKs (MinIO, AWS) and raw credentials
  (`SecretAccessKey`) must NEVER leak into `domain`, `ports`, or `application`.
- Storage profile secret rotation must protect past revisions until
  explicitly retired/revoked.
- Path traversal and object key attacks must be blocked at domain boundaries
  (`NormalizeUploadObjectKey`, `NormalizeStorageUserSegment`).
- Presigned URLs must clamp TTL to safe bounds
  (`DefaultPresignTTL` / `MaxPresignTTL`).
- The root package is a composition surface only; production code depends
  on the owning subpackage (`domain`, `ports`, `application`,
  or `infrastructure`), never on root-level contracts.
