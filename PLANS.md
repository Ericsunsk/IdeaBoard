# ExecPlans

## P5 Canvas Structure Migration (2026-10-12)

- Goal: complete the P5/PR2 Canvas directory migration and align governance artifacts with the current `core/session/ui/tools` tree.
- Scope: move-only Canvas layout changes and their canonical inventory, API baseline, dependency report, Knip ledger, ADR, CONTEXT, and migration evidence. No runtime behavior or generated API wrapper edits.
- Success criteria: active Seam targets and consumers resolve to existing paths; Seam schema self-test, strict P5 migration ledger, public API surface update/check, and current P1 dependency report run against the new tree. Remaining dependency findings must be concrete source violations rather than stale inventory paths.
- Risks: preserve public exports and consumer edges across renames; do not delete historical evidence, shrink frozen quotas, or convert advisory deep edges into blanket exemptions.
- Rollback: revert only this migration's path and governance updates; retain the prior committed P4 checkpoint and do not reset unrelated worktree changes.
- Results: active Canvas seams now map to `core/graph`, `session/*`, and `ui/*`; moved tool paths are reflected in the Seam inventory, fan report, Knip manifest/ledger, public API baseline, and ADR. `Canvas Context` now documents the post-P5 ownership boundaries.
- Verification: Seam schema self-test and render passed; migration-ledger consistency/fixtures passed; `npm run api:surface:update` and `npm run api:surface:check` passed with 255 entrypoints / 3603 symbols. Strict P1 dependency checking passed with no stale Canvas inventory errors; it still reports 219 historical advisory undeclared deep edges.
- Status: COMPLETE locally; P5 formal acceptance, commit, and remote CI remain separate gates.

## P4 Audit Repair (2026-10-12)

- Goal: Resolve the P4 diff review findings without changing runtime behavior or claiming formal phase acceptance.
- Scope: Align the Admin toast boundary documentation/check with its tested contract; rerun current-revision P4 behavior specs and replace stale `verifiedRun` records; reconcile Admin shared bridge-shim inventory prose and the generated active Seam/report shim sets; record P4 verification and limitations here. No unrelated P4 refactors or push.
- Success criteria: The feedback guard has a positive adapter-consumer canary and negative bypass cases; P4 behavior evidence names current commands/results; shim status descriptions match the current inventory/report; formal acceptance remains pending.
- Risks: Do not overstate toast enforcement, retroactively treat old P0 runs as P4 evidence, or rewrite frozen IDs, quotas, or historical phase records.
- Rollback: Revert only this repair's targeted checker, context, plan, and ledger edits; preserve the P4 implementation and prior historical evidence.
- Results: Current-revision Admin behavior specs passed (3 specs / 24 tests); the feedback boundary self-test and source scan passed (8 cases / 453 Admin sources); P1 ledger remains green (0 unmet). The four stale P4 `verifiedRun` records now point to the current targeted command and explicitly do not claim formal acceptance or a committed checkpoint. Admin shared bridge-shim prose now distinguishes the retired 70-shim temporary set from canonical shared paths. Active Seam measurements were refreshed from the current dependency graph, and all 113 stale shim declarations plus their retired consumer edges were removed from the active inventory; the generated report now records zero active shim files.
- Verification: `npm run lint:admin-feedback-boundary`, `npm run test:run -- src/features/admin/users/session/commands.contract.spec.ts src/features/admin/users/session/commandsInvalidation.contract.spec.ts src/features/admin/credits/ui/pricing/AdminBillingPricingPage.component.spec.ts`, `npm run test:typecheck`, `npm run lint:dependencies -- --strict --phase=P1`, `npm run lint:migration-ledger`, `npm run lint:shim-ttl`, seam schema self-test, targeted Prettier checks, and `git diff --check` passed. Strict all-active Seam acceptance still reports the existing 210 non-batch advisory deep edges; this is not claimed as a P4 pass.
- Limitations: P4 formal acceptance, CI, and a committed green checkpoint remain pending. Inventory/report formatting was normalized with Prettier after the generated measurement refresh; no unrelated document content was removed.
- Status: COMPLETE locally; formal P4 acceptance remains pending.

## P3 审计修复（2026-10-11）

- Goal: 按用户“依次修复”指令完成 P3 审计的六项缺口修复与独立复核。
- Scope: 当前 main 的未提交 P3 diff；依次修复 production Query reads/observers/scope cancellation、真实 clean/dirty 判定、Account core purity、theme retirement guard、当前 revision evidence 与 P3 trio status。integrator 独占 canonical ledger，保留用户其他 staged/unstaged/untracked 内容；不 commit、push、切 branch 或创建 worktree。
- Success criteria: 真实 gallery/Canvas 读取由 Query 驱动；clean SSE refetch、dirty stale-only 与 scope abort 有 observable tests；Account core 无 Vue/i18n/DOM runtime；retirement negative fixtures 拦截错绑定与无条件描述；当前 P2.5/P3 证据与实际检查一致，历史 acceptance/red 缺口不伪造；相关 tests/type/build/lint/ledger 与独立复核通过。
- Risks: 保持 drafts/base revisions、404/409 fail-closed、HTTP adapter boundary、storage/locale keys、frozen 494 IDs/R403/B57/quotas。真实 red-before-original-implementation 未记录时保留历史缺口，并记录本轮修复前真实失败与修复后 green；不通过改写时间宣称历史通过。
- Rollback: 只撤回本轮 targeted patches；保留原 P3 迁移及历史证据，不 reset 或变更持久数据。
- Results: Board production Query ownership 已接入 `session/sync`，Gallery 使用 list-only hydrate，Canvas 使用 document hydrate；scope switch 会 abort 并丢弃 late response；clean SSE 触发 refetch，dirty/saving draft 只 mark stale。Account runtime helpers 已从 `core` 移至 `session`，theme retirement guard 改为 SFC/DOM AST 校验，Board public barrel 不再暴露 observer。ADR/Core/Board CONTEXT、Seam inventory 与 migration ledger 已同步当前 ownership。按实际 graph 更新 5 条受迁移影响的 P1 Seam consumers，`lint:dependencies -- --strict --phase=P1` 通过。用户同意将 trio 原始 implementation-before red 缺失记录为 scoped deviation；原始证据仍标 missing，三项 `productionStatus` 保持 pending。
- Verification: Board/Canvas focused suites、Account/retirement suites、typecheck、normal/production Knip、theme contract、locale parity、shim TTL、stylelint、production build、strict public API surface、strict P1 Seam current-batch check 与 migration ledger（60 fixtures / 0 warnings / 0 unmet）通过。独立 reviewer 通过核心行为（43 files / 312 tests、255 entrypoints / 3363 symbols）。无 phase filter 的 all-active-Seams strict 仍覆盖非本批 legacy records，不能据此宣称全仓 Seam strict 已通过；其 P0 advisory 是 205 条，当前 P1 report 的 `undeclaredDeepImports` 为 0。
- Status: COMPLETE：P3 audit repair、文档/ledger 修正、P1 current-batch Seam 收敛和独立复核完成；P3 formal acceptance 与本地 committed green checkpoint 尚待完成。未 commit/push。

