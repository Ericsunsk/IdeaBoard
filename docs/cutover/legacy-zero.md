# Media Clean-break Zero-result Contract

Issue #667 permits no legacy Media URL compatibility path in shipping code.
`scripts/ci-legacy-media-zero-scan.sh` is intentionally a zero-result gate,
not an inventory with production allowlists. Until all prohibited surfaces
are removed, `Media Contract` and therefore `CI Gate` must fail.

## Prohibited shipping contracts

The scan examines non-test Go/TypeScript source and the shipping Web env
files. It rejects:

1. `original_image_url` in API or Web source, including generated client
   code. Task input is represented by the immutable `TaskInputManifest`, not
   a durable URL parameter.
2. `VITE_PUBLIC_ASSET_BASE_URL` in Web source or `.env` files. Browser media
   delivery uses typed, same-origin `MediaRef` resolution.
3. A `"/uploads/` literal in Web source. UI code must not reconstruct a
   storage path from a string.
4. A concrete `internal/media/storage` import in non-test Workflow or Worker
   code. Those packages cross the Media boundary through typed ports only.

Tests are excluded because they may compose concrete adapters. That exclusion
does not permit a shipping compatibility branch, an environment fallback, or
a new allowlist entry.

## Remediation

Remove the retired field, configuration, route construction, or import at
its ownership boundary. Regenerate API clients when the API contract changes.
Do not rename a legacy field, suppress the scan, or preserve an old route for
rollback.

Validate locally with:

```sh
bash scripts/ci-media-package-boundary.sh
bash scripts/ci-legacy-media-zero-scan.sh
bash scripts/ci-media-package-boundary-test.sh
bash scripts/ci-legacy-media-zero-scan-test.sh
```

The first two commands must report `OK` for a release candidate. The latter
two verify that the gates themselves fail closed.
