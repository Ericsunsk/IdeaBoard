# Media / Task Production Cutover Runbook

Issue #667 is a clean-break release gate for the Media / Task pipeline.
This runbook describes only operations that exist in this repository. It is
not evidence that a Fresh Database rehearsal, provider smoke, or Media
telemetry has already been completed.

## Preconditions

Before creating a Production release, require all of the following for the
release commit:

1. `CI Gate` is green. `Media Contract` is an always-required lane and runs:

   ```sh
   bash scripts/ci-media-package-boundary.sh
   bash scripts/ci-legacy-media-zero-scan.sh
   ```

   Both must report `OK`; a compatibility allowlist is not an acceptable
   workaround.

2. The API migration is represented by a versioned Atlas migration, its
   `atlas.sum` update, schema fragments, and regenerated sqlc output. The
   existing `Migration Integration` workflow validates the Atlas history on
   a Fresh Database and applies API then Auth migrations twice.

3. The implementation-specific acceptance tests for Media readiness, signed
   provider delivery, repair bounds, Billing atomicity, and correlation are
   green. Do not substitute a unit-only run for an integration or provider
   smoke that the implementation explicitly requires.

4. A staging rehearsal has been scheduled and its evidence is attached to
   the release issue: release commit, migration result, provider result,
   and rollback decision. This repository does not provide a generic
   `run-api-migration.sh`, a database-only production deploy, or a River
   pause endpoint.

## Deploy

Use the normal immutable-release path in
[the deployment runbook](../runbooks/deployment.md): create a strict SemVer
tag, let `Docker Images` consume the green `CI Gate`, and approve the
Production deployment. If image publication succeeded but deployment failed,
use the existing `Deploy Existing Production Release` workflow with its
original `release-manifest.json`.

The VPS entrypoint applies API and Auth migrations as part of the deployed
immutable manifest. Do not run ad-hoc migration, `--db-only`, or untracked
River control commands during this cutover.

## Verify

After the deployment reports success:

1. Confirm the deployment workflow's controlled smoke result is successful.
2. Confirm the API metrics listener is available only on the private
   `METRICS_ADDR` network boundary; its documented default is
   `127.0.0.1:9091` and it exposes `/metrics`.
3. Query the concrete Media metric names delivered by the implementation.
   Do not infer metric names from this runbook. Every accepted metric must
   support the #661 correlation and error-taxonomy contract without putting
   signed URLs or other secrets in labels.
4. Run the approved staging/provider smoke against the deployed release and
   attach its result to the release issue. A failed readiness check must not
   create a Credit Hold.

## Fallback

This is a clean break: do not restore the previous API binary against a
changed Media schema and do not retain an old URL path as a fallback.

If a release fails after migrations, stop further deployment attempts,
preserve the release result and service logs, then use the database backup /
restore or Fresh Database recovery procedure in
[the deployment runbook](../runbooks/deployment.md). The recovery decision
must be made against the actual migration and data state; it is not a
generic one-command image rollback.