## P2.5 Closeout（2026-10-11）

- Goal: 按用户“收尾p2.5”指令完成当前具名 delegated acceptance、前置验收及本地交付。
- Scope: canonical plan/ledger、ADR/Core/Board CONTEXT 和验收 checker；无 production Query migration、push 或后续 lane 执行。
- Success criteria: P1/P2 前置验收与六项设计有当前 review；明确记录已完成阶段的 historical checkpoint deviation，用真实 green committed SHA 建立当前 rollback baseline；所有 P2.5 gates accepted，checker accepted 正反 fixtures、artifact freshness、strict checks 与独立复核通过，commit 后工作区干净。
- Risks: 不伪造历史 before-start、user 亲自 signoff 或远程 CI；只为已完成 P1/P2/P2.5 的记录缺口登记 scoped closeout revision，保留未来 before-start checkpoint、frozen quotas 和 P3 production trio obligations。
- Rollback: revert 本轮 closeout commit，恢复原 pending 状态及 prior artifacts；不 reset 既有历史或数据。
- Results: P1/P2 retrospective acceptance 和 P2.5 11 gates 已由当前具名 delegated integrator `/root` 接受；历史 checkpoint 缺口保留为 scoped deviation。`5b7af20b` / `8184c2b9` 为 committed green baseline，closeout artifact fingerprint `9d73659d` 单独记录。ADR/Core/Board 状态同步；P3 blocked、trio production pending。
- Verification: strict P1/P2/P2.5：60 fixtures /0 warnings /0 unmet；Sync 38 tests、dual Knip、checker ESLint/node typecheck、format、plan JSON 和 diff check PASS；独立 verifier 核对 44 current +44 committed baseline hashes及所有 refs PASS。首次 fingerprint sort mismatch 的 strict 失败保留在 ledger history，canonical algorithm 修正后通过，未放宽 guards。
- Status: COMPLETE：验收与独立复核完成，local closeout commit 由 Git history 标识；无 push，P3/P4 未启动。

## Acceptance Without Push（2026-10-11）

- Goal: 按用户要求从验收标准移除 push 和远程 CI 的强制依赖。
- Scope: Desktop canonical plan 与 execution ledger；允许有实际 command/revision evidence 的 local workflow-equivalent CI，不改 runtime、workflow 或历史执行结果。
- Success criteria: push/remote publication 不再阻断验收；保留检查范围、immutable committed SHA、历史 before-start checkpoint 和 plan-owner review 要求；ledger consistency 与 format 通过。
- Rollback: 撤回本轮 plan/ledger 文档 patches，不影响已有 commits 或运行数据。
- Verification: strict P2.5 ledger（51 fixtures /0 warnings /0 unmet）、ledger Prettier、plan JSON registry/state-machine parsing 与 diff check PASS。
- Status: COMPLETE：canonical plan/ledger 已移除 push/remote CI 强制验收依赖；本地等价检查可作为 CI 证据，未变更剩余历史 checkpoint 和 review 状态。

## P2.5 Formal Review and CI（2026-10-11）

- Goal: 执行六项 Sync design 的受委托正式 review，运行当前 revision 的 CI lanes，并记录真实验收结果。
- Scope: 当前 main；subagents 独立审查设计与 CI。修复确认的 CI blockers：generated drift/worker scan 旧路径、无 backend smoke 的 network setup/进程清理、废弃 media key 的 production reject-list、standalone smoke regression 的 workflow/Knip entry，以及 ADR template。integrator 独占 canonical evidence；不 push、不切 branch，不开展 P3/P4。
- Success criteria: 六项 design review 有独立、具名、精确 revision evidence；实际 CI 等价 lanes、相关 regression 和 governance checks 通过；失败历史保留。local CI 与 GitHub Actions、local integration 与 remote merge 明确区分，历史 before-start checkpoint 缺失不补造。
- Risks: 不允许通过抑制全部 browser errors、allowlist 或字符串改写绕过 clean-break scan；保留 HTTP payload fail-closed、generated bytes、frozen IDs/quotas、permanent specs 和 production Query 边界。formal phase acceptance 依赖历史 checkpoint 与前置验收，未满足时继续 pending。
- Rollback: 仅撤回本轮 targeted CI/test/docs patches，保留已提交 P2.5 design；无 schema、persistent data 或真实 external writes。
- Results: 六项 design 与 Common/Knip/Quotas 独立 review PASS；真实 workflow checker、worker negative fixture、retired-param rejection、受限 anonymous smoke fixtures 与 direct Vite 生命周期修复完成。canonical ledger 刷新 44 artifact hashes、当前 command/review/local-main integration evidence，保留初始失败；formal acceptance 与 P3 start 继续 pending/blocked。
- Verification: 540 files / 3624 unit tests；patch 后 adapter 40、Sync 38、standalone helper 2 tests PASS。generated drift、full lint（0 errors /171 warnings）、style、typecheck、build/bundle budget、dual Knip、strict P1/P2/P2.5（51 fixtures /0 warnings /0 unmet）、实际 Linux smoke 自然 exit0、accessibility、Media/Handbook contracts 与完整 Ops（22 selftests 等）通过；494 frozen IDs、9 quotas、263+417 mappings 与435 generated file bytes 不变。
- Limitations: 当前 local main 比 remote main 领先230 commits；publication 会启动所有 contexts 的 CI，GitHub 尚无当前 SHA。未授权 push；计划的 named plan-owner acceptance、历史 before-start green checkpoint 与P1/P2前置正式验收需要明确处置，不补造过去记录。local checks 覆盖本轮scope，无 backend/authenticated browser integration claim。
- Status at formal review: NEEDS_DECISION；保留当时的 historical checkpoint/predecessor acceptance 缺口。本轮 P2.5 Closeout 已按当前 delegated mandate 完成 scoped deviation 和正式记录，见上方 closeout。

## P2.5 Diff 冗余清理与提交（2026-10-11）

