# Static Analysis Baseline

Related: [Wiki Index](../index.md), [DEVELOPMENT.md](../../../DEVELOPMENT.md)

## 2026-11-01 — auth/realtime/ai-proxy PASS

- As of `IdeaBoard@782e87c00` (2026-11-01), background static-analysis subagent reported:
  - `apps/auth` `npm run build` (clean+tsc+copy-email-templates) PASS ~4s; `npm run lint` (prettier --check) PASS ~2s.
  - `apps/realtime` `npm run build` (clean+tsc) PASS ~2s; `lint` PASS ~1s.
  - `apps/ai-proxy` `npm run build` (tsc --noEmit) PASS ~1s; `lint` PASS ~2s.
- Note recorded on 2026-11-01: the three `lint` scripts are `prettier --check` only (format, not ESLint semantics).
- Deferred on 2026-11-01: `auth contract:check / architecture:check`, `ai-proxy test:contract` not run (out of task scope).

## 2026-11-01 — api: vet/build/sqlc/atlas PASS, golangci-lint 57 issues (superseded 2026-11-01 by fix below)

- As of `IdeaBoard@782e87c00` (2026-11-01), background static-analysis subagent reported (read-only, no code change):
  - `go vet ./...` PASS exit 0; `go build ./...` PASS exit 0.
  - `go tool sqlc generate` exit 0 + `git diff --exit-code -- internal/db/sqlc` exit 0, no drift.
  - `atlas migrate validate` (atlas v1.3.4, no --dev-url) PASS checksum only; 5 .sql ↔ atlas.sum 5 entries 1:1. Full Fresh replay needs isolated ATLAS_DEV_URL via `scripts/atlas-migration-check.sh`, skipped, not counted as full contract pass.
  - `golangci-lint run ./...` (config `apps/api/.golangci.yml`, binary 2.13.2) FAIL exit 1, 57 issues = unused 50 + gofmt 3 + errorlint 1 + gocritic 1 + gosec G705 1 + noctx 1.
  - Unused cluster: `internal/runtime/interfaces/http/v1/routes.go` ~10 (e.g. `:170 authorizeBoardByID`, `:174 requireBoardsStore`, `:223 publicAvatar`), `tenancy/member_batch.go:18,19,29,35` 4, `billing/billing.go:107,253,260,263` 4, plus `*_test.go` helpers (`testsupport.X` aliases); `_test.go` not exempted for `unused` in config.
  - Others: gofmt 3 in `system/notifications.go:13`, `notifications_events.go:38`, `search.go:51`; errorlint `internal/media/workspacemedia/ingest.go:39` (%v wrapping err); gocritic `request.go:16` comment spacing; gosec G705 + noctx in `v1/testsupport/test_client.go:459,:132`.
- Suggested next (cheapest first): `gofmt -w` 3 files + comment space; fix errorlint `%v`→`%w`; `NewRequestWithContext` + G705 nolint review; prune confirmed-dead v1 handlers/aliases after grep; decide `_test.go` unused exemption vs deletion with owner.

## 2026-11-01 — web: lint/style/typecheck/build all PASS

- As of `IdeaBoard@782e87c00` (2026-11-01), background static-analysis subagent reported (read-only, node_modules present):
  - `npm run lint` (eslint:fast --max-warnings=0 + import + governance full chain incl. `audit:complexity`) PASS ~116s, `✓ All production modules within 1000-line budget`.
  - `npm run lint:style` (stylelint) PASS ~1s; `npm run test:typecheck` (vue-tsc test + test-node) PASS ~7s.
  - `npm run build` (vue-tsc -b + vite + bundle:size:check) PASS ~7s, built in 1.78s; JS 5008.6KB / gzip 1462.4KB, Initial JS 831.6KB.
  - Extras: `lint:locale-parity` PASS 1291 keys en/zh-CN/es/ko consistent; `audit:complexity` PASS 0 modules over 1000-line budget.
