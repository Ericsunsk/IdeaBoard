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

## 2026-11-01 — Local CI with isolated dependencies

- On 2026-11-01, local verification of `Ericsunsk/IdeaBoard@97a364044` plus pending documentation edits used isolated PostgreSQL 17 / authenticated NATS JetStream, Go 1.26.10, Atlas 1.3.2, golangci-lint 2.14.0, and the canonical Linux Playwright image. Both temporary dependency containers were removed after verification; application source and existing development databases were unchanged.
- On 2026-11-01, the earlier DB/NATS execution gap was superseded by strict `ci-parity.sh api-test` runs: standard (824s) and race (451s), each selecting all 237 packages, both failed `TestV1BoardCopyAcceptancePG` (`routes_test.go:2167`, empty Media refs instead of source+copy). A targeted rerun reproduced it; the race run emitted no data-race warnings. This supersedes the earlier missing-integration-evidence status, not the failing acceptance result.
- On 2026-11-01, Schema Release, independent Atlas Fresh semantic replay, API→Auth migrations twice, sqlc diff/vet, nested test infrastructure, Go vet/build, pinned lint and reachable-vulnerability checks passed. The Fresh execution gap was superseded; the migration risk gate remained blocked by `20261111000000_model_pool_provider_split.sql` dropping a constraint without approved destructive-DDL handling.
- On 2026-11-01, comparison against successful ancestor `ffce04c119d2df3215bf38a392c866227f397ed7` ([CI run 40681029248](https://github.com/Ericsunsk/IdeaBoard/actions/runs/40681029248)) found eight error-level Control OpenAPI breaks. The Media contract scan also rejected `worker_transfer.go` importing concrete `internal/media/storage`. No approval bypass was added.
- On 2026-11-01, Web full unit verification passed with two workers (644 files, 4979 tests, 2 todo), preserving the original timeouts after a default-concurrency bootstrap timeout; Linux browser (125 tests), accessibility, route smoke, lint/typecheck/build and generated API checks passed. Optional live API smoke probes were skipped because their URL was unset. Dependency audit reported 5 high / 2 moderate advisories; an extra formatter check flagged 54 files. Auth (227 tests with PostgreSQL), Realtime (67), ai-proxy (98), 32 Ops self-tests and local deployment contracts passed. No deployment or external publication ran.
- Evidence recorded on 2026-11-01: local ignored artifacts under `.cache/local-ci/`, especially `local-ci-summary.json`, `api-results.json`, `migration-results.json` and `web-summary.json`; reproduction entry points remain in [DEVELOPMENT.md](../../../DEVELOPMENT.md#ci-parity-gate). These results do not constitute a green CI Gate.

## 2026-11-01 — CI 修复与 DB baseline

- 基于 `97a364044` 的本次修复（[Issue #957](https://github.com/Ericsunsk/IdeaBoard/issues/957)）：Board Copy 上传夹具改读 `data.asset.id` 并验证 ID；Worker media handoff 归入 Media；升级 `vue-i18n 11.4.17` / Vitest `4.1.12`，production audit 清零；54 个 Web 文件按 Prettier 格式化。用户明确审核的八项 Control API 变更按可信 base 与前后 spec SHA-256 精确记录，实际比较和未审核变更门禁保留。
- 最终验证使用 Go `1.26.10`：238 packages 的 strict PostgreSQL/NATS standard 与 race 均通过，vet/build、完整 pinned lint、32 项 Ops 自测通过；Web 644 files、4979 tests（2 todo）、lint/type/build/generated 检查与 Linux browser 125 tests、accessibility、route smoke 通过。可选 live API smoke 因 URL 未设置跳过。新 `20261113000000_baseline.sql` 来自 canonical schema artifact，替代旧 baseline 和四个增量；Atlas Fresh、Schema Release/schema 等价性、API→Auth 两轮迁移通过，旧 revisions 的 canonical migrator 明确拒绝自动 reset。可信累计比较基线仍为 `ffce04c119d2df3215bf38a392c866227f397ed7`。
- 本机 Go 高于 `go.mod` 时不会自动降级；复现 CI 显式设置 `GOTOOLCHAIN=go1.26.10`，本地 Web 使用两 workers，保留原断言和 timeout。证据位于 ignored `.cache/local-ci-fixes/`。未部署或重置现存数据库；跨 baseline 只能按既有 clean-break 流程导出并重建，回退应恢复与旧 revisions 匹配的镜像和 migration chain。

## 2026-11-01 — Auth CI 冷缓存超时

- `Ericsunsk/IdeaBoard@7e803581ed5c1511788dd96b86413819533f1e17` 的 [CI run 41313760231](https://github.com/Ericsunsk/IdeaBoard/actions/runs/41313760231) 只有 Auth Quality 失败：live Go↔Fastify contract 的 60 秒 deadline 包含依赖下载和编译，Auth job 未配置 canonical Go setup。API、Web、Ops、Media 和 Migration lanes 通过。这补充上一轮本地通过的记录，未将本地结果等同于远端成功。
- 在 2026-11-01，以固定 Go `1.26.10`、独立 module/build cache 和两核并发复现相同的 60 秒超时；热缓存约 4 秒通过。修复将 Go setup 与 contract package 编译移到 `Test Auth` 之前，使用独立 `auth-contract` build cache，保留原 timeout 和全部接口断言；workflow 负向 fixtures 拦截缺失准备、替换编译命令、错误顺序和忽略失败。复现与验证证据位于 ignored `.cache/github-ci-fix/`；本地冷环境操作见 [CI parity gate](../../../DEVELOPMENT.md#ci-parity-gate)。

## 2026-11-01 — clean-break 发布准备

- 截至 `Ericsunsk/IdeaBoard@d4b91619fc69390785325091cbc20f94459ddb76`，Release Eligibility 在 `persist-credentials:false` 的 checkout 后，用 command-scoped `gh auth git-credential` 和既有 `GH_TOKEN` 认证私库 fetch；保留 ancestry 与成功 CI Gate 检查，不写入 Git credential。隔离 Git config 的私库只读 fetch 已验证认证路径。
- 在 2026-11-01，用户说明旧 VPS stack 已删除，production `INFISICAL_TOKEN` 配置在 GitHub Environment。`Ericsunsk/IdeaBoard@d4b91619fc69390785325091cbc20f94459ddb76` 的自动部署通过受限 SSH stdin 送 token，VPS Infisical CLI 注入 `prod` 双 path，deployment 子进程移除 token；首次 owner 凭据独立存于 root-only `/var/lib/ideaboard/owner.env`，已有 PostgreSQL volume 缺该凭据时拒绝部署，保留 encrypted offline fallback。协议升级先安装同版本 root-owned entrypoint；不得把删除旧 stack 解释为额外 reset 数据库的授权。详见 [部署手册](../../../docs/runbooks/deployment.md)。

- 在 2026-11-02，用户采用 用户提供的 CI/CD review proposal（2026-11-02） 的影响范围原则：孤立 Ops 工具不触发应用 suite，发布复用 exact-source canonical main CI，完整 immutable image batch 暂不混用。基于 `Ericsunsk/IdeaBoard@d4b91619fc69390785325091cbc20f94459ddb76` 的后续修复保留成功祖先累计比较及未知影响 full fallback；同 SHA 的 repository、workflow path、event、branch、run 与唯一 CI Gate 均需成功。发布 helper 直接写 `GITHUB_OUTPUT`，不以 pipeline 吞掉失败。proposal 为设计输入，实施证据以提交和 Actions 结果为准。