- Goal: 审计当前 P2.5 未提交 diff，直接清理确认冗余，验证后在当前 main commit。
- Scope: subagents 只读审计 tests/helpers、governance 和 docs；integrator 顺序执行最小 patches，并独占 canonical ledger/hash/evidence 更新。不切 branch、创建 worktree 或 push，不开展 P3 production migration。
- Success criteria: 保留六项 decisions、38 active tests、negative mutations、51 ledger fixtures、exact P3 trio、frozen IDs/quotas、public surface、generated bytes 与现有 gates；cleanup 后相关验证及独立复核通过，commit 后工作区干净。
- Risks: 不为减少 diff 删除有效证据或放宽 guards；source/config 变化后刷新 artifact revision 和实际 command/review evidence，保留历史与失败记录；technical verification 不替代 plan-owner acceptance/CI。
- Rollback: commit 前仅撤回本轮 cleanup patches，保留用户原 P2.5 内容；commit 后可 revert 本轮 commit，不重置已有工作或持久数据。
- Results: 共用 Revision spec 的 PUT 筛选，移除 SSE EventSource 替身无消费者的方法，positive ledger fixture 复用单次验证结果；Core 保留 Query Scope eligibility/ownership/bans，将重复 Board policy 参数改为 canonical link。六项 verifiedRun 与历史 verification 清单改引用 owning evidence，全部 scenarios、assertion fingerprints 和 exact trio 保留；更新行号及 current artifact/command/review 记录，历史与失败 evidence 保持。
- Verification: cleanup 后六 specs / 38 active tests、test:typecheck、目标 ESLint（0 errors / 8 retained warnings）、Prettier、strict P1 gates、strict P2/P2.5 ledger（51 fixtures / 0 warnings / 0 unmet）、dual Knip/Knip ledger（0 issues）、diff check 与独立复核 PASS。33 current artifact hashes 全部匹配；494 frozen IDs / 9 quotas / 263+417 mappings / 255+3346 public surface / 110 shims / 435 frontend+backend generated file hashes 保持；无 production runtime 修改，沿用前轮 full tests/build/lint/style/generated 记录。
- Limitations: technical local PASS 与 commit 不替代 plan-owner acceptance 或 green CI/rollback checkpoint；formal acceptance/CI gate 仍 pending，P3/P4 未启动。
- Status: COMPLETE：冗余审计、清理与验证完成，本批完整 P2.5 改动通过当前 main 的一个 commit 交付，commit identity 以 Git history 为准。

## P2.5 审计修复（2026-10-11）

- Goal: 依次修复 P2.5 audit 的测试假通过/状态泄漏、永久 Sync 行为证据缺失、404/trio 不完整、文档契约和验收声明失真，以及 Seam/format gates 失败。
- Scope: 当前 main 未提交 P2.5 diff；subagents 顺序处理 tests/fixtures、隔离 Query design behavior harness、ADR/CONTEXT；integrator 独占 canonical ledger、Seam/report/governance。保留 P2 checkpoint 07805f4c，不切 branch、创建 worktree、commit 或 push，不落地 P3 production Query migration。
- Success criteria: 六 decisions 有 active observable regression，race/broadcast/404/revision/focus 可验证；独立复核确认 no-op/乱序/缺失 evidence 等失效路径确实失败。精确冻结 P3 red/green trio；verification、review、merge/CI/checkpoint acceptance 分开记录。相关 tests、test:typecheck、lint/build、strict dependencies、P1/P2 与 P2.5 ledger、dual Knip、format 和 frozen identity checks 通过。
- Risks: test-only design harness 保存 isolated spike 结论，不引入生产 Query cache 或模拟断言假绿；不扩 blanket exemptions，不手改 generated，不伪造 plan-owner acceptance/CI/merged 状态，不删除 frozen IDs 或改变 quota denominator。
- Rollback: 仅撤回本轮 targeted patches，保留用户原 P2.5 staged/unstaged/untracked 内容；无 production state/schema/storage key 迁移。
- Results: 移除 10 个 skipped placeholders，以真实 QueryClient/QueryObserver、HTTP adapter 和 SSE subscription 的 private executable design harness 保存六项决策；hidden-save 回归仅由真实 hidden/visibilitychange 触发，visibilityState descriptor 完整恢复。补齐 PUT/GET 404、revision、乱序广播与 workspace race，抽取共享 fixtures；DraftOverlay spec 移回 Board 私有边界。ADR 改为中文 proposed，CONTEXT 保持 English；纠正废弃 wire authority，冻结 P3 trio 的精确 assertions 与 production red/green bindings。精确登记 25 个 consumer edges 与两个具名消费者验证的 Knip test-support files，未扩 public surface 或 blanket exemptions。canonical ledger 分离 command、技术 review 与正式 acceptance，并验证 artifact freshness、gate scope 和无 CI/merge 的 false acceptance。
- Verification: 六 specs / 38 active tests、全量 540 files / 3624 tests 与相关 gates 通过，独立技术复核 PASS。实际 command/review、negative mutations 与 frozen invariants 详见 canonical `frontend-contract-inventory.md` 的 `syncDesign` evidence 和下方 Limitations。
- Limitations: 最后的 checker artifact scope 与两项 Knip 文件登记仅改变 governance；最终 specs、ledger、format、strict P1/dual Knip 重跑，保留先前全量 tests/build/type/style/generated 记录的历史 artifact fingerprint。无 production UI 修改，无 browser/visual 工作；未声明 CI/checkpoint 或 merged design。plan-owner acceptance、design/spec merge 和 immutable green CI checkpoint 仍 pending，P3/P4 启动 gate 未满足。
- Status: COMPLETE：本轮审计修复、验证与独立技术复核完成；保持 main/HEAD，未 stage、commit 或 push。正式 phase acceptance 与本轮修复完成状态分开记录。

## P2 Diff 冗余清理与提交（2026-10-11）

- Goal: 审计当前 P2 diff，直接清理有证据支持的重复、失效或无消费者内容，验证后在当前 branch commit。
- Scope: subagents 分别只读审计 runtime/tests、governance、docs；integrator 顺序执行最小 patches。保留已有 P2 行为修复与公开契约，不切 branch、创建 worktree 或 push。
- Success criteria: 清理不削弱真实 regression、frozen IDs/quotas、public surface、shim TTL 或既有 gates；相关 tests/type/lint/build、strict P1/P2 ledger、dual Knip 和独立复核通过，commit 后工作区干净。
- Risks: 不以移除有效 evidence 或 guard 减少 diff；不扩 blanket exemptions，不手改 generated 内容，不开展后续阶段迁移。
- Rollback: commit 前仅撤回本轮 cleanup patches；commit 后可 revert 本轮 commit，不重置用户原 P2 工作或持久数据。
- Results: 删除 Auth pending tests 无消费者的 InputText/Password/Message stubs 与无关 reset spy，共用有效 stubs；ledger positive fixtures 共用 phase completion setup，所有正反 cases 保留。quota 验证重复列表改为引用 canonical ExecPlan，删去无信息 verifiedRun 尾句与重复 public export prose，同步 Auth assertion 行号并格式化 Seam inventory。Gallery regressions、selection port、正式 TTL shim 和有效治理记录保持。
- Verification: cleanup 后 Auth 3 tests、test:typecheck、修改代码 ESLint（0 errors / 3 warnings）、strict P1/P2 ledger（38 fixtures / 0 warnings / 0 unmet）、完整 strict P1 gates（含 255 entries / 3346 symbols surface、dual Knip、locale parity、110 shim TTL）、Knip ledger（0 issues）、修改文件格式与 diff check 均通过，独立复核 PASS。沿用本轮前序 P2 审计修复的全量 534 files / 3586 tests、Gallery Chromium 2 tests、build/bundle budget、完整 lint/style/generated 验证；cleanup 未修改 runtime 或 generated bytes。494 frozen IDs SHA、R403/B57/P1b149、263 ordinary / 417 generated 与所有 quota denominator 不变。
- Status: COMPLETE；冗余审计、清理与验证完成，本批完整 P2 改动通过当前 main 的一个 commit 交付，commit identity 以 Git history 为准。