- Non-blocking notes: `guardrail:make-factory-warn` + `locale-structure:warn` (en depth 5 > 4, canvas ns 274/300 leaves); jscpd 13 clones 0.06% within threshold; hotspots `CanvasScreen.vue` 747 lines, `boardSync.ts` 920 lines, `CanvasPage` chunk 644KB (gzip 192KB lazy), `heic2any` 1352KB (gzip 345KB largest).

## 2026-11-01 — api lint fix: 108 → 0 (working tree, uncommitted)

- On 2026-11-01, fixer subagent (ses_e6915c4c4ffeXmzjtfzfdFyVay) reported against `IdeaBoard@782e87c00` working tree: default `golangci-lint run` truncates via `max-same-issues=3` + `max-issues-per-linter=50`; untruncated (`--max-issues-per-linter 0 --max-same-issues 0`) true baseline was 108 = unused 70 + gofmt 34 + errorlint 1 + gocritic 1 + gosec 1 + noctx 1. All cleared to 0 in both modes.
- Fix summary: `gofmt -w internal cmd` 34 files; `ingest.go:39` `%v`→`%w`; `request.go:16,19` comment spacing; `test_client.go:132` → `NewRequestWithContext`, `:459` G705 `//nolint:gosec` (test stub); deleted zero-ref v1 stubs/aliases/test fakes (e.g. `routes.go` 10, `member_batch.go` 4, `billing.go` 4, `boards.go`, `branding.go`, `members.go`, `collaboration_test.go` file delete, `helpers_test.go` aliases). `.golangci.yml` untouched; `sqlc`/`migrations` untouched.
- Verified on 2026-11-01 by chief (working tree): `go vet ./...` exit 0; `go build ./...` exit 0; `golangci-lint run ./...` 0 issues exit 0; `git diff --stat` 54 files +96/-408. `go test ./...` (needs DB) and Fresh replay not run — residual risk.
- Rollback: `git checkout -- apps/api` or per-path checkout.

## 2026-11-01 — close-out: lint holds, DB tests + Fresh replay still open

- On 2026-11-01, close-out subagent (ses_e690a163cffe9tqHqrs1Msg6Zo) re-verified working tree (`782e87c00` + 54 files +96/-408, `docs/wiki/` untracked, `sqlc`/`migrations` clean): `go vet` 0, `go build` 0, `golangci-lint` 0 in default and untruncated modes.
- `go test ./...` exit 0 (181 pkgs ok, 0 FAIL) but partial: PG-backed cases SKIP (`set API_TEST_DATABASE_DSN...`); `ci-parity.sh api-test` not run (missing `API_TEST_DATABASE_DSN` + `API_TEST_NATS_URL`, fail-fast by design); `scripts/atlas-migration-check.sh` exit 1 at `ATLAS_DEV_URL is required`, Fresh replay skipped. Checksum-only `atlas migrate validate` still stands, not full contract pass.
- Decision: lint fix verified, do not merge on local only; remaining are ① parity api-test with DB/NATS (or CI), ② `ATLAS_DEV_URL` + `atlas-migration-check.sh`, ③ wiki empty-header merge done.

## 2026-11-01 — redundancy review and local verification

- On 2026-11-01, review of `IdeaBoard@782e87c00` plus the pending lint-cleanup diff confirmed that removed handlers, aliases and fixtures had no remaining callers; the deleted `collaboration_test.go` contained no `Test*` cases. Runtime wiring and task-store changes were formatting only. The image-ingestion error retained its field-validation classification after wrapping both causes.
- On 2026-11-01, the follow-up cleanup removed the stale `h.boardRoles()` comment reference and this wiki's redundant completion-status section and empty index categories; dated verification evidence was retained.
- On 2026-11-01, follow-up checks passed: `go test ./...`, `go build ./...`, v1 HTTP and workspace-media package tests, untruncated `golangci-lint` (`0 issues`), and `git diff --check`. DB/NATS integration remains unverified locally because connection settings were absent. No schema or migration changed in this cleanup.
