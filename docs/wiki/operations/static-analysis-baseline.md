# Static Analysis Baseline

Related: [Wiki Index](../index.md), [DEVELOPMENT.md](../../../DEVELOPMENT.md)

## 2026-11-17 — imgproxy 启动失败与部署前校验（未提交）

- 2026-11-17（Asia/Shanghai），以 `Ericsunsk/IdeaBoard@fa8f73e2a836f3bc7ad4172b206a79a2018ff3bf` 为本地代码基线，对生产环境只读诊断确认：`IMGPROXY_USE_S3=ture` 拼写错误触发 boolean 解析失败，导致 imgproxy 启动退出并持续重启；浏览器中的外部 Search preview、受管素材 derivative 与 Branding Logo 均返回 503。根因证据来自该日生产容器日志与浏览器请求，不将本地 HEAD 当作生产部署版本。
- 2026-11-17，同一基线上的未提交修复在既有 [部署环境校验](../../../scripts/deploy-stack.sh) 中验证 `IMGPROXY_USE_S3`，保留 Go `ParseBool` 语法、环境变量优先级和 Compose 空值默认；[回归测试](../../../scripts/deploy-stack-test.sh) 先红后绿，部署脚本 tests、ShellCheck、shell syntax、env schema 与 diff 检查通过。截至本次记录，尚未获准修改生产配置或重建容器，线上未恢复；本地预防性校验不等于线上修复。
- 2026-11-17，用户切换 Storage Profile 后的只读复查取代此前 active Local 的运行态事实：active profile 已为 S3-compatible，但 Cloud 与容器的 `IMGPROXY_USE_S3` 仍为 `ture`，三类图片请求仍为 503。Profile 与 sidecar 的 endpoint、region、access key、bucket 和 allowlist 对照匹配；Profile 持有 encrypted secret，Cloud 对应 sidecar secret 缺失。UI Apply Profile 只更新应用配置，不能证明 sidecar 配置已同步或容器已重建。
- 2026-11-17，同一本地基线另确认 S3 credential 映射缺陷：[imgproxy v4.0.14 S3 client](https://github.com/imgproxy/imgproxy/blob/v4.0.14/storage/s3/storage.go) 使用 AWS SDK 默认凭据链，[transport configuration](https://github.com/imgproxy/imgproxy/blob/v4.0.14/fetcher/transport/config.go) 未声明 `IMGPROXY_S3_ACCESS_KEY_ID` / `IMGPROXY_S3_SECRET_ACCESS_KEY` 别名。未提交 [Compose 修复](../../../docker/compose.yml) 保留这两个部署输入，改为输出容器实际读取的 `AWS_ACCESS_KEY_ID` / `AWS_SECRET_ACCESS_KEY`，同步 env schema；真实 Compose render 回归先红后绿，前述脚本检查再次通过。Local 与 S3 transport 可同时启用；Workspace Media 和 Search 临时 preview 仍走 Local，Avatar / Branding Logo 随 active profile。此补充说明 typo 并非唯一配置问题；截至本次记录未发布签名 release、未修改生产配置，线上恢复仍未验证。
- 2026-11-17，同次只读复查另见 active S3 Profile 的 `force_path_style=false`，sidecar 未设置 `IMGPROXY_S3_ENDPOINT_USE_PATH_STYLE`，继承 v4.0.14 的 `true` 默认；这是配置不一致，尚未证明单独导致故障。未提交修复补充该官方开关透传并保留默认 `true`，复用 boolean 校验，验证显式 `false` 与缺省 `true` 的真实 Compose rendering；定向脚本与静态检查通过，生产仍待授权发布与恢复验证。

## 2026-11-17 — AI Proxy affected CI 补齐

- 2026-11-17，审计 `Ericsunsk/IdeaBoard@7988bc7d5` 确认已有按应用分类、Go/Vitest affected planner、成功 `main` push 祖先的累计比较、`CI Gate` 与 release component image reuse。本轮不替换这些机制。缺口是 `apps/ai-proxy/**` 落入 unknown 全量分类，却没有执行 Worker 自身测试。
- `Ericsunsk/IdeaBoard@27f5f4f6b` 新增独立 Worker lane，执行 TypeScript build、98 个本地 mocked 单测、Go↔Worker wire contract；依赖 manifest/lock 变化另做 production dependency audit。`workerwire/**`、Go module/lock、Node/Go setup 等共享输入选择 Worker，其他应用的原有要求保留；`CI Gate` 拒绝所选 Worker lane 失败、取消或 skipped。Cloudflare Git 直连发布、签名 bundle、Production Environment、手动全量与 unknown 全量回退保持原有规则。
- 2026-11-17，对上述两个 commit 的真实 selector 以 `apps/ai-proxy/src/index.ts` 为输入比较：旧结果选择 Web/API/Auth/Realtime/Migration Integration/Ops、preview images 与 browser smoke，新结果只选择 AI Proxy；always-run Media/Handbook/PR size/Gate 未缩小。`workerwire/contract.go` 的 API/browser 要求保留，并增加 AI Proxy；docs-only 与 Web-only 结果保留。累计范围若仍有其他应用未验证改动，仍须运行其 lane，不能把单路径对照当作每次 PR 的保证。
- 2026-11-17，本地 22 行路径选择表、Gate 终态矩阵、baseline rename/delete/new-branch fixtures、workflow 正反 fixtures、derived dependency map（51 edges / 28 inputs）、`actionlint`、ShellCheck、Worker build/98 tests/wire contract 与 production audit（0 vulnerabilities）通过；独立安全复核通过。未触发部署或更改 GitHub/Cloudflare settings，未运行全量应用 suites；新增 workflow 的 GitHub-hosted 首次执行与实际 runner 成本仍待远端证据，不以本地时长推断节省金额。

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


## 2026-11-02 — CI/release optimization scope

- On 2026-11-02, the user approved the remaining optimization proposal except periodic full validation and required implementation/review before the next push. The spec and acceptance boundaries were published in [IdeaBoard #958](https://github.com/Ericsunsk/IdeaBoard/issues/958); component reuse, measurements and integration were split into [#959](https://github.com/Ericsunsk/IdeaBoard/issues/959), [#960](https://github.com/Ericsunsk/IdeaBoard/issues/960) and [#961](https://github.com/Ericsunsk/IdeaBoard/issues/961).
- On 2026-11-02, the accepted compatibility rule required a complete immutable manifest plus per-component input fingerprints and original source/signer provenance. Legacy releases without that evidence must build every component. Reuse must retain current vulnerability scanning and exact digest signature checks; migration/reset safeguards remain mandatory.
- As of `Ericsunsk/IdeaBoard@51e7d7def202778550f43cdf98a2a5221ea95e86`, successful `main` CI [41324592242](https://github.com/Ericsunsk/IdeaBoard/actions/runs/41324592242) selected Ops and always-required contracts while skipping unchanged Web/API/Auth/Realtime/Migration lanes. Changes to core CI classification/gate semantics still require full fallback.

## 2026-11-02 — Release image security refresh

- On 2026-11-02, verification of `Ericsunsk/IdeaBoard@20eec3ab18d351389f710fbaf09787bc3dcdcfd5` plus the security-refresh working tree passed actual-image Grype scans for all five release components at the existing High/Critical threshold with `ignore: []`. Node moved to `22.23.4`; the API runtime base digest and Atlas binary were refreshed. Unknown-severity advisories remain outside that threshold; this record does not claim zero vulnerabilities.
- On 2026-11-02, the selected official Atlas `1.3.7-0a182ba-canary` build was pinned by exact version and separate AMD64/ARM64 SHA-256 values. Docker and both migration CI callers share the checksum-verifying installer. Canary stability remains a risk; actual ARM64 API/Auth migrator images passed PostgreSQL 17 Fresh validation, two apply rounds and Schema Release equivalence. Evidence lives in ignored `.cache/tag-vps-deploy/`; Production deployment had not run at this verification point.


## 2026-11-02 — VPS topology ownership

- On 2026-11-02, deployment of `Ericsunsk/IdeaBoard@6877395972b188c82d120f69a16cd415bec79f3e` verified the baseline and healthy runtime containers, while the public reverse proxy could not resolve API/Auth because the NPM Compose override was absent. Cloud mode reads runtime configuration from the injected process environment, so putting topology in the local PostgreSQL owner credential file did not configure it.
- On 2026-11-02, the deployment decision assigned optional NPM topology to root-owned `/etc/ideaboard/deployment.json`, separate from Cloud secrets and owner credentials. The restricted entrypoint validates this bounded exact-key file and passes its topology after Infisical injection. An absent file preserves standalone topology; malformed or writable configuration refuses deployment. See the [deployment runbook](../../../docs/runbooks/deployment.md#nginx-proxy-manager).

- On 2026-11-02, the user required the VPS to retain only deployment-required worktree files. A read-only audit of `Ericsunsk/IdeaBoard@f3577d3e6df56568e95a0fa0c39c8200249bd926` identified ten mandatory host inputs; application code and migrations are supplied by immutable images, and component fingerprints use Git objects. This transitional setup kept full Git history, exact non-cone sparse paths and installed root-owned operations outside the release checkout; runtime state, credentials, backups and Docker volumes remain outside source cleanup. Temporary two-release fixtures verified source exclusion, state preservation and unchanged provenance fingerprints.

- On 2026-11-02, the user superseded sparse checkout with a signed deployment bundle and required a single clean implementation. CI owns Git/source fingerprints and verifies component provenance; VPS verifies a complete canonical tar before parsing/execution, retains versioned signed rollback artifacts and stable state, and requires no repository. See the canonical [deployment runbook](../../runbooks/deployment.md). Local and live acceptance evidence is recorded only after the corresponding checks complete.
- Local acceptance for the signed-bundle implementation passed all 41 Ops self-test scripts, the 45-edge CI dependency map, workflow positive/negative contracts, actionlint, ShellCheck, environment schema validation and Python syntax checks. Independent security review resolved two findings and reported no demonstrated bypass. Live keyless publication and repository removal remain separate release acceptance.


## 2026-11-02 — Signed bundle live acceptance and deployment stability

- On 2026-11-02, `Ericsunsk/IdeaBoard@00b826e27c551bdfd21f7298f8b814ffb20fcf98` was released as `v0.1.31`; [canonical CI](https://github.com/Ericsunsk/IdeaBoard/actions/runs/41344410551), [signed-bundle release/deploy](https://github.com/Ericsunsk/IdeaBoard/actions/runs/41344702412), and [saved signed-bundle recovery deployment](https://github.com/Ericsunsk/IdeaBoard/actions/runs/41345931057) succeeded. The live VPS retained verified bundles, stable configuration/state, backups and Docker data; the repository, `.git`, application source and sparse-checkout helpers were removed. This supersedes the earlier sparse-checkout transition and the earlier uncompleted live-acceptance record.
- On 2026-11-02, the same release's network audit required only API, Auth and Realtime on `npm-nw`, alongside the private application network. PostgreSQL, NATS, Worker, Imgproxy and migration jobs stayed private. Realtime DNS failure had caused NPM to quarantine the existing proxy host; after network repair, the owning NPM configuration flow restored that host without changing its domain or certificate.
- On 2026-11-02, the recovery exercise exposed a versioned PostgreSQL init-script bind source that unnecessarily recreated PostgreSQL between identical extracted bundles; Auth recovered with one historical container restart. Follow-up local verification against `Ericsunsk/IdeaBoard@00b826e27c551bdfd21f7298f8b814ffb20fcf98` plus the stable-bind patch passed signed-input promotion, unsafe-input rejection, Cloud override prevention, and identical-source Compose hash regression checks. The same follow-up added the Pages `/realtime` proxy route and preserved the upstream Cloudflare WebSocket response; targeted proxy contracts, Web type/build and boundary checks passed. Final combined CI and the new stable-path live redeployment remain separate acceptance checks; these local results do not claim authenticated collaboration was exercised.


## 2026-11-05 — 未提交 worktree 审计与验证边界

- 2026-11-05，审计 `Ericsunsk/IdeaBoard` 全部未提交 worktree，BASE `c8d1b5fa11f391dd3df04620b0ac81470070cb1e` 仅作锚点；不能把发现归因到该 commit。审计前范围 **265 tracked（+12272/-4847）、27 untracked**，staged 为空。追加本段 wiki 之前，`git diff HEAD --` SHA256 为 `b0492af969ec75f5c9a3ae80ca27f929f98961a146d6151f7df055490a91adba`；两个关键 frontend 文件 `commandHandlers.ts`、`useMemberQuotaSection.ts` 的 scoped diff SHA256 为 `f24de4f98c0e54cab6757ee457da08e7eaab8d1954b71e997faf245bd744195b`。27 个 untracked 文件按路径排序、以 `sha256  path\n` 编码的 manifest SHA256 为 `e623ff64f95396ceb70417202007596f1c2ae0ee48ae474e578043a079ab3ad6`（临时 manifest：`/tmp/ideaboard-audit-untracked-sha256.txt`）；wiki 追加将改变后续 worktree 指纹。
- 复现范围必须组合 `git diff HEAD --` 与 `git ls-files --others --exclude-standard`；只比较 HEAD/origin 或只看 tracked diff 会漏审。图索引 coverage 与原文/diff 交叉核对已执行；巨大 Knip/Seam metadata 未逐行业务审计。确认结论为 [UI/UX 复审](ui-ux-audit.md#2026-11-05--未提交-diff-的行为与规范复审) 的 P1/P2 与 Admin Copy P3，以及 [Actor-Pays 复审](../decisions/personal-actor-pays.md#2026-11-05--actor-pays-复审与-org-quota-边界) 的文档 P3；本轮只审计和记录，未修复。
- 相关 frontend 既有 **858 passed、1 todo**，Browser **25 passed**；新增边界场景 **2 failed / 1 passed**（真实 Router 创建时序、跨 Org/Workspace role）。独立 Standards 复跑 Board 对照 **1 failed / 1 passed**。typecheck:app/node、token graph、style governance（2455 files）、locale parity（1298 keys）通过；parity 保留 depth 5/4 warn。未执行完整 frontend build、全套 tests 或人工视觉验收。绿测不能替代真实路由生命周期与不同上下文 role 组合的验收。
- 后端初轮 **9 packages passed、343 test/subtests passed、196 skipped**，原因是缺少测试 DSN。复用本地 PostgreSQL volume 时初始化密码与存量密码不符，出现 `28P01`，该次未进入业务断言；后改用**独立临时 DB + 一次性非 superuser role**，关键 Fresh DB **11/11、0 skip**、补充终态场景 **5/5** 通过。两轮 DB/role 均已删除，未重置现有 volume 或改其密码。NATS 未做集成验证；Fresh DB 基于 canonical schema snapshot，不等同于完整 migration replay。
- 本轮采用的低风险复盘改进仅为留存可复现边界场景与隔离 DB/role 经验；回归测试落库、代码修复、lint/广泛标准调整留待后续，不安排定时审计。指定 `retro/SKILL.md` 在已查 skill roots 中缺失，未创建或安装；本轮为轻量证据复盘，不声称执行完整 retro skill。临时 sources：`/tmp/ideaboard-audit-scope.md`、`/tmp/ideaboard-audit-frontend.md`、`/tmp/ideaboard-audit-standards.md`、`/tmp/ideaboard-audit-spec.md`、`/tmp/ideaboard-audit-retro.md`；测试 JSON 为 `/tmp/ideaboard-spec-go-tests.json`、`/tmp/ideaboard-spec-db-tests.json`、`/tmp/ideaboard-spec-db-terminals.json`。临时文件不保证跨会话保留，长期规则仍以 [DEVELOPMENT.md](../../../DEVELOPMENT.md#ci-parity-gate)、CONTEXT 与 ADR 为准。


## 2026-11-06 — 冗余清理验证与产物保留（未提交 worktree）

- 2026-11-06，本轮依据完整 tracked + untracked 开始快照清理 `Ericsunsk/IdeaBoard` dirty worktree；BASE `c8d1b5fa11f391dd3df04620b0ac81470070cb1e` 只作锚点，不能归因为该 commit。开始范围 302 paths，scope SHA256 `71c8d66ef9d08c0b063005f0981b7e82cc7c121045a8285ec27095ab73ade79e`；业务结果见 [UI/UX 记录](ui-ux-audit.md#2026-11-06--冗余清理与行为回归保留未提交-worktree)。
- 2026-11-06，独立 focused **7 files / 98 tests**、Backend Reader lifetime/error/Personal 重点 race 与唯一最终 `npm run build`（vue-tsc / Vite / bundle size）通过。API surface **286 entries / 4171 symbols**、strict dependencies、manifest、Knip、style governance（2455 files）、User copy 均通过；locale parity **1291 keys** 一致但保留既有 depth 5/4 warning，locale usage 的 **83** 个静态候选为 warn-only，不当作可删证据。
- 2026-11-06，canonical gates 未发现 drift，[API baseline](../../../apps/web/publicApiSurface.baseline.json)、[Seam inventory](../../frontend-seam-inventory.md) 与 [Knip manifest](../../../apps/web/scripts/knipGovernanceManifest.json) 均保持开始快照字节不变；未无条件刷新 ledger、扩 deep-import allowance 或增加 waiver。唯一额外 canonical doc 修正是 [Account CONTEXT](../../../apps/web/src/features/account/CONTEXT.md) 的旧 Personal quota 片段，按 accepted ADR-0042 对齐。
- 2026-11-06，轻量复盘保留三点：graph 漏 Vue/template caller 时以 coverage + 当前 source 补证；保留真实生命周期/权限行为回归，避免 LOC 至上；artifact 同步由 canonical gate 的真实 drift 驱动。未重新查找或安装已知缺失的 retro skill，未新增仪式或定时审计。
- 2026-11-06，Backend 实现方 focused/race 有 **1 个缺 DSN skip**，不计为 DB 通过；本轮未做 DB/NATS 集成、migration replay、完整 frontend suites 或 Browser/视觉。持久来源为上述仓库实现/回归与 [ADR-0042](../../adr/0042-personal-payer-is-actor.md)；`/tmp/ideaboard-redundancy-{backend,board,frontend,artifacts}.md`、`/tmp/ideaboard-redundancy-verify-*.log` 仅为临时补证，不保证跨会话保留。


## 2026-11-06 — Deepening 验证与边界（未提交 worktree）

- 2026-11-06，验证对象为 `Ericsunsk/IdeaBoard` 未提交 worktree，BASE `c8d1b5fa11f391dd3df04620b0ac81470070cb1e` 仅作锚点；[两候选的职责与持久来源](ui-ux-audit.md#2026-11-06--member-quota-与-task-readiness-deepening未提交-worktree) 为本轮结论，旧清理记录保持历史含义。
- 2026-11-06，Member Quota focused 与独立回归、app/test typecheck、定向 ESLint/format、strict dependencies 与正反 fixtures、API surface 通过。最终 frontend `npm run build`（vue-tsc/Vite/bundle size）、只读 manifest diff 与 normal/production Knip 通过。依赖 gate 首次发现真实 measurements drift 后，仅同步 [Seam inventory](../../frontend-seam-inventory.md) 九个既有 entries 的 consumer/fan measurements；ownership、public exports/deep-import allowances 未变，无新增 waiver。
- 2026-11-06，Task Readiness semantic/HTTP 独立回归、实现方相关包测试、targeted race（3 packages，0 skip）与 Go build 通过；相关包报告的 **181 skip events** 中，178 项受 DSN gate、3 项受 NATS gate 限制，不计作集成通过。实现方另用隔离临时 DB 通过 [两个 production adapter/Submit parity cases](../../../apps/api/internal/tasking/application/workspacetasks/readiness_test.go)：Personal Actor + private Board → Editor 的零预检写入，以及 Org Admin cap 拒绝与 pending Hold parity；独立复核核对源码、原始 JSON 与 DB/role cleanup 机制，未独立重跑 DB。
- 2026-11-06，本轮不声称全量 Org Owner/role matrix 的 DB 覆盖；queue 证据为 canonical transaction enqueue probe，非 live River/NATS worker。未重跑完整 frontend suites、Browser/视觉、live backend 或 migration replay。持久依据为上述实现/tests 与 Account/Workspace/Tasking CONTEXT；`/tmp/ideaboard-deepen-*.md` 和日志仅为临时补证。