## P2 审计修复（2026-10-11）

- Goal: 依次修复 P2 audit 确认的 persisted selection 初始化、Legal consumer drift、compatibility shim TTL、Gallery identity/focus 与 frozen contract relocation。
- Scope: 当前 checkout/branch 的未提交 P2 diff；subagents 顺序实现，integrator 负责共享 inventory/baseline/evidence。保留原 staged/unstaged/untracked 工作，不切 branch、创建 worktree、commit 或 push。
- Success criteria: 每项缺陷有有效 regression 或正反 gate 证据；strict P1 dependencies、strict ledger（0 warnings）、P2 quotas、shim TTL、dual Knip、public surface、相关 ESLint、typecheck、build 和全量 tests 通过；独立复核无遗留问题。
- Risks: Gallery 移列时保留真实 DOM/child state/focus，不能以父级状态或不移列的目标替代；不缩 frozen denominator，不放宽 dependency/TTL/Knip gates，不手改 generated files；只审核实际新增 consumers。
- Rollback: 仅撤回本轮 targeted patches；保留用户原 P2 diff。无持久数据/schema 改动，原 selection storage key 不变。
- Results: persisted selection 在首次 port 安装时静默 hydrate；3 条 Legal test consumer edges 精确登记。必要 context wrapper 改为正式 TTL shim，无消费者 resource wrapper 退休。Gallery 采用同一父级下的 asset-id keyed cards，保留 round-robin 布局并恢复跨列 DOM/local state/focus；实测卡片高度与 rAF 合并测量支持异高图片、fallback 和 resize。6 个 retained matcher 完成 exact relocation/current assertion mapping，17 个新增 observable constraints 单独登记；P2 acceptance 复用 P1 检查并增加有效正反 fixtures。
- Verification: 全量 534 files / 3586 tests；Gallery Chromium 2 tests；strict P1 dependencies、strict P1/P2 ledger（38 fixtures / 0 warnings / 0 unmet）、strict public surface 255 entries / 3346 symbols、dual Knip 与 Knip ledger（0 issues）、test:typecheck、build/bundle budget、完整 lint（0 errors / 143 warnings）、format、style、generated drift 和 diff check 全通过。格式化后 Legal/DnD 7 tests 再次通过。494 frozen IDs SHA / R403 / B57 / P1b149 / 263 ordinary / 417 generated 及全部 quota denominator 不变；110 shims 包含新 context shim，其隔离 expiry 验证在 P2 通过、P2.5 正确失败。独立复核 5 项均 PASS。
- Limitations: ego-browser DOM 几何读取符合预期，但截图接口两次超时；视觉截图检查未完成，布局/focus 验证依据真实 Chromium regression tests。canonical TTL marker 仍按既有 owning-lane 清理要求保持 P1；未声明 CI、committed checkpoint 或后续阶段验收。
- Status: COMPLETE：本轮 5 项修复与验证完成，未 stage、commit、push 或启动后续 migration。

## P1 Diff 冗余审计与清理（2026-10-11）

- Goal: 清理当前未提交 P1 diff 中确认重复、失效或无消费者的代码与记录。
- Scope: subagents 只读审计 governance、runtime/tests、docs；integrator 依次执行最小 patches。不改 branch、worktree，不 commit/push。
- Success criteria: 保留 frozen 494 IDs / R403 / P1b149、public surface 254/3330、generated bytes、109 shim contracts；相关正反 fixtures、strict P1 gates、type/build/lint checks 通过，独立复核无遗漏。
- Risks: 不以删除有效 guard、测试场景或验收证据减小 diff；保持实际 runtime behavior 与公开契约。
- Rollback: 仅撤回本轮 cleanup patches，保留原 staged/unstaged/untracked 改动。
- Results: Knip factory 共用单次 active Seam snapshot（inventory reads 38→1），migration ledger 共用一次当前 spec 扫描；删除过期 P0 file decisions/duplicate reason 分支和一处弱于相邻精确断言的 browser helper 调用。196 条 private resolutions 的公共验证提到表格上方，逐项 file/symbol/disposition/evidence 尾句保持一致；Seam verification 引用 canonical ExecPlan，明确历史状态并修正 Auth runbook 路径。
- Verification: strict P1 gates、native normal/production strict Knip、Knip ledger、26 ledger fixtures（0 warnings/0 unmet）、12 governance/production tests、18 theme browser tests、test:typecheck、build/bundle budget、Web format、修改代码 ESLint（0 errors/9 warnings）与 diff check 通过。254/3330 public surface、494 frozen IDs SHA、R403/P1b149、417 generated facts 与 109 shim TTL 保持。独立代码复核 PASS；后续 subagents 因服务端 402 中断，integrator 补完文档核对，确认 196 条处置和局部 links 完整。未改 branch/HEAD，未 commit/push。
- Status: COMPLETE。

## P1 剩余验收项处理（2026-10-10）

- Goal: 完成 149 frozen + 5 supplemental retirements 的有效 replacements、4 mandatory behaviors，以及 full normal/production Knip 处置，使 P1 gates 实际通过。
- Scope: 当前 checkout/branch 的 P1 diff；subagents 依次实现，独立只读调查可并行；integrator 独占 configs/ledgers。保留 public contracts、runtime/type consumers 和 generated 内容；不启动 P2+ 目录迁移，不 commit/push。
- Success criteria: replacement positive/negative fixtures 或可观测 browser behaviors 先通过，再删除原 matcher；每个 frozen ID 有精确 replacement evidence。实际 normal 与 production strict Knip、strict P1 Seam/surface/ledger、locale/shim gates、相关 tests/build/type/lint 全部通过，独立复核无遗漏。
- Risks: 不缩 frozen denominator，不用 blanket ignores、entry roots 扩张或无依据 public tags 掩盖 unused symbols；generated 内容只由 generator 维护。Runtime behavior 只在有真实 defect 时改变，保持公开 routes、API 和数据契约。
- Rollback: 只撤回本轮 targeted patches；保留所有原 staged/unstaged/untracked 工作；无持久数据迁移。
- Progress: 110 semantic lint + 44 browser/icon replacements 已独立复核通过，149 frozen + 5 supplemental matchers 实际退役；4 mandatory behaviors verified。保留 494 IDs/R403/P1b149；strict phase ledger 0 unmet。84 lint fixtures、12 browser files/47 tests、strict surface254/3330、strict P1 dependencies/fixtures、test typecheck 已通过；最终 Knip manifest、private type cleanup 与真实 CLI fixtures 已完成，full dual Knip 和全体验证已 green。
- Verification: strict P1 gates 与 native full normal/production strict Knip 均通过；ledger 26 fixtures / 0 warnings / 0 unmet；surface 254/3330。全量 unit 531 files / 3571 tests + 最终 Knip fixtures 12 tests、browser 12 files / 47 tests、build/bundle budget、test:typecheck、完整 lint（0 errors / 139 warnings）、Web format、style、generated drift 和 diff check 全部通过。独立复核 PASS，未改 branch/HEAD，未 commit/push；CI/checkpoint 证据保持 unset。
- Status: COMPLETE。

## P1 审计修复（2026-10-10）

- Goal: 依次修复 P1 audit 的 identity races、迁移后失效的 dependency rules、Seam/inventory drift、quota 验收失真和缺失的 dual Knip gate。
- Scope: 保留当前 staged/unstaged/untracked P1 工作；subagents 按文件范围顺序执行，integrator 独占共享 config/ledger 更新；不启动 P2+ migration，不切 branch、创建 worktree、commit 或 push。
- Success criteria: stale disposers 与 batch late completion 有有效 regression；horizontal direction 真实正反 fixtures 通过；P1 consumer/surface gates 通过；恢复 frozen quota/evidence，未完成 quota/behavior 与 full dual Knip 正确阻断 P1 acceptance；全量 tests、build、typecheck、相关 lint/generated/format checks 与独立复核完成并如实记录结果。
- Risks: identity/auth 修复按 Production-mode 验证和独立复核；不改变公开 route URLs、wire contracts 或 serialized data；保留真实 runtime/types 和 generated 内容；不缩 quota、不以重新冻结当前违规 graph 或 blanket ignores 获得 green；未验证的验收记录不得标 accepted。
- Rollback: 仅撤回本轮 targeted patches；不重置用户 P1 diff；数据、locale 内容与持久化 cache keys 不迁移，无数据 rollback；runtime cache 可重建。
- Results: 修复旧 SSE/media disposers 破坏 replacement identity 及 batch late completion 污染 cache；horizontal rules 覆盖所有迁移 roots 并收窄为 11 个已登记 compatibility shims；auth route policy 独立于 features，legal links 回到 legal feature。按 frozen graph 和实际 relocation 修正 consumers/targets，补齐 public subpaths 和 109 shims；统一 canonical CONTEXT 与引用。恢复原 494 IDs / R=403 / P1b=149，撤销 conditional acceptance 和不足的 behavior 验证，新增真实 quota 与 full dual Knip gates。
- Verification: 全量 531 files / 3533 tests、build/bundle budget、test/node typecheck、style、generated drift、locale parity、shim TTL、修改代码 ESLint/Prettier、diff check 与独立复核通过。141 P1 active Seams 的 strict consumers/targets/purity/deep imports 全部通过；strict surface 254 entries / 3330 symbols；ledger 24 fixtures / 0 consistency warnings；Knip 8 个真实 CLI fixtures 通过。完整 lint 的前置 gates 通过（ESLint 0 errors / 127 warnings），最终在 full dual Knip 的 2314 issues 处 exit 1；完整 format 的既有 DEVELOPMENT 表格问题保留，避免无关 formatting churn。
- Status: COMPLETE：本轮代码与治理缺陷修复、验证及复核完成；当轮结束时 P1 acceptance 仍 INCOMPLETE，149 frozen + 5 supplemental retirements、4 mandatory behaviors 和 full Knip issues 继续阻断。P2+ lanes、commit 和 push 未执行。

## P0 Diff 冗余清理与提交（2026-10-10）

- Goal: 清理当前 P0 diff 中有证据支持的冗余，验证后在当前 branch commit。
- Scope: 去除治理脚本的重复 inventory 读取、重复 public symbol 扫描、无消费者的扫描结果字段与重复 fixtures 调用；同步本文件中的最终验收数据。保留冻结 inventory、ledgers、public contracts 与所有有效 regression fixtures。
- Success criteria: surface 与 fan report 内容不变；相关 schema、surface、boundary、ledger fixtures、typecheck、lint 与格式检查通过；完整 P0 diff 提交后工作区干净。
- Risks: 保留循环 re-export 的预扫描、Vue named exports 和 missing-export 检查；P0 advisory/error 边界与既有 gates 不变；不执行 P1 migration 或 push。
- Rollback: commit 前仅撤回本轮 cleanup patches；commit 后可 revert 本轮 commit，不切 branch 或创建 worktree。
- Results: boundary checker 复用一次 inventory 读取（原本 124 次）并移除重复排序；surface scanner 只预收集一次直接/具名 exports，移除未使用的结果字段与单次调用 wrappers，并复用同一 inventory snapshot；migration-ledger fixtures 每次 lint 只执行一次；前轮 P0 记录统一为最终验收数据。冻结的 surface 与 fan report JSON 内容保持一致。
- Verification: 完整 lint 通过（0 errors / 131 warnings），3 specs / 64 tests、12 surface fixtures、Seam schema/projection fixtures、strict migration ledger、node typecheck、Knip ledger、修改文件 Prettier 与 diff check 通过；原 13 public contracts 不变。前轮全量 3503 tests 与 build 基线保留，本轮仅清理 scripts/docs；完整 format:check 的 14 个既有未修改文件问题与 Full Knip 的 2316 个 P0 baseline issues 保留。
- Status: COMPLETE；清理与验证完成，通过当前 branch 的一个 commit 交付，commit identity 以 Git history 为准。

## P0 Diff 审计修复（2026-10-10）

- Goal: 依次修复 P0 审计发现，使工具规则、fixtures、Seam inventory 和验收 ledger 满足 SaaS Frontend Standard v4 的 P0 exit criteria。
- Scope: 保留现有未提交 P0 diff；当前 checkout/branch 内顺序执行，每一步由 subagent 实现或复核：
  1. [x] i18n structure 保持 warn-only；真实 lint 与边界 fixtures 共用 warning helper，7 tests、locale lint、相关 ESLint/Prettier 通过。
  2. [x] 冻结 382 Seam records 的 source/target、exports、consumer edges 和目标；report 覆盖 260 active entries，surface 覆盖 237 entries / 3261 symbols，原 13 contracts 不变；schema/surface/boundary fixtures、typecheck、ESLint、Prettier 和既有 Knip gate 通过。
  3. [x] 将既有 factory/URL 字段豁免收窄到 19 个 file+symbol / 6 个 type.member 身份；9 factory fixtures、74 naming fixtures、537 spec guardrails 与脚本 ESLint/Prettier 通过，实际新增 warnings 为 0。
  4. [x] Query placement 接入实际 ESLint rule，51 fixtures 覆盖 warn/error、动态/type imports、shared/core paths、harness/virtualization 与 Overlay coexistence；相关 ESLint/Prettier、Overlay smoke 和 test typecheck 通过。
  5. [x] Knip 6 个真实 CLI fixtures、test dependency 最小 lock 更新、barrel/type-aware boolean warnings 与具名 tooling owner/deadline 完成；逐项登记 2316 个 full Knip baseline issues，既有 subset gate 保留。surface missing/cycle/SFC exports、pure-runtime fixtures 与 shared public-subpath 覆盖已补；schema/projection/boundary/ledger/脚本检查通过。
  6. [x] 冻结 494 constraints（R=403、18 i18n candidates）/57 behavior scenarios、8 个 P1b packages 的 56h 计划预算、integrator/checkpoint 和 680 source files / 11 CONTEXT mappings；12 schema/AST fixtures 与 strict consistency 通过，settled 验证基线已更新。
- Success criteria: 每项修复有有效回归验证；P0 新增约束仍 warn-only（Canvas 高频态 Query 禁令除外）；既有 error gates 不削弱；相关 lint/format/type/tests/build 通过，完整 Knip 红项有精确处置；独立 Standards/Spec 复核无遗漏。
- Risks: 不为双绿删除真实 runtime/types，不手改 generated files，不执行 P1 目录迁移，不编造已有业务覆盖或已执行的迁移事实；配额与未来预算须标明实测/计划含义。
- Rollback: 仅撤回本轮 patches；保留用户原有未提交工作，不 reset、不切 branch、不创建 worktree、不 commit 或 push。
- Verification: 全量 Web 526 files / 3503 tests、build/bundle budget、test/node typecheck、完整 lint、各治理 fixtures、Knip ledger 校验与 diff check 通过；独立复核 PASS。完整 format:check 仅在 14 个既有未修改文件失败，修改文件格式通过。Full Knip 保留 2316 个逐项登记的 P0 baseline issues，P1 双绿尚未启用。
- Status: COMPLETE；审计发现与复核新增漏报已依次修复，保持当前 branch/HEAD，不执行 P1 迁移、commit 或 push。

## 未提交 Diff 冗余审计与提交（2026-10-10）

- Goal: 审计当前未提交 diff，删除有证据支持的冗余，并在当前 branch 提交验证后的完整改动。
- Scope: 以当前 HEAD 为基线，subagents 并行审计 Retouch、Board/Inpaint/Image Tools 及其余 API/Account/Admin/governance/docs；按互不重叠的文件范围清理。保留已接受的 feature-first layout、retouchStack hard cut 和前轮四项修复。
- Success criteria: 每项删除有调用链或 contract 证据；相关 tests、lint、type/build、generated consistency 通过，独立复核无遗留问题；commit 后工作区干净。
- Risks: 不删除 public seam、必要 boundary adapter、兼容约束或真实 regression coverage；generated files 仅通过 generator 更新；不改变 runtime behavior 或外部数据。
- Rollback: commit 前仅撤回本轮清理 patches；commit 后可 revert 本轮 commit。不切换 branch、创建 worktree 或 push。
- Results: 12 个代码/文档文件净减少 84 行；删除无消费者的私有 types/test-only helpers、重复 patch 委托与条件、无效类型包装和过时说明。保留公开 seam、默认工厂、错误分类差异及真实 regression coverage。
- Verification: Web 全量 524 files / 3441 tests、build（含 bundle budget）、test typecheck、完整 lint、style、generated API consistency、browser 3 files / 7 tests 和 Go build 全部通过。独立复核 PASS，并自行通过 6 specs / 117 tests、55 naming self-tests 与 diff check。
- Status: COMPLETE；冗余审计、清理与验证全部完成。本批通过当前 branch 的一个 commit 交付，commit identity 以 Git history 为准。

## 未提交 Diff 审计修复（2026-10-10）

- Goal: 修复本轮确认的 4 项 P2，并保留现有 staged、unstaged 和 untracked 工作。
- Scope: 当前 checkout 与 branch，由 subagents 依次修复：
  1. [x] 同步 `/user` profile fields 的 canonical OpenAPI，并重新生成 client；response/schema regression、HTTP package tests 与 generated drift check 通过。
  2. [x] 移除 Retouch `core/` 对 runtime i18n 的依赖，在 session/UI 翻译 label keys，并收紧 purity guard；166 targeted tests、dependency fixtures、lint、test typecheck 通过。
  3. [x] 保留 Sharpen visually neutral draft 参数，同时保持延迟 commit；4 个 regression 先失败后通过，100 targeted tests、lint、guardrails 通过。
  4. [x] 隔离 Gallery refresh 的旧 scope completion，防止 Workspace 切换产生虚假失败提示；11 个 regression 先失败后通过，72 targeted tests 及 boundary/contract checks 通过。
- Success criteria: 新增回归测试在修复前失败、修复后通过；相关 contract、test、lint、type/build checks 通过，独立复核无遗漏。
- Risks: 保持已有 API wire fields、layer labels、pending state machine 和真实同步错误反馈；generated client 仅通过 Orval 更新，不改 schema、auth policy 或外部数据。
- Rollback: 仅撤回本轮 targeted patches；不重置现有 diff，不切换 branch，不提交或 push。
- Verification: Web 全量 524 files / 3441 tests、build（含 bundle budget）、test typecheck、完整 lint、style、generated API consistency 及 browser 3 files / 7 tests 全部通过；Go HTTP package tests 通过。独立 subagent 复核 PASS，并自行通过 9 files / 150 targeted tests、build、test typecheck、修改文件 ESLint、dependency fixtures、generated drift 与 Go HTTP package tests。
- Status: COMPLETE；4 项已依次修复并通过整体验证与独立复核。保留现有工作区改动，当前 branch 与 HEAD 不变，未提交或 push。

## Canvas Session Save Recovery (2026-10-10)

- Goal: fix the false missing-session failure when autosaving an authenticated Canvas.
- Scope: CanvasPage identity wiring and its regression tests; preserve existing workspace diffs.
- Success criteria: the page passes the authoritative session storage namespace to the real save path; missing identity retains a local draft; refreshed identity permits saving without remounting; relevant tests, lint, formatting and type/build checks pass.
- Risk: do not invent a namespace from public_id or bypass the missing-session guard.
- Rollback: reverse only this task's patches; no schema changes or external writes.
- Status: COMPLETE; two regression tests reproduced AUTH_UNAVAILABLE before the fix and pass afterward. All 42 targeted tests, ESLint, Prettier, test typecheck and Web build passed. Browser verification confirmed import and drag autosave, durable MediaRef and matching remote positions (revision 4 to 6) without a failure toast.

## Archived Settings Diff Audit and Cleanup (2026-10-09)

- Status: implementation, redundancy cleanup and independent review complete.
- Goal: resolve the five settings audit findings, remove confirmed redundancy
  from the current batch and commit the reviewed result.
- Scope: strict Control request schemas; regenerated Auth Mail contracts; OAuth
  Settings ownership; optional Mail enabled requests and required responses;
  Mail toggle draft retention; unused resource wrappers/types/options, duplicate
  SKU validation, unreachable defaults, no-op factory and stale descriptions.
- Success criteria: valid request payloads pass while unknown fields fail;
  generated contracts have no drift; omitted enabled preserves disabled Mail;
  toggles neither save nor discard drafts; cleanup preserves existing behavior.
- Results: independent review found no remaining defect. Final verification
  passed 235 Web tests across 42 files, full frontend lint, Web build/test
  typecheck, generated API consistency, focused Auth tests/build, complete Go
  Auth/HTTP package tests, formatting and diff whitespace checks. Earlier audit
  verification also passed the full Auth suite and Go build.
- Risks: preserve operator reasons, strict binding, secret masking, Mail
  readiness and UI contracts. No deployment or live data writes.
- Rollback: revert the resulting commit; no database rollback is needed for
  cleanup. Retain the additive Mail enabled backfill if reverting application
  behavior separately.

## Archived Audit and Acceptance for #904–#914

- Status (2026-10-08): implementation and cleanup verification complete. Git
  history and issue acceptance comments record the commit and closure.

- Goal: fix the six audit defects, remove confirmed redundancy, verify the batch,
  commit on the current branch and close accepted issues.
- Scope: current batch files only. Fence Control attempts and unknown dispatch
  outcomes; restore Library's original schema/recovery scope; complete policy
  parity, persisted avatar projection and actual-folder Search feedback. Remove
  unused wrappers, duplicate gates, unreachable checks and stale descriptions.
- Success criteria: uncertain external effects never restart automatically; old
  attempts cannot settle newer claims; runtime policy matches the spec; avatar
  survives cold login; deduplicated saves report their actual folder. Preserve
  ownership seams and behavior tests, pass affected checks, commit and close issues.
- Verification: parallel Standards/Spec review; focused Control/Library/avatar/
  Search tests; frontend test typecheck/build/lint and backend build; relevant
  PostgreSQL tests; sqlc/schema/migration/API drift and formatting checks.
- Results: batch verification passed 504 frontend tests and 106 fixture-related
  tests plus relevant PostgreSQL tests and schema/generated gates. After cleanup,
  92 frontend tests, typecheck/lint/build, targeted Go tests, backend build,
  generated API drift and formatting checks passed.
- Rollback: revert the batch commit; retain additive database columns and the
  unknown-outcome fence. Reopen issues if acceptance is invalidated; never revert
  persistent databases automatically.

## Archived Image Catalog Structure

- Goal: concentrate existing Unsplash/Pexels knowledge and supplier HTTP handling.
- Scope: shared frontend provider vocabulary consumed by Search, Library and Admin;
  separate backend supplier adapters from shared HTTP handling; remove duplicate
  provider guards and unused search constants. No new supplier, UI or schema.
- Verification: existing provider IDs, errors, ordering, attribution, cursor
  encoding, URL allowlists and request behavior remain compatible; focused tests,
  frontend type/build/lint and backend build/tests pass.
- Rollback: revert only this refactor's working-tree changes; no data migration.

## Archived Workspace Task Restoration ExecPlans

> Status (2026-10-08): **ARCHIVED**. The plans below describe completed work.
> Issue acceptance comments and git history are the source of truth; do not use
> this file as an active queue. New work needs a new plan or an issue-specific
> execution note.

## Archived 8xx Batch ExecPlan (#886–#897)

This scheduler plan is archived. Issues #886–#897 were completed, verified, and
committed; the former working-tree-only constraint and lane sequencing are no
longer active. The issue comments and git history are the source of truth for
their acceptance evidence and commits. The detailed lane checklist was removed
because it duplicated the issue tracker and could imply that this work is still
pending.

## Archived Issue #886 ExecPlan: Payment Refund Persistence Boundary

- Goal: persist the admin refund reservation, `PurchaseRefundHold`, and phase
  audit before any Stripe I/O; make provider result convergence observe the
  committed refund and reuse its durable identity after retries.
- Scope: isolate the refund reservation and post-provider settlement writes
  from the Control guard's still-open transaction; preserve pending recovery,
  existing Credits commands, and the Stripe idempotency key derived from the
  durable refund ID; add focused production-path tests with a fake gateway.
- Success criteria: no Stripe call occurs before the reservation commit; a
  provider result can be recorded and settled from a fresh database connection;
  commit/audit failures leave a recoverable pending fact; retries reuse the same
  refund ID and Stripe idempotency key without a second cash refund.
- Rollback: revert the issue commit. No schema changes are expected.

## Archived Support Channels ExecPlan

- Goal: expose operator-configured external support channels from the account menu.
- Scope: typed runtime settings in the existing system config store; audited
  Control Plane reads/writes; anonymous Public API read; General settings editor;
  channel menu with a third-level QR Popover and external QR link.
- Success criteria: disabled channels never appear publicly, empty configuration
  hides Support, safe HTTP(S) links generate QR locally, channel order persists,
  and mouse/keyboard/touch interactions remain usable within the viewport.
- Verification: settings persistence and ambient rollback on isolated PostgreSQL
  schemas, route/control contract tests, frontend focused tests, browser geometry,
  type/build/lint and generated API drift.
- Rollback: revert the feature changes; no schema migration is required. The
  optional `support_channels` system config key can remain inert.

---

## Archived Workspace Task Restoration (#825–#830)

The workspace task production path, the #809 hard cut, and the runtime-grant
audit were completed and committed. The original detailed scope is retained
below as a historical acceptance summary.

### Goal

Complete the workspace task production path for issues #825 through #830:
submit, execute, render, settle, and materialize while preserving the #809
hard cut without weakening release guards.

### Scope

- #825: compile and persist a durable workflow execution plan for v1 submits.
- #826: persist task billing snapshots and settlement facts in the existing
  settlement transaction.
- #827: wire the production result fetcher and derivative renderer.
- #828: wire the library saver and make materialization billing/link state
  converge correctly on errors.
- #829: preserve the intentional #809 hard cut. No legacy routes, handlers,
  adapters, aliases, or dual-read/dual-write compatibility paths are kept.
- #830: retain the grant audit as documentation only.
- Restore the Board state contract guard removed by this diff.
- Cleanup: remove the unreferenced pre-canonical `TaskIntake` readiness,
  media-preparation, and submit-validation path. Keep the canonical workspace
  compiler, Admin monitor DTOs, and event hubs that remain production-owned.
- Routing: construct one `RoutingResolver` at the composition root and share it
  with worker settlement, task submit/readiness, task quotes, and billing
  quotes. Availability checks and routing snapshot persistence fail closed.
- Consistency: read model pools from one database transaction snapshot and keep
  task billing, routing snapshot, task insert, and River enqueue in the same
  settlement transaction.

### Success Criteria

- A submitted task carries a valid durable execution plan and can reach the
  provider gateway.
- Task state, Credits commands, and task billing facts commit atomically.
- Remote provider outputs become media assets and materialize into Quick Saves.
- The #809 hard cut is documented and tested.
- Targeted Go and Web checks pass with no generated-code hand edits.

### Risks And Rollback

- Transactional billing changes affect financial facts; all writes use the
  existing pgx transaction and Credits command seams.
- Remote output fetching is constrained to trusted provider origins and size
  limits. A fetch or render failure remains terminal and releases task billing.
- Each issue is committed independently, so rollback is a commit revert.
- Legacy cleanup is a hard cut: deleted symbols and files have no compatibility
  aliases, dual reads, dual writes, or fail-open fallbacks. Rollback is the
  commit revert before deployment.

---

## 本轮审计修复 ExecPlan

## Goal

修复本轮审计确认的 5 项缺陷，并使新增测试符合现有 guardrails。

## Scope

保持当前 checkout 与 branch；保留已有 diff。由 subagents 依次修复：

1. [x] Auth login lockout 并发计数丢失；真实 PostgreSQL 与 Auth 209/209 tests、build、lint 通过。
2. [x] Schema / migration 对固定 runtime role 的依赖；fresh snapshot、Atlas replay/parity、sqlc、DB targeted tests、Auth privilege tests 通过。
3. [x] Admin z-index utilities 未生成；Tailwind compile regression、production CSS、typecheck、style lint、build 通过。
4. [x] Storage toggle 成功后状态回跳；workspace/page 29/29 regression tests、scope lint、Prettier 通过。
5. [x] Search provider 同一行并发写入状态失真；同时保护父级列表乱序回填，page/component/resource/helper 49/49 tests、typecheck、scope lint、Prettier 通过。
6. [x] 修复新增测试的 guardrails 问题，完成整体验证与独立复核。

## Verification Results

- Auth 210/210 tests（真实 PostgreSQL，0 skipped）、build、lint 通过。
- Web 全量 3282/3282 tests、lint、test typecheck、style lint、build 通过；追加乱序回填补修后，相关 49/49 tests、typecheck、build 通过。
- OpenAPI client 经 Orval 生成，api:check-generated 通过；schema release、Atlas replay/parity、sqlc 及真实 DB settings/http tests 通过，Go build 通过。
- 独立 verifier 最终 PASS；补修后独立 page/table 36/36 tests 通过。
- Go 全量仍有原有 worker integration fixture 缺少 task_status_event_outbox 的失败；该目录与 events 无本轮 diff。Identity 全量出现一次失败，单独复跑通过。
- 全局 Prettier 仍有 4 个 pristine 文件警告：routes.ts、pricing/demoData.ts、canvas/CONTEXT.md、SiteBrandLogo.vue；当前 diff 的格式检查已修正。

## Success Criteria

每项有覆盖实际触发场景的 regression check。相关 tests、typecheck/build、lint
通过；Auth 并发与 PostgreSQL schema/runtime privileges 尽可能使用 isolated local
database 验证。说明任何未完成的环境验证，不将 skipped tests 视为通过。

## Risks

Auth 并发必须覆盖多实例，而非仅进程内锁。Schema 修改不手改 generated snapshot；
不改写已应用 migration。保留 fail-open policy 与 runtime least privilege。

## Rollback

仅撤销本轮针对目标文件的补丁，保留用户已有改动。数据库验证使用 isolated schema / 临时
database；本轮不执行生产 migration、部署、branch 切换或 remote push。

---

## Fixture 与 Diff 收尾 ExecPlan

### Goal

修复 worker integration fixture 缺少 task_status_event_outbox，审计并清理当前 diff
中的确定性冗余，完成验证后提交当前已审计的改动。

### Scope

- [x] 对齐 worker fixture 与既有 outbox schema，真实 PostgreSQL worker tests 通过；追加成功提交与失败清理断言。
- [x] 由 subagents 审计 Web 与 API/Auth diff 的重复、无调用代码、过时注释及多余分支，按证据做最小清理。
- [x] 完成 Go 全量及受影响 Web/Auth checks，独立复核。
- [x] 创建 local commit（与本 ExecPlan 一并提交）。

### Success Criteria

worker integration tests 不再缺表；Go 全量使用真实 UTF8 PostgreSQL；冗余清理保留行为
与 regression coverage；生成代码使用生成器更新。提交前检查 staged diff 与敏感内容。

### Risks And Rollback

当前 diff 含 auth/permission 与 migration 改动，必须保留既有 least privilege、transaction
和 schema replay 契约。本轮 fixture 不变更生产 schema。仅在当前 branch 提交，不部署或 push；
回滚本轮补丁或 local commit 时保留用户其余工作。

### Progress

Go 全量再次复现 Identity sweep fixture 的毫秒 rounding 竞赛：TIMESTAMPTZ(3)
DEFAULT now() 可能晚于立即执行的 app clock。测试 seed 改为明确已到期 intent，
保留严格行为断言；目标测试连续 100 次、Identity 全套均通过，无 production 改动。

### Verification Results

- Go 全量 `-count=1 -p 2 ./...` 使用真实 UTF8 PostgreSQL 且设置 required，通过；Go build、schema release 通过。
- Auth 210/210 tests（真实 PostgreSQL，0 skipped）、build、lint 通过。
- Web 516 files / 3287 tests、build、lint、test typecheck、style lint、generated drift 通过。
- 冗余清理：删除未发布重复 REVOKE migration；StatusBadge 成为单一状态渲染实现；删除 18 个同值 dark overrides；16 处 routine headers 构造收口，保留 reason/retry-key contracts；清理空分支和过时注释。
- 独立 verifier PASS；focused Web 38/38、真实 PostgreSQL settlement test、diff check 通过。
- 全局 Prettier 的 4 个原有 pristine 文件警告未纳入本轮清理；当前修改文件的格式检查通过。
